package dashboard

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wangchaozhi/dlna-audio-recorder/internal/metadata"
	"github.com/wangchaozhi/dlna-audio-recorder/internal/playback"
)

//go:embed static/index.html
var staticFS embed.FS

type StatusProvider interface {
	Status() string
}

type Config struct {
	DeviceName string
	BaseURL    string
	OutputDir  string
	StartedAt  time.Time
	Recorder   StatusProvider
}

type Server struct {
	cfg     Config
	mu      sync.RWMutex
	current metadata.Track
	next    metadata.Track
	source  string
	state   string
	events  []eventView
	subs    map[chan eventView]struct{}
	static  http.Handler
}

type eventView struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Source string    `json:"source"`
	Title  string    `json:"title,omitempty"`
	Artist string    `json:"artist,omitempty"`
	Error  string    `json:"error,omitempty"`
}

type trackView struct {
	Title    string `json:"title,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	Duration string `json:"duration,omitempty"`
	Origin   string `json:"origin,omitempty"`
}

type recordingView struct {
	Name string    `json:"name"`
	Size int64     `json:"size"`
	Time time.Time `json:"time"`
}

type statusResponse struct {
	DeviceName string          `json:"deviceName"`
	BaseURL    string          `json:"baseURL"`
	Uptime     string          `json:"uptime"`
	State      string          `json:"state"`
	Source     string          `json:"source,omitempty"`
	Current    trackView       `json:"current"`
	Next       trackView       `json:"next"`
	Events     []eventView     `json:"events"`
	Recordings []recordingView `json:"recordings"`
}

func New(cfg Config) *Server {
	sub, _ := fs.Sub(staticFS, "static")
	return &Server{cfg: cfg, state: "IDLE", subs: make(map[chan eventView]struct{}), static: http.FileServer(http.FS(sub))}
}

func (s *Server) OnPlaybackEvent(e playback.Event) {
	v := eventView{At: e.At, Kind: kindName(e.Kind), Source: e.Source, Title: e.Track.Title, Artist: e.Track.Artist, Error: e.Error}
	s.mu.Lock()
	s.source = e.Source
	switch e.Kind {
	case playback.SetTrack:
		if e.Error == "" {
			s.current = e.Track
			s.state = "RECORDING"
		}
	case playback.SetNext:
		s.next = e.Track
	case playback.Stop:
		s.state = "IDLE"
		s.current = metadata.Track{}
		s.next = metadata.Track{}
	}
	s.events = append(s.events, v)
	if len(s.events) > 100 {
		s.events = append([]eventView(nil), s.events[len(s.events)-100:]...)
	}
	for ch := range s.subs {
		select {
		case ch <- v:
		default:
		}
	}
	s.mu.Unlock()
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/status":
		s.handleStatus(w, r)
	case r.URL.Path == "/api/events":
		s.handleEvents(w, r)
	case r.URL.Path == "/api/recordings":
		s.handleRecordings(w, r)
	case strings.HasPrefix(r.URL.Path, "/recordings/"):
		s.handleRecordingFile(w, r)
	case r.URL.Path == "/" || r.URL.Path == "/index.html":
		s.static.ServeHTTP(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	cur, next, source, state := s.current, s.next, s.source, s.state
	events := append([]eventView(nil), s.events...)
	s.mu.RUnlock()
	if s.cfg.Recorder != nil && strings.HasPrefix(s.cfg.Recorder.Status(), "RECORDING") {
		state = "RECORDING"
	}
	resp := statusResponse{DeviceName: s.cfg.DeviceName, BaseURL: s.cfg.BaseURL, Uptime: time.Since(s.cfg.StartedAt).Round(time.Second).String(), State: state, Source: source, Current: viewTrack(cur), Next: viewTrack(next), Events: events, Recordings: s.recordings()}
	writeJSON(w, resp)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := make(chan eventView, 16)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.subs, ch); s.mu.Unlock(); close(ch) }()
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		case <-time.After(20 * time.Second):
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) handleRecordings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.recordings())
}

func (s *Server) handleRecordingFile(w http.ResponseWriter, r *http.Request) {
	name, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/recordings/"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	name = filepath.Base(name)
	if name == "." || name == "" || strings.Contains(name, string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.cfg.OutputDir, name)
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, name, st.ModTime(), f)
}

func (s *Server) recordings() []recordingView {
	entries, err := os.ReadDir(s.cfg.OutputDir)
	if err != nil {
		return nil
	}
	out := make([]recordingView, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".m4a") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, recordingView{Name: e.Name(), Size: info.Size(), Time: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	if len(out) > 50 {
		out = out[:50]
	}
	return out
}

func viewTrack(t metadata.Track) trackView {
	return trackView{Title: t.Title, Artist: t.Artist, Album: t.Album, Duration: formatDuration(t.Duration), Origin: redactOrigin(t.URI)}
}
func redactOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return ""
	}
	if u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	return u.Scheme + "://"
}
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	m := int(d.Minutes())
	return fmt.Sprintf("%d:%02d", m, int(d.Seconds())%60)
}
func kindName(k playback.Kind) string {
	switch k {
	case playback.SetTrack:
		return "track"
	case playback.SetNext:
		return "next"
	case playback.Stop:
		return "stop"
	default:
		return "unknown"
	}
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
