package testdb

import (
	"path/filepath"
	"testing"
)

func Path(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "test.db")
}
