package sailpkg

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/klauspost/compress/zstd"
)

const (
	rpmLeadMagic   = 0xedabeedb
	rpmHeaderMagic = 0x8eade801

	tagName       = 1000
	tagVersion    = 1001
	tagRelease    = 1002
	tagLicense    = 1014
	tagURL        = 1020
	tagArch       = 1022
	tagCompressor = 1125

	qmlPrefix       = "usr/share/harbour-sailwallet/qml/"
	licensePath     = "usr/share/harbour-sailwallet/licenses/sailwallet.txt"
	entryQML        = "harbour-sailwallet.qml"
	decompressedSum = "57c635608e11265c1113f8ceca3470a8beba17b7c72a9e5010224dd680a6effa"
)

// RPMSum is the SHA-256 of the embedded installation package.
const RPMSum = "29f2325806c8bebfb0e78f772e32a08c0ed4d924162b30043984153b24b14e4a"

type headerEntry struct {
	tag uint32
	typ uint32
	off uint32
	cnt uint32
}

type packageMeta struct {
	name       string
	version    string
	release    string
	arch       string
	homepage   string
	compressor string
}

type parsedPackage struct {
	meta    packageMeta
	qml     map[string][]byte
	license string
	payload []byte
}

var (
	loadOnce sync.Once
	loaded   parsedPackage
	loadErr  error
)

func loadPackage() (parsedPackage, error) {
	loadOnce.Do(func() {
		loaded, loadErr = parseRPM(rpmBytes)
	})
	return loaded, loadErr
}

// RPM returns the unmodified package bytes.
func RPM() []byte {
	return rpmBytes
}

// PayloadSHA256 is the SHA-256 of the decompressed cpio payload.
func PayloadSHA256() (string, error) {
	pkg, err := loadPackage()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(pkg.payload)
	return hex.EncodeToString(sum[:]), nil
}

// ExtractQML returns plaintext QML and JS files from the package, keyed by
// path relative to the upstream qml directory. Binaries, images, and
// translations are left inside the package.
func ExtractQML() (map[string][]byte, error) {
	pkg, err := loadPackage()
	if err != nil {
		return nil, err
	}
	return pkg.qml, nil
}

// Original describes the embedded installation package. The license text is
// the copy stored inside that package.
func Original() (map[string]any, error) {
	pkg, err := loadPackage()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(rpmBytes)
	return map[string]any{
		"fileName":    RPMFileName,
		"version":     pkg.meta.version,
		"release":     pkg.meta.release,
		"arch":        pkg.meta.arch,
		"size":        fmt.Sprintf("%d", len(rpmBytes)),
		"sha256":      hex.EncodeToString(sum[:]),
		"homepage":    pkg.meta.homepage,
		"licenseText": pkg.license,
	}, nil
}

func parseRPM(b []byte) (parsedPackage, error) {
	if len(b) < 96 || binary.BigEndian.Uint32(b[:4]) != rpmLeadMagic {
		return parsedPackage{}, fmt.Errorf("not an rpm")
	}
	off := 96
	sigLen, err := headerSize(b[off:])
	if err != nil {
		return parsedPackage{}, fmt.Errorf("signature header: %w", err)
	}
	off += sigLen
	off = (off + 7) &^ 7
	entries, store, hdrLen, err := readHeader(b[off:])
	if err != nil {
		return parsedPackage{}, fmt.Errorf("main header: %w", err)
	}
	meta, err := metaFrom(entries, store)
	if err != nil {
		return parsedPackage{}, err
	}
	if meta.compressor != "zstd" {
		return parsedPackage{}, fmt.Errorf("payload compressor %q", meta.compressor)
	}
	off += hdrLen
	if off+4 > len(b) {
		return parsedPackage{}, fmt.Errorf("rpm payload missing")
	}
	// The signature header is padded to 8 bytes. The payload itself starts at
	// the zstd frame, which in the published package is immediately after the
	// main header.
	if binary.LittleEndian.Uint32(b[off:]) != 0xFD2FB528 {
		aligned := (off + 7) &^ 7
		if aligned+4 <= len(b) && binary.LittleEndian.Uint32(b[aligned:]) == 0xFD2FB528 {
			off = aligned
		}
	}
	payload, err := decompressZstd(b[off:])
	if err != nil {
		return parsedPackage{}, err
	}
	sum := sha256.Sum256(payload)
	if hex.EncodeToString(sum[:]) != decompressedSum {
		return parsedPackage{}, fmt.Errorf("decompressed payload sha256 %x", sum)
	}
	files, err := readCPIO(payload)
	if err != nil {
		return parsedPackage{}, err
	}
	qml := map[string][]byte{}
	var license string
	for name, body := range files {
		name = strings.TrimPrefix(name, "./")
		if name == licensePath {
			if err := plaintext(body); err != nil {
				return parsedPackage{}, fmt.Errorf("%s: %w", name, err)
			}
			license = string(body)
			continue
		}
		rel, ok := qmlRel(name)
		if !ok {
			continue
		}
		if err := plaintext(body); err != nil {
			return parsedPackage{}, fmt.Errorf("%s: %w", rel, err)
		}
		qml[rel] = body
	}
	if license == "" {
		return parsedPackage{}, fmt.Errorf("sailwallet license missing from package")
	}
	if len(qml) == 0 {
		return parsedPackage{}, fmt.Errorf("no plaintext qml in package")
	}
	return parsedPackage{meta: meta, qml: qml, license: license, payload: payload}, nil
}

