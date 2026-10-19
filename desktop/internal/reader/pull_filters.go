package reader

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"taskpicker/internal/model"
)

// ReadOptions adds document-scoped filtering without changing legacy readers.
// SchemaOnly never downloads pictures or returns task records.
type ReadOptions struct {
	SchemaOnly bool
	Conditions *model.DocumentPullConditions
}

func readOptions(values []ReadOptions) ReadOptions {
	if len(values) > 0 {
		return values[0]
	}
	return ReadOptions{}
}

func ValidatePullConditions(c model.DocumentPullConditions) error {
	if !utf8.ValidString(c.Owner) || utf8.RuneCountInString(c.Owner) > 80 || strings.ContainsAny(c.Owner, "\r\n") {
		return errors.New("负责人条件无效。")
	}
	if len(c.Filters) > 30 {
		return errors.New("每份文档最多设置 30 个拉取条件。")
	}
	for _, f := range c.Filters {
		if f.Sheet == "" || len(f.Sheet) > 400 || f.Column < 1 || f.Column > 16384 || len(f.Header) > 2000 || len(f.Value) > 4000 || !utf8.ValidString(f.Sheet+f.Header+f.Value) {
			return errors.New("拉取条件字段或内容无效。")
		}
		switch f.Operator {
		case "equals", "not_equals", "contains", "not_contains", "member":
			if strings.TrimSpace(f.Value) == "" {
				return errors.New("请填写拉取条件的匹配内容。")
			}
		case "is_empty", "not_empty":
		default:
			return errors.New("不支持的拉取条件。")
		}
	}
	return nil
}

type compiledFilter struct {
	column int
	rule   model.DocumentFilter
}

// Bind by unique label when a column has moved. Duplicate/blank labels stay
// positional; a missing label fails rather than silently broadening the pull.
func filterColumn(headers []any, f model.DocumentFilter) (int, error) {
	col := f.Column - 1
	if col >= 0 && col < len(headers) && StringValue(headers[col]) == f.Header {
		return col, nil
	}
	found := -1
	for i, h := range headers {
		if f.Header != "" && StringValue(h) == f.Header {
			if found >= 0 {
				return -1, errors.New("筛选字段出现重名，请重新选择字段。")
			}
			found = i
		}
	}
	if found < 0 {
		return -1, fmt.Errorf("筛选字段「%s」已变化，请重新设置拉取条件。", f.Header)
	}
	return found, nil
}
func compileFilters(headers []any, sheet string, c *model.DocumentPullConditions) ([]compiledFilter, error) {
	result := []compiledFilter{}
	if c == nil {
		return result, nil
	}
	if err := ValidatePullConditions(*c); err != nil {
		return nil, err
	}
	for _, f := range c.Filters {
		if f.Sheet != sheet {
			continue
		}
		col, err := filterColumn(headers, f)
		if err != nil {
			return nil, err
		}
		result = append(result, compiledFilter{col, f})
	}
	return result, nil
}
func matchesFilters(values []any, filters []compiledFilter) bool {
	for _, f := range filters {
		value := ""
		if f.column < len(values) {
			value = strings.TrimSpace(StringValue(values[f.column]))
		}
		wanted := strings.TrimSpace(f.rule.Value)
		match := false
		switch f.rule.Operator {
		case "equals":
			match = value == wanted
		case "not_equals":
			match = value != wanted
		case "contains":
			match = strings.Contains(strings.ToLower(value), strings.ToLower(wanted))
		case "not_contains":
			match = !strings.Contains(strings.ToLower(value), strings.ToLower(wanted))
		case "member":
			match = OwnerMatches(value, wanted)
		case "is_empty":
			match = value == ""
		case "not_empty":
			match = value != ""
		}
		if !match {
			return false
		}
	}
	return true
}
func validateFilterSheets(sheets []model.TaskSheet, c *model.DocumentPullConditions) error {
	if c == nil {
		return nil
	}
	for _, f := range c.Filters {
		found := false
		for _, s := range sheets {
			if s.Name == f.Sheet {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("筛选条件对应的工作表「%s」已不存在，请重新配置。", f.Sheet)
		}
	}
	return nil
}

// Generic discovery prefers familiar task headers, with the first non-empty
// multi-column row as a fallback for a regular one-row-per-record table.
func discoverHeader(grid *GridData, smart bool) int {
	if smart {
		return 0
	}
	best, bestScore, first := -1, 0, -1
	known := map[string]bool{"任务": true, "任务名称": true, "任务内容": true, "事项": true, "事项名称": true, "标题": true, "名称": true, "问题描述": true, "描述": true, "功能": true, "模块": true, "项目": true, "状态": true, "任务状态": true, "处理状态": true, "完成状态": true, "优先级": true, "截止日期": true, "备注": true, "负责人": true, "负责人员": true, "责任人": true}
	counts, scores := map[int]int{}, map[int]int{}
	for pos, v := range grid.Cells {
		if pos.Row >= 0 && pos.Row < 100 && Normalized(v) != "" {
			counts[pos.Row]++
			if known[Normalized(v)] {
				scores[pos.Row]++
			}
		}
	}
	for row := 0; row < 100; row++ {
		count, score := counts[row], scores[row]
		if count > 0 && first < 0 {
			first = row
		}
		if count >= 2 && score > bestScore {
			best, bestScore = row, score
		}
		if count >= 2 && best < 0 {
			best = row
		}
	}

	if best >= 0 {
		return best
	}
	return first
}
