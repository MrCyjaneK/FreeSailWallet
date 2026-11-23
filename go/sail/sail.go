// Package sail is the QML-facing wallet. Pass bytes, verification, barcode
// drawing, and PassKit refresh come from harbour-freesailwallet/engine.
package sail

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/aztec"
	"github.com/boombuler/barcode/codabar"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/code39"
	"github.com/boombuler/barcode/datamatrix"
	"github.com/boombuler/barcode/ean"
	"github.com/boombuler/barcode/pdf417"
	"github.com/boombuler/barcode/qr"
	"github.com/boombuler/barcode/twooffive"

	"harbour-freesailwallet/engine/pass"
	"harbour-freesailwallet/engine/wallet"
	"harbour-freesailwallet/sailpkg"
)

const ownType = "net.mrcyjanek.freesailwallet.owncard-v2"

func isOwnCard(passType string) bool {
	return strings.HasPrefix(passType, "net.mrcyjanek.freesailwallet.owncard")
}

// Engine is the process-wide wallet.
type Engine struct {
	mu         sync.Mutex
	root       string
	downloads  string
	store      *wallet.Store
	openErr    string
	internet   bool
	currency   string
	auto       map[string]bool
	preview    *pass.Pass
	previewRaw []byte
	previewOwn map[string]any
	pageHTML   string
	pageURL    string
	downloaded []string
	cancel     bool
}

var eng = &Engine{auto: map[string]bool{}, currency: "EUR"}

// Call runs one QML method. args is a JSON object.
func Call(method, args string) string {
	var in map[string]any
	if strings.TrimSpace(args) != "" {
		_ = json.Unmarshal([]byte(args), &in)
	}
	if in == nil {
		in = map[string]any{}
	}
	return string(mustJSON(eng.dispatch(method, in)))
}

// ImagePNG draws a pass image or an own-card icon.
func ImagePNG(key, name string, w, h int) []byte {
	eng.mu.Lock()
	defer eng.mu.Unlock()
	if key == "icon" {
		return iconPNG(name)
	}
	family := name
	if i := strings.IndexByte(name, '/'); i >= 0 {
		family = name[:i]
	}
	var p *pass.Pass
	if key == "preview" {
		p = eng.preview
	} else if eng.store != nil {
		got, err := eng.store.Get(key)
		if err == nil {
			p = got
		}
	}
	if p == nil {
		return nil
	}
	if family == "barcode" {
		return barcodePNG(p, w, h)
	}
	path := imagePath(p, family)
	return readImage(path)
}

func (e *Engine) dispatch(method string, in map[string]any) any {
	switch method {
	case "originalPackage":
		info, err := sailpkg.Original()
		if err != nil {
			return map[string]string{"error": err.Error()}
		}
		return info
	case "open":
		return e.open(str(in["dataDir"]), str(in["downloadsDir"]), str(in["locale"]), num(in["scale"]), str(in["currency"]))
	case "setInternetAllowed":
		e.mu.Lock()
		e.internet = truth(in["value"])
		e.mu.Unlock()
		return map[string]bool{"ok": true}
	case "status":
		return e.status()
	case "list":
		return e.list(truth(in["archived"]))
	case "get":
		return e.get(str(in["passId"]))
	case "removePass":
		return map[string]bool{"ok": e.remove(str(in["passId"]))}
	case "archive":
		return map[string]bool{"ok": e.setArchived(str(in["passId"]), true)}
	case "unarchive":
		return map[string]bool{"ok": e.setArchived(str(in["passId"]), false)}
	case "bringToFront":
		return e.bringToFront(str(in["passId"]))
	case "setAutoUpdate":
		e.setAuto(str(in["passId"]), truth(in["enabled"]))
		return map[string]bool{"ok": true}
	case "startEmptyWallet":
		return map[string]string{"file": e.emptyWallet()}
	case "ownCard":
		return e.ownCard(str(in["passId"]))
	case "shareResource":
		return e.sharePass(str(in["passId"]))
	case "inspectFile":
		return e.inspectFile(str(in["path"]))
	case "pendingUpdates":
		return e.pendingUpdates(paths(in["paths"]))
	case "importFiles":
		return e.importFiles(paths(in["paths"]), truth(in["replaceExisting"]))
	case "addInspected":
		return e.addInspected(truth(in["autoUpdate"]))
	case "replaceOwnCard":
		return e.replaceOwnCard(str(in["passId"]))
	case "inspectOwnCard":
		return e.inspectOwnCard(in)
	case "ownCardColors":
		return colors
	case "ownCardIcons":
		return iconList(str(in["templateId"]))
	case "ownCardCurrencies":
		return currencies
	case "defaultOwnCardCurrency":
		e.mu.Lock()
		c := e.currency
		e.mu.Unlock()
		if c == "" {
			c = "EUR"
		}
		return c
	case "formatOwnCardDate":
		return formatDate(str(in["isoDate"]))
	case "newOwnCardSerialNumber":
		return newSerial()
	case "codeTypes":
		return codeTypes()
	case "isWebAddress":
		return isWeb(str(in["text"]))
	case "createBackup":
		return e.createBackup()
	case "inspectBackup":
		return e.inspectBackup(str(in["path"]))
	case "restoreBackup":
		return e.restoreBackup(str(in["path"]))
	case "shareBackup":
		return shareFile(str(in["filePath"]))
	case "download":
		return e.download(str(in["url"]))
	case "findPassOnPage":
		return e.findOnPage()
	case "removeDownloaded":
		e.removeDownloaded()
		return map[string]bool{"ok": true}
	case "cancel":
		e.mu.Lock()
		e.cancel = true
		e.mu.Unlock()
		return map[string]bool{"ok": true}
	case "updatePass":
		return e.updateOne(str(in["passId"]), true)
	case "updateAll":
		return e.updateAll()
	case "scan":
		return map[string]any{"found": false}
	default:
		return map[string]string{"error": "unknown method"}
	}
}

