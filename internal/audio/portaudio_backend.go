/*
 * This file is part of Loqa (https://github.com/loqalabs/loqa).
 * Copyright (C) 2025 Loqa Labs
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program. If not, see <https://www.gnu.org/licenses/>.
 */

package audio

import (
	"fmt"

	"github.com/gordonklaus/portaudio"
)

// PortAudioBackend implements AudioBackend using the real PortAudio library
type PortAudioBackend struct {
	initialized bool
}

// NewPortAudioBackend creates a new PortAudio backend
func NewPortAudioBackend() *PortAudioBackend {
	return &PortAudioBackend{}
}

// Initialize initializes the PortAudio subsystem
func (p *PortAudioBackend) Initialize() error {
	if p.initialized {
		return nil
	}

	if err := portaudio.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize PortAudio: %w", err)
	}

	p.initialized = true
	return nil
}

// Terminate terminates the PortAudio subsystem
func (p *PortAudioBackend) Terminate() error {
	if !p.initialized {
		return nil
	}

	err := portaudio.Terminate()
	p.initialized = false
	return err
}

// CreateInputStream creates an input stream for recording
func (p *PortAudioBackend) CreateInputStream(sampleRate float64, channels, bufferSize int) (StreamInterface, error) {
	if !p.initialized {
		return nil, fmt.Errorf("PortAudio not initialized")
	}

	// Create input buffer
	inputBuffer := make([]float32, bufferSize*channels)

	stream, err := portaudio.OpenDefaultStream(
		channels, // input channels
		0,        // output channels (none for input stream)
		sampleRate,
		bufferSize,
		inputBuffer,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to open input stream: %w", err)
	}

	return &PortAudioStream{
		stream:      stream,
		inputBuffer: inputBuffer,
		isInput:     true,
	}, nil
}

// CreateOutputStream creates an output stream for playback
func (p *PortAudioBackend) CreateOutputStream(sampleRate float64, channels, bufferSize int) (StreamInterface, error) {
	if !p.initialized {
		return nil, fmt.Errorf("PortAudio not initialized")
	}

	// Create output buffer
	outputBuffer := make([]float32, bufferSize*channels)

	stream, err := portaudio.OpenDefaultStream(
		0,        // input channels (none for output stream)
		channels, // output channels
		sampleRate,
		bufferSize,
		outputBuffer,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to open output stream: %w", err)
	}

	return &PortAudioStream{
		stream:       stream,
		outputBuffer: outputBuffer,
		isInput:      false,
	}, nil
}

// PortAudioStream implements StreamInterface using PortAudio streams
type PortAudioStream struct {
	stream       *portaudio.Stream
	inputBuffer  []float32
	outputBuffer []float32
	isInput      bool
	callback     StreamCallback
}

// Start starts the audio stream
func (p *PortAudioStream) Start() error {
	if p.stream == nil {
		return fmt.Errorf("stream is nil")
	}
	return p.stream.Start()
}

// Stop stops the audio stream
func (p *PortAudioStream) Stop() error {
	if p.stream == nil {
		return fmt.Errorf("stream is nil")
	}
	return p.stream.Stop()
}

// Close closes the audio stream
func (p *PortAudioStream) Close() error {
	if p.stream == nil {
		return fmt.Errorf("stream is nil")
	}
	return p.stream.Close()
}

// Write writes audio data to the output stream
func (p *PortAudioStream) Write(data []float32) error {
	if p.stream == nil {
		return fmt.Errorf("stream is nil")
	}
	if p.isInput {
		return fmt.Errorf("cannot write to input stream")
	}

	// Copy data to output buffer
	copy(p.outputBuffer, data)
	return p.stream.Write()
}

// Read reads audio data from the input stream
func (p *PortAudioStream) Read(data []float32) error {
	if p.stream == nil {
		return fmt.Errorf("stream is nil")
	}
	if !p.isInput {
		return fmt.Errorf("cannot read from output stream")
	}

	if err := p.stream.Read(); err != nil {
		return err
	}

	// Copy data from input buffer
	copy(data, p.inputBuffer)
	return nil
}

// IsActive returns true if the stream is active
func (p *PortAudioStream) IsActive() bool {
	if p.stream == nil {
		return false
	}
	// PortAudio doesn't have IsActive method, we track state manually
	// In practice, a stream is active if it has been started and not stopped
	return true // Simplified for now - could track state if needed
}

// SetCallback sets the callback function (not used in this implementation)
func (p *PortAudioStream) SetCallback(callback StreamCallback) error {
	p.callback = callback
	return nil
}