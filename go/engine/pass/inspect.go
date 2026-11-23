package pass

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Options controls verification anchors and how images and strings are chosen.
type Options struct {
	Roots    *x509.CertPool
	Now      time.Time
	Locale   string
	Scale    float64
	ImageDir string
}

// Inspect reads a .pkpass. A readable pass is returned even when a check
// fails. An error means the bytes are not a pass at all.
func Inspect(data []byte, opt Options) (*Pass, error) {
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	if opt.Scale <= 0 {
		opt.Scale = 1
	}
	files, err := readZip(data)
	if err != nil {
		return nil, err
	}
	rawJSON, ok := files["pass.json"]
	if !ok {
		return nil, fmt.Errorf("pass.json is missing")
	}
	var doc document
	if err := json.Unmarshal(rawJSON, &doc); err != nil {
		return nil, fmt.Errorf("pass.json is not valid JSON: %w", err)
	}
	var problems []string
	if doc.FormatVersion != 1 {
		problems = append(problems, fmt.Sprintf("formatVersion is %d", doc.FormatVersion))
	}
	for _, key := range []struct {
		name, val string
	}{
		{"passTypeIdentifier", doc.PassTypeIdentifier},
		{"serialNumber", doc.SerialNumber},
		{"teamIdentifier", doc.TeamIdentifier},
		{"organizationName", doc.OrganizationName},
		{"description", doc.Description},
	} {
		if strings.TrimSpace(key.val) == "" {
			problems = append(problems, key.name+" is missing")
		}
	}
	if doc.WebServiceURL != "" && len(doc.AuthenticationToken) < 16 {
		problems = append(problems, "authenticationToken must be at least 16 characters when webServiceURL is set")
	}
	if doc.WebServiceURL == "" && doc.AuthenticationToken != "" {
		problems = append(problems, "authenticationToken is set without webServiceURL")
	}

	styleName, style, styleProblems := pickStyle(&doc)
	problems = append(problems, styleProblems...)

	manifestOK, manifestProblems := checkManifest(files, files["manifest.json"])
	problems = append(problems, manifestProblems...)

	sigOK, chainOK, current, signer, sigProblems := checkSignature(files["manifest.json"], files["signature"], opt.Roots, opt.Now)
	problems = append(problems, sigProblems...)
	idOK, idProblems := checkIdentity(signer, doc.PassTypeIdentifier, doc.TeamIdentifier)
	problems = append(problems, idProblems...)

	catalogs := loadCatalogs(files)
	cat := pickCatalog(catalogs, opt.Locale)

	bg := colorOrDefault(doc.BackgroundColor, "#FFFFFF", &problems, "background")
	fg := colorOrDefault(doc.ForegroundColor, "#000000", &problems, "foreground")
	label := colorOrDefault(doc.LabelColor, "#555555", &problems, "label")
	footerBg := ""
	if strings.TrimSpace(doc.FooterBackgroundColor) != "" {
		footerBg = colorOrDefault(doc.FooterBackgroundColor, bg, &problems, "footer background")
	}

	if _, ok := files["icon.png"]; !ok && !hasLocalized(files, "icon.png") {
		problems = append(problems, "icon.png is missing")
	}

	images := chooseImages(files, opt.Locale, opt.Scale, opt.ImageDir)
	expired := false
	if t, ok := parseTime(doc.ExpirationDate); ok && opt.Now.After(t) {
		expired = true
	}

	out := &Pass{
		ID:                    ID(doc.PassTypeIdentifier, doc.SerialNumber),
		Readable:              true,
		Style:                 styleName,
		OrganizationName:      translate(cat, doc.OrganizationName),
		Description:           translate(cat, doc.Description),
		LogoText:              translate(cat, doc.LogoText),
		BackgroundColor:       bg,
		ForegroundColor:       fg,
		LabelColor:            label,
		FooterBackgroundColor: footerBg,
		Voided:                doc.Voided,
		Expired:               expired,
		ExpirationDate:        doc.ExpirationDate,
		RelevantDate:          doc.RelevantDate,
		SharingProhibited:     doc.SharingProhibited,
		GroupingIdentifier:    doc.GroupingIdentifier,
		Images:                images,
		Barcode:               chooseBarcode(&doc),
		HasWebService:         doc.WebServiceURL != "" && doc.AuthenticationToken != "",
		WebServiceURL:         doc.WebServiceURL,
		AuthenticationToken:   doc.AuthenticationToken,
		Locations:             doc.Locations,
		Beacons:               doc.Beacons,
		PassTypeIdentifier:    doc.PassTypeIdentifier,
		SerialNumber:          doc.SerialNumber,
		TeamIdentifier:        doc.TeamIdentifier,
	}
	if style != nil {
		out.TransitType = style.TransitType
		out.HeaderFields = fields(style.HeaderFields, opt.Now, cat)
		out.PrimaryFields = fields(style.PrimaryFields, opt.Now, cat)
		out.SecondaryFields = fields(style.SecondaryFields, opt.Now, cat)
		out.AuxiliaryFields = fields(style.AuxiliaryFields, opt.Now, cat)
		out.BackFields = fields(style.BackFields, opt.Now, cat)
		out.AdditionalInfoFields = fields(style.AdditionalInfoFields, opt.Now, cat)
	}
	if doc.NFC != nil {
		out.NFC = &NFCView{
			Message:                doc.NFC.Message,
			RequiresAuthentication: doc.NFC.RequiresAuthentication,
			HasEncryptionKey:       doc.NFC.EncryptionPublicKey != "",
		}
	}
	trusted := manifestOK && sigOK && chainOK && idOK && current && doc.FormatVersion == 1 && len(styleProblems) == 0
	out.Trusted = trusted
	out.Verification = Verification{
		ManifestOK:  manifestOK,
		SignatureOK: sigOK,
		ChainOK:     chainOK,
		IdentityOK:  idOK,
		Trusted:     trusted,
		Problems:    problems,
		Signer:      signer,
	}
	return out, nil
}

