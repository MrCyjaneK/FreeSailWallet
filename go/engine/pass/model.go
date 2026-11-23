package pass

import "encoding/json"

// Pass is the display model returned across the FFI boundary. Flutter renders
// this document and does not read pass.json itself.
type Pass struct {
	ID                    string       `json:"id"`
	Trusted               bool         `json:"trusted"`
	Readable              bool         `json:"readable"`
	Style                 string       `json:"style"`
	TransitType           string       `json:"transitType,omitempty"`
	OrganizationName      string       `json:"organizationName"`
	Description           string       `json:"description"`
	LogoText              string       `json:"logoText,omitempty"`
	BackgroundColor       string       `json:"backgroundColor"`
	ForegroundColor       string       `json:"foregroundColor"`
	LabelColor            string       `json:"labelColor"`
	FooterBackgroundColor string       `json:"footerBackgroundColor,omitempty"`
	Voided                bool         `json:"voided"`
	Expired               bool         `json:"expired"`
	ExpirationDate        string       `json:"expirationDate,omitempty"`
	RelevantDate          string       `json:"relevantDate,omitempty"`
	SharingProhibited     bool         `json:"sharingProhibited"`
	GroupingIdentifier    string       `json:"groupingIdentifier,omitempty"`
	HeaderFields          []FieldView  `json:"headerFields"`
	PrimaryFields         []FieldView  `json:"primaryFields"`
	SecondaryFields       []FieldView  `json:"secondaryFields"`
	AuxiliaryFields       []FieldView  `json:"auxiliaryFields"`
	BackFields            []FieldView  `json:"backFields"`
	AdditionalInfoFields  []FieldView  `json:"additionalInfoFields"`
	Images                Images       `json:"images"`
	Barcode               *BarcodeView `json:"barcode,omitempty"`
	Verification          Verification `json:"verification"`
	HasWebService         bool         `json:"hasWebService"`
	WebServiceURL         string       `json:"webServiceURL,omitempty"`
	AuthenticationToken   string       `json:"authenticationToken,omitempty"`
	NFC                   *NFCView     `json:"nfc,omitempty"`
	Locations             []Location   `json:"locations,omitempty"`
	Beacons               []Beacon     `json:"beacons,omitempty"`
	PassTypeIdentifier    string       `json:"passTypeIdentifier"`
	SerialNumber          string       `json:"serialNumber"`
	TeamIdentifier        string       `json:"teamIdentifier"`
}

// FieldView is one pass field after localization and date formatting.
type FieldView struct {
	Key           string `json:"key"`
	Label         string `json:"label,omitempty"`
	Value         string `json:"value"`
	Alignment     string `json:"alignment,omitempty"`
	Row           int    `json:"row,omitempty"`
	ChangeMessage string `json:"changeMessage,omitempty"`
}

// Images holds either an absolute file path or a data URL.
type Images struct {
	Icon       string `json:"icon,omitempty"`
	Logo       string `json:"logo,omitempty"`
	Strip      string `json:"strip,omitempty"`
	Thumbnail  string `json:"thumbnail,omitempty"`
	Background string `json:"background,omitempty"`
	Footer     string `json:"footer,omitempty"`
}

// BarcodeView is the first barcode the wallet can show.
type BarcodeView struct {
	Format      string `json:"format"`
	Message     string `json:"message"`
	AltText     string `json:"altText,omitempty"`
	Encoding    string `json:"encoding,omitempty"`
	Supported   bool   `json:"supported"`
	Unsupported string `json:"unsupported,omitempty"`
}

// NFCView is the pass NFC payload. It is shown, not transmitted.
type NFCView struct {
	Message                string `json:"message,omitempty"`
	RequiresAuthentication bool   `json:"requiresAuthentication,omitempty"`
	HasEncryptionKey       bool   `json:"hasEncryptionKey,omitempty"`
}

// Location is a relevance location from pass.json.
type Location struct {
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	Altitude     float64 `json:"altitude,omitempty"`
	RelevantText string  `json:"relevantText,omitempty"`
}

// Beacon is an iBeacon relevance record.
type Beacon struct {
	ProximityUUID string `json:"proximityUUID"`
	Major         *int   `json:"major,omitempty"`
	Minor         *int   `json:"minor,omitempty"`
	RelevantText  string `json:"relevantText,omitempty"`
}

