# Loqa Test Puck Service

A test implementation of the Loqa puck that captures audio and streams it to the hub via gRPC.

## Features

- 🎤 **Real-time audio capture** with PortAudio
- 🎧 **Voice Activity Detection** with pre-buffering
- 🎯 **Wake word detection** for "Hey Loqa"
- 📡 **gRPC audio streaming** to hub
- 🔊 **Audio playback** for TTS responses
- ⚡ **Production-like architecture**

## Usage

```bash
# Build the puck service
cd test-go
go build -o test-puck ./cmd

# Run the puck (hub must be running on localhost:50051)
./test-puck

# Run with custom hub address and puck ID
./test-puck -hub hub.local:50051 -id kitchen-puck
```

## Wake Word Detection

The puck includes wake word detection for "Hey Loqa":

- **Wake word**: "Hey Loqa" 
- **Algorithm**: Simple energy envelope pattern matching
- **Threshold**: Configurable confidence level (default: 0.7)
- **Status**: Enabled by default

The puck will only transmit audio to the hub after detecting the wake word, providing privacy and reducing network traffic.

## Architecture

```
Puck (this service)     gRPC Stream      Hub (Docker)
┌─────────────────┐    ──────────────►   ┌─────────────┐
│ Audio Capture   │    Audio Chunks     │ LLM Parser  │
│ Voice Detection │                     │ Commands    │
│ Wake Word Det   │    ◄──────────────  │ Responses   │
│ Audio Playback  │    Command/TTS      │ Device Ctrl │
└─────────────────┘                     └─────────────┘
```

## gRPC Protocol

- **StreamAudio**: Bidirectional stream for audio chunks and responses
- **PlayAudio**: Stream TTS audio from hub to puck
- **Audio format**: 16kHz, mono, PCM

## Testing

Use the provided test script to verify wake word functionality:

```bash
# Run the test environment
./tools/test-wake-word.sh

# In another terminal, run the puck
cd puck/test-go
./test-puck --hub localhost:50051

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
cd puck/test-go
go mod tidy
go build -o test-puck ./cmd
```

### Configuration
Environment variables:
- `HUB_ADDRESS`: Hub gRPC address (default: localhost:50051)
- `PUCK_ID`: Unique puck identifier (default: test-puck)
- `WAKE_WORD_THRESHOLD`: Detection confidence (default: 0.7)
- `AUDIO_SAMPLE_RATE`: Audio capture rate (default: 16000)

### Hardware Requirements
- Microphone input
- Audio output (for TTS responses)
- Network connectivity to hub