func decompressZstd(src []byte) ([]byte, error) {
	dec, err := zstd.NewReader(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	out, err := io.ReadAll(dec)
	if err != nil {
		return nil, fmt.Errorf("zstd: %w", err)
	}
	return out, nil
}

func headerSize(b []byte) (int, error) {
	_, _, n, err := readHeader(b)
	return n, err
}

func readHeader(b []byte) ([]headerEntry, []byte, int, error) {
	if len(b) < 16 {
		return nil, nil, 0, fmt.Errorf("short header")
	}
	if binary.BigEndian.Uint32(b[:4]) != rpmHeaderMagic {
		return nil, nil, 0, fmt.Errorf("bad header magic %08x", binary.BigEndian.Uint32(b[:4]))
	}
	nindex := int(binary.BigEndian.Uint32(b[8:12]))
	hsize := int(binary.BigEndian.Uint32(b[12:16]))
	if nindex < 0 || hsize < 0 {
		return nil, nil, 0, fmt.Errorf("invalid header size")
	}
	indexSize := nindex * 16
	total := 16 + indexSize + hsize
	if total < 16 || len(b) < total {
		return nil, nil, 0, fmt.Errorf("truncated header")
	}
	index := b[16 : 16+indexSize]
	store := b[16+indexSize : total]
	entries := make([]headerEntry, nindex)
	for i := 0; i < nindex; i++ {
		e := index[i*16 : (i+1)*16]
		entries[i] = headerEntry{
			tag: binary.BigEndian.Uint32(e[0:4]),
			typ: binary.BigEndian.Uint32(e[4:8]),
			off: binary.BigEndian.Uint32(e[8:12]),
			cnt: binary.BigEndian.Uint32(e[12:16]),
		}
	}
	return entries, store, total, nil
}

func metaFrom(entries []headerEntry, store []byte) (packageMeta, error) {
	var meta packageMeta
	want := map[uint32]*string{
		tagName:       &meta.name,
		tagVersion:    &meta.version,
		tagRelease:    &meta.release,
		tagArch:       &meta.arch,
		tagURL:        &meta.homepage,
		tagCompressor: &meta.compressor,
	}
	for _, e := range entries {
		dst := want[e.tag]
		if dst == nil {
			continue
		}
		s, err := headerString(store, e)
		if err != nil {
			return packageMeta{}, fmt.Errorf("tag %d: %w", e.tag, err)
		}
		*dst = s
	}
	if meta.version == "" || meta.release == "" || meta.arch == "" || meta.compressor == "" {
		return packageMeta{}, fmt.Errorf("rpm header missing name, version, or compressor")
	}
	return meta, nil
}

func headerString(store []byte, e headerEntry) (string, error) {
	if int(e.off) > len(store) {
		return "", fmt.Errorf("offset %d past store", e.off)
	}
	rest := store[e.off:]
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return "", fmt.Errorf("unterminated string")
	}
	return string(rest[:i]), nil
}

func qmlRel(name string) (string, bool) {
	if !strings.HasPrefix(name, qmlPrefix) {
		return "", false
	}
	rel := strings.TrimPrefix(name, qmlPrefix)
	if rel == "" || strings.HasSuffix(rel, "/") || strings.Contains(rel, "..") {
		return "", false
	}
	dot := strings.LastIndex(rel, ".")
	if dot < 0 {
		return "", false
	}
	switch strings.ToLower(rel[dot:]) {
	case ".qml", ".js":
		return rel, true
	default:
		return "", false
	}
}

func plaintext(b []byte) error {
	if bytes.Contains(b, []byte{0}) {
		return fmt.Errorf("contains a NUL byte")
	}
	if !utf8.Valid(b) {
		return fmt.Errorf("not utf-8")
	}
	return nil
}
