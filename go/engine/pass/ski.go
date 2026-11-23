package pass

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"hash"
	"math/big"
)

// verifySubjectKeyID checks a CMS signature whose signer identifier is a
// subject key identifier. github.com/digitorus/pkcs7 only reads the older
// issuer-and-serial form, and Apple passes use both.
func verifySubjectKeyID(sig, manifest []byte, certs []*x509.Certificate) error {
	var info contentInfoASN
	if _, err := asn1.Unmarshal(sig, &info); err != nil {
		return err
	}
	var signed asn1.RawValue
	if _, err := asn1.Unmarshal(info.Content.Bytes, &signed); err != nil {
		return err
	}
	fields, err := rawElements(signed.Bytes)
	if err != nil {
		return err
	}
	var signers asn1.RawValue
	for _, f := range fields {
		if f.Class == asn1.ClassUniversal && f.Tag == asn1.TagSet {
			signers = f
		}
	}
	if len(signers.Bytes) == 0 {
		return errNoSKI
	}
	infos, err := rawElements(signers.Bytes)
	if err != nil {
		return err
	}
	var last error = errNoSKI
	for _, info := range infos {
		if err := verifyOneSigner(info.Bytes, manifest, certs); err == nil {
			return nil
		} else {
			last = err
		}
	}
	return last
}

var errNoSKI = errString("signature has no subject key identifier")

type errString string

func (e errString) Error() string { return string(e) }

type contentInfoASN struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"explicit,tag:0"`
}

func verifyOneSigner(body, manifest []byte, certs []*x509.Certificate) error {
	fields, err := rawElements(body)
	if err != nil || len(fields) < 4 {
		return errNoSKI
	}
	sid := fields[1]
	var leaf *x509.Certificate
	switch {
	case sid.Class == asn1.ClassContextSpecific && sid.Tag == 0 && !sid.IsCompound:
		leaf = certBySKI(certs, sid.Bytes)
	case sid.Class == asn1.ClassUniversal && sid.Tag == asn1.TagSequence:
		var parsed struct {
			Issuer asn1.RawValue
			Serial *big.Int
		}
		if _, err := asn1.Unmarshal(sid.FullBytes, &parsed); err != nil {
			return err
		}
		leaf = certByIssuer(certs, parsed.Issuer.FullBytes, parsed.Serial)
	default:
		return errNoSKI
	}
	if leaf == nil {
		return errString("no certificate matches the signer identifier")
	}
	digestOID := firstOID(fields[2].Bytes)
	h, alg, err := hashAndAlg(digestOID, leaf.PublicKey)
	if err != nil {
		return err
	}
	idx := 3
	var attrs *asn1.RawValue
	if idx < len(fields) && fields[idx].Class == asn1.ClassContextSpecific && fields[idx].Tag == 0 && fields[idx].IsCompound {
		attrs = &fields[idx]
		idx++
	}
	if idx < len(fields) && fields[idx].Tag == asn1.TagSequence {
		idx++
	}
	if idx >= len(fields) {
		return errNoSKI
	}
	enc := fields[idx].Bytes
	signed := manifest
	if attrs != nil {
		sum := h()
		sum.Write(manifest)
		want, err := messageDigest(attrs.Bytes)
		if err != nil {
			return err
		}
		if !bytes.Equal(sum.Sum(nil), want) {
			return errString("manifest digest does not match the signature")
		}
		raw := append([]byte(nil), attrs.FullBytes...)
		raw[0] = 0x31
		signed = raw
	}
	return leaf.CheckSignature(alg, signed, enc)
}

func hashAndAlg(oid asn1.ObjectIdentifier, pub any) (func() hash.Hash, x509.SignatureAlgorithm, error) {
	switch pub.(type) {
	case *ecdsa.PublicKey:
		switch {
		case oid.Equal(oidSHA1):
			return sha1.New, x509.ECDSAWithSHA1, nil
		case oid.Equal(oidSHA256):
			return sha256.New, x509.ECDSAWithSHA256, nil
		default:
			return nil, 0, errString("unsupported signature digest")
		}
	case *rsa.PublicKey:
		switch {
		case oid.Equal(oidSHA1):
			return sha1.New, x509.SHA1WithRSA, nil
		case oid.Equal(oidSHA256):
			return sha256.New, x509.SHA256WithRSA, nil
		default:
			return nil, 0, errString("unsupported signature digest")
		}
	default:
		return nil, 0, errString("unsupported signer key")
	}
}

var (
	oidSHA1          = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}
	oidSHA256        = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidMessageDigest = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
)

func certByIssuer(certs []*x509.Certificate, issuer []byte, serial *big.Int) *x509.Certificate {
	for _, c := range certs {
		if c == nil || c.SerialNumber == nil || serial == nil || c.SerialNumber.Cmp(serial) != 0 {
			continue
		}
		if bytes.Equal(c.RawIssuer, issuer) {
			return c
		}
	}
	return nil
}

func certBySKI(certs []*x509.Certificate, ski []byte) *x509.Certificate {
	for _, c := range certs {
		if c != nil && bytes.Equal(c.SubjectKeyId, ski) {
			return c
		}
	}
	return nil
}

func messageDigest(attrBytes []byte) ([]byte, error) {
	attrs, err := rawElements(attrBytes)
	if err != nil {
		return nil, err
	}
	for _, attr := range attrs {
		parts, err := rawElements(attr.Bytes)
		if err != nil || len(parts) < 2 {
			continue
		}
		oid := firstOID(parts[0].FullBytes)
		if !oid.Equal(oidMessageDigest) {
			continue
		}
		vals, err := rawElements(parts[1].Bytes)
		if err != nil || len(vals) == 0 {
			return nil, errString("message digest attribute is empty")
		}
		return vals[0].Bytes, nil
	}
	return nil, errString("signature has no message digest")
}

func firstOID(b []byte) asn1.ObjectIdentifier {
	var oid asn1.ObjectIdentifier
	_, _ = asn1.Unmarshal(b, &oid)
	return oid
}

func rawElements(b []byte) ([]asn1.RawValue, error) {
	var out []asn1.RawValue
	for len(b) > 0 {
		var v asn1.RawValue
		rest, err := asn1.Unmarshal(b, &v)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		b = rest
	}
	return out, nil
}
