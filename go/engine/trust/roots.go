package trust

import (
	"crypto/x509"
	"embed"
	"fmt"
	"sync"
)

// Apple Root CA, G2, and G3 are the trust anchors. WWDR certificates are
// intermediates only: a chain is trusted when it ends at one of the roots.
// Real passes often omit the WWDR certificate, and older ones chain to the
// original Apple Root CA rather than G2 or G3.

//go:embed *.pem
var pems embed.FS

var (
	once          sync.Once
	roots         *x509.CertPool
	intermediates *x509.CertPool
	loadErr       error
)

func load() {
	roots = x509.NewCertPool()
	intermediates = x509.NewCertPool()
	rootNames := []string{"AppleRootCA.pem", "AppleRootCA-G2.pem", "AppleRootCA-G3.pem"}
	interNames := []string{"WWDR.pem", "WWDR-G2.pem", "WWDR-G3.pem", "WWDR-G4.pem", "WWDR-G5.pem", "WWDR-G6.pem"}
	for _, name := range rootNames {
		if err := appendPEM(roots, name); err != nil {
			loadErr = err
			return
		}
	}
	for _, name := range interNames {
		if err := appendPEM(intermediates, name); err != nil {
			loadErr = err
			return
		}
	}
}

func appendPEM(pool *x509.CertPool, name string) error {
	pem, err := pems.ReadFile(name)
	if err != nil {
		return err
	}
	if !pool.AppendCertsFromPEM(pem) {
		return fmt.Errorf("trust: %s did not contain a certificate", name)
	}
	return nil
}

// AppleRoots is the pinned Apple root set.
func AppleRoots() (*x509.CertPool, error) {
	once.Do(load)
	return roots, loadErr
}

// Intermediates adds the bundled WWDR certificates to pool.
func Intermediates(pool *x509.CertPool) error {
	once.Do(load)
	if loadErr != nil {
		return loadErr
	}
	// CertPool has no export, so re-read the PEM into the caller pool.
	for _, name := range []string{"WWDR.pem", "WWDR-G2.pem", "WWDR-G3.pem", "WWDR-G4.pem", "WWDR-G5.pem", "WWDR-G6.pem"} {
		pem, err := pems.ReadFile(name)
		if err != nil {
			return err
		}
		if !pool.AppendCertsFromPEM(pem) {
			return fmt.Errorf("trust: %s did not contain a certificate", name)
		}
	}
	return nil
}
