package sailpkg

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const manifestName = ".sailpkg-files"

// ownFiles are FreeSailWallet sources. They are not produced from the
// SailWallet package and are not turned into patches.
var ownFiles = map[string]bool{
	"harbour-freesailwallet.qml":    true,
	"components/Contrast.js":        true,
	"pages/OriginalPackagePage.qml": true,
	"pages/FirstRunPage.qml":        true,
}

func installable(rel string) bool {
	return rel != entryQML
}

// PatchedQML is the plaintext QML that install writes: upstream files with
// FreeSailWallet patches applied, without the upstream application entry.
func PatchedQML() (map[string][]byte, error) {
	orig, err := ExtractQML()
	if err != nil {
		return nil, err
	}
	patches, err := loadPatches()
	if err != nil {
		return nil, err
	}
	return materialize(orig, patches)
}

func materialize(orig, patches map[string][]byte) (map[string][]byte, error) {
	out := map[string][]byte{}
	for rel, body := range orig {
		if !installable(rel) {
			continue
		}
		patched := body
		if p, ok := patches[rel]; ok {
			var err error
			patched, err = Apply(body, p)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", rel, err)
			}
		}
		out[rel] = patched
	}
	for rel := range patches {
		if _, ok := orig[rel]; !ok {
			return nil, fmt.Errorf("patch %s has no upstream file", rel)
		}
		if !installable(rel) {
			return nil, fmt.Errorf("patch %s targets %s", rel, entryQML)
		}
	}
	return out, nil
}

func loadPatches() (map[string][]byte, error) {
	out := map[string][]byte{}
	err := fs.WalkDir(patchFS, "patches", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".patch") {
			return nil
		}
		rel := strings.TrimPrefix(path, "patches/")
		rel = strings.TrimSuffix(rel, ".patch")
		body, err := patchFS.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = body
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// BuildPatches diffs qmlDir against the plaintext extract. Own files are
// ignored. A qml or js file that is neither ours nor in the package is an error.
func BuildPatches(qmlDir string) (map[string][]byte, error) {
	orig, err := ExtractQML()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	err = filepath.WalkDir(qmlDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(qmlDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		ext := strings.ToLower(filepath.Ext(rel))
		if ext != ".qml" && ext != ".js" {
			return nil
		}
		seen[rel] = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	for rel := range seen {
		if ownFiles[rel] {
			continue
		}
		if _, ok := orig[rel]; !ok {
			return nil, fmt.Errorf("%s is not part of the SailWallet package", rel)
		}
	}
	for rel := range orig {
		if !installable(rel) {
			continue
		}
		if !seen[rel] {
			return nil, fmt.Errorf("%s is missing from %s", rel, qmlDir)
		}
	}
	out := map[string][]byte{}
	var names []string
	for rel := range orig {
		if !installable(rel) {
			continue
		}
		names = append(names, rel)
	}
	sort.Strings(names)
	for _, rel := range names {
		body, err := os.ReadFile(filepath.Join(qmlDir, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		diff, err := Diff(rel, orig[rel], body)
		if err != nil {
			return nil, err
		}
		if len(diff) > 0 {
			out[rel] = diff
		}
	}
	return out, nil
}

// UpdatePatches rewrites the embedded patch directory from qmlDir.
func UpdatePatches(qmlDir, patchDir string) error {
	built, err := BuildPatches(qmlDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(patchDir, 0o755); err != nil {
		return err
	}
	err = filepath.WalkDir(patchDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasSuffix(path, ".patch") || base == ".keep" || base == "placeholder" {
			return os.Remove(path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	var names []string
	for rel := range built {
		names = append(names, rel)
	}
	sort.Strings(names)
	for _, rel := range names {
		dst := filepath.Join(patchDir, filepath.FromSlash(rel+".patch"))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, built[rel], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Install extracts plaintext QML, applies patches, and writes the result to dest.
// Files that are not part of that set, including FreeSailWallet's own QML, are
// left in place. The paths written are recorded in .sailpkg-files.
func Install(dest string) error {
	files, err := PatchedQML()
	if err != nil {
		return err
	}
	var names []string
	for rel := range files {
		names = append(names, rel)
	}
	sort.Strings(names)
	var listed []string
	for _, rel := range names {
		dst := filepath.Join(dest, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, files[rel], 0o644); err != nil {
			return err
		}
		listed = append(listed, rel)
	}
	manifest := strings.Join(listed, "\n")
	if manifest != "" {
		manifest += "\n"
	}
	return os.WriteFile(filepath.Join(dest, manifestName), []byte(manifest), 0o644)
}

// Checkout writes the patched QML into dest. A derived file that does not
// already match that result is left untouched unless force is set.
func Checkout(dest string, force bool) error {
	files, err := PatchedQML()
	if err != nil {
		return err
	}
	var names []string
	for rel := range files {
		names = append(names, rel)
	}
	sort.Strings(names)
	if !force {
		for _, rel := range names {
			dst := filepath.Join(dest, filepath.FromSlash(rel))
			have, err := os.ReadFile(dst)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return err
			}
			if !bytes.Equal(have, files[rel]) {
				return fmt.Errorf("%s has local edits; run patches or checkout --force", rel)
			}
		}
	}
	for _, rel := range names {
		dst := filepath.Join(dest, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, files[rel], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Prune removes upstream plaintext QML from dest so a package payload does not
// carry those files. FreeSailWallet's own files are kept.
func Prune(dest string) error {
	orig, err := ExtractQML()
	if err != nil {
		return err
	}
	var names []string
	for rel := range orig {
		if installable(rel) {
			names = append(names, rel)
		}
	}
	sort.Strings(names)
	for _, rel := range names {
		path := filepath.Join(dest, filepath.FromSlash(rel))
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// RepoRoot is the FreeSailWallet checkout that contains qml/ and go/sailpkg.
func RepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		qml := filepath.Join(dir, "qml")
		pkg := filepath.Join(dir, "go", "sailpkg")
		if isDir(qml) && isDir(pkg) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("run from the FreeSailWallet repository")
		}
		dir = parent
	}
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
