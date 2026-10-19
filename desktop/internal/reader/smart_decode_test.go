package reader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func smartTestState(count int) (map[string]any, map[string]any) {
	rowIDs := make([]any, count)
	records := map[string]any{}
	for i := range rowIDs {
		id := fmt.Sprintf("r%d", i+1)
		rowIDs[i] = id
		// Tencent retains edit metadata in cleared cells and empty row slots.
		records[id] = map[string]any{"1": map[string]any{"title": map[string]any{"30": 1, "31": "editor", "32": "timestamp"}}}
	}
	state := map[string]any{
		"3": map[string]any{
			"title":   map[string]any{"30": "描述", "31": 1},
			"status":  map[string]any{"30": "完成状态", "31": 17, "17": map[string]any{"3": []any{map[string]any{"1": "doing", "2": "进行中"}}}},
			"picture": map[string]any{"30": "图片1", "31": 5},
		},
		"4": []any{map[string]any{"30": "view", "1": map[string]any{"1": rowIDs, "2": []any{"title", "status", "picture"}}}},
	}
	if count > 0 {
		records["r1"] = map[string]any{"1": map[string]any{
			"title":  map[string]any{"1": []any{map[string]any{"1": "text", "2": "唯一任务"}}},
			"status": map[string]any{"17": []any{"doing"}},
		}}
	}
	return state, records
}

type smartTestTransport func(*http.Request) (*http.Response, error)

func (f smartTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSmartPublicReadSeparatesSchemaFromCompleteRecords(t *testing.T) {
	for _, schemaOnly := range []bool{true, false} {
		t.Run(fmt.Sprintf("schema=%t", schemaOnly), func(t *testing.T) {
			state, records := smartTestState(135)
			calls := 0
			client := &http.Client{Transport: smartTestTransport(func(r *http.Request) (*http.Response, error) {
				body := `<script src="https://docs.qq.com/dop-api/opendoc?test=1"></script>`
				if r.URL.Path == "/dop-api/opendoc" {
					calls++
					end, _ := strconv.Atoi(r.URL.Query().Get("endrow"))
					if schemaOnly && end != 99 {
						t.Fatalf("schema request fetched task pages: %s", r.URL)
					}
					page := map[string]any{}
					for i := 0; i < min(end+1, 135); i++ {
						id := fmt.Sprintf("r%d", i+1)
						page[id] = records[id]
					}
					ops, _ := json.Marshal([]any{[]any{
						map[string]any{"t": 3005, "c": map[string]any{"1": "target", "3": state}},
						map[string]any{"t": 3028, "c": map[string]any{"1": "target", "2": map[string]any{"1": page}}},
					}})
					payload, _ := json.Marshal(map[string]any{"clientVars": map[string]any{
						"title": "个人任务", "privilegeAttribute": map[string]any{"can_read": 1},
						"collab_client_vars": map[string]any{
							"header":                []any{map[string]any{"d": []any{map[string]any{"id": "target", "name": "任务", "type": "smartsheet"}}}},
							"initialAttributedText": map[string]any{"text": []any{map[string]any{"max_row": 135, "max_col": 3, "smartsheet": string(ops)}}},
						},
					}})
					body = string(payload)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
			})}
			doc, err := ReadPublicTasks(context.Background(), "https://docs.qq.com/sheet/test?tab=target&viewId=view", "", func(string) {}, client, false, ReadOptions{SchemaOnly: schemaOnly})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || len(doc.Sheets) != 1 || len(doc.Sheets[0].Headers) != 3 {
				t.Fatalf("unexpected discovery: %+v, requests=%d", doc, calls)
			}
			rows := doc.Sheets[0].Rows
			if schemaOnly {
				if len(rows) != 0 {
					t.Fatal("schema discovery returned tasks")
				}
			} else if len(rows) != 1 || rows[0].Number != 1 || rows[0].RecordID != "r1" || rows[0].Values[0] != "唯一任务" {
				t.Fatalf("empty rows became tasks or identity changed: %+v", rows)
			}
		})
	}
}

func TestSmartFullReadStillRejectsMissingRecords(t *testing.T) {
	state, records := smartTestState(135)
	delete(records, "r135")
	if _, err := DecodeSmart(state, records, "view"); err == nil {
		t.Fatal("incomplete task pull was accepted")
	}
	grid, err := DecodeSmart(state, nil, "view", ReadOptions{SchemaOnly: true})
	if err != nil || len(grid.Cells) != 3 || len(grid.RowIDs) != 0 {
		t.Fatalf("schema depends on records: %+v %v", grid, err)
	}
}

func TestSmartEmptyRowsKeepPositionsAndUnsupportedContent(t *testing.T) {
	state, records := smartTestState(4)
	records["r2"] = map[string]any{"1": map[string]any{"title": map[string]any{"1": []any{map[string]any{"1": "text", "2": " \n"}}}, "picture": map[string]any{"5": []any{}}}}
	records["r4"] = map[string]any{"1": map[string]any{"picture": map[string]any{"5": []any{map[string]any{"attachment": "nonempty"}}}}}
	grid, err := DecodeSmart(state, records, "view")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := DocumentFromGrid(context.Background(), grid, "任务", "任务", "", "", func(string) {}, true, nil, true)
	if err != nil || len(doc.Sheets[0].Rows) != 2 || doc.Sheets[0].Rows[1].Number != 4 || doc.Sheets[0].Rows[1].RecordID != "r4" || len(doc.Warnings) == 0 {
		t.Fatalf("blank row removal changed row identity or hid unsupported content: %+v %v", doc, err)
	}
}
