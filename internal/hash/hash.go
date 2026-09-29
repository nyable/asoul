package hash

import (
	"fmt"

	"golang.org/x/mod/sumdb/dirhash"
)

// HashDir computes the deterministic h1 content hash for a directory using dirhash.Hash1.
func HashDir(dirPath string) (string, error) {
	h, err := dirhash.HashDir(dirPath, "", dirhash.Hash1)
	if err != nil {
		return "", fmt.Errorf("failed to compute directory hash for %s: %w", dirPath, err)
	}
	return h, nil
}
