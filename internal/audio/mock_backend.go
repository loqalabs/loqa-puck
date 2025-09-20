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
	"math"
	"sync"
	"time"
)

// MockAudioBackend implements AudioBackend for testing without hardware dependencies
type MockAudioBackend struct {
	mu                  sync.Mutex
	initialized         bool
	streams             map[string]*MockStream
	streamCounter       int
	initError           error
	terminateError      error
	createStreamError   error
	simulateRealTiming  bool
	recordedAudioData   [][]float32
	playbackAudioData   [][]float32
}

// NewMockAudioBackend creates a new mock audio backend
func NewMockAudioBackend() *MockAudioBackend {
	return &MockAudioBackend{
		streams:           make(map[string]*MockStream),
		simulateRealTiming: true,
		recordedAudioData:  make([][]float32, 0),
		playbackAudioData:  make([][]float32, 0),
	}
}

// SetInitError configures the backend to return an error on Initialize()
func (m *MockAudioBackend) SetInitError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initError = err
}

// SetCreateStreamError configures the backend to return an error on stream creation
func (m *MockAudioBackend) SetCreateStreamError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createStreamError = err
}

// SetSimulateRealTiming controls whether the mock simulates real audio timing
func (m *MockAudioBackend) SetSimulateRealTiming(simulate bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.simulateRealTiming = simulate
}

// GetRecordedAudioData returns all audio data that was "recorded"
func (m *MockAudioBackend) GetRecordedAudioData() [][]float32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([][]float32, len(m.recordedAudioData))
	copy(result, m.recordedAudioData)
	return result
}

// GetPlaybackAudioData returns all audio data that was "played back"
func (m *MockAudioBackend) GetPlaybackAudioData() [][]float32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([][]float32, len(m.playbackAudioData))
	copy(result, m.playbackAudioData)
	return result
}

// Initialize initializes the mock audio subsystem
func (m *MockAudioBackend) Initialize() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.initError != nil {
		return m.initError
	}

	m.initialized = true
	return nil
}

// Terminate terminates the mock audio subsystem
func (m *MockAudioBackend) Terminate() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.terminateError != nil {
		return m.terminateError
	}

	// Stop all streams first without holding locks
	var streams []StreamInterface
	for _, stream := range m.streams {
		streams = append(streams, stream)
	}

	// Release the lock before calling Stop/Close to avoid deadlocks
	m.mu.Unlock()

	for _, stream := range streams {
		_ = stream.Stop() // Ignore errors during cleanup
		_ = stream.Close() // Ignore errors during cleanup
	}

	// Re-acquire lock to update state
	m.mu.Lock()
	m.initialized = false
	return nil
}

// CreateInputStream creates a mock input stream
func (m *MockAudioBackend) CreateInputStream(sampleRate float64, channels, bufferSize int) (StreamInterface, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized {
		return nil, fmt.Errorf("mock audio backend not initialized")
	}

	if m.createStreamError != nil {
		return nil, m.createStreamError
	}

	streamID := fmt.Sprintf("input_%d", m.streamCounter)
	m.streamCounter++

	stream := &MockStream{
		id:                 streamID,
		backend:            m,
		sampleRate:         sampleRate,
		channels:           channels,
		bufferSize:         bufferSize,
		isInput:            true,
		simulateRealTiming: m.simulateRealTiming,
		stopChannel:        make(chan bool, 1),
	}

	m.streams[streamID] = stream
	return stream, nil
}

// CreateOutputStream creates a mock output stream
func (m *MockAudioBackend) CreateOutputStream(sampleRate float64, channels, bufferSize int) (StreamInterface, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized {
		return nil, fmt.Errorf("mock audio backend not initialized")
	}

	if m.createStreamError != nil {
		return nil, m.createStreamError
	}

	streamID := fmt.Sprintf("output_%d", m.streamCounter)
	m.streamCounter++

	stream := &MockStream{
		id:                 streamID,
		backend:            m,
		sampleRate:         sampleRate,
		channels:           channels,
		bufferSize:         bufferSize,
		isInput:            false,
		simulateRealTiming: m.simulateRealTiming,
		stopChannel:        make(chan bool, 1),
	}

	m.streams[streamID] = stream
	return stream, nil
}

// MockStream implements StreamInterface for testing
type MockStream struct {
	mu                 sync.Mutex
	id                 string
	backend            *MockAudioBackend
	sampleRate         float64
	channels           int
	bufferSize         int
	isInput            bool
	isOpen             bool
	isActive           bool
	simulateRealTiming bool
	callback           StreamCallback
	stopChannel        chan bool
	startError         error
	stopError          error
	closeError         error
	writeError         error
	readError          error
	audioDataGenerator func([]float32) // For generating mock audio input
}

// SetStartError configures the stream to return an error on Start()
func (m *MockStream) SetStartError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startError = err
}

