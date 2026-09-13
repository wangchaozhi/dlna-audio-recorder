package dlna

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wangchaozhi/dlna-audio-recorder/internal/metadata"
	"github.com/wangchaozhi/dlna-audio-recorder/internal/playback"
)

type qplayAction struct {
	XMLName        xml.Name
	QueueID        string `xml:"QueueID"`
	StartingIndex  int    `xml:"StartingIndex"`
	NextIndex      int    `xml:"NextIndex"`
	NumberOfTracks int    `xml:"NumberOfTracks"`
	TracksMetaData string `xml:"TracksMetaData"`
}

func (s *Server) qplayControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		soapFault(w, 402, "Invalid Args")
		return
	}
	var env envelope
	if xml.Unmarshal(b, &env) != nil {
		soapFault(w, 402, "Invalid Args")
		return
	}
	var a qplayAction
	if xml.Unmarshal(env.Body.Inner, &a) != nil {
		soapFault(w, 401, "Invalid Action")
		return
	}
	action := a.XMLName.Local
	s.Log.Info("QPlay action", "action", action, "queue_id", redactID(a.QueueID), "metadata_bytes", len(a.TracksMetaData))
	switch action {
	case "InsertTracks", "SetTracksInfo":
		s.mu.Lock()
		expected := s.qplayQueueID
		s.mu.Unlock()
		if expected != "" && a.QueueID != "" && a.QueueID != expected {
			soapFault(w, 718, "Invalid QueueID")
			return
		}
		tracks := parseQPlayTracks(a.TracksMetaData)
		s.mu.Lock()
		if action == "SetTracksInfo" && a.StartingIndex < 0 {
			s.qplayTracks = nil
		}
		s.qplayTracks = mergeTracks(s.qplayTracks, tracks, a.StartingIndex)
		all := append([]metadata.Track(nil), s.qplayTracks...)
		s.mu.Unlock()
		if len(all) > 0 {
			idx := 0
			if a.NextIndex > 0 && a.NextIndex <= len(all) {
				idx = a.NextIndex - 1
			}
			_ = s.Playback.Handle(playback.Command{Kind: playback.SetTrack, Track: all[idx], Source: "qplay"})
			if idx+1 < len(all) {
				_ = s.Playback.Handle(playback.Command{Kind: playback.SetNext, Track: all[idx+1], Source: "qplay"})
			}
		}
		soapOKService(w, action, "urn:schemas-tencent-com:service:QPlay:1", fmt.Sprintf("<NumberOfSuccess>%d</NumberOfSuccess>", len(tracks)))
	case "RemoveTracks":
		s.mu.Lock()
		s.qplayTracks = removeTracks(s.qplayTracks, a.StartingIndex, a.NumberOfTracks)
		s.mu.Unlock()
		soapOKService(w, action, "urn:schemas-tencent-com:service:QPlay:1", "<NumberOfSuccess>0</NumberOfSuccess>")
	case "GetTracksCount":
		s.mu.Lock()
		n := len(s.qplayTracks)
		s.mu.Unlock()
		soapOKService(w, action, "urn:schemas-tencent-com:service:QPlay:1", fmt.Sprintf("<NrTracks>%d</NrTracks>", n))
	case "GetMaxTracks":
		soapOKService(w, action, "urn:schemas-tencent-com:service:QPlay:1", "<MaxTracks>1000</MaxTracks>")
	case "GetTracksInfo":
		s.mu.Lock()
		raw := qplayJSON(s.qplayTracks)
		s.mu.Unlock()
		soapOKService(w, action, "urn:schemas-tencent-com:service:QPlay:1", "<TracksMetaData>"+xmlEscape(raw)+"</TracksMetaData>")
	case "QPlayAuth":
		soapOKService(w, action, "urn:schemas-tencent-com:service:QPlay:1", "<Code>0</Code><MID>dlna-audio-recorder</MID><DID>dlna-audio-recorder</DID>")
	default:
		s.Log.Warn("unsupported QPlay action", "action", action)
		soapFault(w, 401, "Invalid Action")
	}
}

func parseQPlayTracks(raw string) []metadata.Track {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return nil
	}
	var out []metadata.Track
	var walk func(any)
	walk = func(x any) {
		switch z := x.(type) {
		case []any:
			for _, e := range z {
				walk(e)
			}
		case map[string]any:
			uri := firstString(z, "trackURI", "trackUri", "uri", "url", "playUrl")
			if uri == "" {
				if a, ok := z["trackURIs"].([]any); ok {
					for _, e := range a {
						if q, ok := e.(string); ok && strings.HasPrefix(q, "http") {
							uri = q
							break
						}
					}
				}
			}
			if uri != "" {
				t := metadata.Track{URI: uri, Title: firstString(z, "title", "songName", "name"), Artist: firstString(z, "artist", "singer", "artistName"), Album: firstString(z, "album", "albumName")}
				if d := firstString(z, "duration", "durationMS", "durationMs"); d != "" {
					if n, err := strconv.ParseInt(d, 10, 64); err == nil {
						if n > 10000 {
							t.Duration = time.Duration(n) * time.Millisecond
						} else {
							t.Duration = time.Duration(n) * time.Second
						}
					}
				}
				out = append(out, t)
				return
			}
			for _, e := range z {
				walk(e)
			}
		}
	}
	walk(v)
	return out
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch x := v.(type) {
			case string:
				return x
			case float64:
				return strconv.FormatInt(int64(x), 10)
			}
		}
	}
	return ""
}

func mergeTracks(cur, add []metadata.Track, start int) []metadata.Track {
	if len(add) == 0 {
		return cur
	}
	if start <= 0 || start > len(cur)+1 {
		return append(cur, add...)
	}
	i := start - 1
	r := append([]metadata.Track{}, cur[:i]...)
	r = append(r, add...)
	if i < len(cur) {
		r = append(r, cur[i:]...)
	}
	return r
}

func removeTracks(cur []metadata.Track, start, n int) []metadata.Track {
	if start < 1 || start > len(cur) {
		return cur
	}
	i := start - 1
	j := len(cur)
	if n >= 0 && i+n < j {
		j = i + n
	}
	return append(append([]metadata.Track{}, cur[:i]...), cur[j:]...)
}

func redactID(v string) string {
	if len(v) <= 6 {
		return "***"
	}
	return v[:3] + "***" + v[len(v)-3:]
}

func qplayJSON(ts []metadata.Track) string {
	b, _ := json.Marshal(ts)
	return string(b)
}
