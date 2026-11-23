package wallet

import (
	"archive/zip"
	"bytes"
	"strings"

	"harbour-freesailwallet/engine/pass"
)

func isBackup(data []byte) bool {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	for _, f := range r.File {
		if f.Name == "wallet.json" {
			return true
		}
	}
	return false
}

func looksLikePkpasses(data []byte) bool {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	for _, f := range r.File {
		if f.Name == "pass.json" {
			return false
		}
	}
	return true
}

// bundleParts returns each pkpass inside data. A single pass is one part.
func bundleParts(data []byte) ([][]byte, error) {
	entries, err := pass.OpenZip(data)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Name == "pass.json" {
			return [][]byte{data}, nil
		}
	}
	var parts [][]byte
	for _, entry := range entries {
		if !strings.HasSuffix(strings.ToLower(entry.Name), ".pkpass") {
			continue
		}
		parts = append(parts, entry.Body)
	}
	if len(parts) == 0 {
		return nil, errNotPass
	}
	return parts, nil
}
