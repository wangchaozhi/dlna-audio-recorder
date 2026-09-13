package recorder

import (
	"testing"

	"github.com/wangchaozhi/dlna-audio-recorder/internal/metadata"
)

func TestControlPlaneBoundaryPolicy(t *testing.T) {
	p := ControlPlaneBoundaryPolicy{}
	current := metadata.Track{URI: "http://media/stream", Title: "A", Artist: "Artist"}

	if got := p.Decide(current, current, current.URI); got != BoundaryKeep {
		t.Fatalf("same track: got %v", got)
	}
	changed := current
	changed.Title = "B"
	if got := p.Decide(current, changed, current.URI); got != BoundaryRotate {
		t.Fatalf("metadata boundary: got %v", got)
	}
	changed.URI = "http://media/other"
	if got := p.Decide(current, changed, current.URI); got != BoundaryRestart {
		t.Fatalf("URI change: got %v", got)
	}
}