func (e *Engine) open(dataDir, downloads, locale string, scale float64, currency string) any {
	e.mu.Lock()
	defer e.mu.Unlock()
	if dataDir == "" {
		e.openErr = "no data directory"
		return map[string]any{"ok": false}
	}
	st, err := wallet.Open(dataDir)
	if err != nil {
		e.openErr = err.Error()
		e.store = nil
		return map[string]any{"ok": false, "error": err.Error()}
	}
	if locale == "" {
		locale = "en"
	}
	if scale <= 0 {
		scale = 2
	}
	st.SetDisplay(locale, scale)
	e.store = st
	e.root = dataDir
	e.downloads = downloads
	e.openErr = ""
	if len(currency) == 3 {
		e.currency = strings.ToUpper(currency)
	}
	e.auto = map[string]bool{}
	if b, err := os.ReadFile(filepath.Join(dataDir, "autoupdate.json")); err == nil {
		_ = json.Unmarshal(b, &e.auto)
	}
	if e.auto == nil {
		e.auto = map[string]bool{}
	}
	return map[string]any{"ok": true}
}

func (e *Engine) status() any {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.store == nil {
		return map[string]any{
			"storageAvailable": false, "loadFailed": true, "passCount": 0,
			"activeCount": 0, "archivedCount": 0,
			"newerDatabase": false, "newerDatabaseVersion": "",
		}
	}
	list, err := e.store.List()
	active, archived := 0, 0
	if err == nil {
		active = len(list.Active)
		archived = len(list.Archived)
	}
	return map[string]any{
		"storageAvailable": true, "loadFailed": false,
		"passCount": active + archived, "activeCount": active, "archivedCount": archived,
		"newerDatabase": false, "newerDatabaseVersion": "",
	}
}

func (e *Engine) list(archived bool) any {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.store == nil {
		return []any{}
	}
	list, err := e.store.List()
	if err != nil {
		return []any{}
	}
	items := list.Active
	if archived {
		items = list.Archived
	}
	// The stack paints the last card in front. Home index 0 is the front card.
	var rows []any
	for i := len(items) - 1; i >= 0; i-- {
		if m := e.row(items[i].ID); m != nil {
			rows = append(rows, m)
		}
	}
	if rows == nil {
		rows = []any{}
	}
	return rows
}

func (e *Engine) get(id string) any {
	e.mu.Lock()
	defer e.mu.Unlock()
	if m := e.row(id); m != nil {
		return m
	}
	return map[string]any{}
}

func (e *Engine) row(id string) map[string]any {
	if e.store == nil || id == "" {
		return nil
	}
	p, err := e.store.Get(id)
	if err != nil || p == nil {
		return nil
	}
	return passMap(p, id, e.auto[id], e.store.IsArchived(id))
}

func (e *Engine) setArchived(id string, archived bool) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.store == nil || id == "" {
		return false
	}
	var err error
	if archived {
		err = e.store.Archive(id)
	} else {
		err = e.store.Unarchive(id)
	}
	return err == nil
}

// bringToFront places an active pass at the front of the home stack.
// Home index 0 is that front card, which the stack paints at the bottom.
func (e *Engine) bringToFront(id string) any {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.store == nil || id == "" {
		return map[string]any{"ok": false, "moved": false}
	}
	moved, err := e.store.BringToFront(id)
	if err != nil {
		return map[string]any{"ok": false, "moved": false}
	}
	return map[string]any{"ok": true, "moved": moved}
}

func (e *Engine) remove(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.store == nil {
		return false
	}
	if err := e.store.Delete(id); err != nil {
		return false
	}
	delete(e.auto, id)
	e.saveAuto()
	return true
}

func (e *Engine) setAuto(id string, on bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if on {
		e.auto[id] = true
	} else {
		delete(e.auto, id)
	}
	e.saveAuto()
}

func (e *Engine) saveAuto() {
	if e.root == "" {
		return
	}
	b, _ := json.MarshalIndent(e.auto, "", "  ")
	_ = os.WriteFile(filepath.Join(e.root, "autoupdate.json"), b, 0o644)
}

func (e *Engine) emptyWallet() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.store == nil {
		return ""
	}
	res, err := e.store.Export()
	name := ""
	if err == nil && res != nil && res.Path != "" {
		name = e.copyDownload(res.Path, "freesailwallet-"+time.Now().Format("20060102-150405")+".swbackup")
	}
	_ = os.RemoveAll(e.root)
	st, err := wallet.Open(e.root)
	if err != nil {
		e.store = nil
		e.openErr = err.Error()
		return name
	}
	e.store = st
	e.auto = map[string]bool{}
	return name
}

func (e *Engine) copyDownload(src, name string) string {
	if e.downloads == "" {
		return filepath.Base(src)
	}
	if err := os.MkdirAll(e.downloads, 0o755); err != nil {
		return ""
	}
	in, err := os.ReadFile(src)
	if err != nil {
		return ""
	}
	dest := filepath.Join(e.downloads, name)
	if err := os.WriteFile(dest, in, 0o644); err != nil {
		return ""
	}
	return name
}