// OrganizationName reads the displayed organization name. It opens pass.json
// and string catalogs only, so pass images stay compressed in the archive.
func OrganizationName(data []byte, locale string) string {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ""
	}
	var rawJSON []byte
	catalogs := map[string]map[string]string{}
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name, ok := cleanZipName(f.Name)
		if !ok {
			continue
		}
		if name == "pass.json" {
			rawJSON = readZipEntry(f)
			continue
		}
		if name != "pass.strings" && !strings.HasSuffix(name, "/pass.strings") {
			continue
		}
		b := readZipEntry(f)
		if len(b) == 0 {
			continue
		}
		lang := "en"
		if i := strings.Index(name, ".lproj/"); i >= 0 {
			lang = name[:i]
		}
		catalogs[normLang(lang)] = parseStrings(b)
	}
	if len(rawJSON) == 0 {
		return ""
	}
	var doc struct {
		OrganizationName string `json:"organizationName"`
	}
	if json.Unmarshal(rawJSON, &doc) != nil {
		return ""
	}
	return translate(pickCatalog(catalogs, locale), doc.OrganizationName)
}

func readZipEntry(f *zip.File) []byte {
	rc, err := f.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()
	b, err := ReadLimited(rc, 1<<20)
	if err != nil {
		return nil
	}
	return b
}

// ID is the stable wallet key for a pass type and serial number.
func ID(passType, serial string) string {
	sum := sha256.Sum256([]byte(passType + "\x00" + serial))
	return hex.EncodeToString(sum[:16])
}

func pickStyle(doc *document) (string, *styleDoc, []string) {
	type pair struct {
		name string
		doc  *styleDoc
	}
	var found []pair
	for _, p := range []pair{
		{"boardingPass", doc.BoardingPass},
		{"coupon", doc.Coupon},
		{"eventTicket", doc.EventTicket},
		{"storeCard", doc.StoreCard},
		{"generic", doc.Generic},
	} {
		if p.doc != nil {
			found = append(found, p)
		}
	}
	switch len(found) {
	case 0:
		return "", nil, []string{"pass has no style"}
	case 1:
		if found[0].name == "boardingPass" && found[0].doc.TransitType == "" {
			return found[0].name, found[0].doc, []string{"boarding pass is missing transitType"}
		}
		return found[0].name, found[0].doc, nil
	default:
		names := make([]string, len(found))
		for i, f := range found {
			names[i] = f.name
		}
		return found[0].name, found[0].doc, []string{"pass has more than one style: " + strings.Join(names, ", ")}
	}
}

func fields(in []fieldDoc, now time.Time, cat map[string]string) []FieldView {
	if len(in) == 0 {
		return []FieldView{}
	}
	out := make([]FieldView, 0, len(in))
	for _, f := range in {
		view := formatField(f, now, cat)
		view.Label = translate(cat, f.Label)
		if s, ok := f.Value.(string); ok {
			if f.AttributedValue == nil && f.DateStyle == "" && f.TimeStyle == "" && f.CurrencyCode == "" {
				view.Value = translate(cat, s)
			}
		}
		if s, ok := f.AttributedValue.(string); ok && f.DateStyle == "" && f.TimeStyle == "" {
			view.Value = plainText(translate(cat, s))
		}
		out = append(out, view)
	}
	return out
}

