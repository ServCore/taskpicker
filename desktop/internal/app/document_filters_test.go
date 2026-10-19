package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"taskpicker/internal/model"
	"taskpicker/internal/reader"
)

func TestGenericDocumentConditionsPersistApplyAndKeepIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "个人任务.xlsx")
	book := excelize.NewFile()
	for r, row := range [][]any{{"事项", "模块", "状态", "备注"}, {"做首页", "前端", "未开始", ""}, {"修接口", "后端", "已完成", ""}, {"改接口", "后端", "进行中", ""}} {
		axis, _ := excelize.CoordinatesToCellName(1, r+1)
		if err := book.SetSheetRow("Sheet1", axis, &row); err != nil {
			t.Fatal(err)
		}
	}
	if err := book.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	book.Close()
	a := &App{root: t.TempDir()}
	doc, err := a.importPath(path, "", false)
	if err != nil || len(doc.Sheets[0].Rows) != 2 {
		t.Fatalf("import: %+v %v", doc, err)
	}
	config := model.DocumentPullConditions{IncludeCompleted: true, Filters: []model.DocumentFilter{{Sheet: "Sheet1", Column: 2, Header: "模块", Operator: "equals", Value: "后端"}}}
	updated, err := a.SaveDocumentFilters(doc.Key, config)
	if err != nil || updated.Key != doc.Key || len(updated.Sheets[0].Rows) != 2 {
		t.Fatalf("save: %+v %v", updated, err)
	}
	b := &App{root: a.root}
	result, err := b.SyncConfiguredDocument(doc.Key, false)
	if err != nil || result.Key != doc.Key || len(result.Sheets[0].Rows) != 2 || result.Sheets[0].Rows[0].Number != 3 {
		t.Fatalf("apply: %+v %v", result, err)
	}
	persisted, err := ReadDocument(a.root, doc.Key)
	if err != nil || persisted.AppliedPullConditions == nil || persisted.PullConditions == nil || !reflect.DeepEqual(*persisted.AppliedPullConditions, config) {
		t.Fatalf("applied config: %+v %v", persisted, err)
	}
	before, _ := os.ReadFile(documentConditionsPath(a.root))
	bad := config
	bad.Filters = []model.DocumentFilter{{Sheet: "Sheet1", Column: 9, Header: "bad", Operator: "equals", Value: "a"}}
	if _, err := a.SaveDocumentFilters(doc.Key, bad); err == nil {
		t.Fatal("missing field saved")
	}
	after, _ := os.ReadFile(documentConditionsPath(a.root))
	if string(before) != string(after) {
		t.Fatal("bad config overwrote good config")
	}
	// Saving a local note blocks condition changes and any subsequent pull.
	result.Sheets[0].Rows[1].Values[3] = "本地备注"
	result.Sheets[0].Rows[1].StatusEdit = &model.StatusEdit{Original: "进行中", SavedAt: "2026-10-18", Notes: []model.NoteEdit{{Column: 4, Header: "备注", Original: ""}}}
	if _, err := persistStatusDocument(a.root, result); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveDocumentFilters(doc.Key, model.DocumentPullConditions{}); err == nil {
		t.Fatal("pending note allowed scope change")
	}
	if _, err := a.SyncConfiguredDocument(doc.Key, false); err == nil {
		t.Fatal("pending note was overwritten")
	}
}

