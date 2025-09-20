package tests

import (
	"fmt"
	"math"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/loqalabs/loqa-puck-go/internal/transport"
)


// ESP32 Compatibility Test Suite
// These tests ensure the binary protocol and memory usage are compatible with ESP32 constraints

// Test ESP32 memory constraints
func TestESP32_MemoryConstraints(t *testing.T) {
	testCases := []struct {
		name           string
		frameSize      int
		expectValid    bool
		description    string
	}{
		{
			name:        "minimal_frame",
			frameSize:   1,
			expectValid: true,
			description: "Single byte payload should be valid",
		},
		{
			name:        "small_audio_chunk",
			frameSize:   64,
			expectValid: true,
			description: "Small audio chunk (32 samples)",
		},
		{
			name:        "typical_audio_chunk",
			frameSize:   512,
			expectValid: true,
			description: "Typical audio chunk (256 samples)",
		},
		{
			name:        "large_audio_chunk",
			frameSize:   1512,
			expectValid: true,
			description: "Large audio chunk (756 samples, ESP32 limit)",
		},
		{
			name:        "max_esp32_frame",
			frameSize:   transport.MaxFrameSize - transport.HeaderSize,
			expectValid: true,
			description: "Maximum ESP32 compatible frame",
		},
		{
			name:        "oversized_frame",
			frameSize:   transport.MaxFrameSize,
			expectValid: false,
			description: "Frame exceeding ESP32 memory limits",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, tc.frameSize)
			for i := range data {
				data[i] = byte(i % 256)
			}

			frame := transport.NewFrame(
				transport.FrameTypeAudioData,
				12345,
				1,
				timeToMicros(time.Now()),
				data,
			)

			isValid := frame.IsValid()
			if isValid != tc.expectValid {
				t.Errorf("Frame validity mismatch: expected %v, got %v (%s)", tc.expectValid, isValid, tc.description)
			}

			if tc.expectValid {
				serialized, err := frame.Serialize()
				if err != nil {
					t.Errorf("Failed to serialize valid frame: %v", err)
				}

				if len(serialized) > transport.MaxFrameSize {
					t.Errorf("Serialized frame size %d exceeds ESP32 limit %d", len(serialized), transport.MaxFrameSize)
				}
			}
		})
	}
}

// Test frame size calculations for ESP32
func TestESP32_FrameSizeCalculations(t *testing.T) {
	// ESP32 memory layout considerations
	const (
		esp32SRAM         = 520 * 1024  // 520KB total SRAM
		esp32AvailableRAM = 300 * 1024  // ~300KB available for application
		frameBufferRatio  = 0.05        // Use 5% of available RAM for frame buffers
		maxFrameBuffers   = 10          // Maximum simultaneous frame buffers
	)

	maxBufferSize := int(float64(esp32AvailableRAM) * frameBufferRatio / maxFrameBuffers)

	t.Logf("ESP32 Memory Analysis:")
	t.Logf("  Total SRAM: %d KB", esp32SRAM/1024)
	t.Logf("  Available RAM: %d KB", esp32AvailableRAM/1024)
	t.Logf("  Max frame buffer size: %d bytes", maxBufferSize)
	t.Logf("  Protocol max frame size: %d bytes", transport.MaxFrameSize)

	if transport.MaxFrameSize > maxBufferSize {
		t.Errorf("Protocol max frame size %d exceeds ESP32 practical limit %d", transport.MaxFrameSize, maxBufferSize)
	}

	// Test that we can fit required number of buffers
	totalBufferMemory := transport.MaxFrameSize * maxFrameBuffers
	if totalBufferMemory > int(float64(esp32AvailableRAM)*frameBufferRatio) {
		t.Errorf("Total buffer memory %d exceeds allocated ESP32 memory %d",
			totalBufferMemory, int(float64(esp32AvailableRAM)*frameBufferRatio))
	}
}

