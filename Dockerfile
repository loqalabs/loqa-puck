FROM golang:1.24-alpine AS builder

# Install PortAudio dependencies
RUN apk add --no-cache \
    portaudio-dev \
    alsa-lib-dev \
    gcc \
    musl-dev \
    pkgconfig

WORKDIR /build
# Copy loqa-proto dependency to match the replace path ../../loqa-proto/go
COPY loqa-proto/ loqa-proto/

# Set up the test-go directory structure
WORKDIR /build/loqa-puck/test-go
COPY loqa-puck/test-go/go.mod loqa-puck/test-go/go.sum ./
RUN go mod download

COPY loqa-puck/test-go/ .
RUN go build -o test-puck ./cmd

FROM alpine:latest
RUN apk --no-cache add \
    ca-certificates \
    portaudio \
    alsa-lib

WORKDIR /root/

COPY --from=builder /build/loqa-puck/test-go/test-puck .

ENV HUB_ADDRESS=hub:50051
ENV PUCK_ID=docker-puck
ENV WAKE_WORD_THRESHOLD=0.7

CMD ["./test-puck"]