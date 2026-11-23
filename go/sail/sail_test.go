package sail

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func passesDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		cand := filepath.Join(dir, "pkwallet", "test_passes")
		if st, err := os.Stat(cand); err == nil && st.IsDir() {
			return cand
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("pkwallet/test_passes not found")
	return ""
}

func TestFinnairRoundTrip(t *testing.T) {
	root := t.TempDir()
	down := t.TempDir()
	open := Call("open", string(mustJSON(map[string]any{
		"dataDir": root, "downloadsDir": down, "locale": "en", "scale": 2, "currency": "EUR",
	})))
	if !stringsOK(open, `"ok":true`) {
		t.Fatal(open)
	}
	sample := filepath.Join(passesDir(t), "AY116120SEP1.pkpass")
	got := Call("inspectFile", string(mustJSON(map[string]string{"path": sample})))
	var inspected map[string]any
	if err := json.Unmarshal([]byte(got), &inspected); err != nil {
		t.Fatal(err, got)
	}
	if inspected["status"] != "new" {
		t.Fatalf("status %v (%s)", inspected["status"], got)
	}
	pass, _ := inspected["pass"].(map[string]any)
	if pass["organizationName"] != "Finnair" {
		t.Fatalf("organization %v", pass["organizationName"])
	}
	if pass["passStyle"] != "boardingPass" {
		t.Fatalf("style %v", pass["passStyle"])
	}
	ver, _ := pass["verification"].(map[string]any)
	if ver == nil || ver["signatureOK"] != true || ver["subject"] == "" || ver["notBefore"] == "" || ver["notAfter"] == "" {
		t.Fatalf("verification %v", pass["verification"])
	}
	if pass["serialNumber"] == "" || pass["passTypeIdentifier"] == "" || pass["teamIdentifier"] == "" {
		t.Fatalf("identity serial=%v type=%v team=%v", pass["serialNumber"], pass["passTypeIdentifier"], pass["teamIdentifier"])
	}
	if _, ok := pass["webServiceURL"]; !ok {
		t.Fatal("webServiceURL missing")
	}
	added := Call("addInspected", `{"autoUpdate":false}`)
	if !stringsOK(added, `"status":"added"`) {
		t.Fatal(added)
	}
	// The sample boarding pass is past its date, so import files it in the archive.
	list := append(decodeList(t, Call("list", `{}`)), decodeList(t, Call("list", `{"archived":true}`))...)
	if len(list) != 1 || list[0]["organizationName"] != "Finnair" {
		t.Fatalf("list %v", list)
	}
	id := list[0]["passId"].(string)
	if id == "" || list[0]["serialNumber"] != pass["serialNumber"] {
		t.Fatalf("list identity %v", list[0])
	}
	listed, _ := list[0]["verification"].(map[string]any)
	if listed == nil || listed["subject"] != ver["subject"] {
		t.Fatalf("listed verification %v", list[0]["verification"])
	}
	png := ImagePNG(id, "barcode", 280, 280)
	if len(png) < 100 {
		t.Fatalf("barcode png %d", len(png))
	}
	again := Call("inspectFile", string(mustJSON(map[string]string{"path": sample})))
	if !stringsOK(again, `"status":"existing"`) {
		t.Fatal(again)
	}
	backup := Call("createBackup", `{}`)
	var b map[string]any
	if err := json.Unmarshal([]byte(backup), &b); err != nil || b["status"] != "ok" {
		t.Fatal(backup)
	}
	empty := Call("startEmptyWallet", `{}`)
	if !stringsOK(empty, `.swbackup`) {
		t.Fatal(empty)
	}
	restored := Call("restoreBackup", string(mustJSON(map[string]string{"path": filepath.Join(down, b["fileName"].(string))})))
	if !stringsOK(restored, `"status":"added"`) || !stringsOK(restored, "Finnair") {
		t.Fatal(restored)
	}
}

func TestOwnCard(t *testing.T) {
	root := t.TempDir()
	Call("open", string(mustJSON(map[string]any{"dataDir": root, "downloadsDir": t.TempDir(), "currency": "EUR"})))
	body := map[string]any{
		"serialNumber": "own-1", "name": "Cafe", "code": "hello-wallet",
		"format": "PKBarcodeFormatQR", "color": "#1f7a4d",
		"fields": map[string]any{"template": "loyalty", "icon": "coffee", "holderNameLabel": "Name", "holderName": "Ada"},
	}
	got := Call("inspectOwnCard", string(mustJSON(body)))
	if !stringsOK(got, `"valid":true`) {
		t.Fatal(got)
	}
	var inspected map[string]any
	if err := json.Unmarshal([]byte(got), &inspected); err != nil {
		t.Fatal(err, got)
	}
	card, _ := inspected["pass"].(map[string]any)
	ver, _ := card["verification"].(map[string]any)
	if card["serialNumber"] != "own-1" || card["ownCard"] != true || ver["signatureOK"] == true || ver["subject"] != "" {
		t.Fatalf("own card %v", card)
	}
	added := Call("addInspected", `{"autoUpdate":false}`)
	if !stringsOK(added, `"status":"added"`) {
		t.Fatal(added)
	}
	var addedMap map[string]any
	_ = json.Unmarshal([]byte(added), &addedMap)
	own := Call("ownCard", string(mustJSON(map[string]string{"passId": addedMap["passId"].(string)})))
	if !stringsOK(own, `"name":"Cafe"`) || !stringsOK(own, `"icon":"coffee"`) {
		t.Fatal(own)
	}
	if len(ImagePNG("icon", "coffee", 64, 64)) < 50 {
		t.Fatal("icon")
	}
}

