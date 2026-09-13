package recorder

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wangchaozhi/dlna-audio-recorder/internal/metadata"
	"github.com/wangchaozhi/dlna-audio-recorder/internal/sanitize"
)

type Config struct {
	OutputDir, TempDir, FFMpeg, UserAgent string
	Grace, HTTPTimeout                    time.Duration
	KeepRaw                               bool
}

type Dependencies struct {
	Source    StreamSource
	Finalizer Finalizer
	Policy    BoundaryPolicy
}

type Manager struct {
	cfg       Config
	log       *slog.Logger
	source    StreamSource
	finalizer Finalizer
	policy    BoundaryPolicy
	mu        sync.Mutex
	session   *session
}

type session struct {
	ctx    context.Context
	cancel context.CancelFunc
	url    string
	events chan command
	done   chan struct{}
}

type command struct {
	kind  string
	track metadata.Track
}

type segment struct {
	track  metadata.Track
	path   string
	file   *os.File
	opened time.Time
}

type chunk struct {
	at   time.Time
	data []byte
}

type tailBuffer struct {
	grace  time.Duration
	chunks []chunk
}

func New(cfg Config, log *slog.Logger) *Manager {
	return NewWithDependencies(cfg, log, Dependencies{})
}

func NewWithDependencies(cfg Config, log *slog.Logger, deps Dependencies) *Manager {
	if deps.Source == nil {
		deps.Source = HTTPStreamSource{UserAgent: cfg.UserAgent, Timeout: cfg.HTTPTimeout}
	}
	if deps.Finalizer == nil {
		deps.Finalizer = FFmpegFinalizer{Executable: cfg.FFMpeg, OutputDir: cfg.OutputDir, KeepRaw: cfg.KeepRaw, Log: log}
	}
	if deps.Policy == nil {
		deps.Policy = ControlPlaneBoundaryPolicy{}
	}
	return &Manager{cfg: cfg, log: log, source: deps.Source, finalizer: deps.Finalizer, policy: deps.Policy}
}

func (m *Manager) SetTrack(t metadata.Track) error {
	if strings.TrimSpace(t.URI) == "" {
		return errors.New("empty media URI")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.session == nil {
		m.startLocked(t)
		return nil
	}
	if t.URI != m.session.url {
		m.session.cancel()
		m.startLocked(t)
		return nil
	}
	select {
	case m.session.events <- command{kind: "track", track: t}:
		return nil
	default:
		return errors.New("recorder command queue full")
	}
}

func (m *Manager) startLocked(t metadata.Track) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &session{ctx: ctx, cancel: cancel, url: t.URI, events: make(chan command, 16), done: make(chan struct{})}
	m.session = s
	go m.run(s, t)
}

func (m *Manager) SetNext(t metadata.Track) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.session == nil {
		return
	}
	select {
	case m.session.events <- command{kind: "next", track: t}:
	default:
		m.log.Warn("dropping next-track hint")
	}
}

func (m *Manager) Stop() {
	m.mu.Lock()
	s := m.session
	m.session = nil
	m.mu.Unlock()
	if s != nil {
		s.cancel()
		<-s.done
	}
}

func (m *Manager) run(s *session, initial metadata.Track) {
	defer close(s.done)
	body, info, err := m.source.Open(s.ctx, s.url)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			m.log.Error("open upstream", "url", s.url, "err", err)
		}
		return
	}
	defer body.Close()
	m.log.Info("recording stream", "url", s.url, "content_type", info.ContentType, "content_length", info.ContentLength)
	seg, err := m.openSegment(initial)
	if err != nil {
		m.log.Error("open segment", "err", err)
		return
	}
	var pending *metadata.Track
	tail := tailBuffer{grace: m.cfg.Grace}
	reader := bufio.NewReaderSize(body, 128*1024)
	buf := make([]byte, 64*1024)
	for {
		m.drainCommands(s, &seg, &pending, &tail)
		n, rerr := reader.Read(buf)
		if n > 0 {
			now := time.Now()
			data := append([]byte(nil), buf[:n]...)
			if _, err := seg.file.Write(data); err != nil {
				m.log.Error("write segment", "err", err)
				break
			}
			tail.add(now, data)
		}
		m.drainCommands(s, &seg, &pending, &tail)
		if rerr != nil {
			if !errors.Is(rerr, io.EOF) && !errors.Is(rerr, context.Canceled) {
				m.log.Warn("upstream ended", "err", rerr)
			}
			break
		}
		select {
		case <-s.ctx.Done():
			m.closeAndFinalize(seg)
			return
		default:
		}
	}
	m.closeAndFinalize(seg)
}

