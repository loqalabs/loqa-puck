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

package main

import (
	"testing"
)

func TestBytesToFloat32Array(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []float32
		wantLen  int
	}{
		{
			name:     "empty input",
			input:    []byte{},
			expected: []float32{},
			wantLen:  0,
		},
		{
			name:     "odd number of bytes - should drop last byte",
			input:    []byte{0x00, 0x01, 0xFF},
			expected: []float32{256.0 / 32767.0}, // 0x0100 = 256
			wantLen:  1,
		},
		{
			name:     "single 16-bit sample - positive value",
			input:    []byte{0x00, 0x10}, // 0x1000 = 4096
			expected: []float32{4096.0 / 32767.0},
			wantLen:  1,
		},
		{
			name:     "single 16-bit sample - negative value",
			input:    []byte{0x00, 0x80}, // 0x8000 = -32768 (int16)
			expected: []float32{-32768.0 / 32767.0},
			wantLen:  1,
		},
		{
			name:     "multiple samples",
			input:    []byte{0x00, 0x00, 0xFF, 0x7F, 0x00, 0x80}, // 0, 32767, -32768
			expected: []float32{0.0, 1.0, -32768.0 / 32767.0},
			wantLen:  3,
		},
		{
			name:     "zero samples",
			input:    []byte{0x00, 0x00, 0x00, 0x00},
			expected: []float32{0.0, 0.0},
			wantLen:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bytesToFloat32Array(tt.input)

			if len(result) != tt.wantLen {
				t.Errorf("bytesToFloat32Array() length = %d, want %d", len(result), tt.wantLen)
				return
			}

			for i, expected := range tt.expected {
				if i >= len(result) {
					t.Errorf("bytesToFloat32Array() missing sample at index %d", i)
					continue
				}

				// Use epsilon comparison for float values
				epsilon := float32(0.000001)
				if abs(result[i]-expected) > epsilon {
					t.Errorf("bytesToFloat32Array()[%d] = %f, want %f", i, result[i], expected)
				}
			}
		})
	}
}

func TestBytesToFloat32Array_Performance(t *testing.T) {
	// Test with large audio buffer (10 seconds at 16kHz)
	largeInput := make([]byte, 16000*10*2) // 10 seconds, 16-bit samples
	for i := 0; i < len(largeInput); i += 2 {
		largeInput[i] = byte(i % 256)
		largeInput[i+1] = byte((i / 256) % 256)
	}

	result := bytesToFloat32Array(largeInput)
	expectedLen := len(largeInput) / 2

	if len(result) != expectedLen {
		t.Errorf("Performance test: length = %d, want %d", len(result), expectedLen)
	}
}

func TestBytesToFloat32Array_EdgeCases(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := bytesToFloat32Array(nil)
		if len(result) != 0 {
			t.Errorf("nil input should return empty slice, got length %d", len(result))
		}
	})

	t.Run("single byte", func(t *testing.T) {
		result := bytesToFloat32Array([]byte{0xFF})
		if len(result) != 0 {
			t.Errorf("single byte should return empty slice, got length %d", len(result))
		}
	})

	t.Run("max positive value", func(t *testing.T) {
		input := []byte{0xFF, 0x7F} // 32767
		result := bytesToFloat32Array(input)
		expected := float32(1.0) // 32767 / 32767 = 1.0

		if len(result) != 1 {
			t.Errorf("Expected 1 sample, got %d", len(result))
			return
		}

		epsilon := float32(0.000001)
		if abs(result[0]-expected) > epsilon {
			t.Errorf("Max positive value: got %f, want %f", result[0], expected)
		}
	})

	t.Run("max negative value", func(t *testing.T) {
		input := []byte{0x00, 0x80} // -32768
		result := bytesToFloat32Array(input)
		expected := float32(-32768.0 / 32767.0) // Slightly less than -1.0

		if len(result) != 1 {
			t.Errorf("Expected 1 sample, got %d", len(result))
			return
		}

		epsilon := float32(0.000001)
		if abs(result[0]-expected) > epsilon {
			t.Errorf("Max negative value: got %f, want %f", result[0], expected)
		}
	})
}

// Helper function for float comparison
func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}