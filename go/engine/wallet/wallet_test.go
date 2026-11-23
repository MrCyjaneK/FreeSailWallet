package wallet

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"harbour-freesailwallet/engine/internal/fixture"
	"harbour-freesailwallet/engine/pass"
)

func TestWalletImportArchiveHistoryExportPoll(t *testing.T) {
	const (
		passType = "pass.example.ticket"
		team     = "TEAM123456"
	)
	m, err := fixture.NewMaterial(passType, team)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	now := time.Now().UTC()
	var srv *httptest.Server
	polling := false
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !polling {
			t.Errorf("polled while archived: %s", r.URL.Path)
			http.Error(w, "no", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "ApplePass secret-token-1234" {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("If-Modified-Since") != "" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		body := mustPassWithWeb(t, m, passType, team, "100", "Ada Updated", srv.URL)
		w.Header().Set("Last-Modified", now.Add(time.Hour).UTC().Format(http.TimeFormat))
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	st, err := OpenOptions(dir, Options{
		Now:   func() time.Time { return now },
		Poll:  poller{Client: srv.Client()},
		Roots: m.Pool,
	})
	if err != nil {
		t.Fatal(err)
	}
	original := mustPassWithWeb(t, m, passType, team, "100", "Ada", srv.URL)
	imported, err := st.Import(original)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Passes) != 1 || imported.Passes[0].PrimaryValue != "Ada" {
		t.Fatalf("import %+v", imported)
	}
	id := imported.Passes[0].ID

	list, err := st.List()
	if err != nil || len(list.Active) != 1 || len(list.Archived) != 0 {
		t.Fatalf("list %+v %v", list, err)
	}
	if err := st.Archive(id); err != nil {
		t.Fatal(err)
	}
	list, _ = st.List()
	if len(list.Active) != 0 || len(list.Archived) != 1 {
		t.Fatalf("archive list %+v", list)
	}
	if _, err := st.Poll(); err != nil {
		t.Fatal(err)
	}
	if err := st.Unarchive(id); err != nil {
		t.Fatal(err)
	}
	polling = true

	// The first poll has no If-Modified-Since, so the server returns a new pass.
	polled, err := st.Poll()
	if err != nil {
		t.Fatal(err)
	}
	if polled.Updated != 1 {
		t.Fatalf("poll %+v", polled)
	}
	hist, err := st.History(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Revisions) < 2 {
		t.Fatalf("history %+v", hist)
	}
	if hist.Revisions[0].Current != true || hist.Revisions[0].Source != "poll" {
		t.Fatalf("newest revision %+v", hist.Revisions[0])
	}
	if len(hist.Revisions[0].Changes) == 0 {
		t.Fatal("expected a field diff")
	}
	old, err := st.GetRevision(id, hist.Revisions[len(hist.Revisions)-1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if old.PrimaryFields[0].Value != "Ada" {
		t.Fatalf("old value %q", old.PrimaryFields[0].Value)
	}

	exp, err := st.Export()
	if err != nil {
		t.Fatal(err)
	}
	names := zipNames(t, exp.Path)
	joined := strings.Join(names, "\n")
	for _, want := range []string{"wallet.json", "passes/", "history/"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("export missing %s in %s", want, joined)
		}
	}

	dir2 := t.TempDir()
	st2, err := OpenOptions(dir2, Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(exp.Path)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := st2.Import(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !restored.Restored || len(restored.Passes) != 1 {
		t.Fatalf("restore %+v", restored)
	}
	hist2, err := st2.History(id)
	if err != nil || len(hist2.Revisions) < 2 {
		t.Fatalf("restored history %+v %v", hist2, err)
	}

	// A second poll sends If-Modified-Since and keeps the current pass.
	again, err := st.PollPass(id)
	if err != nil {
		t.Fatal(err)
	}
	if again.Unchanged != 1 || again.Updated != 0 {
		t.Fatalf("second poll %+v", again)
	}
}

func TestExpiredAndVoidedGoToArchive(t *testing.T) {
	const (
		passType = "pass.example.ticket"
		team     = "TEAM123456"
	)
	m, err := fixture.NewMaterial(passType, team)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	hit := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		http.Error(w, "no", http.StatusBadRequest)
	}))
	defer srv.Close()
	st, err := OpenOptions(t.TempDir(), Options{
		Now:   func() time.Time { return now },
		Poll:  poller{Client: srv.Client()},
		Roots: m.Pool,
	})
	if err != nil {
		t.Fatal(err)
	}
	pack := func(org, serial string, extra map[string]any) []byte {
		t.Helper()
		doc := map[string]any{
			"formatVersion": 1, "passTypeIdentifier": passType, "serialNumber": serial,
			"teamIdentifier": team, "organizationName": org, "description": org,
			"generic": map[string]any{"primaryFields": []any{map[string]any{"key": "n", "value": org}}},
		}
		for k, v := range extra {
			doc[k] = v
		}
		raw, _ := json.Marshal(doc)
		out, err := fixture.Pack(map[string][]byte{"pass.json": raw, "icon.png": fixture.IconPNG()}, m)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	web := map[string]any{"webServiceURL": srv.URL, "authenticationToken": "secret-token-1234"}
	expiredExtra := map[string]any{"expirationDate": "2020-01-01T00:00:00Z"}
	for k, v := range web {
		expiredExtra[k] = v
	}
	expired, err := st.Import(pack("Zed", "expired", expiredExtra))
	if err != nil {
		t.Fatal(err)
	}
	voided, err := st.Import(pack("Ada", "voided", map[string]any{
		"voided": true, "webServiceURL": srv.URL, "authenticationToken": "secret-token-1234",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Import(pack("Zulu", "z", nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Import(pack("Alpha", "a", nil)); err != nil {
		t.Fatal(err)
	}

	list, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Active) != 2 || list.Active[0].OrganizationName != "Alpha" || list.Active[1].OrganizationName != "Zulu" {
		t.Fatalf("active %+v", list.Active)
	}
	if len(list.Archived) != 2 || list.Archived[0].OrganizationName != "Ada" || !list.Archived[0].Voided {
		t.Fatalf("archived voided %+v", list.Archived)
	}
	if list.Archived[1].OrganizationName != "Zed" || !list.Archived[1].Expired {
		t.Fatalf("archived expired %+v", list.Archived[1])
	}
	if _, err := st.Poll(); err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Fatal("polled an archived pass")
	}

	expiredID := expired.Passes[0].ID
	if err := st.Unarchive(expiredID); err != nil {
		t.Fatal(err)
	}
	list, _ = st.List()
	if !containsOrg(list.Active, "Zed") || containsOrg(list.Archived, "Zed") {
		t.Fatalf("unarchive did not stick %+v %+v", list.Active, list.Archived)
	}
	list, _ = st.List()
	if !containsOrg(list.Active, "Zed") {
		t.Fatal("expired pass was archived again")
	}
	if _, err := st.Poll(); err != nil {
		t.Fatal(err)
	}
	if !hit {
		t.Fatal("unarchived expired pass was not polled")
	}

	if err := st.Unarchive(voided.Passes[0].ID); err != nil {
		t.Fatal(err)
	}
	list, _ = st.List()
	list, _ = st.List()
	if !containsOrg(list.Active, "Ada") || containsOrg(list.Archived, "Ada") {
		t.Fatalf("voided unarchive %+v", list)
	}

	exp, err := st.Export()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(exp.Path)
	if err != nil {
		t.Fatal(err)
	}
	st2, err := OpenOptions(t.TempDir(), Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st2.Import(raw); err != nil {
		t.Fatal(err)
	}
	restored, err := st2.List()
	if err != nil {
		t.Fatal(err)
	}
	if !containsOrg(restored.Active, "Zed") || !containsOrg(restored.Active, "Ada") {
		t.Fatalf("restore lost keepActive %+v", restored.Active)
	}
}

func containsOrg(list []pass.Summary, org string) bool {
	for _, item := range list {
		if item.OrganizationName == org {
			return true
		}
	}
	return false
}

func TestBarcodeAndPkpasses(t *testing.T) {
	const (
		passType = "pass.example.ticket"
		team     = "TEAM123456"
	)
	m, err := fixture.NewMaterial(passType, team)
	if err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{
		"formatVersion": 1, "passTypeIdentifier": passType, "serialNumber": "7",
		"teamIdentifier": team, "organizationName": "Example", "description": "QR",
		"generic": map[string]any{"primaryFields": []any{map[string]any{"key": "n", "value": "7"}}},
		"barcodes": []any{map[string]any{
			"format": "PKBarcodeFormatQR", "message": "hello-qr", "messageEncoding": "utf-8", "altText": "hello",
		}},
	}
	rawJSON, _ := json.Marshal(doc)
	one, err := fixture.Pack(map[string][]byte{"pass.json": rawJSON, "icon.png": fixture.IconPNG()}, m)
	if err != nil {
		t.Fatal(err)
	}
	bundle := zipOf(t, map[string][]byte{"a.pkpass": one})
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	imported, err := st.Import(bundle)
	if err != nil {
		t.Fatal(err)
	}
	png, err := st.BarcodePNG(imported.Passes[0].ID, 128)
	if err != nil {
		t.Fatal(err)
	}
	if !png.Supported || len(png.PNG) == 0 || png.AltText != "hello" {
		t.Fatalf("barcode %+v", png)
	}
}

func mustPass(t *testing.T, m *fixture.Material, passType, team, serial, holder, _ string) []byte {
	t.Helper()
	return mustPassWithWeb(t, m, passType, team, serial, holder, "")
}

func mustPassWithWeb(t *testing.T, m *fixture.Material, passType, team, serial, holder, web string) []byte {
	t.Helper()
	doc := map[string]any{
		"formatVersion": 1, "passTypeIdentifier": passType, "serialNumber": serial,
		"teamIdentifier": team, "organizationName": "Example", "description": "Example pass",
		"backgroundColor": "rgb(10, 20, 30)", "foregroundColor": "#FFFFFF",
		"generic": map[string]any{
			"primaryFields": []any{map[string]any{
				"key": "holder", "label": "Name", "value": holder,
				"changeMessage": "Name changed to %@",
			}},
		},
	}
	if web != "" {
		doc["webServiceURL"] = web
		doc["authenticationToken"] = "secret-token-1234"
	}
	raw, _ := json.Marshal(doc)
	out, err := fixture.Pack(map[string][]byte{"pass.json": raw, "icon.png": fixture.IconPNG()}, m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func zipNames(t *testing.T, path string) []string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var names []string
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	return names
}

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, b := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMoveOrderPersists(t *testing.T) {
	const (
		passType = "pass.example.ticket"
		team     = "TEAM123456"
	)
	m, err := fixture.NewMaterial(passType, team)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	pack := func(org, serial string) []byte {
		t.Helper()
		doc := map[string]any{
			"formatVersion": 1, "passTypeIdentifier": passType, "serialNumber": serial,
			"teamIdentifier": team, "organizationName": org, "description": org,
			"generic": map[string]any{"primaryFields": []any{map[string]any{"key": "n", "value": org}}},
		}
		raw, _ := json.Marshal(doc)
		out, err := fixture.Pack(map[string][]byte{"pass.json": raw, "icon.png": fixture.IconPNG()}, m)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	imported := map[string]string{}
	for org, serial := range map[string]string{"Alpha": "a", "Mike": "m", "Zulu": "z"} {
		got, err := st.Import(pack(org, serial))
		if err != nil {
			t.Fatal(err)
		}
		imported[org] = got.Passes[0].ID
	}
	assertOrgs := func(st *Store, want ...string) {
		t.Helper()
		list, err := st.List()
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, item := range list.Active {
			got = append(got, item.OrganizationName)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("order %v, want %v", got, want)
		}
	}
	assertOrgs(st, "Alpha", "Mike", "Zulu")
	if err := st.Move(imported["Zulu"], "top"); err != nil {
		t.Fatal(err)
	}
	assertOrgs(st, "Zulu", "Alpha", "Mike")
	if err := st.Move(imported["Alpha"], "down"); err != nil {
		t.Fatal(err)
	}
	assertOrgs(st, "Zulu", "Mike", "Alpha")
	if err := st.Move(imported["Mike"], "bottom"); err != nil {
		t.Fatal(err)
	}
	assertOrgs(st, "Zulu", "Alpha", "Mike")
	if err := st.Move(imported["Zulu"], "up"); err != nil {
		t.Fatal(err)
	}
	assertOrgs(st, "Zulu", "Alpha", "Mike")
	if err := st.Move(imported["Zulu"], "sideways"); err == nil {
		t.Fatal("expected an error for an unknown move")
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	assertOrgs(reopened, "Zulu", "Alpha", "Mike")
	if _, err := reopened.Import(pack("Bravo", "b")); err != nil {
		t.Fatal(err)
	}
	assertOrgs(reopened, "Zulu", "Alpha", "Mike", "Bravo")

	if err := reopened.Archive(imported["Alpha"]); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Unarchive(imported["Alpha"]); err != nil {
		t.Fatal(err)
	}
	assertOrgs(reopened, "Zulu", "Mike", "Bravo", "Alpha")
}

func TestLocalizedStrings(t *testing.T) {
	if got := parseViaPass(t, "\xef\xbb\xbf\"Hello\" = \"World\";\n"); got != "World" {
		t.Fatal(got)
	}
}

func parseViaPass(t *testing.T, stringsFile string) string {
	t.Helper()
	const (
		passType = "pass.example.ticket"
		team     = "TEAM123456"
	)
	m, err := fixture.NewMaterial(passType, team)
	if err != nil {
		t.Fatal(err)
	}
	doc := []byte(`{"formatVersion":1,"passTypeIdentifier":"pass.example.ticket","serialNumber":"1","teamIdentifier":"TEAM123456","organizationName":"Hello","description":"Hello","generic":{"primaryFields":[{"key":"k","value":"Hello"}]}}`)
	raw, err := fixture.Pack(map[string][]byte{
		"pass.json":             doc,
		"icon.png":              fixture.IconPNG(),
		"en.lproj/pass.strings": []byte(stringsFile),
	}, m)
	if err != nil {
		t.Fatal(err)
	}
	p, err := pass.Inspect(raw, pass.Options{Roots: m.Pool, Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	return p.PrimaryFields[0].Value
}

func TestPollRejectsUntrustedAndDifferentPass(t *testing.T) {
	const (
		passType = "pass.example.ticket"
		team     = "TEAM123456"
	)
	m, err := fixture.NewMaterial(passType, team)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	var mode string
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch mode {
		case "other":
			_, _ = w.Write(mustPassWithWeb(t, m, passType, team, "999", "Other", srv.URL))
		default:
			_, _ = w.Write(unsignedPass(t, passType, team, "100", "Hacked", srv.URL))
		}
	}))
	defer srv.Close()
	st, err := OpenOptions(t.TempDir(), Options{
		Now:   func() time.Time { return now },
		Poll:  poller{Client: srv.Client()},
		Roots: m.Pool,
	})
	if err != nil {
		t.Fatal(err)
	}
	imported, err := st.Import(mustPassWithWeb(t, m, passType, team, "100", "Ada", srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	id := imported.Passes[0].ID
	polled, err := st.PollPass(id)
	if err != nil {
		t.Fatal(err)
	}
	if polled.Updated != 0 || len(polled.Errors) != 1 || !strings.Contains(polled.Errors[0].Message, "not trusted") {
		t.Fatalf("untrusted poll %+v", polled)
	}
	got, err := st.Get(id)
	if err != nil || got.PrimaryFields[0].Value != "Ada" {
		t.Fatalf("pass changed %+v %v", got, err)
	}
	mode = "other"
	polled, err = st.PollPass(id)
	if err != nil {
		t.Fatal(err)
	}
	if polled.Updated != 0 || len(polled.Errors) != 1 || !strings.Contains(polled.Errors[0].Message, "does not match") {
		t.Fatalf("identity poll %+v", polled)
	}
	got, err = st.Get(id)
	if err != nil || got.PrimaryFields[0].Value != "Ada" {
		t.Fatalf("pass changed %+v %v", got, err)
	}
}

func TestPollRequiresHTTPSAndSameHost(t *testing.T) {
	const (
		passType = "pass.example.ticket"
		team     = "TEAM123456"
	)
	m, err := fixture.NewMaterial(passType, team)
	if err != nil {
		t.Fatal(err)
	}
	evilHit := false
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		evilHit = true
		http.Error(w, "no", http.StatusBadRequest)
	}))
	defer evil.Close()
	good := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+"/stolen", http.StatusFound)
	}))
	defer good.Close()
	st, err := OpenOptions(t.TempDir(), Options{
		Poll:  poller{Client: good.Client()},
		Roots: m.Pool,
	})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := st.Import(mustPassWithWeb(t, m, passType, team, "1", "Ada", "http://example.com"))
	if err != nil {
		t.Fatal(err)
	}
	polled, err := st.PollPass(plain.Passes[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(polled.Errors) != 1 || !strings.Contains(polled.Errors[0].Message, "https") {
		t.Fatalf("http poll %+v", polled)
	}
	redirected, err := st.Import(mustPassWithWeb(t, m, passType, team, "2", "Bea", good.URL))
	if err != nil {
		t.Fatal(err)
	}
	polled, err = st.PollPass(redirected.Passes[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if evilHit || polled.Updated != 0 || len(polled.Errors) != 1 {
		t.Fatalf("redirect poll hit=%v %+v", evilHit, polled)
	}
}

func TestPollDoesNotHoldLockDuringFetch(t *testing.T) {
	const (
		passType = "pass.example.ticket"
		team     = "TEAM123456"
	)
	m, err := fixture.NewMaterial(passType, team)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(started) })
		<-release
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()
	finish := func() {
		once.Do(func() { close(started) })
		select {
		case <-release:
		default:
			close(release)
		}
	}
	defer finish()
	st, err := OpenOptions(t.TempDir(), Options{Poll: poller{Client: srv.Client()}, Roots: m.Pool})
	if err != nil {
		t.Fatal(err)
	}
	imported, err := st.Import(mustPassWithWeb(t, m, passType, team, "100", "Ada", srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := st.PollPass(imported.Passes[0].ID)
		done <- err
	}()
	select {
	case <-started:
	case err := <-done:
		t.Fatalf("poll finished before the request: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("poll did not reach the server")
	}
	listed := make(chan error, 1)
	go func() {
		_, err := st.List()
		listed <- err
	}()
	select {
	case err := <-listed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("list blocked while a pass update was in flight")
	}
	finish()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRestoreAndRevisionStayInPassDir(t *testing.T) {
	const (
		passType = "pass.example.ticket"
		team     = "TEAM123456"
	)
	m, err := fixture.NewMaterial(passType, team)
	if err != nil {
		t.Fatal(err)
	}
	body := mustPass(t, m, passType, team, "100", "Ada", "")
	dir := t.TempDir()
	st, err := OpenOptions(dir, Options{Roots: m.Pool})
	if err != nil {
		t.Fatal(err)
	}
	backup := zipOf(t, map[string][]byte{
		"wallet.json":       []byte(`{"passes":{"../outside":{"passTypeIdentifier":"pass.example.ticket","serialNumber":"100","bundlePath":"passes/one.pkpass"}}}`),
		"passes/one.pkpass": body,
	})
	restored, err := st.Import(backup)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Passes) != 0 || len(restored.Problems) == 0 {
		t.Fatalf("restore %+v", restored)
	}
	if _, err := os.Stat(filepath.Join(dir, "outside")); !os.IsNotExist(err) {
		t.Fatalf("escaped pass dir err=%v", err)
	}

	imported, err := st.Import(body)
	if err != nil {
		t.Fatal(err)
	}
	id := imported.Passes[0].ID
	raw, err := os.ReadFile(filepath.Join(dir, "wallet.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state disk
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	when := time.Date(2020, 1, 2, 3, 4, 5, 6, time.UTC)
	state.Passes[id].Revisions = append(state.Passes[id].Revisions, revision{
		At: when, Source: "import", File: "../../wallet.json",
	})
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wallet.json"), encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenOptions(dir, Options{Roots: m.Pool})
	if err != nil {
		t.Fatal(err)
	}
	hist, err := reopened.History(id)
	if err != nil {
		t.Fatal(err)
	}
	escaped := false
	for _, rev := range hist.Revisions {
		_, err := reopened.GetRevision(id, rev.ID)
		if err != nil && strings.Contains(err.Error(), "escapes") {
			escaped = true
		}
	}
	if !escaped {
		t.Fatalf("revision path was not confined: %+v", hist)
	}
}

func TestListReportsUnreadablePass(t *testing.T) {
	const (
		passType = "pass.example.ticket"
		team     = "TEAM123456"
	)
	m, err := fixture.NewMaterial(passType, team)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	st, err := OpenOptions(dir, Options{Roots: m.Pool})
	if err != nil {
		t.Fatal(err)
	}
	imported, err := st.Import(mustPass(t, m, passType, team, "100", "Ada", ""))
	if err != nil {
		t.Fatal(err)
	}
	id := imported.Passes[0].ID
	if err := os.Remove(filepath.Join(dir, "passes", id, "current.pkpass")); err != nil {
		t.Fatal(err)
	}
	list, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Active) != 0 || len(list.Problems) != 1 || !strings.Contains(list.Problems[0], id) {
		t.Fatalf("list %+v", list)
	}
}

func unsignedPass(t *testing.T, passType, team, serial, holder, web string) []byte {
	t.Helper()
	doc := map[string]any{
		"formatVersion": 1, "passTypeIdentifier": passType, "serialNumber": serial,
		"teamIdentifier": team, "organizationName": "Example", "description": "Example pass",
		"generic": map[string]any{"primaryFields": []any{map[string]any{"key": "holder", "value": holder}}},
	}
	if web != "" {
		doc["webServiceURL"] = web
		doc["authenticationToken"] = "secret-token-1234"
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string][]byte{"pass.json": raw, "icon.png": fixture.IconPNG()} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
