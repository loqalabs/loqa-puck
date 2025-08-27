# 🎤 Loqa Puck

Embedded and test clients for capturing and streaming audio to the Loqa Hub.

## Overview

Loqa Puck provides audio input devices for the Loqa voice assistant platform:
- ESP32-based embedded pucks with local wake word detection
- Go-based test clients for development and testing
- Audio streaming via gRPC to the Hub service

## Components

### ESP32 Firmware
- Local wake word detection
- Audio capture and streaming
- Low-power operation
- WiFi connectivity

### Go Test Puck
- Development and testing client
- PortAudio integration for microphone access
- Configurable wake word sensitivity
- Debug logging and monitoring

## Features

- 🎙️ **Audio Capture**: High-quality audio recording and streaming
- 🔊 **Wake Word Detection**: Local "Hey Loqa" detection
- 📡 **gRPC Streaming**: Real-time audio transmission to Hub
- 🔋 **Low Power**: Optimized for battery operation (ESP32)
- 🛠️ **Development Tools**: Test clients for rapid prototyping

## Hardware Requirements

### ESP32 Puck
- ESP32-S3 with PSRAM
- I2S microphone (INMP441 or similar)
- WiFi connectivity

### Test Puck
- Computer with microphone
- PortAudio-compatible audio system

## Getting Started

See the main [Loqa documentation](https://github.com/loqalabs/loqa-docs) for setup and usage instructions.

## License

Licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.