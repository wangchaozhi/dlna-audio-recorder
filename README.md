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

- Go 1.23+ (building from source)
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
-listen :1400                 HTTP/SOAP listener
-advertise-ip 192.168.1.20    LAN address exposed through SSDP
-name "DLNA Audio Recorder"   renderer name shown to senders
-output recordings            completed files
-temp recordings/.tmp         in-progress captures
-ffmpeg ffmpeg                FFmpeg executable/path
-segment-grace 1.5s           overlap retained around track boundaries
-keep-raw                     keep .capture files after successful conversion
```

## Docker

DLNA discovery uses multicast, so host networking is recommended on Linux:

```bash
docker build -t dlna-audio-recorder .
docker run --rm --network host \
  -v "$PWD/recordings:/recordings" \
  dlna-audio-recorder -advertise-ip 192.168.1.20
```

Docker Desktop networking on macOS/Windows may not expose SSDP multicast the same way as native host networking; running the native binary is generally simpler there.

## Architecture

```text
Phone / DLNA controller
        |
        | SSDP + SOAP AVTransport
        v
+---------------------------+
| DLNA Audio Recorder       |
|                           |
| MediaRenderer             |
|   -> track state machine  |
|   -> DIDL-Lite metadata   |
|                           |
| StreamSession (long-lived)|------ HTTP GET ------> media source
|   -> current segment      |
|   -> tail overlap buffer  |
|   -> segment rotation     |
+-------------+-------------+
              |
              v
        raw .capture
              |
           FFmpeg
              |
              v
      Artist - Title.m4a
```

## Track-boundary behavior

1. A new URI starts a new `StreamSession`.
2. A metadata change for the **same URI** rotates the segment but keeps the upstream session alive.
3. A different URI cancels the previous fetch, finalizes it, and starts the next stream.
4. `SetNextAVTransportURI` is stored as a hint, not treated as an immediate boundary, because many controllers announce the next song well before it starts.
5. `Stop` closes and finalizes the current segment.

This is deliberately control-plane driven rather than silence detection; live mixes and gapless albums often have no silence at boundaries.

## Supported input

The recorder declares common HTTP audio protocols (MP3, AAC, MP4/M4A, FLAC and HLS). Actual decoding support is determined by your FFmpeg build. Plain HTTP(S) media URLs work best. Authentication is supported when credentials/tokens are embedded in or otherwise accepted by the media URL supplied by the controller.

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
- HLS streams are accepted by FFmpeg after capture only when the sender gives a fetchable stream form; a dedicated HLS segment fetcher would improve compatibility.

Contributions and device/app compatibility reports are welcome.

## QQ Music / QPlay experimental support

The renderer now advertises Tencent's QPlay service (`urn:schemas-tencent-com:service:QPlay:1`) in addition to standard DLNA services.

QPlay queue mode differs from ordinary DLNA: QQ Music can first call `SetAVTransportURI` with a virtual URI such as `qplay://<QueueID>` and then send the real tracks through `InsertTracks` or `SetTracksInfo`. The recorder therefore treats `qplay://` as a queue identifier, not as a fetchable media URL.

Implemented QPlay actions:

- `InsertTracks`
- `SetTracksInfo`
- `RemoveTracks`
- `GetTracksInfo`
- `GetTracksCount`
- `GetMaxTracks`
- diagnostic `QPlayAuth` response

QPlay SOAP activity is logged with queue identifiers redacted and only metadata byte counts, so a compatibility test can reveal which actions QQ Music uses without dumping account tokens or full media URLs into logs. Track metadata JSON is parsed defensively because device/controller implementations differ in field naming.

This support is intentionally marked experimental until it is tested against a real current QQ Music client. QPlay 2 authentication and vendor certification behavior can vary by client/device and may require additional compatibility work.

## Web dashboard

Open the recorder base URL (for example `http://192.168.1.20:1400/`) to use the embedded, read-only dashboard. It is served from the same Go binary and does not require Node.js or a separate frontend deployment.

The first dashboard version shows:

- renderer/runtime status and uptime;
- current DLNA/QPlay track and next-track hints;
- a live protocol event feed over Server-Sent Events (SSE);
- the latest finalized `.m4a` recordings with browser playback/download links;
- redacted media origins only (scheme + host), so signed QQ Music media paths/query tokens are not exposed to the browser.

The dashboard observes playback-domain events; the recorder core does not depend on the UI. This keeps future QPlay/device integrations independent from the control panel.
