package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
)

func writeFileAtomically(path string, data []byte, mode os.FileMode) error {
	return atomicWriteFile(path, data, mode, replaceFile)
}

func atomicWriteFile(path string, data []byte, mode os.FileMode, replace func(string, string) error) error {
	// Replace the target of an existing symlink, rather than the link itself.
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return err // do not replace a dangling symlink
		}
		target = path
	}
	file, err := os.CreateTemp(filepath.Dir(target), ".structivedit-save-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	defer file.Close()
	if err := prepareSavePermissions(file, target, mode); err != nil {
		return err
	}
	if _, err := io.Copy(file, bytes.NewReader(data)); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return replace(name, target)
}
