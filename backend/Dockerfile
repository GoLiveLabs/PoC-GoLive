FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./cmd/server

FROM alpine:3.20

# ffmpeg is required at runtime: internal/broadcast relays the OBS program feed
# to each platform via one `ffmpeg` subprocess per destination.
RUN apk add --no-cache ffmpeg

WORKDIR /app
COPY --from=builder /out/server /app/server

EXPOSE 8080

ENTRYPOINT ["/app/server"]
