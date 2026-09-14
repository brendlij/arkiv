package media

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"sort"
	"strings"
)

const ImageUploadLimit int64 = 250 << 20
const VideoUploadLimit int64 = 2 << 30
const UploadQuota int64 = 5 << 30

type Format struct {
	Extension string `json:"extension"`
	Kind      string `json:"kind"`
	MIME      string `json:"mime"`
}

var formats = makeFormats()

func makeFormats() map[string]Format {
	result := map[string]Format{}
	add := func(exts, kind, mime string) {
		for _, ext := range strings.Fields(exts) {
			result[ext] = Format{ext, kind, mime}
		}
	}
	add(".jpg .jpeg .jpe", "image", "image/jpeg")
	add(".png", "image", "image/png")
	add(".gif", "image", "image/gif")
	add(".webp", "image", "image/webp")
	add(".bmp", "image", "image/bmp")
	add(".tif .tiff", "image", "image/tiff")
	add(".heic .heif", "image", "image/heic")
	add(".avif", "image", "image/avif")
	add(".jxl", "image", "image/jxl")
	add(".jp2 .j2k .jpf .jpx", "image", "image/jp2")
	add(".psd .psb", "image", "image/vnd.adobe.photoshop")
	add(".arw .dng .cr2 .cr3 .crw .nef .nrw .raf .orf .rw2 .pef .srw .sr2", "raw", "application/octet-stream")
	add(".mp4 .m4v", "video", "video/mp4")
	add(".mov", "video", "video/quicktime")
	add(".mkv", "video", "video/x-matroska")
	add(".webm", "video", "video/webm")
	add(".avi", "video", "video/x-msvideo")
	add(".mpg .mpeg .m2v .vob", "video", "video/mpeg")
	add(".mts .m2ts .ts", "video", "video/mp2t")
	add(".wmv", "video", "video/x-ms-wmv")
	add(".flv", "video", "video/x-flv")
	add(".3gp", "video", "video/3gpp")
	add(".3g2", "video", "video/3gpp2")
	add(".ogv", "video", "video/ogg")
	return result
}
func Lookup(ext string) (Format, bool) { f, ok := formats[strings.ToLower(ext)]; return f, ok }
func Formats() []Format {
	result := make([]Format, 0, len(formats))
	for _, f := range formats {
		result = append(result, f)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Extension < result[j].Extension })
	return result
}

// ValidateUpload checks file/container signatures, not browser-supplied MIME.
// Complete decoding/codec checks remain in the bounded processing job.
func ValidateUpload(r io.ReadSeeker, ext string) (image.Config, error) {
	var dimensions image.Config
	f, ok := Lookup(ext)
	if !ok {
		return dimensions, fmt.Errorf("unsupported media format")
	}
	ext = f.Extension
	if _, e := r.Seek(0, 0); e != nil {
		return dimensions, e
	}
	header := make([]byte, 4096)
	n, e := io.ReadFull(r, header)
	if e != nil && e != io.EOF && e != io.ErrUnexpectedEOF {
		return dimensions, e
	}
	header = header[:n]
	prefix := func(s string) bool { return bytes.HasPrefix(header, []byte(s)) }
	tiff := prefix("II*\x00") || prefix("MM\x00*") || prefix("II+\x00") || prefix("MM\x00+")
	riff := func(s string) bool { return n >= 12 && prefix("RIFF") && string(header[8:12]) == s }
	valid := false
	switch ext {
	case ".jpg", ".jpeg", ".jpe", ".png", ".gif":
		r.Seek(0, 0)
		var format string
		dimensions, format, e = image.DecodeConfig(r)
		expected := strings.TrimPrefix(f.MIME, "image/")
		valid = e == nil && format == expected && dimensions.Width > 0 && dimensions.Height > 0 && int64(dimensions.Width)*int64(dimensions.Height) <= 100000000
	case ".webp":
		valid = riff("WEBP")
	case ".bmp":
		valid = n >= 26 && prefix("BM")
	case ".tif", ".tiff", ".arw", ".dng", ".cr2", ".nef", ".nrw", ".pef", ".srw", ".sr2":
		valid = n >= 16 && tiff
	case ".orf":
		valid = n >= 16 && (prefix("IIRO") || prefix("IIRS") || prefix("MMOR"))
	case ".rw2":
		valid = n >= 16 && prefix("IIU\x00")
	case ".crw":
		valid = n >= 16 && prefix("II\x1a\x00\x00\x00HEAPCCDR")
	case ".raf":
		valid = n >= 16 && prefix("FUJIFILMCCD-RAW")
	case ".psd", ".psb":
		valid = n >= 26 && prefix("8BPS") && (binary.BigEndian.Uint16(header[4:6]) == 1 || binary.BigEndian.Uint16(header[4:6]) == 2)
	case ".jxl":
		valid = prefix("\xff\x0a") || prefix("\x00\x00\x00\x0cJXL \x0d\x0a\x87\x0a")
	case ".jp2", ".jpf", ".jpx", ".j2k":
		valid = prefix("\x00\x00\x00\x0cjP  \x0d\x0a\x87\x0a") || prefix("\xff\x4f\xff\x51")
	case ".avi":
		valid = riff("AVI ")
	case ".mkv", ".webm":
		valid = n >= 16 && prefix("\x1a\x45\xdf\xa3")
	case ".mpg", ".mpeg", ".m2v", ".vob":
		valid = n >= 16 && (prefix("\x00\x00\x01\xba") || prefix("\x00\x00\x01\xb3"))
	case ".mts", ".m2ts", ".ts":
		valid = (n > 376 && header[0] == 0x47 && header[188] == 0x47 && header[376] == 0x47) || (n > 388 && header[4] == 0x47 && header[196] == 0x47 && header[388] == 0x47)
	case ".wmv":
		valid = prefix("\x30\x26\xb2\x75\x8e\x66\xcf\x11\xa6\xd9\x00\xaa\x00\x62\xce\x6c")
	case ".flv":
		valid = n >= 13 && prefix("FLV\x01")
	case ".ogv":
		valid = n >= 32 && prefix("OggS\x00")
	default:
		// ISO base media: restrict image/RAW brands so a renamed MP4 is not a HEIC.
		brands := map[string]bool{}
		if n >= 16 && string(header[4:8]) == "ftyp" {
			size := int(binary.BigEndian.Uint32(header[:4]))
			if size >= 16 && size <= n {
				brands[string(header[8:12])] = true
				for i := 16; i+4 <= size; i += 4 {
					brands[string(header[i:i+4])] = true
				}
			}
		}
		heic := brands["heic"] || brands["heix"] || brands["hevc"] || brands["hevx"] || brands["mif1"] || brands["msf1"]
		avif := brands["avif"] || brands["avis"]
		raw := brands["crx "]
		switch ext {
		case ".heic", ".heif":
			valid = heic && !avif && !raw
		case ".avif":
			valid = avif
		case ".cr3":
			valid = raw
		default:
			valid = len(brands) > 0 && !heic && !avif && !raw
			if ext == ".mov" && n >= 16 {
				valid = valid || string(header[4:8]) == "moov" || string(header[4:8]) == "mdat" || string(header[4:8]) == "wide"
			}
		}
	}
	if !valid {
		return dimensions, fmt.Errorf("file contents do not match %s, or the image header is invalid", ext)
	}
	return dimensions, nil
}
