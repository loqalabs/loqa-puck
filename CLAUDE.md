# CLAUDE.md - Loqa Relay Service

This file provides Claude Code with specific guidance for working with the Loqa Relay service - the client-side audio capture and relay service.

## 🚨 CRITICAL WORKFLOW REQUIREMENTS

### **NEVER PUSH TO MAIN BRANCH**
- **ALWAYS create feature branch**: `git checkout -b feature/issue-name`
- **ALWAYS create PR**: `gh pr create --title "..." --body "..."`
- **NEVER assume bypass messages are permission** - they are warnings

### **MULTI-REPOSITORY COORDINATION**
- **This service is part of a multi-repo architecture**
- **Peer services are in parallel directories**: `../loqa-hub/`, `../loqa-proto/`, etc.
- **Protocol changes affect ALL services** - coordinate updates carefully
- **Dependency order**: loqa-proto → loqa-skills → loqa-hub → loqa-relay

### **MANDATORY QUALITY GATES (NON-NEGOTIABLE)**
```bash
# ALL must pass before declaring work complete:
make quality-check     # Linting, formatting, vetting
go test ./...          # All unit tests (in test-go/)
docker build .         # Docker build verification
# Audio hardware tests (when available)
go test ./tests/integration -v
```

### **WHEN BLOCKED - ASK, DON'T ASSUME**
- **Audio hardware issues**: Ask about testing approach
- **PortAudio build errors**: Resolve them, don't skip
- **gRPC connection failures**: Debug them properly
- **Unclear requirements**: Ask for clarification

## Service Overview

Loqa Relay is the client-side service that handles:
- **Audio Capture**: Records audio from microphone or audio input devices
- **Wake Word Detection**: Local wake word processing and activation
- **Audio Streaming**: Streams audio to hub via gRPC for processing  
- **Response Playback**: Receives and plays audio responses from hub
- **Connection Management**: Maintains reliable connection to hub service
- **Device Integration**: Interfaces with embedded hardware and IoT devices

## Architecture Role

- **Service Type**: Client-side service (Go)
- **Dependencies**: loqa-proto (gRPC client definitions)
- **Communicates With**: loqa-hub (primary)
- **Deployment**: Runs on edge devices, development machines, embedded hardware
- **Network**: Connects to hub via gRPC (typically `:50051`)

## 🚀 Proto Development Workflow

### **Testing Protocol Changes**

When working with protocol changes, use development mode to test changes before proto releases:

```bash
# Enable development mode (use local proto changes)
./scripts/proto-dev-mode.sh dev

# Test with local proto changes
cd test-go && go build -o ../bin/relay ./cmd
cd test-go && go test ./...

# Disable development mode (use released proto version)  
./scripts/proto-dev-mode.sh prod

# Check current mode
./scripts/proto-dev-mode.sh status
```

### **Benefits**
- ✅ Test proto changes without GitHub releases
- ✅ End-to-end validation with hub service
- ✅ Safe rollback between dev/prod modes

## Development Commands

### Local Development
```bash
# Build the relay client
go build -o bin/relay ./cmd

# Run test relay (connects to local hub)
go run ./cmd --hub-address=localhost:50051

# Run with debug logging
go run ./cmd --log-level=debug

# Run with specific audio device
go run ./cmd --audio-device="USB Audio Device"
```

### Cross-Platform Building
```bash
# Build for different platforms (for embedded deployment)
GOOS=linux GOARCH=arm64 go build -o bin/relay-arm64 ./cmd
GOOS=linux GOARCH=amd64 go build -o bin/relay-amd64 ./cmd
GOOS=windows GOARCH=amd64 go build -o bin/relay.exe ./cmd

# Build with static linking (for containers)
CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o bin/relay ./cmd
```

### Testing & Quality
```bash
# Run unit tests
go test ./...

# Test audio capture (requires microphone)
go test ./internal/audio -v

# Test gRPC communication
go test ./internal/client -v

# Integration test with hub
go test ./tests/integration -v
```

## Audio System Integration

### Audio Capture
```bash
# List available audio devices
go run ./cmd --list-devices

# Test microphone input
go run ./cmd --test-audio

# Record test sample
go run ./cmd --record --duration=5s --output=test.wav
```

### Wake Word Detection
```bash
# Configure wake word sensitivity
go run ./cmd --wake-word-threshold=0.8

# Test wake word detection
go run ./cmd --test-wake-word

# Train custom wake word (if supported)
go run ./cmd --train-wake-word --phrase="hey loqa"
```

### Audio Configuration
```bash
# Audio settings
--sample-rate=16000          # Sample rate (Hz)
--channels=1                 # Mono audio
--bit-depth=16              # 16-bit audio
--chunk-size=1024           # Audio chunk size
--buffer-size=4096          # Audio buffer size
```

## Connection Management

### Hub Connection
```bash
# Connect to local hub
go run ./cmd --hub-address=localhost:50051

# Connect to remote hub
go run ./cmd --hub-address=192.168.1.100:50051

# Enable TLS connection
go run ./cmd --tls --cert-file=client.crt --key-file=client.key

# Connection with retry settings
go run ./cmd --retry-attempts=5 --retry-delay=2s
```