func passMap(p *pass.Pass, id string, auto, archived bool) map[string]any {
	logo := p.LogoText
	if logo == "" {
		logo = p.OrganizationName
	}
	format, alt := "", ""
	if p.Barcode != nil {
		format = p.Barcode.Format
		alt = p.Barcode.AltText
	}
	w, h := imageSize(p.Images.Logo)
	return map[string]any{
		"passId": id, "imageKey": id,
		"passStyle": p.Style, "transitType": p.TransitType,
		"logoText": logo, "passLogoText": p.LogoText,
		"ownCard":            isOwnCard(p.PassTypeIdentifier),
		"logoSize":           map[string]int{"width": w, "height": h},
		"initials":           initials(logo),
		"hasLogo":            p.Images.Logo != "",
		"hasThumbnail":       p.Images.Thumbnail != "",
		"hasStrip":           p.Images.Strip != "",
		"hasBackground":      p.Images.Background != "",
		"hasFooter":          p.Images.Footer != "",
		"barcodeFormat":      format,
		"barcodeShape":       shapeOf(format),
		"barcodeAltText":     alt,
		"voided":             p.Voided,
		"expired":            p.Expired,
		"expirationTime":     expMillis(p.ExpirationDate),
		"expirationDate":     p.ExpirationDate,
		"relevantDate":       p.RelevantDate,
		"serialNumber":       p.SerialNumber,
		"passTypeIdentifier": p.PassTypeIdentifier,
		"teamIdentifier":     p.TeamIdentifier,
		"webServiceURL":      p.WebServiceURL,
		"verification":       verificationMap(p.Verification),
		"backgroundColor":    p.BackgroundColor,
		"foregroundColor":    p.ForegroundColor,
		"labelColor":         p.LabelColor,
		"headerFields":       fieldList(p.HeaderFields),
		"primaryFields":      fieldList(p.PrimaryFields),
		"secondaryFields":    fieldList(p.SecondaryFields),
		"auxiliaryFields":    fieldList(p.AuxiliaryFields),
		"backFields":         fieldList(p.BackFields),
		"passDescription":    p.Description,
		"organizationName":   p.OrganizationName,
		"canUpdate":          p.HasWebService,
		"canShare":           !p.SharingProhibited,
		"autoUpdate":         auto,
		"archived":           archived,
	}
}

func verificationMap(v pass.Verification) map[string]any {
	problems := v.Problems
	if problems == nil {
		problems = []string{}
	}
	chain := v.Signer.Chain
	if chain == nil {
		chain = []string{}
	}
	return map[string]any{
		"manifestOK":   v.ManifestOK,
		"signatureOK":  v.SignatureOK,
		"chainOK":      v.ChainOK,
		"identityOK":   v.IdentityOK,
		"trusted":      v.Trusted,
		"problems":     problems,
		"subject":      v.Signer.Subject,
		"issuer":       v.Signer.Issuer,
		"organization": v.Signer.Organization,
		"passTypeId":   v.Signer.PassTypeID,
		"teamId":       v.Signer.TeamID,
		"notBefore":    v.Signer.NotBefore,
		"notAfter":     v.Signer.NotAfter,
		"root":         v.Signer.Root,
		"chain":        chain,
	}
}

func fieldList(in []pass.FieldView) []any {
	out := make([]any, 0, len(in))
	for _, f := range in {
		align := "PKTextAlignmentRight"
		switch f.Alignment {
		case "left":
			align = "PKTextAlignmentLeft"
		case "center":
			align = "PKTextAlignmentCenter"
		}
		out = append(out, map[string]any{
			"label": f.Label, "value": f.Value, "textAlignment": align,
		})
	}
	return out
}

func imagePath(p *pass.Pass, family string) string {
	switch family {
	case "logo":
		return p.Images.Logo
	case "strip":
		return p.Images.Strip
	case "thumbnail":
		return p.Images.Thumbnail
	case "background":
		return p.Images.Background
	case "footer":
		return p.Images.Footer
	case "icon":
		return p.Images.Icon
	default:
		return ""
	}
}

