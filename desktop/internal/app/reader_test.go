package app

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"taskpicker/internal/model"
	"taskpicker/internal/reader"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testPNG(t *testing.T, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 20, 12))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func appendWire(dst []byte, id int, value any) []byte {
	switch v := value.(type) {
	case int:
		dst = binary.AppendUvarint(dst, uint64(id<<3))
		return binary.AppendUvarint(dst, uint64(v))
	case string:
		return appendWire(dst, id, []byte(v))
	case []byte:
		dst = binary.AppendUvarint(dst, uint64(id<<3|2))
		dst = binary.AppendUvarint(dst, uint64(len(v)))
		return append(dst, v...)
	default:
		panic("bad wire fixture")
	}
}

func wireCell(row, col, index int, extra []byte) []byte {
	data := appendWire(nil, 1, 4)
	data = appendWire(data, 2, appendWire(nil, 1, index))
	data = append(data, extra...)
	cell := appendWire(nil, 1, row)
	cell = appendWire(cell, 2, col)
	return appendWire(nil, 6, appendWire(cell, 3, data))
}

func encodedGrid(t *testing.T, first, last int, content []byte) string {
	t.Helper()
	bounds := appendWire(nil, 1, "target")
	bounds = appendWire(bounds, 2, first)
	bounds = appendWire(bounds, 4, last)
	bounds = appendWire(bounds, 5, 2)
	sheet := appendWire(nil, 3, bounds)
	var pool []byte
	for _, value := range []string{"任务", "责任人", "问题截图", "我的问题", "其他人的问题", "王小明"} {
		pool = appendWire(pool, 1, appendWire(nil, 1, value))
	}
	sheet = appendWire(sheet, 5, pool)
	sheet = append(sheet, content...)
	raw := appendWire(nil, 1, appendWire(nil, 5, appendWire(nil, 19, sheet)))
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(compressed.Bytes())
}

func gridFixture(t *testing.T) *reader.GridData {
	t.Helper()
	var cells []byte
	for i := 0; i < 3; i++ {
		cells = append(cells, wireCell(0, i, i, nil)...)
	}
	person, _ := json.Marshal([][]string{{"uid", "王小明", "", ""}})
	people := appendWire(nil, 16, 1)
	people = appendWire(people, 17, appendWire(nil, 1, person))
	imageJSON, _ := json.Marshal([]any{"https://docimg1.docs.qq.com/image/p.png", 20, 12})
	pic := appendWire(nil, 16, 5)
	pic = appendWire(pic, 17, appendWire(nil, 1, imageJSON))
	cells = append(cells, wireCell(1, 0, 3, nil)...)
	cells = append(cells, wireCell(1, 1, 0, people)...)
	cells = append(cells, wireCell(1, 2, 0, pic)...)
	cells = append(cells, wireCell(2, 0, 4, nil)...)
	g, err := reader.DecodeGrid(encodedGrid(t, 0, 2, cells), "target")
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestOwnerNamesAndHeaderAliases(t *testing.T) {
	for _, value := range []string{"王小明", " 王小明 ", "李小华，王小明", "王小明\n陈小林", "@王小明"} {
		if !reader.OwnerMatches(value, "王小明") {
			t.Fatalf("did not match %q", value)
		}
	}
	for _, value := range []any{"王小明双", "小王小明", nil, "王琪 双"} {
		if reader.OwnerMatches(value, "王小明") {
			t.Fatalf("matched %q", value)
		}
	}
	for _, header := range []string{"负责人", "负责 人员", "责任人"} {
		if !reader.OwnerHeader(header) {
			t.Fatal(header)
		}
	}
}

func TestGridPeoplePicturesAndReferenceHeader(t *testing.T) {
	g := gridFixture(t)
	if g.Cells[reader.CellPosition{1, 1}] != "王小明" || g.Images[reader.CellPosition{1, 2}][0].Row != 1 {
		t.Fatalf("bad decoded data: %+v", g)
	}
	raw := testPNG(t, color.NRGBA{B: 255, A: 255})
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Referer") != "https://docs.qq.com/" {
			t.Error("missing image referrer")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(raw)), Header: http.Header{}}, nil
	})}
	doc, err := reader.DocumentFromGrid(context.Background(), g, "test", "任务表", "url", "王小明", func(string) {}, false, client, false)
	if err != nil {
		t.Fatal(err)
	}
	rows := doc.Sheets[0].Rows
	if len(rows) != 1 || rows[0].Number != 2 || len(rows[0].Pictures) != 1 || rows[0].Pictures[0].Column != 3 {
		t.Fatalf("lost row-image association: %+v", rows)
	}
	if _, err := reader.WireDecode([]byte{0x0a, 0xff}); err == nil {
		t.Fatal("accepted truncated wire data")
	}
	empty, err := reader.DecodeGrid(encodedGrid(t, 0, 1, nil), "other")
	if err != nil || len(empty.Cells) != 0 {
		t.Fatal("wrong tab was decoded")
	}
}