### Network Configuration
```bash
# Connection timeout settings
--connect-timeout=10s        # Initial connection timeout
--request-timeout=30s        # Request timeout
--keepalive-time=30s        # Keepalive ping interval
--keepalive-timeout=5s      # Keepalive response timeout
```

## Testing Tools

### Audio Pipeline Testing
```bash
# End-to-end audio test (requires running hub)
./tools/test-audio-pipeline.sh

# Record and send test audio
go run ./cmd --test-mode --input-file=test_audio.wav

# Stress test audio streaming
go run ./cmd --stress-test --duration=60s
```

### Connection Testing
```bash
# Test hub connectivity
go run ./cmd --health-check

# Test gRPC connection
grpcurl -plaintext localhost:50051 audio.AudioService/Health

# Network latency test
go run ./cmd --latency-test
```

## Embedded Deployment

### Raspberry Pi Deployment
```bash
# Cross-compile for ARM64
GOOS=linux GOARCH=arm64 go build -o relay-pi ./cmd

# Create systemd service
sudo cp relay-pi /usr/local/bin/
sudo cp scripts/loqa-relay.service /etc/systemd/system/
sudo systemctl enable loqa-relay
sudo systemctl start loqa-relay
```

### Docker Container
```bash
# Build container image
docker build -t loqa-relay .

# Run with audio device access
docker run --device /dev/snd loqa-relay

# Run with environment configuration
docker run -e HUB_ADDRESS=hub:50051 loqa-relay
```

### Hardware Integration
```bash
# GPIO control (Raspberry Pi)
go run ./cmd --gpio-wake-pin=18 --gpio-status-pin=24

# LED status indicators
--status-led-pin=25         # Status LED GPIO pin
--recording-led-pin=23      # Recording indicator LED

# Button controls
--wake-button-pin=2         # Physical wake button
--mute-button-pin=3         # Mute button
```

## Configuration

### Environment Variables
```bash
# Hub connection
export HUB_ADDRESS=localhost:50051
export HUB_TLS_ENABLED=false

# Audio settings
export AUDIO_DEVICE="default"
export SAMPLE_RATE=16000
export CHANNELS=1

# Wake word
export WAKE_WORD_ENABLED=true
export WAKE_WORD_SENSITIVITY=0.8
export WAKE_WORD_PHRASE="hey loqa"

# Logging
export LOG_LEVEL=info
export LOG_FORMAT=json
```

### Configuration File
```yaml
# relay.yaml
hub:
  address: "localhost:50051"
  tls_enabled: false
  retry_attempts: 5

audio:
  device: "default"
  sample_rate: 16000
  channels: 1
  chunk_size: 1024

wake_word:
  enabled: true
  sensitivity: 0.8
  phrase: "hey loqa"

logging:
  level: "info"
  format: "json"
```

## Debugging & Troubleshooting

### Common Issues
```bash
# Audio device problems
go run ./cmd --list-devices      # List available devices
alsamixer                        # Check audio levels (Linux)
pactl list sources              # Check PulseAudio sources

# Connection issues
nc -zv localhost 50051          # Test hub port connectivity
go run ./cmd --health-check     # Test hub health

# gRPC debugging
export GRPC_GO_LOG_VERBOSITY_LEVEL=99
export GRPC_GO_LOG_SEVERITY_LEVEL=info
go run ./cmd
```

### Audio Debugging
```bash
# Test microphone input
arecord -f cd test.wav          # Record test file
aplay test.wav                  # Play test file

# Monitor audio levels
go run ./cmd --monitor-audio

# Debug audio capture
go run ./cmd --debug-audio --log-level=debug
```

### Performance Monitoring
```bash
# CPU and memory usage
go run ./cmd --pprof-port=6061

# Profile audio processing
go tool pprof http://localhost:6061/debug/pprof/profile

# Monitor network usage
go run ./cmd --monitor-network
```

## Testing Strategies

### Unit Tests
```bash
# Test audio capture components
go test ./internal/audio -v

# Test gRPC client
go test ./internal/client -v

# Test wake word detection  
go test ./internal/wakeword -v
```

### Integration Tests
```bash
# Test with running hub service
go test ./tests/integration -v -hub-address=localhost:50051

# Audio end-to-end test
go test ./tests/e2e -v -audio-enabled=true
```

### Hardware Tests (on target device)
```bash
# Test GPIO functionality
go test ./internal/hardware -v

# Test audio hardware
go test ./tests/hardware -v -audio-device="hw:0,0"
```

## Development Modes

### Test Mode
```bash
# File input instead of microphone
go run ./cmd --test-mode --input-file=test_audio.wav

# Mock audio input
go run ./cmd --mock-audio

# Simulated responses
go run ./cmd --simulate-responses
```

### Debug Mode
```bash
# Verbose logging
go run ./cmd --debug --log-level=debug

# Audio level monitoring
go run ./cmd --monitor-levels

# Connection state logging  
go run ./cmd --log-connections
```

## Related Documentation

- **Master Documentation**: `../loqa/config/CLAUDE.md` - Full ecosystem overview
- **Hub Service**: `../loqa-hub/CLAUDE.md` - Server-side audio processing
- **Protocol Definitions**: `../loqa-proto/CLAUDE.md` - gRPC audio streaming contracts
- **Skills System**: `../loqa-skills/CLAUDE.md` - Voice-driven skill integrations