package yaml

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestDecodeWithOptionsEnforcesLimits(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		options DecodeOptions
		wantErr error
	}{
		{
			name:    "input bytes",
			input:   "key: value\n",
			options: DecodeOptions{MaxInputBytes: 4},
			wantErr: ErrLimitExceeded,
		},
		{
			name:    "node count",
			input:   "key: value\n",
			options: DecodeOptions{MaxNodes: 2},
			wantErr: ErrLimitExceeded,
		},
		{
			name:    "nesting depth",
			input:   "root:\n  child: value\n",
			options: DecodeOptions{MaxDepth: 2},
			wantErr: ErrLimitExceeded,
		},
		{
			name:    "negative limit",
			input:   "key: value\n",
			options: DecodeOptions{MaxNodes: -1},
			wantErr: ErrInvalidDecodeOptions,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeWithOptions(strings.NewReader(test.input), test.options)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("DecodeWithOptions error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func BenchmarkDecodeLargeMapping(b *testing.B) {
	var input bytes.Buffer
	for index := 0; index < 2048; index++ {
		fmt.Fprintf(&input, "key-%d: value-%d\n", index, index)
	}
	data := append([]byte(nil), input.Bytes()...)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := Decode(bytes.NewReader(data)); err != nil {
			b.Fatal(err)
		}
	}
}