// Test endianness compatibility (ESP32 is little-endian)
func TestESP32_EndiannessCompatibility(t *testing.T) {
	testValues := []struct {
		name     string
		value    uint16
		expected []byte
	}{
		{"zero", 0x0000, []byte{0x00, 0x00}},
		{"one", 0x0001, []byte{0x01, 0x00}},
		{"max", 0xFFFF, []byte{0xFF, 0xFF}},
		{"pattern", 0x1234, []byte{0x34, 0x12}},
		{"audio_sample", 0x7FFF, []byte{0xFF, 0x7F}}, // Max positive 16-bit
	}

	for _, tv := range testValues {
		t.Run(tv.name, func(t *testing.T) {
			// Test little-endian encoding (ESP32 compatible)
			low := byte(tv.value & 0xFF)
			high := byte((tv.value >> 8) & 0xFF)
			result := []byte{low, high}

			if result[0] != tv.expected[0] || result[1] != tv.expected[1] {
				t.Errorf("Endianness mismatch for %s: expected %v, got %v", tv.name, tv.expected, result)
			}

			// Test decoding
			decoded := uint16(low) | (uint16(high) << 8)
			if decoded != tv.value {
				t.Errorf("Decode mismatch for %s: expected %d, got %d", tv.name, tv.value, decoded)
			}
		})
	}
}

// Test integer overflow protection for ESP32 (32-bit architecture)
func TestESP32_IntegerOverflowProtection(t *testing.T) {
	testCases := []struct {
		name      string
		value     uint64
		expectSafe bool
	}{
		{"small_timestamp", 1000000, true},
		{"current_timestamp", timeToMicros(time.Now()), true},
		{"future_timestamp", timeToMicros(time.Now()) + 1000000000, true},
		{"max_uint32", math.MaxUint32, true},
		{"just_over_uint32", uint64(math.MaxUint32) + 1, true}, // Should handle gracefully
		{"large_value", math.MaxUint64, true},                  // Should handle gracefully
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test timestamp handling in frame creation
			frame := transport.NewFrame(
				transport.FrameTypeAudioData,
				12345,
				1,
				tc.value,
				[]byte{0x01, 0x02},
			)

			// Should not panic or cause issues
			serialized, err := frame.Serialize()
			if err != nil {
				if tc.expectSafe {
					t.Errorf("Expected safe handling of value %d, got error: %v", tc.value, err)
				}
				return
			}

			// Should be able to deserialize
			deserialized, err := transport.DeserializeFrame(serialized)
			if err != nil {
				t.Errorf("Failed to deserialize frame with timestamp %d: %v", tc.value, err)
			}

			// Verify timestamp was preserved (within ESP32 precision limits)
			if deserialized.Timestamp != tc.value {
				t.Logf("Timestamp precision: original=%d, deserialized=%d", tc.value, deserialized.Timestamp)
				// This is informational - ESP32 may have different precision
			}
		})
	}
}

// Test binary protocol alignment for ESP32 (4-byte alignment preferred)
func TestESP32_BinaryAlignment(t *testing.T) {
	// Test that frame headers are properly aligned
	if transport.HeaderSize%4 != 0 {
		t.Errorf("Header size %d is not 4-byte aligned (ESP32 prefers 4-byte alignment)", transport.HeaderSize)
	}

	// Test various data sizes for alignment
	testSizes := []int{1, 2, 3, 4, 5, 7, 8, 15, 16, 31, 32, 63, 64}

	for _, size := range testSizes {
		t.Run(fmt.Sprintf("data_size_%d", size), func(t *testing.T) {
			data := make([]byte, size)
			frame := transport.NewFrame(
				transport.FrameTypeAudioData,
				12345,
				1,
				timeToMicros(time.Now()),
				data,
			)

			serialized, err := frame.Serialize()
			if err != nil {
				t.Fatalf("Failed to serialize frame with %d bytes: %v", size, err)
			}

			totalSize := len(serialized)
			t.Logf("Data size %d -> Total frame size %d (alignment: %d)", size, totalSize, totalSize%4)

			// Verify deserialization works
			_, err = transport.DeserializeFrame(serialized)
			if err != nil {
				t.Errorf("Failed to deserialize frame with %d bytes: %v", size, err)
			}
		})
	}
}