// SetWriteError configures the stream to return an error on Write()
func (m *MockStream) SetWriteError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeError = err
}

// SetAudioDataGenerator sets a function to generate mock audio input data
func (m *MockStream) SetAudioDataGenerator(generator func([]float32)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audioDataGenerator = generator
}

// Start starts the mock stream
func (m *MockStream) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.startError != nil {
		return m.startError
	}

	if m.isActive {
		return fmt.Errorf("stream already active")
	}

	m.isActive = true
	m.isOpen = true

	// Start background processing for input streams
	if m.isInput {
		go m.simulateAudioInput()
	}

	return nil
}

// Stop stops the mock stream
func (m *MockStream) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopError != nil {
		return m.stopError
	}

	if !m.isActive {
		return nil
	}

	m.isActive = false

	// Signal stop to background goroutine
	select {
	case m.stopChannel <- true:
	default:
	}

	return nil
}

// Close closes the mock stream
func (m *MockStream) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closeError != nil {
		return m.closeError
	}

	if !m.isOpen {
		return nil // Already closed
	}

	m.isOpen = false
	m.isActive = false

	// Signal stop to avoid deadlocks
	select {
	case m.stopChannel <- true:
	default:
	}

	// Remove from backend - use a separate goroutine to avoid deadlock
	go func() {
		m.backend.mu.Lock()
		delete(m.backend.streams, m.id)
		m.backend.mu.Unlock()
	}()

	return nil
}

// Write writes audio data to the mock output stream
func (m *MockStream) Write(data []float32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.writeError != nil {
		return m.writeError
	}

	if !m.isOpen {
		return fmt.Errorf("stream not open")
	}

	if m.isInput {
		return fmt.Errorf("cannot write to input stream")
	}

	// Record the audio data
	dataCopy := make([]float32, len(data))
	copy(dataCopy, data)

	m.backend.mu.Lock()
	m.backend.playbackAudioData = append(m.backend.playbackAudioData, dataCopy)
	m.backend.mu.Unlock()

	// Simulate real timing if enabled
	if m.simulateRealTiming {
		duration := time.Duration(float64(len(data)) / m.sampleRate * float64(time.Second))
		time.Sleep(duration)
	}

	return nil
}

// Read reads audio data from the mock input stream
func (m *MockStream) Read(data []float32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.readError != nil {
		return m.readError
	}

	if !m.isOpen {
		return fmt.Errorf("stream not open")
	}

	if !m.isInput {
		return fmt.Errorf("cannot read from output stream")
	}

	// Generate mock audio data
	if m.audioDataGenerator != nil {
		m.audioDataGenerator(data)
	} else {
		// Default: generate a simple sine wave
		for i := range data {
			// 440 Hz sine wave
			t := float64(i) / m.sampleRate
			data[i] = float32(0.1 * math.Sin(2*math.Pi*440*t))
		}
	}

	// Record the audio data
	dataCopy := make([]float32, len(data))
	copy(dataCopy, data)

	m.backend.mu.Lock()
	m.backend.recordedAudioData = append(m.backend.recordedAudioData, dataCopy)
	m.backend.mu.Unlock()

	// Simulate real timing if enabled
	if m.simulateRealTiming {
		duration := time.Duration(float64(len(data)) / m.sampleRate * float64(time.Second))
		time.Sleep(duration)
	}

	return nil
}

// IsActive returns true if the mock stream is active
func (m *MockStream) IsActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isActive
}

// SetCallback sets the callback function
func (m *MockStream) SetCallback(callback StreamCallback) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callback = callback
	return nil
}

// simulateAudioInput runs in background to simulate continuous audio input
func (m *MockStream) simulateAudioInput() {
	buffer := make([]float32, m.bufferSize*m.channels)
	ticker := time.NewTicker(time.Duration(float64(m.bufferSize)/m.sampleRate*1000) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChannel:
			return
		case <-ticker.C:
			// Check if still active without holding lock for too long
			m.mu.Lock()
			isActive := m.isActive
			m.mu.Unlock()

			if !isActive {
				return
			}

			// Generate audio data
			m.mu.Lock()
			if m.audioDataGenerator != nil {
				m.audioDataGenerator(buffer)
			} else {
				// Default: generate sine wave with some variation
				for i := range buffer {
					t := float64(time.Now().UnixNano()) / 1e9
					buffer[i] = float32(0.1 * math.Sin(2*math.Pi*440*t))
				}
			}
			m.mu.Unlock()

			// Record the data
			dataCopy := make([]float32, len(buffer))
			copy(dataCopy, buffer)

			m.backend.mu.Lock()
			m.backend.recordedAudioData = append(m.backend.recordedAudioData, dataCopy)
			m.backend.mu.Unlock()
		}
	}
}