func readImage(path string) []byte {
	if path == "" {
		return nil
	}
	if strings.HasPrefix(path, "data:image") {
		i := strings.Index(path, ",")
		if i < 0 {
			return nil
		}
		b, err := base64.StdEncoding.DecodeString(path[i+1:])
		if err != nil {
			return nil
		}
		return b
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return b
}

func imageSize(path string) (int, int) {
	b := readImage(path)
	if len(b) == 0 {
		return 0, 0
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return 0, 0
	}
	div := 1
	if strings.Contains(path, "@3x") {
		div = 3
	} else if strings.Contains(path, "@2x") {
		div = 2
	}
	return cfg.Width / div, cfg.Height / div
}

func expMillis(s string) int64 {
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04Z07:00", "2006-01-02T15:04:05Z07:00", "2006-01-02"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

func initials(s string) string {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return ""
	}
	out := ""
	for _, p := range parts {
		r := []rune(p)
		if len(r) > 0 {
			out += strings.ToUpper(string(r[0]))
		}
		if len([]rune(out)) >= 2 {
			break
		}
	}
	return out
}

func shapeOf(format string) string {
	switch format {
	case "PKBarcodeFormatQR", "PKBarcodeFormatAztec", "SWBarcodeFormatDataMatrix":
		return "square"
	case "PKBarcodeFormatPDF417":
		return "stacked"
	case "":
		return ""
	default:
		return "linear"
	}
}

func barcodePNG(p *pass.Pass, w, h int) []byte {
	if p.Barcode == nil || p.Barcode.Message == "" {
		return nil
	}
	size := w
	if h > size {
		size = h
	}
	if size < 64 {
		size = 280
	}
	if png, err := pass.BarcodePNG(p.Barcode, size); err == nil && len(png) > 0 {
		return png
	}
	return encodePNG(p.Barcode.Format, p.Barcode.Message, size)
}

func encodePNG(format, message string, size int) []byte {
	code, err := encode(format, message)
	if err != nil || code == nil {
		return nil
	}
	b := code.Bounds()
	nw, nh := size, size
	if b.Dx() > b.Dy() {
		nh = b.Dy() * size / b.Dx()
		if nh < 40 {
			nh = 40
		}
	}
	img, err := barcode.Scale(code, nw, nh)
	if err != nil {
		return nil
	}
	var buf bytes.Buffer
	if png.Encode(&buf, img) != nil {
		return nil
	}
	return buf.Bytes()
}

func encode(format, message string) (barcode.Barcode, error) {
	switch format {
	case "PKBarcodeFormatQR":
		return qr.Encode(message, qr.M, qr.Auto)
	case "PKBarcodeFormatPDF417":
		return pdf417.Encode(message, 4)
	case "PKBarcodeFormatAztec":
		return aztec.Encode([]byte(message), 23, 0)
	case "PKBarcodeFormatCode128":
		return code128.Encode(message)
	case "SWBarcodeFormatEAN13", "SWBarcodeFormatEAN8", "SWBarcodeFormatUPCA", "SWBarcodeFormatUPCE":
		return ean.Encode(message)
	case "SWBarcodeFormatCode39":
		return code39.Encode(message, false, false)
	case "SWBarcodeFormatCode93":
		return code39.Encode(message, false, false)
	case "SWBarcodeFormatITF":
		return twooffive.Encode(message, true)
	case "SWBarcodeFormatCodabar":
		return codabar.Encode(message)
	case "SWBarcodeFormatDataMatrix":
		return datamatrix.Encode(message)
	default:
		return nil, fmt.Errorf("unsupported")
	}
}

func codeValid(format, message string) bool {
	message = strings.TrimSpace(message)
	if message == "" {
		return false
	}
	_, err := encode(format, message)
	return err == nil
}

var colors = []string{
	"#1f7a4d", "#2a6fdb", "#6b3fa0", "#d9731a", "#c0392b", "#1a1a1a",
	"#0e6b6b", "#8a6a2f", "#3d5a80", "#b85c38", "#2c3e50", "#6b2d5b",
	"#1e8449", "#1a5276", "#7d3c98", "#b9770e",
}

func iconList(template string) []any {
	ids := []string{"cart", "heart", "coffee", "fuel", "percent", "bag", "gift"}
	switch template {
	case "membership":
		ids = []string{"person", "group", "dumbbell", "swim", "bike", "tennis", "trophy"}
	case "ticket":
		ids = []string{"ticket", "ticketcard", "masks", "popcorn", "microphone", "trophy", "star"}
	case "coupon":
		ids = []string{"scissors", "percent", "discount", "gift", "pricetag", "euro", "bag"}
	}
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, map[string]string{"id": id, "source": "image://pass/icon/" + id})
	}
	return out
}

