package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/wangchaozhi/dlna-audio-recorder/internal/config"
	"github.com/wangchaozhi/dlna-audio-recorder/internal/dashboard"
	"github.com/wangchaozhi/dlna-audio-recorder/internal/dlna"
	"github.com/wangchaozhi/dlna-audio-recorder/internal/playback"
	"github.com/wangchaozhi/dlna-audio-recorder/internal/recorder"
)

func main() {
	cfg, err := config.Parse()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	port := strings.TrimPrefix(cfg.ListenAddr, ":")
	if strings.Contains(port, ":") {
		parts := strings.Split(port, ":")
		port = parts[len(parts)-1]
	}
	baseURL := "http://" + cfg.AdvertiseIP + ":" + port
	sum := sha1.Sum([]byte("dlna-audio-recorder:" + cfg.AdvertiseIP))
	h := hex.EncodeToString(sum[:])
	uuid := fmt.Sprintf("%s-%s-%s-%s-%s", h[:8], h[8:12], h[12:16], h[16:20], h[20:32])
	rec := recorder.New(recorder.Config{OutputDir: cfg.OutputDir, TempDir: cfg.TempDir, FFMpeg: cfg.FFMpeg, UserAgent: cfg.UserAgent, Grace: cfg.SegmentGrace, HTTPTimeout: cfg.HTTPTimeout, KeepRaw: cfg.KeepRaw}, log)
	dispatcher := playback.NewDispatcher(rec, log)
	dash := dashboard.New(dashboard.Config{DeviceName: cfg.DeviceName, BaseURL: baseURL, OutputDir: cfg.OutputDir, StartedAt: time.Now(), Recorder: rec})
	dispatcher.AddObserver(dash)
	ds := &dlna.Server{Name: cfg.DeviceName, UUID: uuid, BaseURL: baseURL, Playback: dispatcher, Log: log}
	dlnaHandler := ds.Handler()
	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" || strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/recordings/") {
			dash.ServeHTTP(w, r)
			return
		}
		dlnaHandler.ServeHTTP(w, r)
	})
	httpSrv := &http.Server{Addr: cfg.ListenAddr, Handler: root, ReadHeaderTimeout: 10 * time.Second}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go func() {
		log.Info("HTTP server", "listen", cfg.ListenAddr, "device", baseURL+"/device.xml")
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server", "err", err)
			cancel()
		}
	}()
	ssdp := &dlna.SSDP{UUID: uuid, Location: baseURL + "/device.xml", Interval: cfg.SSDPInterval, Log: log}
	go func() {
		if err := ssdp.Run(ctx); err != nil {
			log.Error("SSDP", "err", err)
			cancel()
		}
	}()
	log.Info("DLNA renderer ready", "name", cfg.DeviceName, "advertise_ip", cfg.AdvertiseIP, "output", cfg.OutputDir)
	<-ctx.Done()
	rec.Stop()
	shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	_ = httpSrv.Shutdown(shutdownCtx)
}
