package dlna

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeviceDescription(t *testing.T) {
	h := &captureHandler{}
	s := &Server{Name: "Recorder & Test", UUID: "abc", Playback: h, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/device.xml", nil)
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "Recorder &amp; Test") {
		t.Fatalf("body=%s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "urn:schemas-tencent-com:service:QPlay:1") {
		t.Fatalf("QPlay service not advertised")
	}
}
