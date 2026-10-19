package reader

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"sort"
	"strings"
	"time"
	"unicode"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	"taskpicker/internal/media"
	"taskpicker/internal/model"
)

type ProgressFunc func(string)

type CellPosition struct{ Row, Col int }

type RemoteImage struct {
	Row, Col      int
	URL           string
	Width, Height float64
}

type gridRange struct{ FirstRow, LastRow, FirstCol, LastCol int }

type GridData struct {
	RowIDs        map[int]string
	StatusOptions []string
	Cells         map[CellPosition]any
	Images        map[CellPosition][]RemoteImage
	Ranges        []gridRange
	Warnings      []string
}

func NewGrid() *GridData {
	return &GridData{RowIDs: map[int]string{}, Cells: map[CellPosition]any{}, Images: map[CellPosition][]RemoteImage{}}
}

func (g *GridData) merge(other *GridData) {
	for k, v := range other.Cells {
		g.Cells[k] = v
	}
	for k, v := range other.Images {
		g.Images[k] = v
	}
	g.Ranges = append(g.Ranges, other.Ranges...)
	g.Warnings = append(g.Warnings, other.Warnings...)
}

func StringValue(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

func Normalized(value any) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '\u200b' {
			return -1
		}
		return r
	}, StringValue(value))
}

func OwnerMatches(value any, owner string) bool {
	for _, token := range strings.FieldsFunc(StringValue(value), func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(",，、;；/|&＆", r)
	}) {
		if Normalized(strings.TrimPrefix(token, "@")) == Normalized(owner) {
			return true
		}
	}
	return false
}

func OwnerHeader(value any) bool {
	switch Normalized(value) {
	case "负责人", "负责人员", "责任人":
		return true
	}
	return false
}

// TitleColumn is shared by task summaries and writeback identity checks.
func TitleColumn(headers []any) int {
	for _, name := range []string{"问题描述", "任务", "任务名称", "任务内容", "事项", "事项名称", "标题", "名称", "描述", "功能"} {
		for col, h := range headers {
			if Normalized(h) == name {
				return col
			}
		}
	}
	for col, h := range headers {
		name := Normalized(h)
		if name != "" && !OwnerHeader(h) && !strings.Contains(name, "状态") && !strings.Contains(name, "备注") && !strings.Contains(name, "日期") && !strings.Contains(name, "时间") && !strings.Contains(name, "优先") && !strings.Contains(name, "紧急") {
			return col
		}
	}
	return -1
}

// Only terminal states are excluded. Unknown, empty and pending states stay actionable.
func StatusColumn(headers []any) int {
	for _, name := range []string{"处理状态", "完成状态", "任务状态", "状态"} {
		for col, header := range headers {
			if Normalized(header) == name {
				return col
			}
		}
	}
	for col, header := range headers {
		if strings.Contains(Normalized(header), "状态") {
			return col
		}
	}
	return -1
}

func CompletedTask(headers, values []any) bool {
	col := StatusColumn(headers)
	if col < 0 || col >= len(values) {
		return false
	}
	switch strings.ToLower(Normalized(values[col])) {
	case "已完成", "完成", "已修复", "已关闭", "关闭", "已解决", "已结束", "已取消", "已验收", "done", "completed", "closed", "resolved", "cancelled", "canceled":
		return true
	}
	return false
}

func UniqueStrings(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		if !seen[value] {
			out = append(out, value)
			seen[value] = true
		}
	}
	return out
}

func Number(value any) float64 {
	switch n := value.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		value, _ := n.Float64()
		return value
	}
	return 0
}

func Object(value any) map[string]any {
	m, _ := value.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func Array(value any) []any { a, _ := value.([]any); return a }

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func PngPicture(raw []byte, row, col int, width, height float64) (model.Picture, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return model.Picture{}, fmt.Errorf("图片无法解码：%w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 40_000_000 {
		return model.Picture{}, errors.New("图片尺寸过大，单张图片最多支持 4000 万像素。")
	}
	decoded, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return model.Picture{}, err
	}
	if format != "png" {
		var out bytes.Buffer
		if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&out, decoded); err != nil {
			return model.Picture{}, err
		}
		raw = out.Bytes()
	}
	if width <= 0 {
		width = float64(config.Width)
	}
	if height <= 0 {
		height = float64(config.Height)
	}
	variants, err := media.ImageVariants(decoded)
	if err != nil {
		return model.Picture{}, err
	}
	return model.Picture{Row: row, Column: col, Data: raw, Variants: variants, Width: width, Height: height}, nil
}

func DateText(value, header any) any {
	if !strings.Contains(StringValue(header), "时间") && !strings.Contains(StringValue(header), "日期") {
		return value
	}
	switch value.(type) {
	case int, int64, float64, json.Number:
	default:
		return value
	}
	n := Number(value)
	if n >= 19000101 && n <= 21991231 {
		if t, err := time.Parse("20060102", fmt.Sprintf("%.0f", n)); err == nil {
			return t.Format("2006-01-02")
		}
	}
	if n >= 20000 && n <= 100000 {
		return time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(n)).Format("2006-01-02")
	}
	return value
}
