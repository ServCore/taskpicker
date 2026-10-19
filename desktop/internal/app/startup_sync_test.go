package app

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartupSyncReportsLoginWithoutChangingCache(t *testing.T) {
	root := t.TempDir()
	doc := fixture(t, root)
	path := filepath.Join(root, doc.Key, "snapshot.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	calls := 0
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("login required")), Request: r}, nil
	})
	app := &App{root: root}
	_, err = app.SyncDocumentOnStartup(doc.SourceURL, doc.Owner, false)
	if err == nil || !strings.Contains(err.Error(), "请手动同步完成登录") {
		t.Fatalf("expected a noninteractive login error, got %v", err)
	}
	if calls != 1 || app.busy {
		t.Fatalf("startup request should finish and release the job: calls=%d, busy=%v", calls, app.busy)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("failed startup sync changed cached data: %v", err)
	}
}
