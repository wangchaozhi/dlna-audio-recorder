package recorder

import "github.com/wangchaozhi/dlna-audio-recorder/internal/metadata"

type BoundaryDecision int

const (
	BoundaryKeep BoundaryDecision = iota
	BoundaryRotate
	BoundaryRestart
)

type BoundaryPolicy interface {
	Decide(current metadata.Track, incoming metadata.Track, currentURL string) BoundaryDecision
}

type ControlPlaneBoundaryPolicy struct{}

func (ControlPlaneBoundaryPolicy) Decide(current metadata.Track, incoming metadata.Track, currentURL string) BoundaryDecision {
	if incoming.URI != "" && incoming.URI != currentURL {
		return BoundaryRestart
	}
	if incoming.Identity() != "" && incoming.Identity() != current.Identity() {
		return BoundaryRotate
	}
	return BoundaryKeep
}
