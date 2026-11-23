// Package fixture builds signed pkpass archives for tests.
package fixture

import (
	"archive/zip"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"math/big"
	"time"

	"github.com/digitorus/pkcs7"
)

// Material is a local certificate chain. It is not an Apple chain.
type Material struct {
	Root         *x509.Certificate
	RootKey      *ecdsa.PrivateKey
	Intermediate *x509.Certificate
	Leaf         *x509.Certificate
	LeafKey      *ecdsa.PrivateKey
	Pool         *x509.CertPool
}

// NewMaterial creates a root, an intermediate, and a Pass Type ID leaf.
func NewMaterial(passType, team string) (*Material, error) {
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	rootTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test Root", Organization: []string{"Test"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		SubjectKeyId:          []byte{1},
	}
	root, err := signCert(rootTmpl, rootTmpl, &rootKey.PublicKey, rootKey)
	if err != nil {
		return nil, err
	}
	interKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	interTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "Test WWDR", Organization: []string{"Test"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		SubjectKeyId:          []byte{2},
		AuthorityKeyId:        root.SubjectKeyId,
	}
	inter, err := signCert(interTmpl, root, &interKey.PublicKey, rootKey)
	if err != nil {
		return nil, err
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			CommonName:         "Pass Type ID: " + passType,
			OrganizationalUnit: []string{team},
			Organization:       []string{"Example"},
			Country:            []string{"US"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		BasicConstraintsValid: true,
		SubjectKeyId:          []byte{3},
		AuthorityKeyId:        inter.SubjectKeyId,
	}
	leaf, err := signCert(leafTmpl, inter, &leafKey.PublicKey, interKey)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AddCert(root)
	return &Material{Root: root, RootKey: rootKey, Intermediate: inter, Leaf: leaf, LeafKey: leafKey, Pool: pool}, nil
}

func signCert(tmpl, parent *x509.Certificate, pub *ecdsa.PublicKey, priv *ecdsa.PrivateKey) (*x509.Certificate, error) {
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, priv)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(der)
}

// PassJSON is a small generic pass.
func PassJSON(passType, serial, team, holder string) []byte {
	doc := map[string]any{
		"formatVersion":      1,
		"passTypeIdentifier": passType,
		"serialNumber":       serial,
		"teamIdentifier":     team,
		"organizationName":   "Example",
		"description":        "Example pass",
		"backgroundColor":    "rgb(10, 20, 30)",
		"foregroundColor":    "#FFFFFF",
		"labelColor":         "#CCCCCC",
		"generic": map[string]any{
			"primaryFields": []any{map[string]any{"key": "holder", "label": "Name", "value": holder}},
		},
	}
	b, _ := json.Marshal(doc)
	return b
}

// Pack signs files as a pkpass. files must include pass.json and should include icon.png.
func Pack(files map[string][]byte, m *Material) ([]byte, error) {
	manifest := map[string]string{}
	for name, b := range files {
		sum := sha1.Sum(b)
		manifest[name] = hex.EncodeToString(sum[:])
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	sd, err := pkcs7.NewSignedData(manifestBytes)
	if err != nil {
		return nil, err
	}
	sd.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
	if err := sd.AddSignerChain(m.Leaf, m.LeafKey, []*x509.Certificate{m.Intermediate}, pkcs7.SignerInfoConfig{}); err != nil {
		return nil, err
	}
	sd.Detach()
	sig, err := sd.Finish()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, b := range files {
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(b); err != nil {
			return nil, err
		}
	}
	w, err := zw.Create("manifest.json")
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(manifestBytes); err != nil {
		return nil, err
	}
	w, err = zw.Create("signature")
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(sig); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// IconPNG is a 1x1 PNG.
func IconPNG() []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	return buf.Bytes()
}
