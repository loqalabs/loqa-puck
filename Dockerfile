FROM golang:1.23.0-alpine AS builder

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
WORKDIR /build/loqa-relay/test-go
COPY loqa-relay/test-go/go.mod loqa-relay/test-go/go.sum ./
RUN go mod download

COPY loqa-relay/test-go/ .
RUN go build -o test-relay ./cmd

FROM alpine:latest
RUN apk --no-cache add \
    ca-certificates \
    portaudio \
    alsa-lib

WORKDIR /root/

COPY --from=builder /build/loqa-relay/test-go/test-relay .

ENV HUB_ADDRESS=hub:50051
ENV RELAY_ID=docker-relay
ENV WAKE_WORD_THRESHOLD=0.7

CMD ["./test-relay"]