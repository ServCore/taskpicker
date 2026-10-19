package reader

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"taskpicker/internal/model"
)

func genericGrid() *GridData {
	g := NewGrid()
	for r, row := range [][]any{{"事项", "模块", "状态", "备注"}, {"登录失败", "账号", "未处理", ""}, {"找回密码", "账号", "已完成", "处理完毕"}, {"账单导出", "财务", "进行中", "跟进"}} {
		for c, v := range row {
			g.Cells[CellPosition{r, c}] = v
		}
	}
	return g
}
func TestGenericGridHasOptionalOwnerAndFiltersBeforePictures(t *testing.T) {
	g := genericGrid()
	g.Images[CellPosition{2, 3}] = []RemoteImage{{Row: 2, Col: 3, URL: "https://bad.invalid/image"}}
	conditions := model.DocumentPullConditions{Filters: []model.DocumentFilter{{Sheet: "任务表", Column: 2, Header: "模块", Operator: "equals", Value: "账号"}}}
	doc, err := DocumentFromGrid(context.Background(), g, "个人工作", "任务表", "", "", func(string) {}, false, nil, false, ReadOptions{Conditions: &conditions})
	if err != nil || len(doc.Sheets[0].Rows) != 1 || doc.Sheets[0].Rows[0].Number != 2 {
		t.Fatalf("generic: %+v %v", doc, err)
	}
	// Discovery must not return records or download images, including matching rows.
	schema, err := DocumentFromGrid(context.Background(), g, "个人工作", "任务表", "", "", func(string) {}, false, &http.Client{}, true, ReadOptions{SchemaOnly: true})
	if err != nil || len(schema.Sheets[0].Rows) != 0 || len(schema.Sheets[0].Headers) != 4 {
		t.Fatalf("schema: %+v %v", schema, err)
	}
}
func TestFilterOperatorsAndAllMatch(t *testing.T) {
	cases := []struct {
		op, wanted, value string
		match             bool
	}{
		{"equals", "高", " 高 ", true}, {"equals", "高", "较高", false}, {"not_equals", "高", "", true},
		{"contains", "bug", "Bug #4", true}, {"not_contains", "bug", "Bug #4", false},
		{"is_empty", "", " \n", true}, {"not_empty", "", "0", true},
		{"member", "王小明", "张三、王小明", true}, {"member", "王小明", "王小明双", false},
	}
	for _, c := range cases {
		t.Run(c.op+c.value, func(t *testing.T) {
			rule := compiledFilter{0, model.DocumentFilter{Operator: c.op, Value: c.wanted}}
			if matchesFilters([]any{c.value}, []compiledFilter{rule}) != c.match {
				t.Fatal(c)
			}
		})
	}
	if matchesFilters([]any{"高", "未开始"}, []compiledFilter{{0, model.DocumentFilter{Operator: "equals", Value: "高"}}, {1, model.DocumentFilter{Operator: "equals", Value: "进行中"}}}) {
		t.Fatal("conditions were not ANDed")
	}
}
func TestInvalidOrMissingFilterNeverBroadensPull(t *testing.T) {
	for _, condition := range []model.DocumentPullConditions{
		{Filters: []model.DocumentFilter{{Sheet: "任务表", Column: 1, Header: "已删除", Operator: "equals", Value: "a"}}},
		{Filters: []model.DocumentFilter{{Sheet: "已删除工作表", Column: 1, Header: "事项", Operator: "equals", Value: "a"}}},
		{Filters: []model.DocumentFilter{{Sheet: "任务表", Column: 1, Header: "事项", Operator: "unknown", Value: "a"}}},
	} {
		_, err := DocumentFromGrid(context.Background(), genericGrid(), "doc", "任务表", "", "", func(string) {}, false, nil, true, ReadOptions{Conditions: &condition})
		if err == nil {
			t.Fatalf("accepted: %+v", condition)
		}
	}
	c := model.DocumentPullConditions{Filters: []model.DocumentFilter{{Sheet: "任务表", Column: 1, Header: "模块", Operator: "equals", Value: "财务"}}}
	// A uniquely named moved column can be rebound safely.
	doc, err := DocumentFromGrid(context.Background(), genericGrid(), "doc", "任务表", "", "", func(string) {}, false, nil, true, ReadOptions{Conditions: &c})
	if err != nil || len(doc.Sheets[0].Rows) != 1 || !strings.Contains(StringValue(doc.Sheets[0].Rows[0].Values[0]), "账单") {
		t.Fatalf("rebind: %+v %v", doc, err)
	}
}
