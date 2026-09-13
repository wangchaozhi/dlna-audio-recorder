package recorder

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wangchaozhi/dlna-audio-recorder/internal/metadata"
	"github.com/wangchaozhi/dlna-audio-recorder/internal/sanitize"
)

type FinalizeRequest struct {
	CapturePath string
	Track       metadata.Track
}

type Finalizer interface {
	Finalize(FinalizeRequest) (string, error)
}

type FFmpegFinalizer struct {
	Executable string
	OutputDir  string
	KeepRaw    bool
	Log        *slog.Logger
}

func (f FFmpegFinalizer) Finalize(req FinalizeRequest) (string, error) {
	title := sanitize.FileName(req.Track.Title)
	if title == "unknown" {
		title = "track"
	}
	base := title
	if req.Track.Artist != "" {
		base = sanitize.FileName(req.Track.Artist) + " - " + title
	}
	out := uniquePath(filepath.Join(f.OutputDir, base+".m4a"))
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", req.CapturePath, "-map", "0:a:0", "-vn", "-c:a", "aac", "-b:a", "256k"}
	if req.Track.Title != "" {
		args = append(args, "-metadata", "title="+req.Track.Title)
	}
	if req.Track.Artist != "" {
		args = append(args, "-metadata", "artist="+req.Track.Artist)
	}
	if req.Track.Album != "" {
		args = append(args, "-metadata", "album="+req.Track.Album)
	}
	args = append(args, out)
	if b, err := exec.Command(f.Executable, args...).CombinedOutput(); err != nil {
		fallback := strings.TrimSuffix(out, ".m4a") + ".capture"
		_ = os.Rename(req.CapturePath, fallback)
		return fallback, fmt.Errorf("ffmpeg finalize failed: %w: %s", err, strings.TrimSpace(string(b)))
	}
	if !f.KeepRaw {
		_ = os.Remove(req.CapturePath)
	}
	return out, nil
}
