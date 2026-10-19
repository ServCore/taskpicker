package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"taskpicker/internal/model"
	"taskpicker/internal/reader"
	"taskpicker/internal/storage"
)

func notesFixture(t *testing.T, root string) model.Document {
	t.Helper()
	doc := statusFixture(t, root)
	doc.Sheets[0].Headers = append(doc.Sheets[0].Headers, "备注 ")
	doc.Sheets[0].Rows[0].Values = append(doc.Sheets[0].Rows[0].Values, "原备注")
	if _, err := persistStatusDocument(root, doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestNotesPersistRetryAndRevertTogetherWithStatus(t *testing.T) {
	root := t.TempDir()
	doc := notesFixture(t, root)
	app := &App{root: root}
	name := doc.Sheets[0].Name
	updated, err := app.UpdateTaskDetails(doc.Key, name, 4, "未处理", []TaskNoteUpdate{{Column: 3, Value: "第一行\n第二行"}})
	if err != nil {
		t.Fatal(err)
	}
	row := updated.Sheets[0].Rows[0]
	if row.StatusEdit == nil || row.StatusEdit.Original != "未处理" || len(row.StatusEdit.Notes) != 1 || row.StatusEdit.Notes[0].Original != "原备注" {
		t.Fatal("notes-only edit lost baseline")
	}
	// The old status API must preserve pending remarks too.
	if _, err := app.UpdateTaskStatus(doc.Key, name, 4, "修复中"); err != nil {
		t.Fatal(err)
	}
	updated, err = app.UpdateTaskDetails(doc.Key, name, 4, "修复中", []TaskNoteUpdate{{Column: 3, Value: ""}})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadDocument(root, doc.Key)
	if err != nil {
		t.Fatal(err)
	}
	row = loaded.Sheets[0].Rows[0]
	if row.Values[3] != "" || row.StatusEdit.Notes[0].Original != "原备注" || row.StatusEdit.Original != "未处理" {
		t.Fatal("restart/clearing lost originals")
	}
	if _, err := saveDocument(context.Background(), root, doc); err == nil {
		t.Fatal("pull overwrote pending remarks")
	}
	updated, err = app.PushTaskStatus(doc.Key, name, 4)
	if err != nil || updated.Sheets[0].Rows[0].StatusEdit == nil || updated.Sheets[0].Rows[0].StatusEdit.Error == "" {
		t.Fatal("failed push discarded pending edits", err)
	}
	reverted, err := (&App{root: root}).RevertTaskStatus(doc.Key, name, 4)
	if err != nil {
		t.Fatal(err)
	}
	row = reverted.Sheets[0].Rows[0]
	if row.Values[2] != "未处理" || row.Values[3] != "原备注" || row.StatusEdit != nil {
		t.Fatal("revert did not restore both fields")
	}
}

func TestNoteValidationIsAtomicAndSupportsEmptyTrailingCells(t *testing.T) {
	root := t.TempDir()
	doc := notesFixture(t, root)
	app := &App{root: root}
	for _, notes := range [][]TaskNoteUpdate{
		{{Column: 1, Value: "不能覆盖描述"}},
		{{Column: 3, Value: "a"}, {Column: 3, Value: "b"}},
		{{Column: 3, Value: strings.Repeat("字", 10001)}},
	} {
		if _, err := app.UpdateTaskDetails(doc.Key, doc.Sheets[0].Name, 4, "已修复", notes); err == nil {
			t.Fatal("invalid note payload accepted")
		}
		loaded, _ := ReadDocument(root, doc.Key)
		if loaded.Sheets[0].Rows[0].Values[2] != "未处理" {
			t.Fatal("invalid notes partially saved the status")
		}
	}
	doc.Sheets[0].Rows[0].Values = doc.Sheets[0].Rows[0].Values[:3]
	if _, err := persistStatusDocument(root, doc); err != nil {
		t.Fatal(err)
	}
	updated, err := app.UpdateTaskDetails(doc.Key, doc.Sheets[0].Name, 4, "未处理", []TaskNoteUpdate{{Column: 3, Value: "=纯文本\n补充说明"}})
	if err != nil || updated.Sheets[0].Rows[0].StatusEdit.Notes[0].Original != "" {
		t.Fatal("could not fill empty trailing notes", err)
	}
	updated, err = app.UpdateTaskDetails(doc.Key, doc.Sheets[0].Name, 4, "未处理", []TaskNoteUpdate{{Column: 3, Value: ""}})
	if err != nil || updated.Sheets[0].Rows[0].StatusEdit != nil {
		t.Fatal("restoring original notes should remove pending marker", err)
	}
}

type detailsRemote struct {
	smart      bool
	values     []string
	writes     []int
	failStatus bool
	formula    bool
	noteType   string
}

func newDetailsRemote(smart bool) *detailsRemote {
	return &detailsRemote{smart: smart, values: []string{"王小明", "无法打开麦克风\n保留完整文本", "未处理", "原备注"}, noteType: "text"}
}

func (m *detailsRemote) Call(_ context.Context, name string, args map[string]any) (map[string]any, error) {
	headers := []string{"责任人", "问题描述", "处理状态", "备注 "}
	switch name {
	case "sheet.get_sheet_info":
		kind := "worksheet"
		if m.smart {
			kind = "smartsheet"
		}
		return map[string]any{"sheets": []any{map[string]any{"sheet_id": "ddczrm", "sheet_name": "示例测试问题清单", "sheet_type": kind}}}, nil
	case "sheet.get_cell_data":
		r := int(reader.Number(args["start_row"]))
		values := m.values
		if r == 0 {
			values = headers
		}
		cells := []any{}
		for c, v := range values {
			cell := map[string]any{"row": r, "col": c, "string_value": v}
			if m.formula && r != 0 && c == 3 {
				cell["formula"] = "=A1"
			}
			cells = append(cells, cell)
		}
		return map[string]any{"cells": cells}, nil
	case "sheet.set_cell_value":
		col := int(reader.Number(args["col"]))
		if col != 2 && col != 3 || args["value_type"] != "STRING" || args["row"] != 3 {
			return nil, errors.New("unexpected target")
		}
		if col == 2 && m.failStatus {
			return nil, errors.New("状态提交失败")
		}
		m.values[col] = reader.StringValue(args["string_value"])
		m.writes = append(m.writes, col)
		return map[string]any{}, nil
	case "smartsheet.list_fields":
		return map[string]any{"fields": []any{
			map[string]any{"field_title": "处理状态", "field_type": "singleSelect", "property_single_select": map[string]any{"options": []any{map[string]any{"text": "未处理"}, map[string]any{"text": "修复中"}}}},
			map[string]any{"field_title": "备注 ", "field_type": m.noteType},
		}}, nil
	case "smartsheet.list_records":
		fields := []any{}
		for c, v := range m.values {
			fields = append(fields, map[string]any{"field": headers[c], "text_value": map[string]any{"items": []any{map[string]any{"type": "text", "text": v}}}})
		}
		return map[string]any{"records": []any{map[string]any{"record_id": "stable-record", "field_values": fields}}, "has_more": false}, nil
	case "smartsheet.update_records":
		record := reader.Object(reader.Array(args["records"])[0])
		if record["record_id"] != "stable-record" {
			return nil, errors.New("wrong record")
		}
		fields := reader.Array(record["field_values"])
		if len(fields) != 1 {
			return nil, errors.New("unexpected fields")
		}
		field := reader.Object(fields[0])
		col := -1
		for c, h := range headers {
			if field["field"] == h {
				col = c
			}
		}
		if col != 2 && col != 3 {
			return nil, errors.New("wrote unrelated field")
		}
		if col == 2 && m.failStatus {
			return nil, errors.New("状态提交失败")
		}
		if col == 3 {
			for _, v := range reader.Array(reader.Object(field["text_value"])["items"]) {
				if reader.Object(v)["type"] != "text" {
					return nil, errors.New("missing text type")
				}
			}
		}
		m.values[col] = smartValue(field)
		m.writes = append(m.writes, col)
		return map[string]any{}, nil
	}
	return nil, errors.New("unexpected tool " + name)
}

func TestNotesAndStatusPreflightConflictsAndRetryPartialWrites(t *testing.T) {
	for _, smart := range []bool{false, true} {
		t.Run(map[bool]string{false: "worksheet", true: "smartsheet"}[smart], func(t *testing.T) {
			doc := notesFixture(t, t.TempDir())
			sheet := &doc.Sheets[0]
			row := &sheet.Rows[0]
			row.RecordID = "stable-record"
			row.Values[2] = "修复中"
			row.Values[3] = "新备注\n第二行"
			row.StatusEdit = &model.StatusEdit{Original: "未处理", Notes: []model.NoteEdit{{Column: 3, Header: "备注 ", Original: "原备注"}}}
			remote := newDetailsRemote(smart)
			remote.values[3] = "他人修改"
			if err := pushStatus(context.Background(), remote, &doc, sheet, row); err == nil || len(remote.writes) != 0 {
				t.Fatal("overwrote conflicting remarks", err)
			}
			remote.values[3] = "原备注"
			remote.values[2] = "已完成"
			if err := pushStatus(context.Background(), remote, &doc, sheet, row); err == nil || len(remote.writes) != 0 {
				t.Fatal("wrote notes before checking status conflict", err)
			}
			remote.values[2] = "未处理"
			remote.failStatus = true
			if err := pushStatus(context.Background(), remote, &doc, sheet, row); err == nil {
				t.Fatal("partial write reported complete")
			}
			if remote.values[3] != "新备注\n第二行" || remote.values[2] != "未处理" || len(remote.writes) != 1 {
				t.Fatal("unexpected partial state")
			}
			remote.failStatus = false
			if err := pushStatus(context.Background(), remote, &doc, sheet, row); err != nil {
				t.Fatal(err)
			}
			if len(remote.writes) != 2 || remote.writes[0] != 3 || remote.writes[1] != 2 {
				t.Fatal("retry duplicated notes or wrong write order")
			}
			if err := pushStatus(context.Background(), remote, &doc, sheet, row); err != nil || len(remote.writes) != 2 {
				t.Fatal("repeated write was not idempotent", err)
			}
		})
	}
}

func TestNotesOnlyCanClearWithoutChangingRemoteStatus(t *testing.T) {
	for _, smart := range []bool{false, true} {
		doc := notesFixture(t, t.TempDir())
		sheet := &doc.Sheets[0]
		row := &sheet.Rows[0]
		row.RecordID = "stable-record"
		row.Values[3] = ""
		row.StatusEdit = &model.StatusEdit{Original: "未处理", Notes: []model.NoteEdit{{Column: 3, Header: "备注 ", Original: "原备注"}}}
		remote := newDetailsRemote(smart)
		remote.values[2] = "已完成"
		if err := pushStatus(context.Background(), remote, &doc, sheet, row); err != nil {
			t.Fatal(err)
		}
		if remote.values[3] != "" || remote.values[2] != "已完成" || len(remote.writes) != 1 || remote.writes[0] != 3 {
			t.Fatal("notes-only update changed status or failed to clear")
		}
	}
}

func TestNotesRejectFormulaAndUnsupportedSmartFields(t *testing.T) {
	for _, smart := range []bool{false, true} {
		doc := notesFixture(t, t.TempDir())
		sheet := &doc.Sheets[0]
		row := &sheet.Rows[0]
		row.RecordID = "stable-record"
		row.Values[2] = "修复中"
		row.Values[3] = "新备注"
		row.StatusEdit = &model.StatusEdit{Original: "未处理", Notes: []model.NoteEdit{{Column: 3, Header: "备注 ", Original: "原备注"}}}
		remote := newDetailsRemote(smart)
		remote.formula = true
		remote.noteType = "formula"
		if err := pushStatus(context.Background(), remote, &doc, sheet, row); err == nil || len(remote.writes) != 0 {
			t.Fatal("accepted readonly notes field", err)
		}
	}
}

// Opt-in verification only: writes the exact existing note text back, then reads
// it again. No test text is inserted and task statuses are never changed.
func TestLiveNotesWriteSameValue(t *testing.T) {
	if os.Getenv("TASKPICKER_LIVE_NOTE_WRITE") != "1" {
		t.Skip("set TASKPICKER_LIVE_NOTE_WRITE=1 for a same-value notes write test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	root := storage.LibraryRoot()
	client, err := newDocsRemoteClient(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, preset := range liveTestDocuments(t) {
		t.Run(preset.Title, func(t *testing.T) {
			doc, err := ReadDocument(root, storage.DocumentKey(preset.SourceURL, preset.Owner))
			if err != nil {
				t.Fatal(err)
			}
			for _, sheet := range doc.Sheets {
				for _, row := range sheet.Rows {
					if row.StatusEdit != nil {
						continue
					}
					for col := range sheet.Headers {
						if !noteColumn(sheet.Headers, col) {
							continue
						}
						target, err := resolveFieldTarget(ctx, client, doc, sheet, row, col)
						if err != nil {
							t.Fatal(err)
						}
						before, err := target.read(ctx)
						if err != nil {
							t.Fatal(err)
						}
						checked, err := target.read(ctx)
						if err != nil || checked != before {
							t.Fatal("note changed before verification; no write attempted", err)
						}
						if err := target.write(ctx, before); err != nil {
							t.Fatal(err)
						}
						after, err := target.read(ctx)
						if err != nil || after != before {
							t.Fatal("same-value note write was not confirmed", err)
						}
						t.Logf("第 %d 行备注已按原值写回并回读确认（%d 字符）", row.Number, utf8.RuneCountInString(before))
						return
					}
				}
			}
			t.Fatal("no note field available")
		})
	}
}

func TestNotesDoNotFillAnUnmarkedStatus(t *testing.T) {
	root := t.TempDir()
	doc := notesFixture(t, root)
	doc.Sheets[0].Rows[0].Values[2] = ""
	if _, err := persistStatusDocument(root, doc); err != nil {
		t.Fatal(err)
	}
	updated, err := (&App{root: root}).UpdateTaskDetails(doc.Key, doc.Sheets[0].Name, 4, "", []TaskNoteUpdate{{Column: 3, Value: "补充说明"}})
	if err != nil {
		t.Fatal(err)
	}
	sheet := &updated.Sheets[0]
	row := &sheet.Rows[0]
	remote := newDetailsRemote(false)
	remote.values[2] = ""
	if err := pushStatus(context.Background(), remote, &updated, sheet, row); err != nil {
		t.Fatal(err)
	}
	if remote.values[2] != "" || rowText(row, 2) != "" || len(remote.writes) != 1 || remote.writes[0] != 3 {
		t.Fatal("notes-only edit filled an empty status")
	}
}