func (m *Manager) drainCommands(s *session, seg **segment, pending **metadata.Track, tail *tailBuffer) {
	for {
		select {
		case cmd := <-s.events:
			switch cmd.kind {
			case "next":
				cp := cmd.track
				*pending = &cp
			case "track":
				incoming := cmd.track
				if *pending != nil && (incoming.Title == "" || incoming.Identity() == (*seg).track.Identity()) {
					incoming = **pending
				}
				if m.policy.Decide((*seg).track, incoming, s.url) == BoundaryRotate {
					*seg = m.rotate(*seg, incoming, tail)
					*pending = nil
				}
			}
		default:
			return
		}
	}
}

func (m *Manager) rotate(old *segment, nt metadata.Track, tail *tailBuffer) *segment {
	ns, err := m.openSegment(nt)
	if err != nil {
		m.log.Error("rotate: open new segment", "err", err)
		return old
	}
	for _, c := range tail.snapshot() {
		_, _ = ns.file.Write(c.data)
	}
	_ = old.file.Sync()
	_ = old.file.Close()
	go m.finalize(old)
	m.log.Info("track boundary", "from", old.track.Title, "to", nt.Title)
	return ns
}

func (m *Manager) openSegment(t metadata.Track) (*segment, error) {
	stamp := time.Now().Format("20060102-150405.000")
	base := sanitize.FileName(t.Title)
	if t.Artist != "" {
		base = sanitize.FileName(t.Artist) + " - " + base
	}
	path := filepath.Join(m.cfg.TempDir, stamp+" - "+base+".capture")
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &segment{track: t, path: path, file: f, opened: time.Now()}, nil
}

func (m *Manager) closeAndFinalize(s *segment) {
	if s == nil {
		return
	}
	_ = s.file.Sync()
	_ = s.file.Close()
	m.finalize(s)
}

func (m *Manager) finalize(s *segment) {
	out, err := m.finalizer.Finalize(FinalizeRequest{CapturePath: s.path, Track: s.track})
	if err != nil {
		m.log.Error("finalize track", "err", err, "file", out)
		return
	}
	m.log.Info("saved track", "file", out, "title", s.track.Title)
}

func (t *tailBuffer) add(at time.Time, data []byte) {
	t.chunks = append(t.chunks, chunk{at: at, data: data})
	cutoff := at.Add(-t.grace)
	i := 0
	for i < len(t.chunks) && t.chunks[i].at.Before(cutoff) {
		i++
	}
	if i > 0 {
		t.chunks = append([]chunk(nil), t.chunks[i:]...)
	}
}

func (t *tailBuffer) snapshot() []chunk { return append([]chunk(nil), t.chunks...) }

func uniquePath(p string) string {
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return p
	}
	ext := filepath.Ext(p)
	stem := strings.TrimSuffix(p, ext)
	for i := 2; ; i++ {
		q := stem + " (" + strconv.Itoa(i) + ")" + ext
		if _, err := os.Stat(q); os.IsNotExist(err) {
			return q
		}
	}
}

func (m *Manager) Status() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.session == nil {
		return "STOPPED"
	}
	return fmt.Sprintf("RECORDING %s", m.session.url)
}
