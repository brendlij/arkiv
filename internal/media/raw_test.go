package media

import (
	"context"
	"crypto/sha256"
	"gallery/internal/metadata"
	"gallery/internal/thumbnails"
	"os"
	"path/filepath"
	"testing"
)

func TestRAWFixtures(t *testing.T) {
	root := os.Getenv("ARKIV_RAW_FIXTURES")
	if root == "" {
		t.Skip("set ARKIV_RAW_FIXTURES to real camera sample directory")
	}
	files, e := os.ReadDir(root)
	if e != nil {
		t.Fatal(e)
	}
	tested := map[string]bool{}
	for _, entry := range files {
		ext := filepath.Ext(entry.Name())
		if ext != ".ARW" && ext != ".DNG" && ext != ".CR2" && ext != ".CR3" && ext != ".NEF" && ext != ".RAF" {
			continue
		}
		tested[ext] = true
		t.Run(ext, func(t *testing.T) {
			source := filepath.Join(root, entry.Name())
			before, e := os.ReadFile(source)
			if e != nil {
				t.Fatal(e)
			}
			hash := sha256.Sum256(before)
			temp := t.TempDir()
			preview := filepath.Join(temp, "embedded.jpg")
			r := metadata.Command{}
			if e = (EmbeddedRAW{Runner: r}).Extract(context.Background(), source, preview); e != nil {
				t.Fatal(e)
			}
			if e = thumbnails.Generate(context.Background(), r, preview, temp, thumbnails.Key(1, 1, 1, 1)); e != nil {
				t.Fatal(e)
			}
			after, e := os.ReadFile(source)
			if e != nil || sha256.Sum256(after) != hash {
				t.Fatal("original changed")
			}
		})
	}
	for _, ext := range []string{".ARW", ".DNG", ".CR2", ".CR3", ".NEF", ".RAF"} {
		if !tested[ext] {
			t.Errorf("missing fixture: %s", ext)
		}
	}
}
