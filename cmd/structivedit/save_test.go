package main

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAtomicSaveFailurePreservesOriginal(t *testing.T) {
	t.Run("original path", func(t *testing.T) {
		testAtomicSaveFailurePreservesOriginal(t, false)
	})
	if runtime.GOOS == "windows" {
		t.Run("case variant path", func(t *testing.T) {
			testAtomicSaveFailurePreservesOriginal(t, true)
		})
	}
}

func testAtomicSaveFailurePreservesOriginal(t *testing.T, caseVariant bool) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if caseVariant {
		path = strings.ToUpper(path)
	}
	original, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("replacement failed")
	err = atomicWriteFile(path, []byte("new\n"), 0600, func(from, to string) error {
		content, err := os.ReadFile(from)
		if err != nil || string(content) != "new\n" {
			t.Fatalf("prepared content: %q, error: %v", content, err)
		}
		// EvalSymlinks can normalize Windows casing and short path names.
		// Verify file identity rather than requiring identical path strings.
		destination, err := os.Stat(to)
		if err != nil || !os.SameFile(original, destination) {
			t.Fatalf("prepared destination %q differs from %q: %v", to, path, err)
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("save error: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "old\n" {
		t.Fatalf("original: %q %v", content, err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("temporary files leaked: %v %v", files, err)
	}
}

func TestAtomicSaveAndExternalChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("old\n"), 0640); err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256([]byte("old\n"))
	if changed, err := fileChanged(path, expected); err != nil || changed {
		t.Fatalf("unchanged: %v %v", changed, err)
	}
	if err := writeFileAtomically(path, []byte("new\n"), 0640); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "new\n" {
		t.Fatalf("saved: %q %v", content, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0640 {
			t.Fatalf("permissions: %v %v", info, err)
		}
	}
	if changed, err := fileChanged(path, expected); err != nil || !changed {
		t.Fatalf("changed: %v %v", changed, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if changed, err := fileChanged(path, expected); err != nil || !changed {
		t.Fatalf("missing: %v %v", changed, err)
	}
}

func TestAtomicSavePreservesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.yaml")
	link := filepath.Join(dir, "link.yaml")
	if err := os.WriteFile(target, []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := writeFileAtomically(link, []byte("new\n"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink replaced: %v %v", info, err)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "new\n" {
		t.Fatalf("target: %q %v", content, err)
	}
}
