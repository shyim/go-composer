package composer

import (
	"github.com/shyim/go-composer/internal/testassert"
	"os"
	"path/filepath"
	"testing"
)

func TestReadComposerLock(t *testing.T) {
	t.Run("valid composer.lock", func(t *testing.T) {
		// Create a temporary composer.lock file
		dir := t.TempDir()
		lockFile := filepath.Join(dir, "composer.lock")
		content := `{
			"packages": [
				{
					"name": "symfony/console",
					"version": "v6.3.0",
					"type": "library"
				}
			]
		}`
		err := os.WriteFile(lockFile, []byte(content), 0o644)
		testassert.NoError(t, err)

		// Test reading the file
		lock, err := ReadLock(lockFile)
		testassert.NoError(t, err)
		testassert.NotNil(t, lock)
		testassert.Len(t, lock.Packages, 1)
		testassert.Equal(t, "symfony/console", lock.Packages[0].Name)
		testassert.Equal(t, "v6.3.0", lock.Packages[0].Version)
	})

	t.Run("non-existent file", func(t *testing.T) {
		lock, err := ReadLock("non-existent-file.lock")
		testassert.Error(t, err)
		testassert.Nil(t, lock)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		// Create a temporary file with invalid JSON
		dir := t.TempDir()
		lockFile := filepath.Join(dir, "invalid.lock")
		err := os.WriteFile(lockFile, []byte("invalid json"), 0o644)
		testassert.NoError(t, err)

		lock, err := ReadLock(lockFile)
		testassert.Error(t, err)
		testassert.Nil(t, lock)
	})
}
