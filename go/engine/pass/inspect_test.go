package pass

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"harbour-freesailwallet/engine/internal/fixture"
	"harbour-freesailwallet/engine/trust"
)

func TestSyntheticPassTrustsLocalRoot(t *testing.T) {
	const (
		passType = "pass.example.ticket"
		team     = "TEAM123456"
	)
	m, err := fixture.NewMaterial(passType, team)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := fixture.Pack(map[string][]byte{
		"pass.json": fixture.PassJSON(passType, "100", team, "Ada"),
		"icon.png":  fixture.IconPNG(),
	}, m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Inspect(raw, Options{Roots: m.Pool})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Trusted {
		t.Fatalf("trusted=false problems=%v", got.Verification.Problems)
	}
	if got.Verification.Signer.PassTypeID != passType || got.Verification.Signer.TeamID != team {
		t.Fatalf("signer %+v", got.Verification.Signer)
	}
	if got.Verification.Signer.Root != "Test Root" {
		t.Fatalf("root %q", got.Verification.Signer.Root)
	}
	if got.PrimaryFields[0].Value != "Ada" {
		t.Fatalf("value %q", got.PrimaryFields[0].Value)
	}
	if got.BackgroundColor != "#0A141E" {
		t.Fatalf("color %s", got.BackgroundColor)
	}
	later, err := Inspect(raw, Options{Roots: m.Pool, Now: time.Now().Add(48 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if !later.Trusted || !later.Verification.SignatureOK || !later.Verification.ChainOK {
		t.Fatalf("expired certificate was untrusted: %+v", later.Verification)
	}
	soon, err := Inspect(raw, Options{Roots: m.Pool, Now: time.Now().Add(-2 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if soon.Trusted {
		t.Fatal("certificate that is not valid yet was trusted")
	}
	png, err := BarcodePNG(nil, 128)
	if err != nil || png != nil {
		t.Fatalf("empty barcode png=%d err=%v", len(png), err)
	}
}

func TestAppleRootsParse(t *testing.T) {
	pool, err := trust.AppleRoots()
	if err != nil {
		t.Fatal(err)
	}
	if pool == nil {
		t.Fatal("nil pool")
	}
}

func TestParseTestPasses(t *testing.T) {
	dir := testPassesDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var parsed, signed, chained int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".pkpass") {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			got, err := Inspect(b, Options{})
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if !got.Readable {
				t.Fatal("not readable")
			}
			if got.PassTypeIdentifier == "" || got.SerialNumber == "" {
				t.Fatal("missing identity")
			}
			if got.Style == "" {
				t.Fatal("missing style")
			}
			parsed++
			if got.Verification.SignatureOK {
				signed++
			}
			if got.Verification.ChainOK {
				chained++
			}
			if e.Name() == "archipelago-expired.pkpass" && got.Verification.SignatureOK {
				t.Fatal("unsigned fixture verified")
			}
		})
	}
	if parsed == 0 {
		t.Fatal("no pkpass files in ./test_passes")
	}
	t.Logf("parsed %d signatureOK %d chainOK %d", parsed, signed, chained)
	if signed < parsed-1 {
		t.Fatalf("expected real passes to verify, signed %d of %d", signed, parsed)
	}
}

func testPassesDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for i := 0; i < 6; i++ {
		for _, candidate := range []string{
			filepath.Join(dir, "test_passes"),
			filepath.Join(dir, "pkwallet", "test_passes"),
		} {
			st, err := os.Stat(candidate)
			if err == nil && st.IsDir() {
				return candidate
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("test_passes directory not found; expected ./test_passes")
	return ""
}
