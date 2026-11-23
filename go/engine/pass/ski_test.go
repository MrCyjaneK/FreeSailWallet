package pass

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"math/big"
	"testing"

	"harbour-freesailwallet/engine/internal/fixture"
)

func TestHashAndAlgMatchesKey(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, alg, err := hashAndAlg(oidSHA256, &ecKey.PublicKey)
	if err != nil || alg != x509.ECDSAWithSHA256 {
		t.Fatalf("ecdsa alg=%v err=%v", alg, err)
	}
	_, alg, err = hashAndAlg(oidSHA1, &ecKey.PublicKey)
	if err != nil || alg != x509.ECDSAWithSHA1 {
		t.Fatalf("ecdsa sha1 alg=%v err=%v", alg, err)
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	_, alg, err = hashAndAlg(oidSHA256, &rsaKey.PublicKey)
	if err != nil || alg != x509.SHA256WithRSA {
		t.Fatalf("rsa alg=%v err=%v", alg, err)
	}
	if _, _, err := hashAndAlg(oidSHA256, "nope"); err == nil {
		t.Fatal("expected unsupported key")
	}
}

func TestCertByIssuerRequiresIssuer(t *testing.T) {
	m, err := fixture.NewMaterial("pass.example.ticket", "TEAM123456")
	if err != nil {
		t.Fatal(err)
	}
	leaf := m.Leaf
	if certByIssuer([]*x509.Certificate{leaf}, []byte{1, 2, 3}, leaf.SerialNumber) != nil {
		t.Fatal("accepted a serial match with the wrong issuer")
	}
	if got := certByIssuer([]*x509.Certificate{leaf}, leaf.RawIssuer, leaf.SerialNumber); got != leaf {
		t.Fatalf("matching issuer returned %v", got)
	}
	other := *leaf
	other.SerialNumber = new(big.Int).Add(leaf.SerialNumber, big.NewInt(1))
	if certByIssuer([]*x509.Certificate{&other}, leaf.RawIssuer, leaf.SerialNumber) != nil {
		t.Fatal("accepted an issuer match with the wrong serial")
	}
}
