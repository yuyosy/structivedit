package yaml

import (
	"bytes"
	"testing"
)

func FuzzDecodeEncode(f *testing.F) {
	for _, seed := range []string{"key: value\n", "- &a {x: 1}\n- *a\n", "a: 1\na: 2\n", "text: |\n  hello\n", "!custom [1, 2]\n", ".nan\n"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		session, err := DecodeWithOptions(bytes.NewReader(data), DecodeOptions{MaxInputBytes: 256 << 10, MaxNodes: 4096, MaxDepth: 64})
		if err != nil {
			return
		}
		var output bytes.Buffer
		if err := session.Encode(&output, session.Document()); err != nil {
			t.Fatalf("accepted input cannot be encoded: %v", err)
		}
		if _, err := DecodeWithOptions(&output, DecodeOptions{MaxInputBytes: 1 << 20, MaxNodes: 4096, MaxDepth: 64}); err != nil {
			t.Fatalf("encoded output cannot be decoded: %v", err)
		}
	})
}
