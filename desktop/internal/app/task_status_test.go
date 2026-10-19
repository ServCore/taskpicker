package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"taskpicker/internal/model"
	"taskpicker/internal/reader"
	"taskpicker/internal/storage"
)

func statusFixture(t *testing.T, root string) model.Document {
	t.Helper()
	doc := fixture(t, root)
	doc.Sheets[0].HeaderRow = 1
	doc.Sheets[0].Headers = append(doc.Sheets[0].Headers, "处理状态")
	doc.Sheets[0].Rows[0].Values = append(doc.Sheets[0].Rows[0].Values, "未处理")
	if _, err := persistStatusDocument(root, doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestStatusEditsSurviveRestartAndCannotBeOverwrittenByPull(t *testing.T) {
	root := t.TempDir()
	doc := statusFixture(t, root)
	app := &App{root: root}
	sheet := doc.Sheets[0].Name
	updated, err := app.UpdateTaskStatus(doc.Key, sheet, 4, "已修复")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Sheets[0].Rows[0].StatusEdit.Original != "未处理" {
		t.Fatal("lost original status")
	}
	// Multiple local changes preserve the first remote baseline.
	if _, err = app.UpdateTaskStatus(doc.Key, sheet, 4, "修复中"); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadDocument(root, doc.Key)
	if err != nil {
		t.Fatal(err)
	}
	row := loaded.Sheets[0].Rows[0]
	if row.StatusEdit.Original != "未处理" || row.Values[2] != "修复中" || len(row.Pictures) != 1 {
		t.Fatal("lost pending edit or images")
	}
	if _, err = saveDocument(context.Background(), root, doc); err == nil || !strings.Contains(err.Error(), "待同步") {
		t.Fatal("pull overwrote pending state", err)
	}
	restored, err := (&App{root: root}).RevertTaskStatus(doc.Key, sheet, 4)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Sheets[0].Rows[0].Values[2] != "未处理" || restored.Sheets[0].Rows[0].StatusEdit != nil {
		t.Fatal("revert failed")
	}
	if _, err = app.UpdateTaskStatus(doc.Key, sheet, 4, "=BAD()"); err == nil {
		t.Fatal("accepted formula as a status")
	}
}

func TestPushWithoutAuthorizationRetainsLocalChange(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")
	doc := statusFixture(t, root)
	app := &App{root: root}
	if _, err := app.UpdateTaskStatus(doc.Key, doc.Sheets[0].Name, 4, "已修复"); err != nil {
		t.Fatal(err)
	}
	updated, err := app.PushTaskStatus(doc.Key, doc.Sheets[0].Name, 4)
	if err != nil {
		t.Fatal(err)
	}
	edit := updated.Sheets[0].Rows[0].StatusEdit
	if edit == nil || !strings.Contains(edit.Error, "授权") {
		t.Fatal("missing retryable authorization failure")
	}
}

type statusRemote struct {
	status, description string
	writes              int
	verifyFail          bool
}

func (m *statusRemote) Call(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	switch name {
	case "sheet.get_sheet_info":
		return map[string]any{"sheets": []any{map[string]any{"sheet_id": "ddczrm", "sheet_name": "示例测试问题清单", "sheet_type": "worksheet"}}}, nil
	case "sheet.get_cell_data":
		r := int(reader.Number(args["start_row"]))
		values := []string{"王小明", m.description, m.status}
		if r == 0 {
			values = []string{"责任人", "问题描述", "处理状态"}
		}
		if m.verifyFail && m.writes > 0 {
			return nil, errors.New("回读网络失败")
		}
		cells := []any{}
		for c, v := range values {
			cells = append(cells, map[string]any{"row": r, "col": c, "string_value": v})
		}
		return map[string]any{"cells": cells}, nil
	case "sheet.set_cell_value":
		if args["col"] != 2 || args["row"] != 3 || args["value_type"] != "STRING" {
			return nil, errors.New("unexpected write target")
		}
		m.status = reader.StringValue(args["string_value"])
		m.writes++
		return map[string]any{}, nil
	}
	return nil, errors.New("unexpected tool " + name)
}

func TestRemoteWriteChecksIdentityConflictAndReadback(t *testing.T) {
	doc := statusFixture(t, t.TempDir())
	sheet := &doc.Sheets[0]
	row := &sheet.Rows[0]
	row.StatusEdit = &model.StatusEdit{Original: "未处理"}
	row.Values[2] = "修复中"
	remote := &statusRemote{status: "未处理", description: reader.StringValue(row.Values[1])}
	if err := pushStatus(context.Background(), remote, &doc, sheet, row); err != nil {
		t.Fatal(err)
	}
	if remote.writes != 1 || remote.status != "修复中" {
		t.Fatal("status was not written")
	}
	if err := pushStatus(context.Background(), remote, &doc, sheet, row); err != nil || remote.writes != 1 {
		t.Fatal("retry duplicated write", err)
	}
	remote.status = "已完成"
	if err := pushStatus(context.Background(), remote, &doc, sheet, row); err == nil || remote.writes != 1 {
		t.Fatal("overwrote a remote status conflict")
	}
	remote.status = "未处理"
	remote.description = "另一个任务"
	if err := pushStatus(context.Background(), remote, &doc, sheet, row); err == nil || remote.writes != 1 {
		t.Fatal("wrote to a moved/replaced row")
	}
	remote.description = reader.StringValue(row.Values[1])
	remote.writes = 0
	remote.verifyFail = true
	if err := pushStatus(context.Background(), remote, &doc, sheet, row); err == nil || !strings.Contains(err.Error(), "尚未确认") {
		t.Fatal("unconfirmed remote write reported success", err)
	}
}

func TestMCPTransportReadsJSONAndSSEAndRejectsBusinessErrors(t *testing.T) {
	for _, mode := range []string{"json", "sse", "error"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing authorization")
				}
				var req map[string]any
				if json.NewDecoder(r.Body).Decode(&req) != nil || req["method"] != "tools/call" {
					t.Error("invalid RPC")
				}
				body := `{"jsonrpc":"2.0","id":1,"result":{"structuredContent":{"value":"ok","error":""}}}`
				if mode == "error" {
					body = `{"jsonrpc":"2.0","id":1,"result":{"structuredContent":{"error":"无编辑权限"}}}`
				}
				if mode == "sse" {
					w.Header().Set("Content-Type", "text/event-stream")
					body = "event: message\ndata: " + body + "\n\n"
				}
				w.Write([]byte(body))
			}))
			defer server.Close()
			client := &docsRemoteClient{token: "test-token", client: server.Client(), endpoint: server.URL}
			result, err := client.Call(context.Background(), "read", map[string]any{})
			if mode == "error" {
				if err == nil {
					t.Fatal("ignored business error")
				}
			} else if err != nil || result["value"] != "ok" {
				t.Fatal(result, err)
			}
		})
	}
}

