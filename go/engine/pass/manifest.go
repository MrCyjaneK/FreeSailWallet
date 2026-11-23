package pass

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"strings"
)

func checkManifest(files map[string][]byte, manifest []byte) (bool, []string) {
	if len(manifest) == 0 {
		return false, []string{"manifest.json is missing"}
	}
	var listed map[string]string
	if err := json.Unmarshal(manifest, &listed); err != nil {
		return false, []string{"manifest.json is not a JSON object"}
	}
	var problems []string
	seen := map[string]bool{}
	for name, want := range listed {
		seen[name] = true
		got, ok := files[name]
		if !ok {
			problems = append(problems, "manifest entry missing from the archive: "+name)
			continue
		}
		sum := sha1.Sum(got)
		have := hex.EncodeToString(sum[:])
		if !strings.EqualFold(have, strings.TrimSpace(want)) {
			problems = append(problems, "manifest digest does not match "+name)
		}
	}
	for name := range files {
		if seen[name] || skipManifestName(name) {
			continue
		}
		problems = append(problems, "archive member is not in the manifest: "+name)
	}
	return len(problems) == 0, problems
}

func skipManifestName(name string) bool {
	if name == "manifest.json" || name == "signature" {
		return true
	}
	if name == ".DS_Store" || strings.HasSuffix(name, "/.DS_Store") {
		return true
	}
	if name == "__MACOSX" || strings.HasPrefix(name, "__MACOSX/") {
		return true
	}
	return false
}