func loadCatalogs(files map[string][]byte) map[string]map[string]string {
	out := map[string]map[string]string{}
	for name, b := range files {
		if !strings.HasSuffix(name, "/pass.strings") && name != "pass.strings" {
			continue
		}
		lang := "en"
		if i := strings.Index(name, ".lproj/"); i >= 0 {
			lang = name[:i]
		}
		out[normLang(lang)] = parseStrings(b)
	}
	return out
}

func hasLocalized(files map[string][]byte, base string) bool {
	for name := range files {
		if strings.HasSuffix(name, "/"+base) {
			return true
		}
	}
	return false
}

func chooseImages(files map[string][]byte, locale string, scale float64, dir string) Images {
	return Images{
		Icon:       placeImage(files, "icon", locale, scale, dir),
		Logo:       placeImage(files, "logo", locale, scale, dir),
		Strip:      placeImage(files, "strip", locale, scale, dir),
		Thumbnail:  placeImage(files, "thumbnail", locale, scale, dir),
		Background: placeImage(files, "background", locale, scale, dir),
		Footer:     placeImage(files, "footer", locale, scale, dir),
	}
}

func placeImage(files map[string][]byte, family, locale string, scale float64, dir string) string {
	name, data := chooseImage(files, family, locale, scale)
	if len(data) == 0 {
		return ""
	}
	if dir == "" {
		return pngDataURL(data)
	}
	base := path.Base(name)
	dest := filepath.Join(dir, base)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return pngDataURL(data)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return pngDataURL(data)
	}
	abs, err := filepath.Abs(dest)
	if err != nil {
		return dest
	}
	return abs
}

func chooseImage(files map[string][]byte, family, locale string, scale float64) (string, []byte) {
	var suffixes []string
	switch {
	case scale >= 2.5:
		suffixes = []string{"@3x", "@2x", ""}
	case scale >= 1.5:
		suffixes = []string{"@2x", "@3x", ""}
	default:
		suffixes = []string{"", "@2x", "@3x"}
	}
	langs := []string{}
	want := normLang(locale)
	base := want
	if i := strings.IndexByte(want, '-'); i >= 0 {
		base = want[:i]
	}
	for _, lang := range []string{want, base, "en"} {
		if lang != "" {
			langs = append(langs, lang)
		}
	}
	for _, suf := range suffixes {
		file := family + suf + ".png"
		if b, ok := files[file]; ok {
			return file, b
		}
		for _, lang := range langs {
			for _, key := range []string{lang + ".lproj/" + file, strings.ReplaceAll(lang, "-", "_") + ".lproj/" + file} {
				if b, ok := files[key]; ok {
					return key, b
				}
			}
		}
	}
	return "", nil
}

func readZip(data []byte) (map[string][]byte, error) {
	entries, err := OpenZip(data)
	if err != nil {
		return nil, err
	}
	files := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		files[entry.Name] = entry.Body
	}
	return files, nil
}

func cleanZipName(name string) (string, bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "/")
	if name == "" || strings.Contains(name, "../") || name == ".." {
		return "", false
	}
	clean := path.Clean(name)
	if clean == "." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

// DiffFields returns field values that differ, for an update history entry.
func DiffFields(oldPass, newPass *Pass) (changes []FieldChange, messages []string) {
	oldMap := fieldMap(oldPass)
	seen := map[string]bool{}
	for _, f := range allFields(newPass) {
		seen[f.Key] = true
		prev, ok := oldMap[f.Key]
		if ok && prev == f.Value {
			continue
		}
		changes = append(changes, FieldChange{Key: f.Key, Label: f.Label, From: prev, To: f.Value})
		if msg := applyChangeMessage(f.ChangeMessage, f.Value); msg != "" {
			messages = append(messages, msg)
		}
	}
	for key, prev := range oldMap {
		if seen[key] {
			continue
		}
		changes = append(changes, FieldChange{Key: key, From: prev})
	}
	return changes, messages
}

// FieldChange is one displayed value that moved between two revisions.
type FieldChange struct {
	Key   string `json:"key"`
	Label string `json:"label,omitempty"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
}

func fieldMap(p *Pass) map[string]string {
	out := map[string]string{}
	if p == nil {
		return out
	}
	for _, f := range allFields(p) {
		out[f.Key] = f.Value
	}
	return out
}

func allFields(p *Pass) []FieldView {
	if p == nil {
		return nil
	}
	var out []FieldView
	out = append(out, p.HeaderFields...)
	out = append(out, p.PrimaryFields...)
	out = append(out, p.SecondaryFields...)
	out = append(out, p.AuxiliaryFields...)
	out = append(out, p.BackFields...)
	out = append(out, p.AdditionalInfoFields...)
	return out
}

// PNGBase64 encodes barcode bytes for JSON.
func PNGBase64(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}