// Explicitly opt in. Writes each existing status back unchanged, then re-reads it.
// It does not change task state, create records, or write any other field.
func TestLiveStatusWriteSameValue(t *testing.T) {
	if os.Getenv("TASKPICKER_LIVE_STATUS_WRITE") != "1" {
		t.Skip("set TASKPICKER_LIVE_STATUS_WRITE=1 for a same-value write permission test")
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
			var lastErr error
			for _, sheet := range doc.Sheets {
				for _, row := range sheet.Rows {
					target, err := resolveStatusTarget(ctx, client, doc, sheet, row)
					if err != nil {
						lastErr = err
						continue
					}
					before, err := target.read(ctx)
					if err != nil {
						lastErr = err
						continue
					}
					if before == "" {
						continue
					}
					if err = target.write(ctx, before); err != nil {
						t.Fatal("same-value write rejected:", err)
					}
					after, err := target.read(ctx)
					if err != nil || before != after {
						t.Fatal("remote status was not verified:", err)
					}
					t.Logf("原表第 %d 行：原状态「%s」已按原值写回并回读确认", row.Number, before)
					return
				}
			}
			t.Fatal("no unambiguous task available for verification:", lastErr)
		})
	}
}

type smartStatusRemote struct {
	status    string
	writes    int
	duplicate bool
	recordID  string
}

func (m *smartStatusRemote) Call(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	switch name {
	case "sheet.get_sheet_info":
		return map[string]any{"sheets": []any{map[string]any{"sheet_id": "ddczrm", "sheet_name": "示例测试问题清单", "sheet_type": "smartsheet"}}}, nil
	case "smartsheet.list_fields":
		return map[string]any{"fields": []any{map[string]any{"field_title": "处理状态", "field_type": "singleSelect", "property_single_select": map[string]any{"options": []any{map[string]any{"text": "未处理"}, map[string]any{"text": "修复中"}}}}}}, nil
	case "smartsheet.list_records":
		record := func(id string) any {
			return map[string]any{"record_id": id, "field_values": []any{map[string]any{"field": "责任人", "option_value": map[string]any{"items": []any{map[string]any{"text": "王小明"}}}}, map[string]any{"field": "问题描述", "text_value": map[string]any{"items": []any{map[string]any{"type": "text", "text": "无法打开麦克风\n保留完整文本"}}}}, map[string]any{"field": "处理状态", "option_value": map[string]any{"items": []any{map[string]any{"text": m.status}}}}}}
		}
		records := []any{record("stable-record")}
		if m.duplicate && args["record_ids"] == nil {
			records = append(records, record("duplicate"))
		}
		return map[string]any{"records": records, "has_more": false}, nil
	case "smartsheet.update_records":
		records := args["records"].([]any)
		if len(records) != 1 {
			return nil, errors.New("unexpected record count")
		}
		record := reader.Object(records[0])
		m.recordID = reader.StringValue(record["record_id"])
		values := record["field_values"].([]any)
		if len(values) != 1 || reader.Object(values[0])["field"] != "处理状态" {
			return nil, errors.New("wrote unrelated field")
		}
		m.status = smartValue(reader.Object(values[0]))
		m.writes++
		return map[string]any{}, nil
	}
	return nil, errors.New("unexpected smart tool " + name)
}

func TestSmartStatusUsesStableIDAndRejectsAmbiguousLegacyRows(t *testing.T) {
	doc := statusFixture(t, t.TempDir())
	sheet := &doc.Sheets[0]
	row := &sheet.Rows[0]
	row.StatusEdit = &model.StatusEdit{Original: "未处理"}
	row.Values[2] = "修复中"
	remote := &smartStatusRemote{status: "未处理", duplicate: true}
	if err := pushStatus(context.Background(), remote, &doc, sheet, row); err == nil || remote.writes != 0 {
		t.Fatal("accepted ambiguous legacy records")
	}
	row.RecordID = "stable-record"
	row.Number = 120 // A stable record remains addressable after sorting.
	if err := pushStatus(context.Background(), remote, &doc, sheet, row); err != nil {
		t.Fatal(err)
	}
	if remote.writes != 1 || remote.recordID != "stable-record" || remote.status != "修复中" {
		t.Fatal("wrote wrong smart record")
	}
	row.Values[2] = "未知选项"
	row.StatusEdit.Original = "修复中"
	if err := pushStatus(context.Background(), remote, &doc, sheet, row); err == nil || remote.writes != 1 {
		t.Fatal("created an unintended status option")
	}
}
