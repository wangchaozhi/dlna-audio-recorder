package dlna

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangchaozhi/dlna-audio-recorder/internal/playback"
)

type captureHandler struct{ commands []playback.Command }

func (h *captureHandler) Handle(c playback.Command) error {
	h.commands = append(h.commands, c)
	return nil
}

func TestQPlayQueueDispatchesTrack(t *testing.T) {
	h := &captureHandler{}
	s := &Server{Name: "Recorder", UUID: "abc", Playback: h, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	setURI := `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:SetAVTransportURI xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"><InstanceID>0</InstanceID><CurrentURI>qplay://queue-123</CurrentURI><CurrentURIMetaData></CurrentURIMetaData></u:SetAVTransportURI></s:Body></s:Envelope>`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/upnp/control/avtransport", strings.NewReader(setURI))
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("set uri code=%d body=%s", rr.Code, rr.Body.String())
	}
	meta := `[{"title":"Song A","artist":"Singer","duration":180000,"trackURIs":["https://example.test/a.mp3"]}]`
	body := `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:SetTracksInfo xmlns:u="urn:schemas-tencent-com:service:QPlay:1"><QueueID>queue-123</QueueID><StartingIndex>-1</StartingIndex><NextIndex>1</NextIndex><TracksMetaData>` + meta + `</TracksMetaData></u:SetTracksInfo></s:Body></s:Envelope>`
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/upnp/control/qplay", strings.NewReader(body))
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("qplay code=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(h.commands) != 1 || h.commands[0].Kind != playback.SetTrack || h.commands[0].Track.Title != "Song A" {
		t.Fatalf("commands=%+v", h.commands)
	}
}
