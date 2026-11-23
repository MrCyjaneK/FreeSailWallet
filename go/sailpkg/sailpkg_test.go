package sailpkg

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestRPMSum(t *testing.T) {
	sum := sha256.Sum256(RPM())
	if hex.EncodeToString(sum[:]) != RPMSum {
		t.Fatalf("rpm sha256 %x", sum)
	}
	got, err := PayloadSHA256()
	if err != nil {
		t.Fatal(err)
	}
	if got != "57c635608e11265c1113f8ceca3470a8beba17b7c72a9e5010224dd680a6effa" {
		t.Fatalf("payload sha256 %s", got)
	}
}

func TestExtractPlaintext(t *testing.T) {
	files, err := ExtractQML()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for name, body := range files {
		names = append(names, name)
		if strings.Contains(name, ".jpg") || strings.Contains(name, ".png") || strings.HasSuffix(name, ".qm") {
			t.Errorf("binary extracted: %s", name)
		}
		if err := plaintext(body); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	sort.Strings(names)
	want := []string{
		"components/BackupReasons.js",
		"components/ImportReasons.js",
		"components/PassActionsMenu.qml",
		"components/PassCard.qml",
		"components/PassField.qml",
		"components/PassFieldRow.qml",
		"components/PassUpdateWatcher.qml",
		"components/PassValidity.qml",
		"components/PlainText.js",
		"components/ScrollPositionKeeper.qml",
		"cover/CoverPage.qml",
		"harbour-sailwallet.qml",
		"pages/AboutPage.qml",
		"pages/AddPage.qml",
		"pages/BackupPage.qml",
		"pages/CardTypePage.qml",
		"pages/ConfirmUpdatesDialog.qml",
		"pages/EmptyWalletDialog.qml",
		"pages/FullscreenCodePage.qml",
		"pages/ImportResultPage.qml",
		"pages/LicenseTextPage.qml",
		"pages/LicensesPage.qml",
		"pages/MainPage.qml",
		"pages/OwnCardDialog.qml",
		"pages/PassBackPage.qml",
		"pages/PassDetailPage.qml",
		"pages/PassPreviewDialog.qml",
		"pages/PassSettingsPage.qml",
		"pages/RemovePassDialog.qml",
		"pages/RestoreBackupDialog.qml",
		"pages/ScanAddressPage.qml",
		"pages/ScanPage.qml",
		"pages/SettingsPage.qml",
	}
	if strings.Join(names, "\n") != strings.Join(want, "\n") {
		t.Fatalf("names:\n%s", strings.Join(names, "\n"))
	}
}

func TestLicenseClause(t *testing.T) {
	info, err := Original()
	if err != nil {
		t.Fatal(err)
	}
	text, _ := info["licenseText"].(string)
	if !strings.Contains(text, "unmodified installation package") {
		t.Fatalf("license text missing redistribution clause:\n%s", text)
	}
	if info["fileName"] != RPMFileName || info["version"] != "1.1.0" || info["release"] != "1" || info["arch"] != "aarch64" {
		t.Fatalf("package info %#v", info)
	}
	if info["sha256"] != RPMSum {
		t.Fatalf("sha %v", info["sha256"])
	}
	if info["size"] != "1313170" {
		t.Fatalf("size %v", info["size"])
	}
	if info["homepage"] != "https://sailwallet.jmacusik.cz" {
		t.Fatalf("homepage %v", info["homepage"])
	}
}

func TestDiffApply(t *testing.T) {
	orig := []byte("hello\nworld\n")
	edited := []byte("hello\nthere\n")
	diff, err := Diff("f", orig, edited)
	if err != nil {
		t.Fatal(err)
	}
	want := "" +
		"--- a/f\n" +
		"+++ b/f\n" +
		"@@ -1,2 +1,2 @@\n" +
		" hello\n" +
		"-world\n" +
		"+there\n"
	if string(diff) != want {
		t.Fatalf("diff:\n%s", diff)
	}
	got, err := Apply(orig, diff)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, edited) {
		t.Fatalf("applied %q", got)
	}
	if _, err := Apply(orig, bytes.Replace(diff, []byte(" hello\n"), []byte(" no\n"), 1)); err == nil {
		t.Fatal("broken patch was applied")
	}
	same, err := Diff("f", orig, orig)
	if err != nil || len(same) != 0 {
		t.Fatalf("identical diff %q %v", same, err)
	}
}

func TestPatchesRoundTrip(t *testing.T) {
	orig, err := ExtractQML()
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := loadPatches()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := Checkout(dir, false); err != nil {
		t.Fatal(err)
	}
	built, err := BuildPatches(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(built) != len(embedded) {
		t.Fatalf("built %d patches, embedded %d", len(built), len(embedded))
	}
	for rel, diff := range built {
		if !bytes.Equal(diff, embedded[rel]) {
			t.Fatalf("checkout then patches changed %s", rel)
		}
		got, err := Apply(orig[rel], diff)
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		want, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s: patched bytes differ from checkout", rel)
		}
	}
}

func TestInstallLeavesOwnFile(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(dir, "components", "Contrast.js")
	if err := os.MkdirAll(filepath.Dir(own), 0o755); err != nil {
		t.Fatal(err)
	}
	const fake = "fake contrast\n"
	if err := os.WriteFile(own, []byte(fake), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(own)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != fake {
		t.Fatalf("own file changed: %q", got)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(manifest, []byte("Contrast.js")) {
		t.Fatalf("manifest lists own file:\n%s", manifest)
	}
	if !bytes.Contains(manifest, []byte("pages/MainPage.qml\n")) {
		t.Fatalf("manifest missing MainPage:\n%s", manifest)
	}
	if _, err := os.Stat(filepath.Join(dir, entryQML)); !os.IsNotExist(err) {
		t.Fatal("upstream entry qml was installed")
	}
}

func TestCheckoutRefusesLocalEdits(t *testing.T) {
	dir := t.TempDir()
	rel := "pages/AddPage.qml"
	dst := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Checkout(dir, false); err == nil {
		t.Fatal("checkout overwrote a local edit")
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "local\n" {
		t.Fatalf("file changed without --force: %q", got)
	}
	if err := Checkout(dir, true); err != nil {
		t.Fatal(err)
	}
	files, err := PatchedQML()
	if err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, files[rel]) {
		t.Fatal("force checkout did not write the patched file")
	}
}
