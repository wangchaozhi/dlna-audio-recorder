package playback

import (
	"errors"
	"log/slog"

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

type Dispatcher struct {
	recorder *recorder.Manager
	log      *slog.Logger
}

func NewDispatcher(r *recorder.Manager, log *slog.Logger) *Dispatcher {
	return &Dispatcher{recorder: r, log: log}
}

func (d *Dispatcher) Handle(c Command) error {
	switch c.Kind {
	case SetTrack:
		d.log.Info("playback command", "kind", "set_track", "source", c.Source, "title", c.Track.Title)
		return d.recorder.SetTrack(c.Track)
	case SetNext:
		d.log.Info("playback command", "kind", "set_next", "source", c.Source, "title", c.Track.Title)
		d.recorder.SetNext(c.Track)
		return nil
	case Stop:
		d.log.Info("playback command", "kind", "stop", "source", c.Source)
		d.recorder.Stop()
		return nil
	default:
		return errors.New("unknown playback command")
	}
}
