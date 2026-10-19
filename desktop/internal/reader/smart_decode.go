package reader

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func smartParts(texts []any, tab string) (map[string]any, map[string]any, error) {
	var state map[string]any
	records := map[string]any{}
	for _, value := range texts {
		encoded := StringValue(Object(value)["smartsheet"])
		if encoded == "" {
			continue
		}
		var groups [][]map[string]any
		if err := json.Unmarshal([]byte(encoded), &groups); err != nil {
			return nil, nil, err
		}
		for _, group := range groups {
			for _, op := range group {
				content := Object(op["c"])
				if StringValue(content["1"]) != tab {
					continue
				}
				switch int(Number(op["t"])) {
				case 3005:
					state = Object(content["3"])
				case 3028:
					for key, value := range Object(Object(content["2"])["1"]) {
						records[key] = value
					}
				}
			}
		}
	}
	return state, records, nil
}

func DecodeSmart(state, records map[string]any, viewID string, options ...ReadOptions) (*GridData, error) {
	opts := readOptions(options)
	fields := Object(state["3"])
	views := Array(state["4"])
	view := map[string]any{}
	if len(views) > 0 {
		view = Object(views[0])
	}
	for _, v := range views {
		if StringValue(Object(v)["30"]) == viewID {
			view = Object(v)
			break
		}
	}
	var config map[string]any
	for _, key := range sortedKeys(view) {
		if key == "30" || key == "31" || key == "32" || key == "33" || key == "34" || key == "35" {
			continue
		}
		candidate := Object(view[key])
		if _, ok := candidate["1"].([]any); ok {
			config = candidate
			break
		}
	}
	if config == nil {
		return nil, errors.New("智能表格未提供完整的行列顺序，无法可靠读取。")
	}
	rowIDs := Array(config["1"])
	// Header discovery does not require task records. The server can return a
	// partial row window even when the view metadata contains every row ID.
	if opts.SchemaOnly {
		rowIDs = nil
	}
	columns := []string{}
	seen := map[string]bool{}
	for _, fid := range Array(config["2"]) {
		key := StringValue(fid)
		if !seen[key] {
			columns = append(columns, key)
			seen[key] = true
		}
	}
	for _, key := range sortedKeys(fields) {
		if !seen[key] {
			columns = append(columns, key)
		}
	}
	missing := 0
	for _, rid := range rowIDs {
		if _, ok := records[StringValue(rid)]; !ok {
			missing++
		}
	}
	if missing > 0 {
		return nil, fmt.Errorf("智能表格还有 %d 行没有加载，未保存不完整的结果。", missing)
	}
	out := NewGrid()
	for index, rid := range rowIDs {
		out.RowIDs[index+1] = StringValue(rid)
	}
	for col, fid := range columns {
		meta, exists := fields[fid]
		if !exists {
			return nil, errors.New("智能表格字段信息不完整。")
		}
		field := Object(meta)
		kind := int(Number(field["31"]))
		label := StringValue(field["30"])
		out.Cells[CellPosition{0, col}] = label
		options := map[string]string{}
		for _, value := range Array(Object(field[strconv.Itoa(kind)])["3"]) {
			option := Object(value)
			options[StringValue(option["1"])] = StringValue(option["2"])
		}
		if strings.Contains(Normalized(label), "状态") {
			for _, value := range Array(Object(field[strconv.Itoa(kind)])["3"]) {
				out.StatusOptions = append(out.StatusOptions, StringValue(Object(value)["2"]))
			}
		}
		for index, rid := range rowIDs {
			row := index + 1
			cell := Object(Object(Object(records[StringValue(rid)])["1"])[fid])
			var value any
			unsupportedContent := false
			switch kind {
			case 1:
				var builder strings.Builder
				nonText := false
				for _, v := range Array(cell["1"]) {
					span := Object(v)
					if StringValue(span["1"]) == "text" {
						builder.WriteString(StringValue(span["2"]))
					} else {
						nonText = true
					}
				}
				if builder.Len() > 0 {
					value = builder.String()
				}
				if nonText {
					unsupportedContent = true
					out.Warnings = append(out.Warnings, fmt.Sprintf("原第 %d 条任务的“%s”含非文字内容，请核对。", row, label))
				}
			case 9, 17:
				names := []string{}
				for _, id := range Array(cell[strconv.Itoa(kind)]) {
					key := StringValue(id)
					if name, ok := options[key]; ok {
						names = append(names, name)
					} else {
						names = append(names, key)
					}
				}
				if len(names) > 0 {
					value = strings.Join(names, "、")
				}
			case 4:
				if stamp := StringValue(cell["4"]); stamp != "" && stamp != "0" {
					ms, err := strconv.ParseFloat(stamp, 64)
					if err != nil {
						return nil, fmt.Errorf("日期字段格式异常：%w", err)
					}
					value = time.UnixMilli(int64(ms)).In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
				}
			case 2:
				value = cell["2"]
			default:
				if v := cell[strconv.Itoa(kind)]; smartValuePresent(v) {
					unsupportedContent = true
					out.Warnings = append(out.Warnings, fmt.Sprintf("原第 %d 条任务的“%s”类型 %d 暂不支持。", row, label, kind))
				}
			}
			// Empty smart-table rows often retain cell edit metadata. They are
			// not tasks, but unsupported nonempty cells must remain visible.
			if (value != nil && strings.TrimSpace(StringValue(value)) != "") || unsupportedContent {
				out.Cells[CellPosition{row, col}] = value
			}
		}
	}
	return out, nil
}

func smartValuePresent(value any) bool {
	switch value := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(value) != ""
	case []any:
		return len(value) > 0
	case map[string]any:
		return len(value) > 0
	default:
		return true
	}
}
