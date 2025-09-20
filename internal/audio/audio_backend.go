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

// AudioBackend provides an abstraction layer for audio operations
// This enables dependency injection and makes testing hardware-independent
type AudioBackend interface {
	// Initialize the audio subsystem
	Initialize() error

	// Terminate the audio subsystem
	Terminate() error

	// CreateInputStream creates an input stream for recording
	CreateInputStream(sampleRate float64, channels, bufferSize int) (StreamInterface, error)

	// CreateOutputStream creates an output stream for playback
	CreateOutputStream(sampleRate float64, channels, bufferSize int) (StreamInterface, error)
}

// StreamInterface abstracts audio stream operations
type StreamInterface interface {
	// Start the audio stream
	Start() error

	// Stop the audio stream
	Stop() error

	// Close the audio stream and release resources
	Close() error

	// Write audio data to output stream
	Write(data []float32) error

	// Read audio data from input stream
	Read(data []float32) error

	// IsActive returns true if the stream is currently active
	IsActive() bool

	// SetCallback sets the callback function for stream processing
	SetCallback(callback StreamCallback) error
}

// StreamCallback is called when audio data is available or needed
type StreamCallback func(input, output []float32) error

// StreamParams holds parameters for stream creation
type StreamParams struct {
	SampleRate  float64
	Channels    int
	BufferSize  int
	Callback    StreamCallback
}