func TestSmartSheetIncludesLateRecordsAndRejectsTruncation(t *testing.T) {
	ids := []any{}
	records := map[string]any{}
	for i := 0; i < 134; i++ {
		rid := fmt.Sprintf("r%d", i)
		ids = append(ids, rid)
		owners := []any{"other"}
		if i == 133 {
			owners = append(owners, "wang")
		}
		records[rid] = map[string]any{"1": map[string]any{"task": map[string]any{"1": []any{map[string]any{"1": "text", "2": rid}}}, "owner": map[string]any{"9": owners}}}
	}
	state := map[string]any{"3": map[string]any{"task": map[string]any{"30": "任务", "31": 1}, "owner": map[string]any{"30": "负责人员", "31": 9, "9": map[string]any{"3": []any{map[string]any{"1": "wang", "2": "王小明"}, map[string]any{"1": "other", "2": "其他人"}}}}}, "4": []any{map[string]any{"30": "view", "1": map[string]any{"1": ids, "2": []any{"task", "owner"}}}}}
	last := records["r133"]
	delete(records, "r133")
	if _, err := reader.DecodeSmart(state, records, "view"); err == nil || !strings.Contains(err.Error(), "没有加载") {
		t.Fatalf("truncation accepted: %v", err)
	}
	records["r133"] = last
	grid, err := reader.DecodeSmart(state, records, "view")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := reader.DocumentFromGrid(context.Background(), grid, "test", "任务表", "url", "王小明", func(string) {}, true, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sheets[0].Rows) != 1 || doc.Sheets[0].Rows[0].Number != 134 {
		t.Fatal("lost record past initial 60 rows")
	}
}

func TestPublicReaderPaginatesGridBlocks(t *testing.T) {
	calls := 0
	client := reader.NewHTTPClient()
	client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		if strings.HasPrefix(r.URL.Path, "/sheet/") {
			w.Header().Set("Set-Cookie", "session=ok; Path=/; Secure")
			fmt.Fprint(w, `<script src="https://docs.qq.com/dop-api/opendoc?foo=1&amp;bar=2"></script>`)
		} else {
			calls++
			if cookie, err := r.Cookie("session"); err != nil || cookie.Value != "ok" {
				t.Error("metadata cookie session lost")
			}
			first, last := 0, 255
			if r.URL.Query().Has("block_start_row") {
				first = reader.IntText(r.URL.Query().Get("block_start_row"))
				last = reader.IntText(r.URL.Query().Get("block_end_row"))
			}
			var cells []byte
			if first == 0 {
				cells = append(cells, wireCell(0, 0, 0, nil)...)
				cells = append(cells, wireCell(0, 1, 1, nil)...)
				cells = append(cells, wireCell(1, 0, 3, nil)...)
				cells = append(cells, wireCell(1, 1, 5, nil)...)
			}
			if first == 512 {
				cells = append(cells, wireCell(512, 0, 3, nil)...)
				cells = append(cells, wireCell(512, 1, 5, nil)...)
			}
			metadata := map[string]any{"clientVars": map[string]any{"title": "分页测试", "privilegeAttribute": map[string]any{"can_read": 1}, "collab_client_vars": map[string]any{"header": []any{map[string]any{"d": []any{map[string]any{"id": "target", "name": "任务表", "type": "sheet"}}}}, "initialAttributedText": map[string]any{"text": []any{map[string]any{"max_row": 513, "max_col": 3, "block_datas": []any{map[string]any{"related_sheet": encodedGrid(t, first, last, cells)}}}}}}}}
			encoded, _ := json.Marshal(metadata)
			fmt.Fprintf(w, "callback(%s)", encoded)
		}
		response := w.Result()
		response.Request = r
		return response, nil
	})
	doc, err := reader.ReadPublicTasks(context.Background(), "https://docs.qq.com/sheet/test?tab=target", "王小明", func(string) {}, client, false)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || len(doc.Sheets[0].Rows) != 2 || doc.Sheets[0].Rows[1].Number != 513 {
		t.Fatalf("bad pagination: %d calls, %+v", calls, doc.Sheets[0].Rows)
	}
}

