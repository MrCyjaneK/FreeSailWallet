package pass

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestOpenZipRejectsTooManyFiles(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 0; i < MaxEntries+1; i++ {
		w, err := zw.Create(fmt.Sprintf("f%d.txt", i))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenZip(buf.Bytes()); err == nil {
		t.Fatal("expected too many files")
	}
}

func TestOpenZipSkipsParentPaths(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("../wallet.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("nope")); err != nil {
		t.Fatal(err)
	}
	w, err = zw.Create("pass.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("{}")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := OpenZip(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "pass.json" {
		t.Fatalf("entries %+v", entries)
	}
}

func TestReadLimitedRejectsOverflow(t *testing.T) {
	_, err := ReadLimited(strings.NewReader("12345"), 4)
	if err == nil {
		t.Fatal("expected overflow")
	}
	got, err := ReadLimited(strings.NewReader("1234"), 4)
	if err != nil || string(got) != "1234" {
		t.Fatalf("got %q err %v", got, err)
	}
}
