package pass

import (
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"strings"
	"time"

	"github.com/digitorus/pkcs7"

	"harbour-freesailwallet/engine/trust"
)

// crypto/sha256 must be linked so PKCS#7 can hash with SHA-256.
var _ = sha256.New

// uidOID is the X.520 UID attribute Apple puts on a Pass Type ID certificate.
var uidOID = asn1.ObjectIdentifier{0, 9, 2342, 19200300, 100, 1, 1}

func checkSignature(manifest, sig []byte, roots *x509.CertPool, now time.Time) (ok, chainOK, current bool, signer Signer, problems []string) {
	if len(sig) == 0 {
		return false, false, false, Signer{}, []string{"signature is missing"}
	}
	p7, err := pkcs7.Parse(sig)
	if err != nil {
		return false, false, false, Signer{}, []string{"signature is not a PKCS#7 signature"}
	}
	if len(p7.Content) == 0 {
		p7.Content = manifest
	}
	if err := p7.Verify(); err != nil {
		if skiErr := verifySubjectKeyID(sig, manifest, p7.Certificates); skiErr == nil {
			ok = true
		} else {
			problems = append(problems, "signature does not match manifest.json")
		}
	} else {
		ok = true
	}
	leaf := leafCert(p7)
	if leaf == nil {
		problems = append(problems, "signature has no signer certificate")
		return ok, false, false, Signer{}, problems
	}
	signer = signerFrom(leaf)
	when := signingTime(p7, leaf)
	chain, rootName, chainErr := buildChain(leaf, p7.Certificates, roots, when)
	if chainErr != nil {
		problems = append(problems, "certificate does not chain to an Apple root: "+chainErr.Error())
	} else {
		chainOK = true
		signer.Root = rootName
		signer.Chain = chain
	}
	// A certificate that has not started yet cannot be trusted. One that has
	// already ended was valid when it signed; that age is not a trust failure.
	if now.Before(leaf.NotBefore) {
		problems = append(problems, "signing certificate is not valid yet")
		return ok, chainOK, false, signer, problems
	}
	return ok, chainOK, true, signer, problems
}

func leafCert(p7 *pkcs7.PKCS7) *x509.Certificate {
	if c := p7.GetOnlySigner(); c != nil {
		return c
	}
	var fallback *x509.Certificate
	for _, c := range p7.Certificates {
		if c == nil {
			continue
		}
		if strings.Contains(c.Subject.CommonName, "Pass Type ID") {
			return c
		}
		if !c.IsCA {
			fallback = c
		}
	}
	if fallback != nil {
		return fallback
	}
	if len(p7.Certificates) > 0 {
		return p7.Certificates[len(p7.Certificates)-1]
	}
	return nil
}

func signerFrom(leaf *x509.Certificate) Signer {
	org := ""
	if len(leaf.Subject.Organization) > 0 {
		org = leaf.Subject.Organization[0]
	}
	return Signer{
		Subject:      leaf.Subject.String(),
		Issuer:       leaf.Issuer.String(),
		PassTypeID:   certPassType(leaf),
		TeamID:       certTeam(leaf),
		Organization: org,
		NotBefore:    leaf.NotBefore.UTC().Format(time.RFC3339),
		NotAfter:     leaf.NotAfter.UTC().Format(time.RFC3339),
	}
}

func signingTime(p7 *pkcs7.PKCS7, leaf *x509.Certificate) time.Time {
	var when time.Time
	if err := p7.UnmarshalSignedAttribute(pkcs7.OIDAttributeSigningTime, &when); err == nil && !when.IsZero() {
		return when
	}
	if leaf != nil && !leaf.NotBefore.IsZero() {
		return leaf.NotBefore.Add(time.Minute)
	}
	return time.Now()
}

func buildChain(leaf *x509.Certificate, certs []*x509.Certificate, roots *x509.CertPool, when time.Time) ([]string, string, error) {
	if roots == nil {
		var err error
		roots, err = trust.AppleRoots()
		if err != nil {
			return nil, "", err
		}
	}
	inter := x509.NewCertPool()
	for _, c := range certs {
		if c == nil || c.Equal(leaf) {
			continue
		}
		inter.AddCert(c)
	}
	if err := trust.Intermediates(inter); err != nil {
		return nil, "", err
	}
	chains, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: inter,
		CurrentTime:   when,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	if err != nil || len(chains) == 0 {
		return nil, "", err
	}
	var names []string
	for _, c := range chains[0] {
		names = append(names, c.Subject.CommonName)
	}
	root := chains[0][len(chains[0])-1].Subject.CommonName
	return names, root, nil
}

func certPassType(c *x509.Certificate) string {
	const prefix = "Pass Type ID: "
	if strings.HasPrefix(c.Subject.CommonName, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(c.Subject.CommonName, prefix))
	}
	if uid := nameValue(c.Subject, uidOID); uid != "" {
		return uid
	}
	return ""
}

func certTeam(c *x509.Certificate) string {
	if len(c.Subject.OrganizationalUnit) > 0 {
		return c.Subject.OrganizationalUnit[0]
	}
	return ""
}

func nameValue(name pkix.Name, oid asn1.ObjectIdentifier) string {
	for _, at := range name.Names {
		if at.Type.Equal(oid) {
			if s, ok := at.Value.(string); ok {
				return s
			}
		}
	}
	return ""
}

func checkIdentity(signer Signer, passType, team string) (bool, []string) {
	var problems []string
	if signer.Subject == "" {
		return false, []string{"no signer certificate to compare with the pass"}
	}
	ok := true
	if signer.PassTypeID != passType {
		ok = false
		problems = append(problems, fmt.Sprintf("pass type %s does not match certificate %s", passType, signer.PassTypeID))
	}
	if signer.TeamID != team {
		ok = false
		problems = append(problems, fmt.Sprintf("team %s does not match certificate %s", team, signer.TeamID))
	}
	return ok, problems
}