// Verification is the four checks plus the certificate the signer presented.
type Verification struct {
	ManifestOK  bool     `json:"manifestOK"`
	SignatureOK bool     `json:"signatureOK"`
	ChainOK     bool     `json:"chainOK"`
	IdentityOK  bool     `json:"identityOK"`
	Trusted     bool     `json:"trusted"`
	Problems    []string `json:"problems"`
	Signer      Signer   `json:"signer"`
}

// Signer describes who signed the pass and which root anchored the chain.
type Signer struct {
	Subject      string   `json:"subject,omitempty"`
	Issuer       string   `json:"issuer,omitempty"`
	PassTypeID   string   `json:"passTypeId,omitempty"`
	TeamID       string   `json:"teamId,omitempty"`
	Organization string   `json:"organization,omitempty"`
	NotBefore    string   `json:"notBefore,omitempty"`
	NotAfter     string   `json:"notAfter,omitempty"`
	Root         string   `json:"root,omitempty"`
	Chain        []string `json:"chain,omitempty"`
}

// Summary is one row in the wallet list.
type Summary struct {
	ID               string      `json:"id"`
	Style            string      `json:"style"`
	OrganizationName string      `json:"organizationName"`
	Description      string      `json:"description"`
	LogoText         string      `json:"logoText,omitempty"`
	BackgroundColor  string      `json:"backgroundColor"`
	ForegroundColor  string      `json:"foregroundColor"`
	LabelColor       string      `json:"labelColor"`
	PrimaryValue     string      `json:"primaryValue,omitempty"`
	HeaderLabel      string      `json:"headerLabel,omitempty"`
	HeaderValue      string      `json:"headerValue,omitempty"`
	HeaderFields     []FieldView `json:"headerFields,omitempty"`
	PrimaryFields    []FieldView `json:"primaryFields,omitempty"`
	SecondaryFields  []FieldView `json:"secondaryFields,omitempty"`
	AuxiliaryFields  []FieldView `json:"auxiliaryFields,omitempty"`
	Strip            string      `json:"strip,omitempty"`
	Thumbnail        string      `json:"thumbnail,omitempty"`
	TransitType      string      `json:"transitType,omitempty"`
	Voided           bool        `json:"voided"`
	Expired          bool        `json:"expired"`
	Trusted          bool        `json:"trusted"`
	Archived         bool        `json:"archived"`
	HasWebService    bool        `json:"hasWebService,omitempty"`
	Icon             string      `json:"icon,omitempty"`
	Logo             string      `json:"logo,omitempty"`
	LastError        string      `json:"lastError,omitempty"`
	WhatChanged      string      `json:"whatChanged,omitempty"`
	RelevantDate     string      `json:"relevantDate,omitempty"`
}

func (p *Pass) Summary() Summary {
	s := Summary{
		ID:               p.ID,
		Style:            p.Style,
		OrganizationName: p.OrganizationName,
		Description:      p.Description,
		LogoText:         p.LogoText,
		BackgroundColor:  p.BackgroundColor,
		ForegroundColor:  p.ForegroundColor,
		LabelColor:       p.LabelColor,
		Voided:           p.Voided,
		Expired:          p.Expired,
		Trusted:          p.Trusted,
		HasWebService:    p.HasWebService,
		Icon:             p.Images.Icon,
		Logo:             p.Images.Logo,
		Strip:            p.Images.Strip,
		Thumbnail:        p.Images.Thumbnail,
		TransitType:      p.TransitType,
		HeaderFields:     p.HeaderFields,
		PrimaryFields:    p.PrimaryFields,
		SecondaryFields:  p.SecondaryFields,
		AuxiliaryFields:  p.AuxiliaryFields,
		RelevantDate:     p.RelevantDate,
	}
	if len(p.PrimaryFields) > 0 {
		s.PrimaryValue = p.PrimaryFields[0].Value
	}
	if len(p.HeaderFields) > 0 {
		s.HeaderLabel = p.HeaderFields[0].Label
		s.HeaderValue = p.HeaderFields[0].Value
	}
	return s
}

