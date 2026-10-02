//go:build !windows

package main

import "os"

func prepareSavePermissions(file *os.File, _ string, mode os.FileMode) error {
	return file.Chmod(mode.Perm())
}

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}