func iconPNG(id string) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	ink := color.RGBA{255, 255, 255, 255}
	n := 0
	for _, r := range id {
		n += int(r)
	}
	for y := 4; y < 28; y++ {
		for x := 4; x < 28; x++ {
			on := false
			switch n % 5 {
			case 0:
				on = (x-16)*(x-16)+(y-16)*(y-16) < 70
			case 1:
				on = y > 20 || (x > 8 && x < 24 && y > 8 && y < 18)
			case 2:
				on = abs(x-16) < 3 || abs(y-16) < 3
			case 3:
				on = abs(x-y) < 3 || abs(x+y-32) < 3
			default:
				on = (x/4+y/4)%2 == 0 && x > 8 && x < 24 && y > 8 && y < 24
			}
			if on {
				img.Set(x, y, ink)
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

var currencies = []any{
	map[string]string{"code": "EUR", "symbol": "€"},
	map[string]string{"code": "USD", "symbol": "$"},
	map[string]string{"code": "GBP", "symbol": "£"},
	map[string]string{"code": "CHF", "symbol": "CHF"},
	map[string]string{"code": "PLN", "symbol": "zł"},
	map[string]string{"code": "CZK", "symbol": "Kč"},
	map[string]string{"code": "SEK", "symbol": "kr"},
	map[string]string{"code": "NOK", "symbol": "kr"},
	map[string]string{"code": "DKK", "symbol": "kr"},
	map[string]string{"code": "JPY", "symbol": "¥"},
}

func formatDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("2 Jan 2006")
}

func newSerial() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func codeTypes() []any {
	type row struct{ format, name, shape string }
	rows := []row{
		{"SWBarcodeFormatEAN13", "EAN-13", "linear"},
		{"SWBarcodeFormatEAN8", "EAN-8", "linear"},
		{"SWBarcodeFormatUPCA", "UPC-A", "linear"},
		{"SWBarcodeFormatUPCE", "UPC-E", "linear"},
		{"SWBarcodeFormatCode39", "Code 39", "linear"},
		{"SWBarcodeFormatCode93", "Code 93", "linear"},
		{"PKBarcodeFormatCode128", "Code 128", "linear"},
		{"SWBarcodeFormatITF", "ITF", "linear"},
		{"SWBarcodeFormatCodabar", "Codabar", "linear"},
		{"PKBarcodeFormatQR", "QR", "square"},
		{"SWBarcodeFormatDataMatrix", "Data Matrix", "square"},
		{"PKBarcodeFormatAztec", "Aztec", "square"},
		{"PKBarcodeFormatPDF417", "PDF417", "stacked"},
	}
	out := make([]any, len(rows))
	for i, r := range rows {
		out[i] = map[string]string{"format": r.format, "name": r.name, "shape": r.shape}
	}
	return out
}

func isWeb(s string) bool {
	s = strings.TrimSpace(s)
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func (e *Engine) inspectFile(path string) any {
	path = cleanPath(path)
	b, err := os.ReadFile(path)
	if err != nil {
		return failFile(filepath.Base(path), "unreadable")
	}
	if len(b) > 32<<20 {
		return failFile(filepath.Base(path), "tooLarge")
	}
	p, err := pass.Inspect(b, pass.Options{})
	if err != nil {
		return failFile(filepath.Base(path), importReason(err))
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	realID := p.ID
	e.preview = p
	e.preview.ID = "preview"
	e.previewRaw = b
	e.previewOwn = nil
	out := map[string]any{
		"status": "new", "fileName": filepath.Base(path),
		"pass": passMap(p, "preview", false, false),
	}
	if e.store != nil {
		if cur, err := os.ReadFile(currentPass(e.root, realID)); err == nil {
			if bytes.Equal(cur, b) {
				out["status"] = "existing"
				out["existingPassId"] = realID
				delete(out, "pass")
			} else {
				out["status"] = "update"
			}
		}
	}
	return out
}

func currentPass(root, id string) string {
	return filepath.Join(root, "passes", id, "current.pkpass")
}

func (e *Engine) pendingUpdates(list []string) any {
	var cards []any
	for _, path := range list {
		path = cleanPath(path)
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, part := range partsOf(b) {
			p, err := pass.Inspect(part, pass.Options{})
			if err != nil || e.store == nil {
				continue
			}
			cur, err := os.ReadFile(currentPass(e.root, p.ID))
			if err != nil || bytes.Equal(cur, part) {
				continue
			}
			cards = append(cards, map[string]string{
				"description": p.Description, "organizationName": p.OrganizationName,
				"fileName": filepath.Base(path),
			})
		}
	}
	if cards == nil {
		cards = []any{}
	}
	return cards
}

func (e *Engine) importFiles(list []string, replace bool) any {
	var results []any
	for _, path := range list {
		path = cleanPath(path)
		name := filepath.Base(path)
		b, err := os.ReadFile(path)
		if err != nil {
			results = append(results, result(name, "failed", "unreadable", "", ""))
			continue
		}
		if len(b) > 32<<20 {
			results = append(results, result(name, "failed", "tooLarge", "", ""))
			continue
		}
		parts := partsOf(b)
		if len(parts) == 0 {
			results = append(results, result(name, "failed", importReason(fmt.Errorf("not a pkpass")), "", ""))
			continue
		}
		for _, part := range parts {
			results = append(results, e.importOne(name, part, replace))
		}
	}
	if results == nil {
		results = []any{}
	}
	return results
}

func (e *Engine) importOne(name string, part []byte, replace bool) any {
	p, err := pass.Inspect(part, pass.Options{})
	if err != nil {
		return result(name, "failed", importReason(err), "", "")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.store == nil {
		return result(name, "failed", "storage", p.Description, p.OrganizationName)
	}
	cur, readErr := os.ReadFile(currentPass(e.root, p.ID))
	existed := readErr == nil
	if existed && (bytes.Equal(cur, part) || !replace) {
		return result(name, "existing", "", p.Description, p.OrganizationName)
	}
	if _, err := e.store.Import(part); err != nil {
		return result(name, "failed", importReason(err), p.Description, p.OrganizationName)
	}
	status := "added"
	if existed {
		status = "updated"
	}
	_ = e.writeOwn(p.ID, nil)
	return result(name, status, "", p.Description, p.OrganizationName)
}

func (e *Engine) addInspected(auto bool) any {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.store == nil {
		return map[string]any{"status": "failed", "reason": "storage"}
	}
	if len(e.previewRaw) == 0 || e.preview == nil {
		return map[string]any{"status": "failed", "reason": "damaged"}
	}
	realID := pass.ID(e.preview.PassTypeIdentifier, e.preview.SerialNumber)
	existed := false
	if _, err := os.Stat(currentPass(e.root, realID)); err == nil {
		existed = true
	}
	if _, err := e.store.Import(e.previewRaw); err != nil {
		return map[string]any{"status": "failed", "reason": importReason(err)}
	}
	if auto {
		e.auto[realID] = true
		e.saveAuto()
	}
	if e.previewOwn != nil {
		_ = e.writeOwn(realID, e.previewOwn)
	}
	status := "added"
	if existed {
		status = "updated"
	}
	return map[string]any{"status": status, "passId": realID}
}

func (e *Engine) writeOwn(id string, fields map[string]any) error {
	if fields == nil {
		return nil
	}
	dir := filepath.Join(e.root, "passes", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "own.json"), b, 0o644)
}

func (e *Engine) ownCard(id string) any {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, err := os.ReadFile(filepath.Join(e.root, "passes", id, "own.json"))
	if err != nil {
		return map[string]any{}
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return map[string]any{}
	}
	return m
}

func (e *Engine) replaceOwnCard(id string) any {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.store == nil || e.preview == nil || len(e.previewRaw) == 0 {
		return map[string]any{"status": "failed", "reason": "storage"}
	}
	realID := pass.ID(e.preview.PassTypeIdentifier, e.preview.SerialNumber)
	if _, err := e.store.Import(e.previewRaw); err != nil {
		return map[string]any{"status": "failed", "reason": importReason(err)}
	}
	if realID != id {
		_ = e.store.Delete(id)
	}
	if e.previewOwn != nil {
		_ = e.writeOwn(realID, e.previewOwn)
	}
	if e.auto[id] {
		e.auto[realID] = true
		if realID != id {
			delete(e.auto, id)
		}
		e.saveAuto()
	}
	return map[string]any{"status": "updated", "passId": realID}
}

func (e *Engine) inspectOwnCard(in map[string]any) any {
	fields, _ := in["fields"].(map[string]any)
	if fields == nil {
		fields = map[string]any{}
	}
	serial := str(in["serialNumber"])
	name := strings.TrimSpace(str(in["name"]))
	code := str(in["code"])
	format := str(in["format"])
	color := str(in["color"])
	valid := name != "" && codeValid(format, strings.TrimSpace(code))
	raw, p, err := buildOwnPass(serial, name, strings.TrimSpace(code), format, color, fields)
	if err != nil || p == nil {
		return map[string]any{"valid": false}
	}
	e.mu.Lock()
	e.preview = p
	e.preview.ID = "preview"
	e.previewRaw = raw
	own := map[string]any{
		"serialNumber": serial, "name": name, "code": code, "format": format, "color": color,
		"template": str(fields["template"]), "icon": str(fields["icon"]),
	}
	for _, k := range []string{"holderName", "cardNumber", "validUntil", "memberName", "memberNumber", "membershipType", "contact", "category", "eventDate", "eventTime", "venue", "price", "currency", "section", "row", "seat", "gate", "ticketNumber", "offer", "validFrom", "conditions"} {
		if v := str(fields[k]); v != "" {
			own[k] = v
		}
	}
	e.previewOwn = own
	e.mu.Unlock()
	return map[string]any{"valid": valid, "pass": passMap(p, "preview", false, false)}
}

func buildOwnPass(serial, name, code, format, bg string, fields map[string]any) ([]byte, *pass.Pass, error) {
	if serial == "" {
		serial = newSerial()
	}
	if name == "" {
		name = "Card"
	}
	if bg == "" {
		bg = "#1a1a1a"
	}
	template := str(fields["template"])
	styleKey := "storeCard"
	switch template {
	case "membership":
		styleKey = "generic"
	case "ticket":
		styleKey = "eventTicket"
	case "coupon":
		styleKey = "coupon"
	}
	style := map[string]any{}
	add := func(bucket, label, value string) {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(label) == "" {
			return
		}
		arr, _ := style[bucket].([]any)
		arr = append(arr, map[string]any{"key": strings.ToLower(label), "label": label, "value": value})
		style[bucket] = arr
	}
	switch template {
	case "membership":
		add("primaryFields", str(fields["memberNameLabel"]), str(fields["memberName"]))
		add("secondaryFields", str(fields["memberNumberLabel"]), str(fields["memberNumber"]))
		add("secondaryFields", str(fields["membershipTypeLabel"]), str(fields["membershipType"]))
		add("auxiliaryFields", str(fields["validUntilLabel"]), formatDate(str(fields["validUntil"])))
		add("backFields", str(fields["contactLabel"]), str(fields["contact"]))
	case "ticket":
		add("headerFields", str(fields["eventDateLabel"]), formatDate(str(fields["eventDate"])))
		add("headerFields", str(fields["eventTimeLabel"]), str(fields["eventTime"]))
		add("secondaryFields", str(fields["venueLabel"]), str(fields["venue"]))
		add("secondaryFields", str(fields["categoryLabel"]), str(fields["category"]))
		add("auxiliaryFields", str(fields["sectionLabel"]), str(fields["section"]))
		add("auxiliaryFields", str(fields["rowLabel"]), str(fields["row"]))
		add("auxiliaryFields", str(fields["seatLabel"]), str(fields["seat"]))
		add("auxiliaryFields", str(fields["gateLabel"]), str(fields["gate"]))
		price := str(fields["price"])
		if price == "" || price == "0" {
			price = str(fields["freeLabel"])
		} else if c := str(fields["currency"]); c != "" {
			price = price + " " + c
		}
		add("auxiliaryFields", str(fields["priceLabel"]), price)
		add("auxiliaryFields", str(fields["holderNameLabel"]), str(fields["holderName"]))
		add("backFields", str(fields["ticketNumberLabel"]), str(fields["ticketNumber"]))
	case "coupon":
		add("primaryFields", str(fields["offerLabel"]), str(fields["offer"]))
		add("secondaryFields", str(fields["validFromLabel"]), formatDate(str(fields["validFrom"])))
		add("secondaryFields", str(fields["validUntilLabel"]), formatDate(str(fields["validUntil"])))
		add("backFields", str(fields["conditionsLabel"]), str(fields["conditions"]))
	default:
		add("headerFields", str(fields["validUntilLabel"]), formatDate(str(fields["validUntil"])))
		add("secondaryFields", str(fields["holderNameLabel"]), str(fields["holderName"]))
		add("secondaryFields", str(fields["cardNumberLabel"]), str(fields["cardNumber"]))
	}
	doc := map[string]any{
		"formatVersion": 1, "passTypeIdentifier": ownType, "serialNumber": serial,
		"teamIdentifier": "FREE", "organizationName": name, "description": name, "logoText": name,
		"backgroundColor": bg, "foregroundColor": "#ffffff", "labelColor": "#f0f0f0",
		styleKey: style,
	}
	if code != "" && format != "" {
		alt := ""
		if shapeOf(format) == "linear" {
			alt = code
		}
		doc["barcodes"] = []any{map[string]string{
			"format": format, "message": code, "messageEncoding": "utf-8", "altText": alt,
		}}
	}
	if until := str(fields["validUntil"]); until != "" {
		if t, err := time.Parse("2006-01-02", until); err == nil {
			doc["expirationDate"] = t.Add(24*time.Hour - time.Second).Format(time.RFC3339)
		}
	}
	rawJSON, err := json.Marshal(doc)
	if err != nil {
		return nil, nil, err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("pass.json")
	if err != nil {
		return nil, nil, err
	}
	if _, err := w.Write(rawJSON); err != nil {
		return nil, nil, err
	}
	icon := iconPNG(str(fields["icon"]))
	if str(fields["icon"]) == "" {
		icon = iconPNG("none")
	}
	iw, err := zw.Create("icon.png")
	if err != nil {
		return nil, nil, err
	}
	_, _ = iw.Write(icon)
	lw, err := zw.Create("logo.png")
	if err != nil {
		return nil, nil, err
	}
	_, _ = lw.Write(icon)
	if err := zw.Close(); err != nil {
		return nil, nil, err
	}
	blob := buf.Bytes()
	p, err := pass.Inspect(blob, pass.Options{})
	if err != nil {
		return blob, nil, err
	}
	return blob, p, nil
}

func (e *Engine) sharePass(id string) any {
	e.mu.Lock()
	defer e.mu.Unlock()
	src := currentPass(e.root, id)
	b, err := os.ReadFile(src)
	if err != nil {
		return map[string]any{}
	}
	dir := filepath.Join(e.root, "share")
	_ = os.MkdirAll(dir, 0o755)
	name := id + ".pkpass"
	dest := filepath.Join(dir, name)
	if err := os.WriteFile(dest, b, 0o644); err != nil {
		return map[string]any{}
	}
	return map[string]string{"name": name, "path": dest}
}

func shareFile(path string) any {
	if path == "" {
		return map[string]any{}
	}
	if _, err := os.Stat(path); err != nil {
		return map[string]any{}
	}
	return map[string]string{"name": filepath.Base(path), "path": path}
}

func (e *Engine) createBackup() any {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.store == nil {
		return map[string]any{"status": "failed", "reason": "storage"}
	}
	res, err := e.store.Export()
	if err != nil || res == nil {
		return map[string]any{"status": "failed", "reason": "writeFailed"}
	}
	name := "freesailwallet-" + time.Now().Format("20060102-150405") + ".swbackup"
	if e.copyDownload(res.Path, name) == "" && e.downloads != "" {
		return map[string]any{"status": "failed", "reason": "writeFailed"}
	}
	dest := res.Path
	if e.downloads != "" {
		dest = filepath.Join(e.downloads, name)
	}
	return map[string]any{"status": "ok", "fileName": name, "filePath": dest, "count": res.Count}
}

func (e *Engine) inspectBackup(path string) any {
	path = cleanPath(path)
	name := filepath.Base(path)
	b, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{"status": "failed", "reason": "unreadable", "fileName": name}
	}
	if len(b) > 64<<20 {
		return map[string]any{"status": "failed", "reason": "tooLarge", "fileName": name}
	}
	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return map[string]any{"status": "failed", "reason": "notBackup", "fileName": name}
	}
	var walletJSON []byte
	for _, f := range r.File {
		if f.Name == "wallet.json" {
			rc, err := f.Open()
			if err != nil {
				return map[string]any{"status": "failed", "reason": "damaged", "fileName": name}
			}
			walletJSON, _ = io.ReadAll(rc)
			rc.Close()
		}
	}
	if walletJSON == nil {
		return map[string]any{"status": "failed", "reason": "notBackup", "fileName": name}
	}
	var doc struct {
		Passes map[string]any `json:"passes"`
	}
	if json.Unmarshal(walletJSON, &doc) != nil {
		return map[string]any{"status": "failed", "reason": "damaged", "fileName": name}
	}
	return map[string]any{"status": "ok", "fileName": name, "count": len(doc.Passes)}
}

func (e *Engine) restoreBackup(path string) any {
	path = cleanPath(path)
	name := filepath.Base(path)
	b, err := os.ReadFile(path)
	if err != nil {
		return []any{result(name, "failed", "unreadable", "", "")}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.store == nil {
		return []any{result(name, "failed", "storage", "", "")}
	}
	res, err := e.store.Import(b)
	if err != nil {
		reason := "damaged"
		if strings.Contains(err.Error(), "not a pkpass") {
			reason = "notBackup"
		}
		return []any{result(name, "failed", reason, "", "")}
	}
	var out []any
	for _, p := range res.Passes {
		out = append(out, result(name, "added", "", p.Description, p.OrganizationName))
	}
	if len(out) == 0 {
		return []any{result(name, "failed", "damaged", "", "")}
	}
	return out
}

func (e *Engine) download(rawURL string) any {
	e.mu.Lock()
	allowed := e.internet
	e.cancel = false
	e.mu.Unlock()
	if !allowed {
		return map[string]string{"status": "failed", "filePath": ""}
	}
	body, kind, err := httpGet(rawURL)
	if err != nil {
		return map[string]string{"status": "failed", "filePath": ""}
	}
	if kind == "html" {
		e.mu.Lock()
		e.pageHTML = string(body)
		e.pageURL = rawURL
		e.mu.Unlock()
		return map[string]string{"status": "web", "filePath": ""}
	}
	if _, err := pass.Inspect(body, pass.Options{}); err != nil {
		return map[string]string{"status": "failed", "filePath": ""}
	}
	return map[string]string{"status": "pass", "filePath": e.saveDownload(body)}
}

func (e *Engine) findOnPage() any {
	e.mu.Lock()
	html := e.pageHTML
	base := e.pageURL
	allowed := e.internet
	e.mu.Unlock()
	if !allowed {
		return map[string]string{"status": "failed", "filePath": ""}
	}
	re := regexp.MustCompile(`https?://[^"'\\\s>]+?\.pkpass`)
	for _, link := range re.FindAllString(html, 8) {
		got := e.download(resolve(base, link))
		if m, ok := got.(map[string]string); ok && m["status"] == "pass" {
			return got
		}
	}
	return map[string]string{"status": "notFound", "filePath": ""}
}

func (e *Engine) saveDownload(body []byte) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	dir := filepath.Join(e.root, "downloads")
	_ = os.MkdirAll(dir, 0o755)
	name := newSerial() + ".pkpass"
	dest := filepath.Join(dir, name)
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		return ""
	}
	e.downloaded = append(e.downloaded, dest)
	return dest
}

func (e *Engine) removeDownloaded() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, p := range e.downloaded {
		_ = os.Remove(p)
	}
	e.downloaded = nil
}

func httpGet(raw string) ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "FreeSailWallet/0.1 (Sailfish OS)")
	req.Header.Set("Accept", "application/vnd.apple.pkpass, application/vnd.apple.pkpasses, text/html;q=0.8")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, "missing", fmt.Errorf("not found")
	}
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, "", err
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "html") || (len(body) > 0 && body[0] == '<') {
		return body, "html", nil
	}
	return body, "bytes", nil
}