// document is the decoded pass.json. Unknown keys stay in Raw.
type document struct {
	FormatVersion         int                        `json:"formatVersion"`
	PassTypeIdentifier    string                     `json:"passTypeIdentifier"`
	SerialNumber          string                     `json:"serialNumber"`
	TeamIdentifier        string                     `json:"teamIdentifier"`
	OrganizationName      string                     `json:"organizationName"`
	Description           string                     `json:"description"`
	LogoText              string                     `json:"logoText"`
	RelevantDate          string                     `json:"relevantDate"`
	ExpirationDate        string                     `json:"expirationDate"`
	Voided                bool                       `json:"voided"`
	GroupingIdentifier    string                     `json:"groupingIdentifier"`
	SharingProhibited     bool                       `json:"sharingProhibited"`
	BackgroundColor       string                     `json:"backgroundColor"`
	ForegroundColor       string                     `json:"foregroundColor"`
	LabelColor            string                     `json:"labelColor"`
	FooterBackgroundColor string                     `json:"footerBackgroundColor"`
	WebServiceURL         string                     `json:"webServiceURL"`
	AuthenticationToken   string                     `json:"authenticationToken"`
	Barcode               *barcodeDoc                `json:"barcode"`
	Barcodes              []barcodeDoc               `json:"barcodes"`
	NFC                   *nfcDoc                    `json:"nfc"`
	Locations             []Location                 `json:"locations"`
	Beacons               []Beacon                   `json:"beacons"`
	BoardingPass          *styleDoc                  `json:"boardingPass"`
	Coupon                *styleDoc                  `json:"coupon"`
	EventTicket           *styleDoc                  `json:"eventTicket"`
	StoreCard             *styleDoc                  `json:"storeCard"`
	Generic               *styleDoc                  `json:"generic"`
	Raw                   map[string]json.RawMessage `json:"-"`
}

type styleDoc struct {
	TransitType          string     `json:"transitType"`
	HeaderFields         []fieldDoc `json:"headerFields"`
	PrimaryFields        []fieldDoc `json:"primaryFields"`
	SecondaryFields      []fieldDoc `json:"secondaryFields"`
	AuxiliaryFields      []fieldDoc `json:"auxiliaryFields"`
	BackFields           []fieldDoc `json:"backFields"`
	AdditionalInfoFields []fieldDoc `json:"additionalInfoFields"`
}

type fieldDoc struct {
	Key               string
	Label             string
	Value             any
	AttributedValue   any
	ChangeMessage     string
	TextAlignment     string
	DateStyle         string
	TimeStyle         string
	CurrencyCode      string
	NumberStyle       string
	DataDetectorTypes []string
	IgnoresTimeZone   bool
	Relative          bool
	Row               int
}

func (f *fieldDoc) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	f.Key = jsonString(raw["key"])
	f.Label = jsonString(raw["label"])
	f.Value = jsonAny(raw["value"])
	f.AttributedValue = jsonAny(raw["attributedValue"])
	f.ChangeMessage = jsonString(raw["changeMessage"])
	f.TextAlignment = jsonString(raw["textAlignment"])
	f.DateStyle = jsonString(raw["dateStyle"])
	f.TimeStyle = jsonString(raw["timeStyle"])
	f.CurrencyCode = jsonString(raw["currencyCode"])
	f.NumberStyle = jsonString(raw["numberStyle"])
	if det, ok := raw["dataDetectorTypes"]; ok {
		_ = json.Unmarshal(det, &f.DataDetectorTypes)
	}
	f.IgnoresTimeZone = jsonBool(raw["ignoresTimeZone"])
	f.Relative = jsonBool(raw["relative"]) || jsonBool(raw["isRelative"])
	f.Row = int(jsonFloat(raw["row"]))
	return nil
}

type barcodeDoc struct {
	Format          string `json:"format"`
	Message         string `json:"message"`
	MessageEncoding string `json:"messageEncoding"`
	AltText         string `json:"altText"`
}

type nfcDoc struct {
	Message                string `json:"message"`
	EncryptionPublicKey    string `json:"encryptionPublicKey"`
	RequiresAuthentication bool   `json:"requiresAuthentication"`
}

func jsonString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

func jsonBool(raw json.RawMessage) bool {
	var b bool
	_ = json.Unmarshal(raw, &b)
	return b
}

func jsonFloat(raw json.RawMessage) float64 {
	var n float64
	_ = json.Unmarshal(raw, &n)
	return n
}

func jsonAny(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	return string(raw)
}
