package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	"taskpicker/internal/media"
	"taskpicker/internal/model"
	"taskpicker/internal/reader"
	"taskpicker/internal/storage"
)

func saveDocument(ctx context.Context, root string, doc model.Document) (model.Document, error) {
	return saveDocumentAtKey(ctx, root, doc, "")
}

func saveDocumentAtKey(ctx context.Context, root string, doc model.Document, key string) (model.Document, error) {
	source := doc.SourceURL
	if source == "" {
		var err error
		source, err = filepath.Abs(doc.SourcePath)
		if err != nil {
			return model.Document{}, err
		}
		doc.SourcePath = source
	}
	if key != "" && !storage.KeyPattern.MatchString(key) {
		return model.Document{}, errors.New("本地文档编号无效。")
	}
	if key == "" {
		key = storage.DocumentKey(source, doc.Owner)
	}
	doc.Key = key
	if existing, err := ReadDocument(root, doc.Key); err == nil && pendingStatusCount(existing) > 0 {
		return model.Document{}, errors.New("这份文档有待同步的状态或备注修改，请先回写或撤销，再重新读取。")
	}
	doc.Version = 1
	doc.Cached = !doc.SetupPending
	doc.UpdatedAt = time.Now().Format("2006-01-02T15:04:05")
	if doc.SetupPending {
		doc.UpdatedAt = ""
	}
	doc.Warnings = reader.UniqueStrings(doc.Warnings)
	folder := filepath.Join(root, doc.Key)
	if err := os.MkdirAll(filepath.Join(folder, "images"), 0700); err != nil {
		return model.Document{}, err
	}
	for si := range doc.Sheets {
		for ri := range doc.Sheets[si].Rows {
			row := &doc.Sheets[si].Rows[ri]
			if row.Pictures == nil {
				row.Pictures = []model.Picture{}
			}
			for pi := range row.Pictures {
				if err := ctx.Err(); err != nil {
					return model.Document{}, err
				}
				pic := &row.Pictures[pi]
				if len(pic.Data) == 0 {
					return model.Document{}, errors.New("图片数据缺失，没有覆盖本地旧结果。")
				}
				hash := sha256.Sum256(pic.Data)
				pic.File = hex.EncodeToString(hash[:]) + ".png"
				if err := storage.AtomicWrite(filepath.Join(folder, "images", pic.File), pic.Data); err != nil {
					return model.Document{}, err
				}
				pic.Data = nil
				for variant, data := range pic.Variants {
					// Display caches are optional; the original is the source of truth.
					_ = media.StoreImageVariant(root, doc.Key, pic.File, variant, data)
				}
				pic.Variants = nil
			}
		}
	}
	data, err := marshalActivitySnapshot(root, doc, nil)
	if err != nil {
		return model.Document{}, err
	}
	if err := ctx.Err(); err != nil {
		return model.Document{}, err
	}
	if err := storage.AtomicWrite(filepath.Join(folder, "snapshot.json"), data); err != nil {
		return model.Document{}, err
	}
	// A committed snapshot is a successful sync even if updating the last-view
	// preference fails. The task data is already safely on disk.
	if err := rememberDocument(root, doc.Key); err != nil {
		doc.Warnings = append(doc.Warnings, "任务已保存，但无法记录上次打开的文档。")
	}

	return doc, nil
}