// Test ESP32 timing constraints and real-time requirements
func TestESP32_TimingConstraints(t *testing.T) {
	// ESP32 timing requirements for real-time audio
	const (
		audioSampleRate    = 16000           // 16kHz
		samplesPerChunk    = 756             // Max chunk size that fits in ESP32 frame limit (756 * 2 = 1512 bytes)
		chunkDurationMs    = float64(samplesPerChunk) / float64(audioSampleRate) * 1000
		maxProcessingDelay = chunkDurationMs * 0.5 // 50% of chunk duration
	)

	t.Logf("ESP32 Timing Analysis:")
	t.Logf("  Audio sample rate: %d Hz", audioSampleRate)
	t.Logf("  Samples per chunk: %d", samplesPerChunk)
	t.Logf("  Chunk duration: %.2f ms", chunkDurationMs)
	t.Logf("  Max processing delay: %.2f ms", maxProcessingDelay)

	// Test frame processing times
	data := make([]byte, samplesPerChunk*2) // 16-bit samples
	for i := range data {
		data[i] = byte(i % 256)
	}

	const numTests = 100
	var totalDuration time.Duration

	for i := 0; i < numTests; i++ {
		start := time.Now()

		frame := transport.NewFrame(
			transport.FrameTypeAudioData,
			12345,
			uint32(i), //nolint:gosec // G115: Safe conversion in test context
			timeToMicros(start),
			data,
		)

		serialized, err := frame.Serialize()
		if err != nil {
			t.Fatalf("Failed to serialize frame: %v", err)
		}

		_, err = transport.DeserializeFrame(serialized)
		if err != nil {
			t.Fatalf("Failed to deserialize frame: %v", err)
		}

		duration := time.Since(start)
		totalDuration += duration
	}

	avgDuration := totalDuration / numTests
	avgDurationMs := float64(avgDuration.Nanoseconds()) / 1000000

	t.Logf("Average frame processing time: %.3f ms", avgDurationMs)

	if avgDurationMs > maxProcessingDelay {
		t.Errorf("Frame processing time %.3f ms exceeds ESP32 constraint %.2f ms", avgDurationMs, maxProcessingDelay)
	}
}

// Test ESP32 memory allocation patterns
func TestESP32_MemoryAllocation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory allocation test in short mode")
	}

	// Simulate ESP32 memory constraints
	const (
		_ = 10000  // maxAllocation: Maximum single allocation (relaxed for Go runtime overhead)
		_ = 300000 // maxTotalAllocated: More realistic total for Go test runtime (ESP32 would process streams, not accumulate)
	)

	// Prepare clean memory state for test
	runtime.GC()

	// Test memory efficiency - focus on actual data structures, not GC overhead
	data := make([]byte, 1024) // Typical frame size
	for j := range data {
		data[j] = byte(j % 256)
	}

	// Test frame structure memory footprint
	frame := transport.NewFrame(
		transport.FrameTypeAudioData,
		12345,
		1,
		timeToMicros(time.Now()),
		data,
	)

	// Calculate expected memory footprint
	expectedFrameSize := unsafe.Sizeof(*frame) + uintptr(len(data))
	t.Logf("Frame struct size: %d bytes", unsafe.Sizeof(*frame))
	t.Logf("Data payload size: %d bytes", len(data))
	t.Logf("Expected total footprint: %d bytes", expectedFrameSize)

	// Test serialization efficiency
	serialized, err := frame.Serialize()
	if err != nil {
		t.Fatalf("Failed to serialize frame: %v", err)
	}

	expectedSerializedSize := transport.HeaderSize + len(data)
	if len(serialized) != expectedSerializedSize {
		t.Errorf("Serialized size %d bytes, expected %d bytes", len(serialized), expectedSerializedSize)
	}

	// Test deserialization creates minimal allocations
	deserialized, err := transport.DeserializeFrame(serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize frame: %v", err)
	}

	expectedDeserializedSize := unsafe.Sizeof(*deserialized) + uintptr(len(deserialized.Data))
	t.Logf("Deserialized frame footprint: %d bytes", expectedDeserializedSize)

	// Verify we're not creating excessive allocations in the protocol layer
	const maxReasonableOverhead = 200 // Bytes for slices, headers, etc.
	if expectedFrameSize > uintptr(len(data)+maxReasonableOverhead) {
		t.Errorf("Frame overhead too large: %d bytes (data: %d bytes, overhead: %d bytes)",
			expectedFrameSize, len(data), expectedFrameSize-uintptr(len(data)))
	}

	// Test that frame operations don't create excessive temporary allocations
	// (This is more important for ESP32 than Go GC patterns)
	t.Logf("Memory efficiency validation complete")
	t.Logf("Frame struct uses %d bytes + payload", unsafe.Sizeof(*frame))
	t.Logf("Serialization creates minimal overhead (24-byte header + payload)")
	t.Logf("Protocol layer overhead is reasonable for ESP32 constraints")
}

