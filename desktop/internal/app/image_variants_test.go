package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"taskpicker/internal/media"
	"taskpicker/internal/model"
	"taskpicker/internal/reader"
)

func largeTestPNG(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 1200, 800))); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestDisplayVariantsPreserveOriginalAndBoundDecodedImageSize(t *testing.T) {
	root := t.TempDir()
	raw := largeTestPNG(t)
	picture, err := reader.PngPicture(raw, 4, 3, 1200, 800)
	if err != nil {
		t.Fatal(err)
	}
	doc := model.Document{SourceURL: presets[0].SourceURL, Owner: "王小明", Title: "截图", Sheets: []model.TaskSheet{{Name: "任务表", Headers: []any{"任务", "负责人"}, Rows: []model.TaskRow{{Number: 4, Values: []any{"任务", "王小明"}, Pictures: []model.Picture{picture}}}}}}
	saved, err := saveDocument(context.Background(), root, doc)
	if err != nil {
		t.Fatal(err)
	}
	pic := saved.Sheets[0].Rows[0].Pictures[0]
	if pic.Data != nil || pic.Variants != nil {
		t.Fatal("saved snapshot retained transient image buffers")
	}
	app := &App{root: root}
	for _, sample := range []struct {
		size          string
		width, height int
	}{{"", 1200, 800}, {"thumbnail", 144, 96}, {"preview", 960, 640}} {
		url := "/local-image/" + saved.Key + "/" + pic.File
		if sample.size != "" {
			url += "?size=" + sample.size
		}
		response := httptest.NewRecorder()
		ImageHandler(app).ServeHTTP(response, httptest.NewRequest("GET", url, nil))
		if response.Code != 200 {
			t.Fatalf("%s returned %d", sample.size, response.Code)
		}
		config, err := png.DecodeConfig(bytes.NewReader(response.Body.Bytes()))
		if err != nil || config.Width != sample.width || config.Height != sample.height {
			t.Fatalf("unexpected %s size: %+v, %v", sample.size, config, err)
		}
		if sample.size == "" && !bytes.Equal(raw, response.Body.Bytes()) {
			t.Fatal("original pixels/bytes were replaced by a preview")
		}
	}
}

func TestLegacyPreviewCacheIsLazyAndCannotExposeOutsideFiles(t *testing.T) {
	root := t.TempDir()
	doc := fixture(t, root)
	raw := largeTestPNG(t)
	hash := sha256.Sum256(raw)
	file := hex.EncodeToString(hash[:]) + ".png"
	if err := os.WriteFile(filepath.Join(root, doc.Key, "images", file), raw, 0600); err != nil {
		t.Fatal(err)
	}
	app := &App{root: root}
	url := "/local-image/" + doc.Key + "/" + file + "?size=thumbnail"
	first := httptest.NewRecorder()
	ImageHandler(app).ServeHTTP(first, httptest.NewRequest("GET", url, nil))
	if first.Code != 200 {
		t.Fatal(first.Code)
	}
	path := media.ImageVariantPath(root, doc.Key, file, "thumbnail")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	second := httptest.NewRecorder()
	ImageHandler(app).ServeHTTP(second, httptest.NewRequest("GET", url, nil))
	after, err := os.Stat(path)
	if err != nil || !before.ModTime().Equal(after.ModTime()) || !bytes.Equal(first.Body.Bytes(), second.Body.Bytes()) {
		t.Fatal("warm preview was regenerated", err)
	}
	bad := httptest.NewRecorder()
	ImageHandler(app).ServeHTTP(bad, httptest.NewRequest("GET", url+"-unknown", nil))
	if bad.Code != 404 {
		t.Fatal("accepted unsupported size")
	}
	// A hostile derivative-cache symlink must never be served or written through.
	outside := t.TempDir()
	secret := filepath.Join(outside, file)
	if err := os.WriteFile(secret, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, doc.Key, "previews")); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	ImageHandler(app).ServeHTTP(response, httptest.NewRequest("GET", "/local-image/"+doc.Key+"/"+file+"?size=preview", nil))
	if response.Code != 200 || response.Body.String() == "private" {
		t.Fatal("unsafe cache response")
	}
	unchanged, err := os.ReadFile(secret)
	if err != nil || string(unchanged) != "private" {
		t.Fatal("wrote through cache directory symlink", err)
	}
}

func BenchmarkImageVariants(b *testing.B) {
	source := image.NewNRGBA(image.Rect(0, 0, 4096, 2048))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := media.ImageVariants(source); err != nil {
			b.Fatal(err)
		}
	}
}
