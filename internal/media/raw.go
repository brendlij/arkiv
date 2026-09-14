package media

import (
	"context"
	"errors"
	"fmt"
	"gallery/internal/metadata"
	"os"
)

type RAWExtractor interface {
	Extract(context.Context, string, string) error
}
type EmbeddedRAW struct{ Runner metadata.Runner }

func (r EmbeddedRAW) Extract(ctx context.Context, source, dest string) error {
	var failures []error
	// Some DNG cameras embed a TIFF thumbnail rather than a JPEG preview.
	// Fall back to that representation without developing or changing the RAW.
	for _, tag := range []string{"JpgFromRaw", "PreviewImage", "OtherImage", "ThumbnailTIFF", "ThumbnailImage"} {
		b, e := r.Runner.Run(ctx, "exiftool", "-b", "-"+tag, source)
		jpeg := len(b) > 3 && b[0] == 0xff && b[1] == 0xd8
		tiff := len(b) > 4 && (string(b[:4]) == "II\x2a\x00" || string(b[:4]) == "MM\x00\x2a")
		if e == nil && (jpeg || tiff) {
			return os.WriteFile(dest, b, 0600)
		}
		if e != nil {
			failures = append(failures, e)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return errors.Join(append([]error{fmt.Errorf("RAW has no supported embedded preview")}, failures...)...)
}
