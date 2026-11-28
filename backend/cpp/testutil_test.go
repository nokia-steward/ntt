package cpp_test

import (
	"os"
	"path/filepath"
)

func mkdirAll(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func writeBytes(path string, b []byte) error {
	return os.WriteFile(path, b, 0o644)
}
