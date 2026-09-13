FROM golang:1.23-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/dlna-recorder ./cmd/dlna-recorder

FROM alpine:3.22
RUN apk add --no-cache ffmpeg ca-certificates
COPY --from=build /out/dlna-recorder /usr/local/bin/dlna-recorder
VOLUME ["/recordings"]
EXPOSE 1400/tcp 1900/udp
ENTRYPOINT ["dlna-recorder","-listen",":1400","-output","/recordings"]
