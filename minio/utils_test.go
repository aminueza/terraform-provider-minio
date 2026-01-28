package minio

import (
	"math"
	"testing"
)

func TestSafeUint64ToInt64(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected int64
		ok       bool
	}{
		{
			name:     "zero value",
			input:    0,
			expected: 0,
			ok:       true,
		},
		{
			name:     "small value",
			input:    1024,
			expected: 1024,
			ok:       true,
		},
		{
			name:     "typical disk space (1TB)",
			input:    1099511627776,
			expected: 1099511627776,
			ok:       true,
		},
		{
			name:     "large disk space (1PB)",
			input:    1125899906842624,
			expected: 1125899906842624,
			ok:       true,
		},
		{
			name:     "max int64 value",
			input:    math.MaxInt64,
			expected: math.MaxInt64,
			ok:       true,
		},
		{
			name:     "max int64 + 1 (overflow)",
			input:    uint64(math.MaxInt64) + 1,
			expected: math.MaxInt64,
			ok:       false,
		},
		{
			name:     "max uint64 value (overflow)",
			input:    math.MaxUint64,
			expected: math.MaxInt64,
			ok:       false,
		},
		{
			name:     "10 exabytes (overflow)",
			input:    10000000000000000000,
			expected: math.MaxInt64,
			ok:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := SafeUint64ToInt64(tt.input)

			if ok != tt.ok {
				t.Errorf("SafeUint64ToInt64(%d) ok = %v, want %v", tt.input, ok, tt.ok)
			}

			if result != tt.expected {
				t.Errorf("SafeUint64ToInt64(%d) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSafeUint64ToInt64_NoNegative(t *testing.T) {
	// Ensure we never return negative values
	testValues := []uint64{
		0,
		1,
		math.MaxInt64,
		math.MaxInt64 + 1,
		math.MaxUint64,
	}

	for _, val := range testValues {
		result, _ := SafeUint64ToInt64(val)
		if result < 0 {
			t.Errorf("SafeUint64ToInt64(%d) returned negative value: %d", val, result)
		}
	}
}

func BenchmarkSafeUint64ToInt64(b *testing.B) {
	testValues := []uint64{
		1024,
		1099511627776, // 1TB
		math.MaxInt64,
		math.MaxUint64,
	}

	for _, val := range testValues {
		b.Run("", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				SafeUint64ToInt64(val)
			}
		})
	}
}
