FROM golang:1.25.1-alpine AS builder

# Install PortAudio dependencies
RUN apk add --no-cache \
    portaudio-dev \
    alsa-lib-dev \
    gcc \
    musl-dev \
    pkgconfig

WORKDIR /build

# Copy Go module files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build
COPY . .
RUN go build -o loqa-puck-go ./cmd

FROM alpine:latest
RUN apk --no-cache add \
    ca-certificates \
    portaudio \
    alsa-lib

WORKDIR /root/

COPY --from=builder /build/loqa-puck-go .

ENV HUB_ADDRESS=http://hub:3000
ENV PUCK_ID=docker-puck
ENV WAKE_WORD_THRESHOLD=0.7
ENV NATS_URL=nats://nats:4222

CMD ["./loqa-puck-go"]