func (e *Engine) updateOne(id string, manual bool) any {
	e.mu.Lock()
	allowed := e.internet
	st := e.store
	e.mu.Unlock()
	if !allowed {
		return failedUpdate(id, manual, "Internet access is turned off")
	}
	if st == nil {
		return failedUpdate(id, manual, "The wallet is not open")
	}
	res, err := st.PollPass(id)
	if err != nil {
		return failedUpdate(id, manual, err.Error())
	}
	status := "current"
	reason := ""
	if res.Updated > 0 {
		status = "updated"
	} else if len(res.Errors) > 0 {
		status = "failed"
		reason = res.Errors[0].Message
	}
	var messages []string
	if hist, err := st.History(id); err == nil && len(hist.Revisions) > 0 {
		messages = hist.Revisions[0].Messages
	}
	if messages == nil {
		messages = []string{}
	}
	return map[string]any{"passId": id, "status": status, "manual": manual, "reason": reason, "messages": messages}
}

func failedUpdate(id string, manual bool, reason string) map[string]any {
	return map[string]any{"passId": id, "status": "failed", "manual": manual, "reason": reason, "messages": []string{}}
}

func (e *Engine) updateAll() any {
	e.mu.Lock()
	e.cancel = false
	var ids []string
	for id, on := range e.auto {
		if on {
			ids = append(ids, id)
		}
	}
	e.mu.Unlock()
	var results []any
	for _, id := range ids {
		e.mu.Lock()
		stop := e.cancel
		e.mu.Unlock()
		if stop {
			break
		}
		results = append(results, e.updateOne(id, false))
	}
	if results == nil {
		results = []any{}
	}
	return map[string]any{"results": results}
}

