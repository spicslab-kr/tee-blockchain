package auditor

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"
)

// Digest returns the hex SHA-256 of b.
func Digest(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// FileDigest returns the hex SHA-256 of a file.
func FileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func isHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// isDigest reports whether s is "sha256:" followed by 64 hex characters.
func isDigest(s string) bool {
	return strings.HasPrefix(s, "sha256:") && isHex(strings.TrimPrefix(s, "sha256:"), 64)
}
