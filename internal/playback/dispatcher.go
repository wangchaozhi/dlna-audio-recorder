package playback

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/wangchaozhi/dlna-audio-recorder/internal/metadata"
	"github.com/wangchaozhi/dlna-audio-recorder/internal/recorder"
)

type Kind int

const (
	SetTrack Kind = iota + 1
	SetNext
	Stop
)

type Command struct {
	Kind   Kind
	Track  metadata.Track
	Source string
}

type Handler interface {
	Handle(Command) error
}

type Event struct {
	At     time.Time      `json:"at"`
	Kind   Kind           `json:"kind"`
	Source string         `json:"source"`
	Track  metadata.Track `json:"track"`
	Error  string         `json:"error,omitempty"`
}

type Observer interface {
	OnPlaybackEvent(Event)
}

type Dispatcher struct {
	recorder  *recorder.Manager
	log       *slog.Logger
	mu        sync.RWMutex
	observers []Observer
}

func NewDispatcher(r *recorder.Manager, log *slog.Logger) *Dispatcher {
	return &Dispatcher{recorder: r, log: log}
}

func (d *Dispatcher) AddObserver(o Observer) {
	if o == nil {
		return
	}
	d.mu.Lock()
	d.observers = append(d.observers, o)
	d.mu.Unlock()
}

func (d *Dispatcher) Handle(c Command) error {
	var err error
	switch c.Kind {
	case SetTrack:
		d.log.Info("playback command", "kind", "set_track", "source", c.Source, "title", c.Track.Title)
		err = d.recorder.SetTrack(c.Track)
	case SetNext:
		d.log.Info("playback command", "kind", "set_next", "source", c.Source, "title", c.Track.Title)
		d.recorder.SetNext(c.Track)
	case Stop:
		d.log.Info("playback command", "kind", "stop", "source", c.Source)
		d.recorder.Stop()
	default:
		err = errors.New("unknown playback command")
	}
	e := Event{At: time.Now(), Kind: c.Kind, Source: c.Source, Track: c.Track}
	if err != nil {
		e.Error = err.Error()
	}
	d.emit(e)
	return err
}

func (d *Dispatcher) emit(e Event) {
	d.mu.RLock()
	observers := append([]Observer(nil), d.observers...)
	d.mu.RUnlock()
	for _, o := range observers {
		o.OnPlaybackEvent(e)
	}
}