func partsOf(b []byte) [][]byte {
	p, err := pass.Inspect(b, pass.Options{})
	if err == nil && p != nil {
		return [][]byte{b}
	}
	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil
	}
	var out [][]byte
	for _, f := range r.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".pkpass") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(rc, 32<<20))
		rc.Close()
		if err == nil && len(body) > 0 {
			out = append(out, body)
		}
	}
	return out
}

func importReason(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "too large"):
		return "tooLarge"
	case strings.Contains(s, "not a pkpass"), strings.Contains(s, "pass.json is missing"):
		return "notPass"
	case strings.Contains(s, "not valid JSON"):
		return "damaged"
	default:
		return "damaged"
	}
}

func result(file, status, reason, desc, org string) map[string]any {
	return map[string]any{
		"fileName": file, "status": status, "reason": reason,
		"description": desc, "organizationName": org,
	}
}

func failFile(name, reason string) map[string]any {
	return map[string]any{"status": "failed", "fileName": name, "reason": reason}
}

func paths(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s := str(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func cleanPath(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "file:") {
		if u, err := url.Parse(s); err == nil && u.Path != "" {
			return u.Path
		}
	}
	return s
}

func resolve(base, ref string) string {
	bu, err := url.Parse(base)
	if err != nil {
		return ref
	}
	ru, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return bu.ResolveReference(ru).String()
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprint(int64(t))
	default:
		return ""
	}
}

func num(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	default:
		return 0
	}
}

func truth(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	default:
		return false
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(`{"error":"encode"}`)
	}
	return b
}
