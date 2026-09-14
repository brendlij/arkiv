package thumbnails

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gallery/internal/metadata"
	"os"
	"path/filepath"
)

var Sizes = []int{256, 1024, 2048}

func Key(id, generation, size, mtime int64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("v1:%d:%d:%d:%d", id, generation, size, mtime)))
	return hex.EncodeToString(h[:])
}
func Path(cache, key string, size int) string {
	return filepath.Join(cache, key[:2], key[2:4], fmt.Sprintf("%s-%d.webp", key, size))
}
func Generate(ctx context.Context, r metadata.Runner, source, cache, key string) error {
	for _, size := range Sizes {
		target := Path(cache, key, size)
		if e := os.MkdirAll(filepath.Dir(target), 0700); e != nil {
			return e
		}
		f, e := os.CreateTemp(filepath.Dir(target), ".preview-*.webp")
		if e != nil {
			return e
		}
		temp := f.Name()
		f.Close()
		_, e = r.Run(ctx, "vipsthumbnail", source, "--size", fmt.Sprintf("%dx%d>", size, size), "--output", temp+"[Q=82,strip]")
		if e == nil {
			info, se := os.Stat(temp)
			if se != nil {
				e = se
			} else if info.Size() == 0 {
				e = fmt.Errorf("empty thumbnail")
			}
		}
		if e == nil {
			e = os.Rename(temp, target)
		}
		os.Remove(temp)
		if e != nil {
			return e
		}
	}
	return nil
}
