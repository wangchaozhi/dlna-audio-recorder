package recorder

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type StreamSource interface {
	Open(ctx context.Context, uri string) (io.ReadCloser, StreamInfo, error)
}

type StreamInfo struct {
	ContentType   string
	ContentLength int64
}

type HTTPStreamSource struct {
	UserAgent string
	Timeout   time.Duration
}

func (s HTTPStreamSource) Open(ctx context.Context, uri string) (io.ReadCloser, StreamInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, StreamInfo{}, err
	}
	req.Header.Set("User-Agent", s.UserAgent)
	req.Header.Set("Icy-MetaData", "1")
	client := &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: s.Timeout}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, StreamInfo{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, StreamInfo{}, fmt.Errorf("upstream status %s", resp.Status)
	}
	return resp.Body, StreamInfo{ContentType: resp.Header.Get("Content-Type"), ContentLength: resp.ContentLength}, nil
}
