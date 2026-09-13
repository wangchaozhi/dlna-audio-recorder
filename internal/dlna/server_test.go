package dlna

import("io";"log/slog";"net/http";"net/http/httptest";"strings";"testing";"github.com/wangchaozhi/dlna-audio-recorder/internal/recorder")
func TestDeviceDescription(t *testing.T){s:=&Server{Name:"Recorder & Test",UUID:"abc",Recorder:recorder.New(recorder.Config{},slog.New(slog.NewTextHandler(io.Discard,nil))),Log:slog.New(slog.NewTextHandler(io.Discard,nil))};rr:=httptest.NewRecorder();req:=httptest.NewRequest(http.MethodGet,"/device.xml",nil);s.Handler().ServeHTTP(rr,req);if rr.Code!=200||!strings.Contains(rr.Body.String(),"Recorder &amp; Test"){t.Fatalf("body=%s",rr.Body.String())}}
