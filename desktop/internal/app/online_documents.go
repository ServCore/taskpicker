package app

import (
	"context"
	"errors"

	"taskpicker/internal/model"
	"taskpicker/internal/reader"
	"taskpicker/internal/storage"
)

// PrepareOnlineDocument registers only the discovered columns. The first task
// pull is explicit, after the user has chosen document-level conditions.
func (a *App) PrepareOnlineDocument(link string) (model.Document, error) {
	link, err := storage.ValidateURL(link)
	if err != nil {
		return model.Document{}, err
	}
	a.projects.Lock()
	state, err := readLibrary(a.root)
	a.projects.Unlock()
	if err != nil {
		return model.Document{}, err
	}
	key, owner := storage.DocumentKey(link, ""), ""
	for _, doc := range state.Documents {
		if doc.SourceURL == link {
			if len(doc.Sheets) > 0 {
				return doc, nil
			}
			key, owner = doc.Key, doc.Owner
			break
		}
	}
	return a.runDocumentJob(owner, key, func(ctx context.Context, notify reader.ProgressFunc) (model.Document, error) {
		opts := reader.ReadOptions{SchemaOnly: true}
		doc, err := reader.ReadPublicTasks(ctx, link, owner, notify, nil, true, opts)
		if errors.Is(err, reader.ErrNeedsLogin) {
			doc, err = reader.ReadAuthenticatedTasks(ctx, link, owner, notify, true, opts)
		}
		if err != nil {
			return model.Document{}, err
		}
		doc.SetupPending = true
		return doc, nil
	})
}