func TestArchiveMovesBetweenLists(t *testing.T) {
	root := t.TempDir()
	down := t.TempDir()
	open := Call("open", string(mustJSON(map[string]any{
		"dataDir": root, "downloadsDir": down, "locale": "en", "scale": 2, "currency": "EUR",
	})))
	if !stringsOK(open, `"ok":true`) {
		t.Fatal(open)
	}
	sample := filepath.Join(passesDir(t), "AY116120SEP1.pkpass")
	if !stringsOK(Call("inspectFile", string(mustJSON(map[string]string{"path": sample}))), `"status":"new"`) {
		t.Fatal("inspect")
	}
	if !stringsOK(Call("addInspected", `{"autoUpdate":false}`), `"status":"added"`) {
		t.Fatal("add")
	}
	// The sample boarding pass is past its date, so import files it in the archive.
	archived := decodeList(t, Call("list", `{"archived":true}`))
	if len(archived) != 1 || archived[0]["archived"] != true {
		t.Fatalf("archived %v", archived)
	}
	id, _ := archived[0]["passId"].(string)
	if id == "" || len(decodeList(t, Call("list", `{}`))) != 0 {
		t.Fatal(archived)
	}
	if !stringsOK(Call("unarchive", string(mustJSON(map[string]string{"passId": id}))), `"ok":true`) {
		t.Fatal("unarchive")
	}
	active := decodeList(t, Call("list", `{}`))
	if len(active) != 1 || active[0]["archived"] == true || active[0]["passId"] != id {
		t.Fatalf("active %v", active)
	}
	if len(decodeList(t, Call("list", `{"archived":true}`))) != 0 {
		t.Fatal("archive still has it")
	}
	if !stringsOK(Call("archive", string(mustJSON(map[string]string{"passId": id}))), `"ok":true`) {
		t.Fatal("archive")
	}
	if len(decodeList(t, Call("list", `{}`))) != 0 {
		t.Fatal("still active")
	}
	archived = decodeList(t, Call("list", `{"archived":true}`))
	if len(archived) != 1 || archived[0]["archived"] != true || archived[0]["passId"] != id {
		t.Fatalf("archived again %v", archived)
	}
	if !stringsOK(Call("get", string(mustJSON(map[string]string{"passId": id}))), `"archived":true`) {
		t.Fatal("get")
	}
	status := Call("status", `{}`)
	if !stringsOK(status, `"activeCount":0`) || !stringsOK(status, `"archivedCount":1`) {
		t.Fatal(status)
	}
}

func TestBringToFront(t *testing.T) {
	root := t.TempDir()
	Call("open", string(mustJSON(map[string]any{
		"dataDir": root, "downloadsDir": t.TempDir(), "locale": "en", "scale": 2, "currency": "EUR",
	})))
	add := func(serial, name string) string {
		t.Helper()
		body := map[string]any{
			"serialNumber": serial, "name": name, "code": "hello-wallet",
			"format": "PKBarcodeFormatQR", "color": "#1f7a4d",
			"fields": map[string]any{"template": "loyalty"},
		}
		if !stringsOK(Call("inspectOwnCard", string(mustJSON(body))), `"valid":true`) {
			t.Fatal(serial)
		}
		added := Call("addInspected", `{"autoUpdate":false}`)
		var m map[string]any
		if err := json.Unmarshal([]byte(added), &m); err != nil || m["status"] != "added" {
			t.Fatal(added)
		}
		return m["passId"].(string)
	}
	alpha := add("a", "Alpha")
	zulu := add("z", "Zulu")
	// The painted stack is back-to-front, so the front card is last.
	orgs := func() string {
		t.Helper()
		var names []string
		for _, row := range decodeList(t, Call("list", `{}`)) {
			names = append(names, row["organizationName"].(string))
		}
		return strings.Join(names, ",")
	}
	if orgs() != "Zulu,Alpha" {
		t.Fatalf("order %s", orgs())
	}
	moved := Call("bringToFront", string(mustJSON(map[string]string{"passId": zulu})))
	if !stringsOK(moved, `"ok":true`) || !stringsOK(moved, `"moved":true`) {
		t.Fatal(moved)
	}
	if orgs() != "Alpha,Zulu" {
		t.Fatalf("after move %s", orgs())
	}
	again := Call("bringToFront", string(mustJSON(map[string]string{"passId": zulu})))
	if !stringsOK(again, `"moved":false`) || orgs() != "Alpha,Zulu" {
		t.Fatal(again, orgs())
	}
	if !stringsOK(Call("archive", string(mustJSON(map[string]string{"passId": alpha}))), `"ok":true`) {
		t.Fatal("archive")
	}
	archived := Call("bringToFront", string(mustJSON(map[string]string{"passId": alpha})))
	if !stringsOK(archived, `"ok":true`) || !stringsOK(archived, `"moved":false`) {
		t.Fatal(archived)
	}
	if len(decodeList(t, Call("list", `{"archived":true}`))) != 1 || orgs() != "Zulu" {
		t.Fatalf("archived order %s", orgs())
	}
}

func decodeList(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var list []map[string]any
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		t.Fatal(err, raw)
	}
	return list
}

func stringsOK(s, want string) bool {
	return strings.Contains(s, want)
}
