package app

import (
	"bytes"
	"context"
	"image/color"
	"io"
	"net/http"
	"reflect"
	"sync"
	"testing"

	"github.com/xuri/excelize/v2"

	"taskpicker/internal/reader"
)

func TestCompletedTaskOnlyExcludesTerminalStates(t *testing.T) {
	for _, status := range []string{"已完成", "已修复", "已关闭", "关闭", "已解决", "已验收", " 已 完 成 ", "DONE", "completed", "closed"} {
		if !reader.CompletedTask([]any{"任务", " 完成状态 "}, []any{"任务", status}) {
			t.Errorf("terminal state %q was kept", status)
		}
	}
	for _, status := range []any{"未完成", "未处理", "验证未通过", "进行中", "已修复待发布", "已完成待验收", "待验证", "暂不处理", "自定义状态", "", nil} {
		if reader.CompletedTask([]any{"任务状态"}, []any{status}) {
			t.Errorf("actionable state %q was removed", status)
		}
	}
	if reader.CompletedTask([]any{"任务", "负责人"}, []any{"已完成", "王小明"}) || reader.CompletedTask([]any{"状态"}, nil) {
		t.Fatal("missing status must stay actionable")
	}
	if reader.CompletedTask([]any{"发布状态", "处理状态"}, []any{"已完成", "进行中"}) {
		t.Fatal("picked unrelated status before task status")
	}
}

func TestGridFilterSkipsCompletedRowsBeforeDownloadingPictures(t *testing.T) {
	grid := reader.NewGrid()
	for col, header := range []any{"任务", "负责人", "处理状态", "图片"} {
		grid.Cells[reader.CellPosition{0, col}] = header
	}
	statuses := []string{"未处理", "已完成", "已修复", "已修复待发布", "已关闭", "", "验证未通过"}
	for index, status := range statuses {
		row := index + 1
		grid.Cells[reader.CellPosition{row, 0}] = "任务"
		grid.Cells[reader.CellPosition{row, 1}] = "王小明"
		grid.Cells[reader.CellPosition{row, 2}] = status
		grid.Images[reader.CellPosition{row, 3}] = []reader.RemoteImage{{Row: row, Col: 3, URL: "https://docimg1.docs.qq.com/image/test.png"}}
	}
	raw := testPNG(t, color.NRGBA{B: 255, A: 255})
	downloads := 0
	var downloadMu sync.Mutex
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		downloadMu.Lock()
		downloads++
		downloadMu.Unlock()
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(raw)), Header: http.Header{}}, nil
	})}
	doc, err := reader.DocumentFromGrid(context.Background(), grid, "任务", "表格", "url", "王小明", func(string) {}, false, client, false)
	if err != nil {
		t.Fatal(err)
	}
	var rows []int
	for _, row := range doc.Sheets[0].Rows {
		rows = append(rows, row.Number)
		if len(row.Pictures) != 1 {
			t.Fatal("missing actionable image")
		}
	}
	if !reflect.DeepEqual(rows, []int{2, 5, 7, 8}) || downloads != 4 || doc.IncludeCompleted {
		t.Fatalf("wrong pending selection: %v, %d downloads", rows, downloads)
	}
	downloads = 0
	all, err := reader.DocumentFromGrid(context.Background(), grid, "任务", "表格", "url", "王小明", func(string) {}, false, client, true)
	if err != nil || len(all.Sheets[0].Rows) != 7 || downloads != 7 || !all.IncludeCompleted {
		t.Fatalf("include completed failed: %v", err)
	}
}

func TestExcelFilterUsesSameCompletionRules(t *testing.T) {
	filename := excelFixture(t)
	book, err := excelize.OpenFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	for cell, value := range map[string]string{"E2": "完成状态", "E3": "已完成", "E4": "进行中", "E6": "已修复", "E7": "已修复待发布"} {
		if err := book.SetCellStr("问题清单", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := book.Save(); err != nil {
		t.Fatal(err)
	}
	book.Close()
	doc, err := reader.FilterWorkbook(context.Background(), filename, "王小明", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	rows := doc.Sheets[0].Rows
	if len(rows) != 2 || rows[0].Number != 4 || rows[1].Number != 7 || len(rows[0].Pictures) != 2 {
		t.Fatalf("incorrect incomplete Excel rows: %+v", rows)
	}
	all, err := reader.FilterWorkbook(context.Background(), filename, "王小明", nil, true)
	if err != nil || len(all.Sheets[0].Rows) != 4 || !all.IncludeCompleted {
		t.Fatalf("full Excel import failed: %v", err)
	}
}
