[![Sponsor](https://img.shields.io/badge/Sponsor-Loqa-ff69b4?logo=githubsponsors&style=for-the-badge)](https://github.com/sponsors/annabarnes1138)
[![Ko-fi](https://img.shields.io/badge/Buy%20me%20a%20coffee-Ko--fi-FF5E5B?logo=ko-fi&logoColor=white&style=for-the-badge)](https://ko-fi.com/annabarnes)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL--3.0-blue?style=for-the-badge)](LICENSE)
[![Made with ❤️ by LoqaLabs](https://img.shields.io/badge/Made%20with%20%E2%9D%A4%EF%B8%8F-by%20LoqaLabs-ffb6c1?style=for-the-badge)](https://loqalabs.com)

# Loqa Relay - Go Puck Reference Implementation

[![CI/CD Pipeline](https://github.com/loqalabs/loqa-puck-go/actions/workflows/ci.yml/badge.svg)](https://github.com/loqalabs/loqa-puck-go/actions/workflows/ci.yml)

A reference implementation of the Loqa puck client that captures audio and streams it to the hub via HTTP/1.1 streaming with binary frame protocol. This implementation serves as the foundation for future ESP32 firmware development.

## Features

- 🎤 **Real-time audio capture** with PortAudio
- 🎧 **Voice Activity Detection** with pre-buffering
- 🎯 **Wake word detection** for "Hey Loqa"
- 📡 **HTTP/1.1 streaming** to hub with binary frame protocol
- 🔊 **Audio playback** for TTS responses via NATS
- ⚡ **ESP32-compatible architecture** with 4KB frame limits
- 🔧 **Production-ready design** for embedded deployment

## Architecture

### HTTP/1.1 Streaming Transport

The relay uses HTTP/1.1 chunked transfer encoding for bidirectional streaming communication with the hub:

```
Puck (loqa-puck-go)           HTTP/1.1 Stream            Hub (loqa-hub)
┌─────────────────────┐    ────────────────────►      ┌─────────────────┐
│ Audio Capture       │    Binary Frame Protocol      │ Intent Cascade  │
│ Wake Word Detection │                               │ • Reflex        │
│ Binary Frame Encode │    ◄────────────────────      │ • LLM           │
│ HTTP Streaming      │    Response Frames            │ • Cloud (opt)   │
└─────────────────────┘                               └─────────────────┘
```

### Binary Frame Protocol

Optimized for ESP32 compatibility with minimal memory overhead:

```
Frame Header (24 bytes):
┌─────────┬──────┬─────────┬────────┬─────────┬──────────┬───────────┐
│ Magic   │ Type │ Reserved│ Length │ Session │ Sequence │ Timestamp │
│ 4 bytes │1 byte│ 1 byte  │2 bytes │ 4 bytes │ 4 bytes  │ 8 bytes   │
└─────────┴──────┴─────────┴────────┴─────────┴──────────┴───────────┘

Frame Types:
• 0x01 - Audio Data      • 0x10 - Heartbeat     • 0x20 - Response
• 0x02 - Audio End       • 0x11 - Handshake     • 0x21 - Status
• 0x03 - Wake Word       • 0x12 - Error
                         • 0x13 - Arbitration

Max Frame Size: 4KB (ESP32 memory optimized)
```

### Streaming Flow

1. **Audio Capture** → PortAudio captures 16kHz mono PCM
2. **Wake Word Detection** → Local pattern matching for "Hey Loqa"
3. **Frame Encoding** → Audio data encoded into binary frames
4. **HTTP Streaming** → POST to `/stream/puck?puck_id={id}`
5. **Response Processing** → TTS and status frames from hub
6. **Audio Playback** → TTS audio via NATS integration

## Usage

### Quick Start

```bash
# Build the relay
go build -o bin/loqa-puck-go ./cmd

# Run with hub connection (hub must be running on localhost:3000)
./bin/loqa-puck-go -hub http://localhost:3000 -id kitchen-puck

# Run with custom configuration
./bin/loqa-puck-go -hub http://hub.local:3000 -id living-room-puck -nats nats://localhost:4222
```

### Configuration

Environment variables:
- `HUB_ADDRESS`: Hub HTTP address (default: http://localhost:3000)
- `PUCK_ID`: Unique puck identifier (default: loqa-puck-001)
- `WAKE_WORD_THRESHOLD`: Detection confidence (default: 0.7)
- `AUDIO_SAMPLE_RATE`: Audio capture rate (default: 16000)
- `NATS_URL`: NATS server URL (default: nats://localhost:4222)

### Wake Word Detection

The relay includes local wake word detection:

- **Wake phrase**: "Hey Loqa"
- **Algorithm**: Energy envelope pattern matching
- **Threshold**: Configurable confidence level (0.0 - 1.0)
- **Privacy**: Only transmits audio after wake word detection

## API Integration

### Streaming Endpoint

The relay connects to the hub's streaming endpoint:

```http
POST /stream/puck?puck_id={puck_id}
Content-Type: application/octet-stream
Transfer-Encoding: chunked
X-Puck-ID: {puck_id}
X-Session-ID: {session_id}

[Binary frame data stream]
```

### Response Handling

The relay processes various frame types from the hub:

- **Response Frames (0x20)**: TTS audio and command responses
- **Status Frames (0x21)**: System status and configuration updates
- **Error Frames (0x12)**: Error messages and diagnostics
- **Heartbeat Frames (0x10)**: Connection keep-alive

## Development

### Building

```bash
# Development build
make build

# Production build with optimizations
go build -ldflags="-w -s" -o bin/loqa-puck-go ./cmd

# Cross-compilation for ARM (ESP32 preparation)
GOOS=linux GOARCH=arm GOARM=7 go build -o bin/loqa-puck-go-arm ./cmd
```

### Testing

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run quality checks
make quality-check
```

### Docker

```bash
# Build Docker image
docker build -t loqalabs/loqa-puck-go .

# Run in container
docker run --rm \
  -e HUB_ADDRESS=http://host.docker.internal:3000 \
  -e PUCK_ID=docker-puck \
  loqalabs/loqa-puck-go
```

## Hardware Requirements

### Development
- Microphone input (built-in or USB)
- Audio output (speakers/headphones)
- Network connectivity to hub

### Production (ESP32 Target)
- ESP32-S3 or compatible (4MB+ PSRAM recommended)
- I2S microphone (INMP441 or similar)
- I2S DAC/amplifier for audio output
- WiFi connectivity

## Implementation Status

- [x] HTTP/1.1 streaming transport
- [x] Binary frame protocol
- [x] Audio capture and playback
- [x] Wake word detection
- [x] Real-time streaming to hub
- [x] TTS response handling via NATS
- [x] ESP32-compatible frame limits
- [ ] Power optimization for embedded deployment
- [ ] ESP32 firmware port
- [ ] Hardware abstraction layer

## ESP32 Preparation

This Go implementation is designed as the reference for ESP32 firmware:

### Memory Optimization
- 4KB maximum frame size
- Efficient binary serialization
- Minimal HTTP headers
- Stream-based processing

### Protocol Simplification
- HTTP/1.1 (simpler than gRPC for embedded)
- Binary frames (efficient parsing)
- Stateless design
- Connection management with heartbeats

### Audio Pipeline
- 16kHz mono PCM (ESP32 I2S compatible)
- Local wake word detection
- Chunked audio transmission
- Real-time response playback

## Testing Integration

### With Hub Service

```bash
# Start hub service (in loqa-hub repository)
docker-compose up -d

# In another terminal, run relay
./bin/loqa-puck-go -hub http://localhost:3000

# Test wake word: "Hey Loqa, what time is it?"
```

### End-to-End Testing

```bash
# Use the provided test script
./scripts/test-wake-word.sh

# Test streaming: "Hey Loqa, turn on the lights"
# Expected: Audio streaming → Intent processing → Device command → TTS response
```

## Performance Characteristics

### Latency Targets
- **Wake word detection**: <100ms
- **Audio transmission**: <50ms per frame
- **Response latency**: <500ms (hub dependent)

### Resource Usage
- **Memory**: <50MB (Go runtime)
- **CPU**: <5% on modern hardware
- **Network**: ~32Kbps for continuous audio

### ESP32 Projections
- **Memory**: <2MB (firmware size)
- **RAM**: <512KB (runtime)
- **Network**: Same ~32Kbps bandwidth

## License

AGPL v3 - Copyleft license requiring source disclosure for derivatives.

## Security

Report security issues to security@loqalabs.com

## Contributing

1. Fork the repository
2. Create a feature branch
3. Run quality checks: `make quality-check`
4. Submit pull request with tests

For ESP32 firmware development, use this implementation as the protocol reference.