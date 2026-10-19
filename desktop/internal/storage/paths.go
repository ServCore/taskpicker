package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var KeyPattern = regexp.MustCompile(`^[a-f0-9]{24}$`)

var ImagePattern = regexp.MustCompile(`^[a-f0-9]{64}\.png$`)

var sheetPathPattern = regexp.MustCompile(`^/(sheet|smartsheet)/[A-Za-z0-9_-]+$`)

func LibraryRoot() string {
	if custom := os.Getenv("TASKPICKER_DATA_DIR"); custom != "" {
		return filepath.Join(custom, "library")
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "TaskPicker", "library")
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "TaskPicker", "library")
	default:
		return filepath.Join(home, ".local", "share", "task-picker", "library")
	}
}

func DocumentKey(source, owner string) string {
	hash := sha256.Sum256([]byte(source + "\n" + strings.TrimSpace(owner)))
	return hex.EncodeToString(hash[:])[:24]
}

func ValidateURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Host != "docs.qq.com" || u.User != nil || !sheetPathPattern.MatchString(u.Path) {
		return "", errors.New("请填写 https://docs.qq.com/sheet/… 或 /smartsheet/… 的表格链接。")
	}
	return value, nil
}