func TestImageFailureAndCancellationKeepOldSnapshot(t *testing.T) {
	root := t.TempDir()
	old := fixture(t, root)
	before, err := os.ReadFile(filepath.Join(root, old.Key, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
	})}
	a := &App{root: root}
	_, err = a.runJob("王小明", func(ctx context.Context, notify reader.ProgressFunc) (model.Document, error) {
		return reader.DocumentFromGrid(ctx, gridFixture(t), "new", "任务表", old.SourceURL, "王小明", notify, false, client, false)
	})
	if err == nil || !strings.Contains(err.Error(), "没有覆盖") {
		t.Fatalf("incomplete success: %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(root, old.Key, "snapshot.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("failed sync overwrote cached data")
	}
	started, done := make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := a.runJob("王小明", func(ctx context.Context, _ reader.ProgressFunc) (model.Document, error) {
			close(started)
			<-ctx.Done()
			return model.Document{}, ctx.Err()
		})
		done <- err
	}()
	<-started
	if _, err := a.runJob("王小明", func(context.Context, reader.ProgressFunc) (model.Document, error) {
		t.Error("concurrent job ran")
		return old, nil
	}); err == nil {
		t.Fatal("allowed concurrent sync")
	}
	a.CancelSync()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "已取消") {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not stop reader")
	}
	after, _ = os.ReadFile(filepath.Join(root, old.Key, "snapshot.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("cancelled sync overwrote cache")
	}
}

func TestSaveReadAndOfflineImageRoundtrip(t *testing.T) {
	root := t.TempDir()
	raw := testPNG(t, color.NRGBA{R: 255, A: 255})
	doc := model.Document{SourceURL: presets[0].SourceURL, Owner: "王小明", Title: "离线任务", Sheets: []model.TaskSheet{{Name: "表格", Headers: []any{"任务", "负责人", "图片"}, Rows: []model.TaskRow{{Number: 4, Values: []any{"完整内容\n第二行", "王小明", nil}, Pictures: []model.Picture{{Row: 4, Column: 3, Data: raw, Width: 20, Height: 12}}}}}}}
	saved, err := saveDocument(context.Background(), root, doc)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadDocument(root, saved.Key)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Sheets[0].Rows[0].Number != 4 || loaded.Sheets[0].Rows[0].Values[0] != "完整内容\n第二行" {
		t.Fatal("lost offline content")
	}
	file := filepath.Join(root, saved.Key, "images", loaded.Sheets[0].Rows[0].Pictures[0].File)
	got, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatal("lost original image bytes")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := saveDocument(ctx, root, doc); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled snapshot committed")
	}
}

