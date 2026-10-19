package app

import (
	"encoding/json"
	"os"
	"testing"

	"taskpicker/internal/model"
)

// Fictional fixtures are available only in test binaries.
func init() {
	presets = []model.Document{
		{Title: "示例应用 任务清单", SourceURL: "https://docs.qq.com/sheet/example-sheet?tab=ddczrm", Owner: "王小明"},
		{Title: "示例项目（数智）", SourceURL: "https://docs.qq.com/sheet/example-smart-sheet?tab=ss_mo1bbr&viewId=ve3McK", Owner: "王小明"},
	}
}

func liveTestDocuments(t *testing.T) []model.Document {
	t.Helper()
	filename := os.Getenv("TASKPICKER_LIVE_DOCUMENTS_FILE")
	if filename == "" {
		t.Fatal("set TASKPICKER_LIVE_DOCUMENTS_FILE to your private test document JSON file")
	}
	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var documents []model.Document
	if err := json.Unmarshal(raw, &documents); err != nil || len(documents) == 0 {
		t.Fatalf("invalid or empty live document configuration: %v", err)
	}
	for _, doc := range documents {
		if doc.SourceURL == "" || doc.Owner == "" {
			t.Fatal("each live document requires source_url and owner")
		}
	}
	return documents
}

func TestFreshLibraryHasNoPersonalDocuments(t *testing.T) {
	fixtures := presets
	presets = nil
	t.Cleanup(func() { presets = fixtures })
	state, err := readLibrary(t.TempDir())
	if err != nil || len(state.Documents) != 0 {
		t.Fatalf("fresh library unexpectedly contains documents: %+v, %v", state, err)
	}
}
