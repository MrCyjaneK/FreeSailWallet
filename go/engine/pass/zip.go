package pass

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
)

const (
	// MaxEntryBytes is the largest accepted member of a pass archive.
	MaxEntryBytes int64 = 32 << 20
	// MaxEntries is the most files an archive may contain.
	MaxEntries = 256
	// MaxTotalBytes is the most uncompressed bytes an archive may contain.
	MaxTotalBytes int64 = 128 << 20
)

// Entry is one file from a pass archive.
type Entry struct {
	Name string
	Body []byte
}

// OpenZip reads a pkpass-style zip. Names that leave the archive are skipped.
// A member longer than MaxEntryBytes, or an archive past the file or byte
// caps, is an error rather than a truncated file.
func OpenZip(data []byte) ([]Entry, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("not a pkpass zip: %w", err)
	}
	var entries []Entry
	seen := map[string]bool{}
	var total int64
	n := 0
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		n++
		if n > MaxEntries {
			return nil, fmt.Errorf("archive has too many files")
		}
		name, ok := cleanZipName(f.Name)
		if !ok {
			continue
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate archive member %s", name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		body, err := ReadLimited(rc, MaxEntryBytes)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		total += int64(len(body))
		if total > MaxTotalBytes {
			return nil, fmt.Errorf("archive is too large")
		}
		seen[name] = true
		entries = append(entries, Entry{Name: name, Body: body})
	}
	return entries, nil
}

// ReadLimited reads r up to limit bytes. A longer stream is an error.
func ReadLimited(r io.Reader, limit int64) ([]byte, error) {
	if limit < 0 {
		return nil, fmt.Errorf("invalid read limit")
	}
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	return b, nil
}
