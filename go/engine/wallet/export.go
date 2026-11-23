package wallet

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"harbour-freesailwallet/engine/pass"
)

var errNotPass = errors.New("not a pkpass or pkpasses archive")

// ExportResult is the zip written for "Export all".
type ExportResult struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

// Export writes a backup zip of active passes, archived passes, history, and wallet.json.
func (s *Store) Export() (*ExportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Join(s.dir, "exports"), 0o755); err != nil {
		return nil, err
	}
	name := "wallet-" + s.now().UTC().Format("20060102T150405Z") + ".zip"
	dest := filepath.Join(s.dir, "exports", name)
	f, err := os.Create(dest)
	if err != nil {
		return nil, err
	}
	zw := zip.NewWriter(f)
	count := 0
	exported := disk{LastPoll: s.state.LastPoll, Passes: map[string]*record{}}
	for id, rec := range s.state.Passes {
		copyRec := *rec
		copyRec.BundlePath = ""
		folder := "passes"
		if rec.ArchivedAt != nil {
			folder = "archive"
		}
		fileName := safe(rec.PassTypeIdentifier+"-"+rec.SerialNumber) + ".pkpass"
		zipPath := folder + "/" + fileName
		cur, err := s.currentPath(id)
		if err != nil {
			zw.Close()
			f.Close()
			return nil, err
		}
		if err := addFile(zw, zipPath, cur); err != nil {
			zw.Close()
			f.Close()
			return nil, err
		}
		copyRec.BundlePath = zipPath
		count++
		for i := range copyRec.Revisions {
			rel := copyRec.Revisions[i].File
			if rel == "" {
				continue
			}
			zipRel := "history/" + id + "/" + filepath.Base(rel)
			src, err := s.passFile(id, rel)
			if err != nil {
				zw.Close()
				f.Close()
				return nil, err
			}
			if err := addFile(zw, zipRel, src); err != nil {
				zw.Close()
				f.Close()
				return nil, err
			}
			copyRec.Revisions[i].File = zipRel
		}
		exported.Passes[id] = &copyRec
	}
	raw, err := json.MarshalIndent(exported, "", "  ")
	if err != nil {
		zw.Close()
		f.Close()
		return nil, err
	}
	w, err := zw.Create("wallet.json")
	if err != nil {
		zw.Close()
		f.Close()
		return nil, err
	}
	if _, err := w.Write(raw); err != nil {
		zw.Close()
		f.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return &ExportResult{Path: dest, Count: count}, nil
}

func addFile(zw *zip.Writer, name, src string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func (s *Store) restore(data []byte) (*ImportResult, error) {
	entries, err := pass.OpenZip(data)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, entry := range entries {
		files[entry.Name] = entry.Body
	}
	var incoming disk
	if err := json.Unmarshal(files["wallet.json"], &incoming); err != nil {
		return nil, fmt.Errorf("wallet.json: %w", err)
	}
	if incoming.Passes == nil {
		incoming.Passes = map[string]*record{}
	}
	res := &ImportResult{Restored: true}
	for id, rec := range incoming.Passes {
		if rec == nil || rec.BundlePath == "" {
			res.Problems = append(res.Problems, id+": backup entry has no pass file")
			continue
		}
		body, ok := files[rec.BundlePath]
		if !ok {
			res.Problems = append(res.Problems, rec.BundlePath+" is missing from the backup")
			continue
		}
		dir, err := s.passDir(id)
		if err != nil {
			res.Problems = append(res.Problems, id+": invalid pass id")
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		cur, err := s.currentPath(id)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(cur, body, 0o644); err != nil {
			return nil, err
		}
		for i := range rec.Revisions {
			rel := rec.Revisions[i].File
			if rel == "" {
				continue
			}
			hist, ok := files[rel]
			if !ok {
				res.Problems = append(res.Problems, rel+" is missing from the backup")
				rec.Revisions[i].File = ""
				continue
			}
			base := filepath.Base(rel)
			local := "history/" + base
			dest, err := s.passFile(id, local)
			if err != nil {
				res.Problems = append(res.Problems, rel+": invalid history path")
				rec.Revisions[i].File = ""
				continue
			}
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(dest, hist, 0o644); err != nil {
				return nil, err
			}
			rec.Revisions[i].File = local
		}
		rec.BundlePath = ""
		s.state.Passes[id] = rec
		p, err := s.inspectCurrent(id)
		if err != nil {
			res.Problems = append(res.Problems, id+": "+err.Error())
			continue
		}
		res.Passes = append(res.Passes, s.summary(id, p, rec))
	}
	if !incoming.LastPoll.IsZero() {
		s.state.LastPoll = incoming.LastPoll
	}
	if err := s.save(); err != nil {
		return nil, err
	}
	return res, nil
}
