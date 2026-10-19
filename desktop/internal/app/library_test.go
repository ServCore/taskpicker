package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"taskpicker/internal/model"
	"taskpicker/internal/storage"
)

func fixture(t *testing.T, root string) model.Document {
	t.Helper()
	key := storage.DocumentKey(presets[0].SourceURL, "王小明")
	filename := strings.Repeat("a", 64) + ".png"
	doc := model.Document{Version: 1, Title: "示例应用 任务清单", Owner: "王小明", SourceURL: presets[0].SourceURL,
		Sheets: []model.TaskSheet{{Name: "示例测试问题清单", Headers: []any{"责任人", "问题描述"}, Rows: []model.TaskRow{
			{Number: 4, Values: []any{"王小明", "无法打开麦克风\n保留完整文本"}, Pictures: []model.Picture{{Row: 4, Column: 3, File: filename}}},
		}}},
	}
	folder := filepath.Join(root, key)
	if err := os.MkdirAll(filepath.Join(folder, "images"), 0700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(doc)
	if err := os.WriteFile(filepath.Join(folder, "snapshot.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "images", filename), []byte("png-fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	doc.Key = key
	return doc
}

func TestExistingSnapshotCompatibility(t *testing.T) {
	root := t.TempDir()
	want := fixture(t, root)
	if want.Key != "fa0cda142f51ba4225dcc026" {
		t.Fatalf("key differs from existing snapshots: %s", want.Key)
	}
	if err := rememberDocument(root, want.Key); err != nil {
		t.Fatal(err)
	}
	state, err := readLibrary(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Documents) != 2 || state.SelectedKey != want.Key || !state.Documents[0].Cached {
		t.Fatalf("bad library: %+v", state)
	}
	row := state.Documents[0].Sheets[0].Rows[0]
	if row.Number != 4 || !strings.Contains(row.Values[1].(string), "\n") || len(row.Pictures) != 1 {
		t.Fatalf("lost row content: %+v", row)
	}
}

func TestImageHandlerServesOfflineImageAndRejectsOtherFiles(t *testing.T) {
	root := t.TempDir()
	doc := fixture(t, root)
	a := &App{root: root}
	image := "/local-image/" + doc.Key + "/" + doc.Sheets[0].Rows[0].Pictures[0].File
	r := httptest.NewRecorder()
	ImageHandler(a).ServeHTTP(r, httptest.NewRequest("GET", image, nil))
	if r.Code != http.StatusOK || r.Body.String() != "png-fixture" || r.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("invalid image response: %d %s", r.Code, r.Body.String())
	}
	for _, path := range []string{"/snapshot.json", "/local-image/../last.json", "/local-image/" + doc.Key + "/snapshot.json"} {
		r := httptest.NewRecorder()
		ImageHandler(a).ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 404 {
			t.Fatalf("exposed %s: %d", path, r.Code)
		}
	}
	outside := filepath.Join(t.TempDir(), "private.png")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	filename := strings.Repeat("b", 64) + ".png"
	if err := os.Symlink(outside, filepath.Join(root, doc.Key, "images", filename)); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRecorder()
	ImageHandler(a).ServeHTTP(r, httptest.NewRequest("GET", "/local-image/"+doc.Key+"/"+filename, nil))
	if r.Code != 404 {
		t.Fatal("image symlink escaped library")
	}
}

func TestCorruptCacheDoesNotHideOtherDocuments(t *testing.T) {
	root := t.TempDir()
	doc := fixture(t, root)
	key := strings.Repeat("b", 24)
	if err := os.MkdirAll(filepath.Join(root, key), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, key, "snapshot.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := readLibrary(root)
	if err != nil || len(state.Warnings) != 1 || state.Documents[0].Key != doc.Key {
		t.Fatalf("corrupt cache not isolated: %+v, %v", state, err)
	}
}

func TestMissingImageIsReported(t *testing.T) {
	root := t.TempDir()
	doc := fixture(t, root)
	if err := os.Remove(filepath.Join(root, doc.Key, "images", doc.Sheets[0].Rows[0].Pictures[0].File)); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDocument(root, doc.Key)
	if err != nil || len(got.Warnings) != 1 {
		t.Fatalf("missing image not reported: %+v, %v", got, err)
	}
}

func TestOnlyTencentSheetURLsAccepted(t *testing.T) {
	for _, value := range []string{presets[0].SourceURL, presets[1].SourceURL} {
		if _, err := storage.ValidateURL(value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []string{"http://docs.qq.com/sheet/abc", "https://docs.qq.com.evil.test/sheet/abc", "https://user@docs.qq.com/sheet/abc", "https://docs.qq.com/other/abc"} {
		if _, err := storage.ValidateURL(value); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
}
