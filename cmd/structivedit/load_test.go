package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"

	yamlcodec "github.com/yuyosy/structivedit/codec/yaml"
)

func TestLoadAndConflictCheckRespectByteLimits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	input := []byte("key: value\n")
	if err := os.WriteFile(path, input, 0600); err != nil {
		t.Fatal(err)
	}
	options := yamlcodec.DecodeOptions{MaxInputBytes: 4}
	if _, _, _, err := loadFileWithOptions(path, options); !errors.Is(err, yamlcodec.ErrLimitExceeded) {
		t.Fatalf("load limit: %v", err)
	}
	if changed, err := fileChangedWithLimit(path, sha256.Sum256(input), 4); err != nil || !changed {
		t.Fatalf("oversized conflict: %v %v", changed, err)
	}
	options.MaxInputBytes = int64(len(input))
	if _, _, hash, err := loadFileWithOptions(path, options); err != nil || hash != sha256.Sum256(input) {
		t.Fatalf("boundary load: %x %v", hash, err)
	}
}

func TestCLIRejectsInvalidResourceLimits(t *testing.T) {
	err := run([]string{"--max-depth=-1", "unused.yaml"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if !errors.Is(err, yamlcodec.ErrInvalidDecodeOptions) {
		t.Fatalf("negative limit: %v", err)
	}
}
