package pass

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/aztec"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/pdf417"
	"github.com/boombuler/barcode/qr"
)

func chooseBarcode(doc *document) *BarcodeView {
	list := doc.Barcodes
	if len(list) == 0 && doc.Barcode != nil {
		list = []barcodeDoc{*doc.Barcode}
	}
	if len(list) == 0 {
		return nil
	}
	var fallback *BarcodeView
	for _, b := range list {
		view := &BarcodeView{
			Format:   b.Format,
			Message:  b.Message,
			AltText:  b.AltText,
			Encoding: b.MessageEncoding,
		}
		if barcodeSupported(b.Format) {
			view.Supported = true
			return view
		}
		view.Unsupported = "this barcode format is shown as text"
		if fallback == nil {
			fallback = view
		}
	}
	return fallback
}

func barcodeSupported(format string) bool {
	switch format {
	case "PKBarcodeFormatQR", "PKBarcodeFormatPDF417", "PKBarcodeFormatAztec", "PKBarcodeFormatCode128":
		return true
	default:
		return false
	}
}

// BarcodePNG draws the pass barcode. size is the longer side in pixels.
func BarcodePNG(view *BarcodeView, size int) ([]byte, error) {
	if view == nil || !view.Supported {
		return nil, nil
	}
	if size < 64 {
		size = 64
	}
	code, err := encodeBarcode(view)
	if err != nil {
		return nil, err
	}
	img, err := scaleBarcode(code, size)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeBarcode(view *BarcodeView) (barcode.Barcode, error) {
	msg := view.Message
	switch view.Format {
	case "PKBarcodeFormatQR":
		return qr.Encode(msg, qr.M, qr.Auto)
	case "PKBarcodeFormatPDF417":
		return pdf417.Encode(msg, 4)
	case "PKBarcodeFormatAztec":
		return aztec.Encode([]byte(msg), 23, 0)
	case "PKBarcodeFormatCode128":
		return code128.Encode(msg)
	default:
		return nil, nil
	}
}

func scaleBarcode(code barcode.Barcode, size int) (image.Image, error) {
	b := code.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	var nw, nh int
	if w >= h {
		nw = size
		nh = h * size / w
	} else {
		nh = size
		nw = w * size / h
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	return barcode.Scale(code, nw, nh)
}

func pngDataURL(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b)
}
