[![Sponsor](https://img.shields.io/badge/Sponsor-Loqa-ff69b4?logo=githubsponsors&style=for-the-badge)](https://github.com/sponsors/annabarnes1138)
[![Ko-fi](https://img.shields.io/badge/Buy%20me%20a%20coffee-Ko--fi-FF5E5B?logo=ko-fi&logoColor=white&style=for-the-badge)](https://ko-fi.com/annabarnes)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL--3.0-blue?style=for-the-badge)](LICENSE)
[![Made with ❤️ by LoqaLabs](https://img.shields.io/badge/Made%20with%20%E2%9D%A4%EF%B8%8F-by%20LoqaLabs-ffb6c1?style=for-the-badge)](https://loqalabs.com)

# Loqa Test Relay Service

[![CI/CD Pipeline](https://github.com/loqalabs/loqa-relay/actions/workflows/ci.yml/badge.svg)](https://github.com/loqalabs/loqa-relay/actions/workflows/ci.yml)

A test implementation of the Loqa relay that captures audio and streams it to the hub via gRPC.

## Features

- 🎤 **Real-time audio capture** with PortAudio
- 🎧 **Voice Activity Detection** with pre-buffering
- 🎯 **Wake word detection** for "Hey Loqa"
- 📡 **gRPC audio streaming** to hub
- 🔊 **Audio playback** for TTS responses
- ⚡ **Production-like architecture**

## Usage

```bash
# Build the relay service
cd test-go
go build -o test-relay ./cmd

# Run the relay (hub must be running on localhost:50051)
./test-relay

# Run with custom hub address and relay ID
./test-relay -hub hub.local:50051 -id kitchen-relay
```

## Wake Word Detection

The relay includes wake word detection for "Hey Loqa":

- **Wake word**: "Hey Loqa" 
- **Algorithm**: Simple energy envelope pattern matching
- **Threshold**: Configurable confidence level (default: 0.7)
- **Status**: Enabled by default

The relay will only transmit audio to the hub after detecting the wake word, providing privacy and reducing network traffic.

## Architecture

```
Relay (this service)     gRPC Stream      Hub (Docker)
┌─────────────────┐    ──────────────►   ┌─────────────┐
│ Audio Capture   │    Audio Chunks     │ LLM Parser  │
│ Voice Detection │                     │ Commands    │
│ Wake Word Det   │    ◄──────────────  │ Responses   │
│ Audio Playback  │    Command/TTS      │ Device Ctrl │
└─────────────────┘                     └─────────────┘
```

## gRPC Protocol

- **StreamAudio**: Bidirectional stream for audio chunks and responses
- **PlayAudio**: Stream TTS audio from hub to relay
- **Audio format**: 16kHz, mono, PCM

## Testing

Use the provided test script to verify wake word functionality:

```bash
# Run the test environment
./tools/test-wake-word.sh

# In another terminal, run the relay
cd relay/test-go
./test-relay --hub localhost:50051

# Test by saying: "Hey Loqa, turn on the lights"
```

## Implementation Status

- [x] Connect to hub service
- [x] Test end-to-end audio pipeline  
- [x] Add wake word detection
- [ ] Implement TTS audio playback
- [ ] Add device-specific configuration
- [ ] Optimize power consumption

## Development

### Building
```bash
# Use the project build script
./tools/build.sh

# Or build manually
cd relay/test-go
go mod tidy
go build -o test-relay ./cmd
```

### Configuration
Environment variables:
- `HUB_ADDRESS`: Hub gRPC address (default: localhost:50051)
- `RELAY_ID`: Unique relay identifier (default: test-relay)
- `WAKE_WORD_THRESHOLD`: Detection confidence (default: 0.7)
- `AUDIO_SAMPLE_RATE`: Audio capture rate (default: 16000)

### Hardware Requirements
- Microphone input
- Audio output (for TTS responses)
- Network connectivity to hub