func TestConfiguredPullOnlyUsesExplicitConditions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "任务.xlsx")
	book := excelize.NewFile()
	for r, row := range [][]any{{"事项", "模块", "状态"}, {"修接口", "后端", "已完成"}, {"改接口", "后端", "进行中"}, {"做首页", "前端", "已关闭"}} {
		axis, _ := excelize.CoordinatesToCellName(1, r+1)
		if err := book.SetSheetRow("Sheet1", axis, &row); err != nil {
			t.Fatal(err)
		}
	}
	if err := book.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	book.Close()
	for _, tc := range []struct {
		name    string
		filters []model.DocumentFilter
		rows    []int
	}{
		{"legacy snapshot without conditions", nil, []int{2, 3, 4}},
		{"legacy saved flag is ignored", []model.DocumentFilter{}, []int{2, 3, 4}},
		{"module condition retains completed tasks", []model.DocumentFilter{{Sheet: "Sheet1", Column: 2, Header: "模块", Operator: "equals", Value: "后端"}}, []int{2, 3}},
		{"explicit status condition still applies", []model.DocumentFilter{{Sheet: "Sheet1", Column: 3, Header: "状态", Operator: "equals", Value: "已完成"}}, []int{2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &App{root: t.TempDir()}
			doc, err := a.importPath(path, "", false)
			if err != nil {
				t.Fatal(err)
			}
			if tc.filters != nil {
				// Simulate an existing config written before the switch was removed.
				condition := model.DocumentPullConditions{IncludeCompleted: false, Filters: tc.filters}
				raw, err := json.Marshal(documentConditionsFile{Version: 1, Documents: map[string]model.DocumentPullConditions{doc.Key: condition}})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(documentConditionsPath(a.root), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			result, err := a.SyncConfiguredDocument(doc.Key, false)
			if err != nil {
				t.Fatal(err)
			}
			var rows []int
			for _, row := range result.Sheets[0].Rows {
				rows = append(rows, row.Number)
			}
			if !reflect.DeepEqual(rows, tc.rows) || !result.IncludeCompleted || !result.AppliedPullConditions.IncludeCompleted {
				t.Fatalf("unexpected pull: rows=%v, document=%+v", rows, result)
			}
			updated, err := a.SaveDocumentFilters(doc.Key, model.DocumentPullConditions{IncludeCompleted: false, Filters: tc.filters})
			if err != nil || !updated.PullConditions.IncludeCompleted {
				t.Fatalf("legacy update restored implicit status filtering: %+v %v", updated, err)
			}
		})
	}
}
func TestPreparedDocumentRetainsSchemaWithoutPretendingTasksAreCached(t *testing.T) {
	a := &App{root: t.TempDir()}
	doc, err := a.runDocumentJob("", "", func(ctx context.Context, _ reader.ProgressFunc) (model.Document, error) {
		return model.Document{SourceURL: "https://docs.qq.com/sheet/test-schema", Title: "新文档", SetupPending: true, Sheets: []model.TaskSheet{{Name: "表一", Headers: []any{"事项", "状态"}, Rows: []model.TaskRow{}}}}, nil
	})
	if err != nil || doc.Cached || doc.UpdatedAt != "" {
		t.Fatalf("discovery cache: %+v %v", doc, err)
	}
	loaded, err := a.PrepareOnlineDocument(doc.SourceURL) // Existing schema is reused without network or duplicate documents.
	if err != nil || loaded.Key != doc.Key || loaded.Cached || !loaded.SetupPending || len(loaded.Sheets[0].Headers) != 2 {
		t.Fatalf("prepared: %+v %v", loaded, err)
	}
	state, _ := a.GetLibrary()
	if len(state.Documents) != 3 {
		t.Fatal("discovery duplicated document")
	}
	if _, err := a.PrepareOnlineDocument("file:///tmp/private"); err == nil {
		t.Fatal("invalid online URL accepted")
	}
}

func TestPublicSchemaDiscoveryDoesNotFetchRemainingRows(t *testing.T) {
	client := reader.NewHTTPClient()
	calls := 0
	client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		response := httptest.NewRecorder()
		if strings.HasPrefix(r.URL.Path, "/sheet/") {
			fmt.Fprint(response, `<script src="https://docs.qq.com/dop-api/opendoc?foo=1"></script>`)
		} else {
			calls++
			if r.URL.Query().Get("endrow") != "99" || calls > 1 {
				t.Fatalf("schema discovery fetched data page: %s", r.URL)
			}
			cells := append(wireCell(0, 0, 0, nil), wireCell(0, 1, 2, nil)...)
			metadata := map[string]any{"clientVars": map[string]any{"title": "个人事项", "privilegeAttribute": map[string]any{"can_read": 1}, "collab_client_vars": map[string]any{"header": []any{map[string]any{"d": []any{map[string]any{"id": "target", "name": "事项", "type": "sheet"}}}}, "initialAttributedText": map[string]any{"text": []any{map[string]any{"max_row": 10000, "max_col": 3, "block_datas": []any{map[string]any{"related_sheet": encodedGrid(t, 0, 99, cells)}}}}}}}}
			raw, _ := json.Marshal(metadata)
			fmt.Fprintf(response, "callback(%s)", raw)
		}
		result := response.Result()
		result.Request = r
		return result, nil
	})
	doc, err := reader.ReadPublicTasks(context.Background(), "https://docs.qq.com/sheet/test-schema?tab=target", "", func(string) {}, client, false, reader.ReadOptions{SchemaOnly: true})
	if err != nil || calls != 1 || len(doc.Sheets) != 1 || len(doc.Sheets[0].Rows) != 0 || doc.Sheets[0].HeaderRow != 1 {
		t.Fatalf("schema: %+v %v", doc, err)
	}
}

func TestOwnerlessTaskIdentityKeepsContentChecks(t *testing.T) {
	cols, err := identityColumns([]any{"事项", "状态", "备注"})
	if err != nil || !reflect.DeepEqual(cols, []int{0}) {
		t.Fatalf("ownerless identity: %v %v", cols, err)
	}
	if _, err = identityColumns([]any{"状态", "备注"}); err == nil {
		t.Fatal("identity-less write allowed")
	}
}
