// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package crypt

import (
	"bytes"
	"testing"
)

func TestChecksumConsistency(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected uint32
		wantErr  bool
	}{
		{
			name:     "empty data",
			data:     []byte{},
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "single byte",
			data:     []byte{1},
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "small data",
			data:     []byte{1, 2, 3, 4, 5},
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "multiple of 4",
			data:     []byte{1, 2, 3, 4, 5, 6, 7, 8},
			expected: 67372044,
			wantErr:  false,
		},
		{
			name:     "large data",
			data:     bytes.Repeat([]byte{255}, 1024),
			expected: 0x00000000,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Checksum(tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("Checksum(%q) error = %v, wantErr %v", tt.data, err, tt.wantErr)

				return
			}

			if result != tt.expected {
				t.Errorf("Checksum(%q) = %d, want %d", tt.data, result, tt.expected)
			}
		})
	}
}
