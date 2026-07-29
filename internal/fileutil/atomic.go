package fileutil

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func WriteAtomic(path string, payload []byte, mode fs.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)

	if err := writeFile(file, payload, mode); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func WriteNew(path string, payload []byte, mode fs.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			os.Remove(path)
		}
	}()
	if err := writeFile(file, payload, mode); err != nil {
		return err
	}
	keep = true
	return nil
}

func Backup(path string, payload []byte, suffix string) (string, error) {
	backup := path + suffix
	if err := WriteNew(backup, payload, 0o600); err != nil {
		return "", fmt.Errorf("write backup: %w", err)
	}
	return backup, nil
}

func writeFile(file *os.File, payload []byte, mode fs.FileMode) (err error) {
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()
	if err := file.Chmod(mode); err != nil {
		return err
	}
	if _, err := file.Write(payload); err != nil {
		return err
	}
	return file.Sync()
}
