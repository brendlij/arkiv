package media

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadContainerSignatures(t *testing.T) {
	box := func(brand string) []byte {
		b := make([]byte, 24)
		binary.BigEndian.PutUint32(b, 24)
		copy(b[4:], "ftyp")
		copy(b[8:], brand)
		copy(b[16:], brand)
		return b
	}
	cases := []struct {
		ext  string
		data []byte
	}{
		{".mp4", box("isom")}, {".mov", box("qt  ")}, {".3gp", box("3gp6")}, {".cr3", box("crx ")}, {".heic", box("heic")}, {".avif", box("avif")},
		{".webp", append([]byte("RIFF\x14\x00\x00\x00WEBPVP8 "), make([]byte, 16)...)},
		{".avi", append([]byte("RIFF\x14\x00\x00\x00AVI "), make([]byte, 16)...)},
		{".mkv", append([]byte("\x1a\x45\xdf\xa3"), make([]byte, 16)...)},
		{".dng", append([]byte("II*\x00"), make([]byte, 16)...)},
		{".raf", append([]byte("FUJIFILMCCD-RAW"), make([]byte, 16)...)},
		{".rw2", append([]byte("IIU\x00"), make([]byte, 16)...)},
		{".orf", append([]byte("IIRO"), make([]byte, 16)...)},
	}
	for _, c := range cases {
		t.Run(c.ext, func(t *testing.T) {
			if _, e := ValidateUpload(bytes.NewReader(c.data), c.ext); e != nil {
				t.Fatal(e)
			}
			if _, e := ValidateUpload(strings.NewReader("<html>not media</html>"), c.ext); e == nil {
				t.Fatal("accepted non-media")
			}
		})
	}
	for _, c := range []struct {
		ext  string
		data []byte
	}{{".heic", box("isom")}, {".mp4", box("heic")}, {".avif", box("heic")}, {".cr3", box("isom")}, {".svg", []byte("<svg/>")}, {".exe", box("isom")}} {
		if _, e := ValidateUpload(bytes.NewReader(c.data), c.ext); e == nil {
			t.Fatalf("accepted mismatch %s", c.ext)
		}
	}
	for _, f := range Formats() {
		if _, e := ValidateUpload(bytes.NewReader(nil), f.Extension); e == nil {
			t.Fatalf("empty %s accepted", f.Extension)
		}
	}
}

func TestLocalMediaFixtures(t *testing.T) {
	root := os.Getenv("ARKIV_MEDIA_FIXTURE_ROOT")
	if root == "" {
		t.Skip("optional local camera and video fixtures")
	}
	count := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if entry.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if _, ok := Lookup(ext); !ok {
			return nil
		}
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		defer f.Close()
		if _, e = ValidateUpload(f, ext); e != nil {
			t.Errorf("%s: %v", entry.Name(), e)
		}
		count++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no fixtures")
	}
	t.Logf("Validated %d real fixture files", count)
}