func TestDates(t *testing.T) {
	if reader.DateText(float64(20260923), "测试时间") != "2026-09-23" || reader.DateText(float64(46300), "日期") != "2026-10-05" {
		t.Fatal("date conversion changed")
	}
	if reader.DateText(float64(46300), "数量") != float64(46300) {
		t.Fatal("non-date number was formatted")
	}
}

// This opt-in integration test compares all rows, values, and image pixels to a
// one-time reference file during migration. Normal tests never access the net.
func TestLiveDocuments(t *testing.T) {
	if os.Getenv("TASKPICKER_LIVE_TESTS") != "1" {
		t.Skip("set TASKPICKER_LIVE_TESTS=1 to read your configured Tencent documents")
	}
	type goldenPicture struct {
		Column, Width, Height int
		Pixels                string
	}
	type goldenRow struct {
		Number   int
		Values   []any
		Pictures []goldenPicture
	}
	type goldenSheet struct {
		Name    string
		Headers []any
		Scanned int
		Rows    []goldenRow
	}
	type goldenDocument struct {
		URL, Title string
		Sheets     []goldenSheet
	}
	var references []goldenDocument
	if file := os.Getenv("TASKPICKER_GOLDEN_FILE"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &references); err != nil {
			t.Fatal(err)
		}
	}
	for _, preset := range liveTestDocuments(t) {
		t.Run(preset.Title, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			doc, err := reader.ReadPublicTasks(ctx, preset.SourceURL, preset.Owner, func(message string) { t.Log(message) }, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			count, pics := 0, 0
			for _, sheet := range doc.Sheets {
				count += len(sheet.Rows)
				for _, row := range sheet.Rows {
					pics += len(row.Pictures)
				}
			}
			t.Logf("%s: %d tasks, %d pictures", doc.Title, count, pics)
			for _, reference := range references {
				if reference.URL != preset.SourceURL {
					continue
				}
				if reference.Title != doc.Title || len(reference.Sheets) != len(doc.Sheets) {
					t.Fatal("document structure differs from reference")
				}
				for i, want := range reference.Sheets {
					got := doc.Sheets[i]
					if want.Name != got.Name || !reflect.DeepEqual(want.Headers, got.Headers) || want.Scanned != got.Scanned || len(want.Rows) != len(got.Rows) {
						t.Fatalf("sheet differs: name=%s scanned=%d rows=%d", got.Name, got.Scanned, len(got.Rows))
					}
					for j, row := range want.Rows {
						actual := got.Rows[j]
						if row.Number != actual.Number || !reflect.DeepEqual(row.Values, actual.Values) || len(row.Pictures) != len(actual.Pictures) {
							t.Fatalf("row %d differs from reference", row.Number)
						}
						for k, picture := range row.Pictures {
							p := actual.Pictures[k]
							img, _, err := image.Decode(bytes.NewReader(p.Data))
							if err != nil {
								t.Fatal(err)
							}
							rgba := image.NewNRGBA(img.Bounds())
							draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
							hash := sha256.Sum256(rgba.Pix)
							if p.Column != picture.Column || img.Bounds().Dx() != picture.Width || img.Bounds().Dy() != picture.Height || hex.EncodeToString(hash[:]) != picture.Pixels {
								t.Fatalf("row %d picture %d pixels differ", row.Number, k)
							}
						}
					}
				}
			}
			root := t.TempDir()
			saved, err := saveDocument(ctx, root, doc)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ReadDocument(root, saved.Key); err != nil {
				t.Fatal(err)
			}
			for _, sheet := range saved.Sheets {
				for _, row := range sheet.Rows {
					for _, pic := range row.Pictures {
						request := httptest.NewRequest("GET", (&url.URL{Path: "/local-image/" + saved.Key + "/" + pic.File}).String(), nil)
						recorder := httptest.NewRecorder()
						ImageHandler(&App{root: root}).ServeHTTP(recorder, request)
						if recorder.Code != 200 {
							t.Fatal("offline image not served")
						}
					}
				}
			}
		})
	}
}
