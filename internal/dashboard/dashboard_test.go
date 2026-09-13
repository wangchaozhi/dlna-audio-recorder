package dashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wangchaozhi/dlna-audio-recorder/internal/metadata"
	"github.com/wangchaozhi/dlna-audio-recorder/internal/playback"
)

type statusStub string

func (s statusStub) Status() string { return string(s) }

func TestStatusRedactsMediaURLAndListsRecordings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Song.m4a"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(Config{DeviceName: "Test", BaseURL: "http://127.0.0.1:1400", OutputDir: dir, StartedAt: time.Now().Add(-time.Minute), Recorder: statusStub("RECORDING https://secret")})
	s.OnPlaybackEvent(playback.Event{At: time.Now(), Kind: playback.SetTrack, Source: "qplay", Track: metadata.Track{Title: "Song", Artist: "Artist", URI: "https://media.example.com/path/file.m4a?token=secret"}})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	body, _ := io.ReadAll(rr.Body)
	if string(body) == "" {
		t.Fatal("empty response")
	}
	var got statusResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Current.Origin != "https://media.example.com" {
		t.Fatalf("origin=%q", got.Current.Origin)
	}
	if len(got.Recordings) != 1 || got.Recordings[0].Name != "Song.m4a" {
		t.Fatalf("recordings=%v", got.Recordings)
	}
}
