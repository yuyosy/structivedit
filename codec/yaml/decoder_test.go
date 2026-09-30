package yaml

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/yuyosy/structivedit/codec"
	"github.com/yuyosy/structivedit/codec/conformance"
	"github.com/yuyosy/structivedit/document"
)

func TestCodecConformance(t *testing.T) {
	expected := conformanceDocument(t)
	conformance.Run(t, func() codec.Codec { return Codec{} }, conformance.Sample{
		Input:    []byte("name: editor\nenabled: true\nretries: 3\nratio: 1.5\nempty: null\nitems:\n  - alpha\n  - beta\nnested:\n  active: false\n"),
		Expected: expected,
	})
}

func conformanceDocument(t *testing.T) *document.Document {
	t.Helper()
	builder := document.NewBuilder()
	newScalar := func(value any) document.NodeID {
		t.Helper()
		id, err := builder.NewScalar(value)
		if err != nil {
			t.Fatalf("create conformance scalar: %v", err)
		}
		return id
	}
	newMapping := func(keys []string, values []document.NodeID) document.NodeID {
		t.Helper()
		entries := make([]document.MappingEntry, len(keys))
		for index, key := range keys {
			entries[index] = document.MappingEntry{Key: newScalar(key), Value: values[index]}
		}
		id, err := builder.NewMapping(entries)
		if err != nil {
			t.Fatalf("create conformance mapping: %v", err)
		}
		return id
	}
	items, err := builder.NewSequence([]document.NodeID{newScalar("alpha"), newScalar("beta")})
	if err != nil {
		t.Fatalf("create conformance sequence: %v", err)
	}
	nested := newMapping([]string{"active"}, []document.NodeID{newScalar(false)})
	root := newMapping(
		[]string{"name", "enabled", "retries", "ratio", "empty", "items", "nested"},
		[]document.NodeID{
			newScalar("editor"),
			newScalar(true),
			newScalar(int64(3)),
			newScalar(float64(1.5)),
			newScalar(nil),
			items,
			nested,
		},
	)
	if err := builder.SetRoot(root); err != nil {
		t.Fatalf("set conformance root: %v", err)
	}
	doc, err := builder.Build()
	if err != nil {
		t.Fatalf("build conformance Document: %v", err)
	}
	return doc
}

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
