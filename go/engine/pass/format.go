package pass

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04Z07:00",
		"2006-01-02T15:04:05.000Z07:00",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func formatField(f fieldDoc, now time.Time, cat map[string]string) FieldView {
	label := translate(cat, f.Label)
	raw := fieldRaw(f)
	value := raw
	if f.DateStyle != "" || f.TimeStyle != "" {
		if t, ok := parseTime(raw); ok {
			if f.Relative {
				value = relativePhrase(t, now)
			} else {
				value = formatDate(t, f.DateStyle, f.TimeStyle)
			}
		}
	} else if n, ok := f.Value.(float64); ok {
		value = formatNumber(n, f.NumberStyle, f.CurrencyCode)
	}
	value = plainText(value)
	view := FieldView{
		Key:           f.Key,
		Label:         label,
		Value:         value,
		Alignment:     alignment(f.TextAlignment),
		Row:           f.Row,
		ChangeMessage: translate(cat, f.ChangeMessage),
	}
	return view
}

func fieldRaw(f fieldDoc) string {
	src := f.Value
	if f.AttributedValue != nil {
		src = f.AttributedValue
	}
	switch v := src.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

func alignment(s string) string {
	switch s {
	case "PKTextAlignmentLeft":
		return "left"
	case "PKTextAlignmentCenter":
		return "center"
	case "PKTextAlignmentRight":
		return "right"
	case "PKTextAlignmentNatural":
		return "natural"
	default:
		return ""
	}
}

func formatDate(t time.Time, dateStyle, timeStyle string) string {
	var parts []string
	if d := datePart(t, dateStyle); d != "" {
		parts = append(parts, d)
	}
	if tm := timePart(t, timeStyle); tm != "" {
		parts = append(parts, tm)
	}
	return strings.Join(parts, " ")
}

func datePart(t time.Time, style string) string {
	switch style {
	case "", "PKDateStyleNone":
		return ""
	case "PKDateStyleShort":
		return t.Format("1/2/06")
	case "PKDateStyleMedium":
		return t.Format("Jan 2, 2006")
	case "PKDateStyleLong":
		return t.Format("January 2, 2006")
	case "PKDateStyleFull":
		return t.Format("Monday, January 2, 2006")
	default:
		return t.Format("Jan 2, 2006")
	}
}

func timePart(t time.Time, style string) string {
	switch style {
	case "", "PKDateStyleNone":
		return ""
	case "PKDateStyleShort":
		return t.Format("3:04 PM")
	case "PKDateStyleMedium", "PKDateStyleLong", "PKDateStyleFull":
		return t.Format("3:04:05 PM")
	default:
		return t.Format("3:04 PM")
	}
}

func relativePhrase(t, now time.Time) string {
	d := t.Sub(now)
	past := d < 0
	if past {
		d = -d
	}
	var n int
	var unit string
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		n = int(d.Minutes())
		unit = "minute"
	case d < 24*time.Hour:
		n = int(d.Hours())
		unit = "hour"
	default:
		n = int(d.Hours() / 24)
		unit = "day"
	}
	if n != 1 {
		unit += "s"
	}
	if past {
		return fmt.Sprintf("%d %s ago", n, unit)
	}
	return fmt.Sprintf("in %d %s", n, unit)
}

func formatNumber(n float64, style, currency string) string {
	var body string
	switch style {
	case "PKNumberStylePercent":
		body = strconv.FormatFloat(n*100, 'f', -1, 64) + "%"
	case "PKNumberStyleScientific":
		body = strconv.FormatFloat(n, 'e', 2, 64)
	default:
		body = strconv.FormatFloat(n, 'f', -1, 64)
	}
	if currency != "" && style != "PKNumberStylePercent" {
		return currency + " " + body
	}
	return body
}

func plainText(s string) string {
	if !strings.Contains(s, "<") {
		return decodeEntities(s)
	}
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(decodeEntities(b.String()))
}

func decodeEntities(s string) string {
	r := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&nbsp;", " ",
	)
	return r.Replace(s)
}

func applyChangeMessage(template, value string) string {
	if template == "" || !strings.Contains(template, "%@") {
		return ""
	}
	return strings.ReplaceAll(template, "%@", value)
}
