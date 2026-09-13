# DLNA Audio Recorder

A small DLNA/UPnP **MediaRenderer** written in Go. A phone or media app can cast audio to it; the recorder receives the `AVTransport` URI/metadata, opens the media stream itself, and saves each track as a separate `.m4a` file using FFmpeg.

> Use this only with audio you are authorized to receive and record. DRM/encrypted sources are intentionally not bypassed.

## Why this project exists

Some DLNA senders keep one HTTP connection alive across multiple tracks. Splitting files when the socket closes therefore loses track boundaries. This project uses **DLNA control-plane boundaries** instead:

- `SetAVTransportURI` identifies/updates the current track.
- `SetNextAVTransportURI` is retained as a next-track hint.
- DIDL-Lite metadata (`dc:title`, `upnp:artist`, `upnp:album`, `res@duration`) names and describes each recording.
- A single long-lived stream session is retained when the URI does not change; metadata changes rotate the output segment without reconnecting upstream.
- A short tail overlap is copied into the next raw segment so delayed control messages are less likely to cut the first samples. FFmpeg re-demuxes/re-encodes final files, which also repairs partial compressed-frame boundaries.

No program can guarantee a perfect split when the sender exposes one unchanging stream **and sends no track metadata/boundary signal at all**. In that case the transport layer simply does not contain enough information. The raw capture is preserved automatically if FFmpeg cannot finalize it.

## Requirements

- Go 1.23+
- FFmpeg 6/7+ available in `PATH`
- Sender and recorder on the same LAN; multicast UDP 1900 and TCP 1400 must be reachable

## Quick start

```bash
go build -o dlna-recorder ./cmd/dlna-recorder
./dlna-recorder -output ./recordings
```

The recorder advertises itself as **DLNA Audio Recorder**. Open your DLNA-capable music app, select that renderer, and play audio. Finished tracks appear under `recordings/`.

If auto-detection chooses the wrong network interface:

```bash
./dlna-recorder -advertise-ip 192.168.1.20 -listen :1400 -output ./recordings
```

## Useful flags

```text
-listen :1400
-advertise-ip 192.168.1.20
-name "DLNA Audio Recorder"
-output recordings
-temp recordings/.tmp
-ffmpeg ffmpeg
-segment-grace 1.5s
-keep-raw
```

## Docker

DLNA discovery uses multicast, so host networking is recommended on Linux:

```bash
docker build -t dlna-audio-recorder .
docker run --rm --network host \
  -v "$PWD/recordings:/recordings" \
  dlna-audio-recorder -advertise-ip 192.168.1.20
```

## Track-boundary behavior

1. A new URI starts a new `StreamSession`.
2. A metadata change for the **same URI** rotates the segment but keeps the upstream session alive.
3. A different URI cancels the previous fetch, finalizes it, and starts the next stream.
4. `SetNextAVTransportURI` is stored as a hint, not treated as an immediate boundary.
5. `Stop` closes and finalizes the current segment.

This is deliberately control-plane driven rather than silence detection; live mixes and gapless albums often have no silence at boundaries.

## Supported input

The recorder declares common HTTP audio protocols (MP3, AAC, MP4/M4A, FLAC and HLS). Actual decoding support is determined by your FFmpeg build. Plain HTTP(S) media URLs work best.

DRM, encrypted application-private transports, and sources that require a separate proprietary playback stack are not bypassed.

## Development

```bash
make test
make vet
make build
```

CI runs tests with the race detector, `go vet`, and a clean build.

## Current limitations / roadmap

- Implements the renderer/control subset required for recording; it is not intended to be a full speaker/audio-output renderer.
- Event subscription returns valid subscription responses but does not yet push `LastChange` callbacks to subscribers.
- Gapless boundaries are protected by overlap and FFmpeg recovery, but sample-exact splitting requires decoded PCM timing and is a future enhancement.
- A dedicated HLS segment fetcher would improve compatibility.

Contributions and device/app compatibility reports are welcome.