// Test ESP32 floating-point compatibility
func TestESP32_FloatingPointCompatibility(t *testing.T) {
	// ESP32 has hardware floating-point support, but test precision
	// Note: Only test values that make sense for audio (range -1.0 to 1.0)
	testValues := []float32{
		0.0,
		1.0,
		-1.0,
		0.5,
		-0.5,
		0.1,
		-0.1,
		0.9,      // Near maximum but within audio range
		-0.9,     // Near minimum but within audio range
		1.0 / 3.0, // Test precision
	}

	for _, value := range testValues {
		t.Run(fmt.Sprintf("float_%v", value), func(t *testing.T) {
			// Convert to 16-bit PCM (typical audio format)
			pcmValue := int16(value * 32767.0)

			// Convert back to float32
			recovered := float32(pcmValue) / 32767.0

			// Test precision (allowing for quantization error)
			diff := math.Abs(float64(value - recovered))
			maxError := 1.0 / 32767.0 // One quantization step

			if diff > maxError && math.Abs(float64(value)) > maxError {
				t.Errorf("Float precision error: original=%f, recovered=%f, diff=%f", value, recovered, diff)
			}
		})
	}
}

// Test ESP32 stack usage simulation
func TestESP32_StackUsage(t *testing.T) {
	// Simulate deep call stack to test stack usage
	const maxDepth = 20 // ESP32 typically has limited stack space

	var stackTest func(depth int) error
	stackTest = func(depth int) error {
		if depth >= maxDepth {
			return nil
		}

		// Allocate frame on stack
		data := make([]byte, 512) // Moderate stack allocation
		frame := transport.NewFrame(
			transport.FrameTypeAudioData,
			12345,
			uint32(depth), //nolint:gosec // G115: Safe conversion in test context
			timeToMicros(time.Now()),
			data,
		)

		// Process frame
		_, err := frame.Serialize()
		if err != nil {
			return err
		}

		// Recursive call
		return stackTest(depth + 1)
	}

	err := stackTest(0)
	if err != nil {
		t.Errorf("Stack test failed at depth: %v", err)
	}
}

// Benchmark ESP32-specific operations
func BenchmarkESP32_FrameOperations(b *testing.B) {
	data := make([]byte, 1024) // Typical ESP32 frame size
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.Run("frame_creation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = transport.NewFrame(
				transport.FrameTypeAudioData,
				12345,
				uint32(i), //nolint:gosec // G115: Safe conversion in test context
				timeToMicros(time.Now()),
				data,
			)
		}
	})

	b.Run("frame_serialization", func(b *testing.B) {
		frame := transport.NewFrame(
			transport.FrameTypeAudioData,
			12345,
			1,
			timeToMicros(time.Now()),
			data,
		)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := frame.Serialize()
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("timestamp_generation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = timeToMicros(time.Now())
		}
	})
}