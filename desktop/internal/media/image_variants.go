package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/image/draw"

	"taskpicker/internal/storage"
)

var ImageVariantSizes = map[string]int{"thumbnail": 144, "preview": 960}

// A cold cache must not decode several full-size screenshots concurrently.
var imageVariantMu sync.Mutex

func encodeImageVariant(source image.Image, maxSide int) ([]byte, error) {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width > maxSide || height > maxSide {
		if width >= height {
			height = max(1, height*maxSide/width)
			width = maxSide
		} else {
			width = max(1, width*maxSide/height)
			height = maxSide
		}
		resized := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.ApproxBiLinear.Scale(resized, resized.Bounds(), source, bounds, draw.Src, nil)
		source = resized
	}
	var out bytes.Buffer
	err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&out, source)
	return out.Bytes(), err
}

func ImageVariants(source image.Image) (map[string][]byte, error) {
	variants := make(map[string][]byte, len(ImageVariantSizes))
	for variant, size := range ImageVariantSizes {
		raw, err := encodeImageVariant(source, size)
		if err != nil {
			return nil, err
		}
		variants[variant] = raw
	}
	return variants, nil
}

func LibraryFile(root, path string) (string, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(resolved, resolvedRoot+string(os.PathSeparator)) {
		return "", errors.New("图片路径不在本地任务库中")
	}
	return resolved, nil
}

func ImageVariantPath(root, key, file, variant string) string {
	return filepath.Join(root, key, variant+"s", file)
}

func StoreImageVariant(root, key, file, variant string, data []byte) error {
	docDir, err := LibraryFile(root, filepath.Join(root, key))
	if err != nil {
		return err
	}
	dir := filepath.Join(docDir, variant+"s")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	dir, err = LibraryFile(root, dir)
	if err != nil {
		return err
	}
	return storage.AtomicWrite(filepath.Join(dir, file), data)
}

// Old snapshots are upgraded lazily. Originals remain unchanged on disk.
func LoadImageVariant(ctx context.Context, root, key, file, variant, original string) (path string, data []byte, err error) {
	imageVariantMu.Lock()
	defer imageVariantMu.Unlock()
	if path, err := LibraryFile(root, ImageVariantPath(root, key, file, variant)); err == nil {
		return path, nil, nil
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	input, err := os.Open(original)
	if err != nil {
		return "", nil, err
	}
	defer input.Close()
	config, err := png.DecodeConfig(input)
	if err != nil {
		return "", nil, err
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 40_000_000 {
		return "", nil, errors.New("图片尺寸过大")
	}
	if _, err = input.Seek(0, 0); err != nil {
		return "", nil, err
	}
	source, err := png.Decode(input)
	if err != nil {
		return "", nil, err
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	data, err = encodeImageVariant(source, ImageVariantSizes[variant])
	if err != nil {
		return "", nil, err
	}
	// A read-only cache can still display the generated small image.
	if err = StoreImageVariant(root, key, file, variant, data); err == nil {
		return ImageVariantPath(root, key, file, variant), nil, nil
	}
	return "", data, nil
}
