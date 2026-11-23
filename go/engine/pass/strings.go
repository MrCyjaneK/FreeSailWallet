package pass

import (
	"strings"
	"unicode/utf16"
)

func decodeStrings(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		return utf16LE(b[2:])
	}
	if len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF {
		return utf16BE(b[2:])
	}
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:]
	}
	if looksUTF16LE(b) {
		return utf16LE(b)
	}
	if looksUTF16BE(b) {
		return utf16BE(b)
	}
	return string(b)
}

func looksUTF16LE(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	zeros := 0
	for i := 1; i < len(b) && i < 16; i += 2 {
		if b[i] == 0 {
			zeros++
		}
	}
	return zeros >= 2
}

func looksUTF16BE(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	zeros := 0
	for i := 0; i < len(b) && i < 16; i += 2 {
		if b[i] == 0 {
			zeros++
		}
	}
	return zeros >= 2
}

func utf16LE(b []byte) string {
	if len(b)%2 == 1 {
		b = b[:len(b)-1]
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[i*2]) | uint16(b[i*2+1])<<8
	}
	return string(utf16.Decode(u))
}

func utf16BE(b []byte) string {
	if len(b)%2 == 1 {
		b = b[:len(b)-1]
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[i*2+1]) | uint16(b[i*2])<<8
	}
	return string(utf16.Decode(u))
}

// parseStrings reads an Apple .strings catalog.
func parseStrings(b []byte) map[string]string {
	text := stripComments(decodeStrings(b))
	out := map[string]string{}
	i := 0
	for i < len(text) {
		for i < len(text) && isSpace(text[i]) {
			i++
		}
		if i >= len(text) {
			break
		}
		if text[i] != '"' {
			i++
			continue
		}
		key, next, ok := readQuoted(text, i)
		if !ok {
			break
		}
		i = next
		for i < len(text) && isSpace(text[i]) {
			i++
		}
		if i >= len(text) || text[i] != '=' {
			continue
		}
		i++
		for i < len(text) && isSpace(text[i]) {
			i++
		}
		if i >= len(text) || text[i] != '"' {
			continue
		}
		val, next, ok := readQuoted(text, i)
		if !ok {
			break
		}
		out[key] = val
		i = next
	}
	return out
}

func stripComments(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '/' && i+1 < len(s) && s[i+1] == '/' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			if i < len(s) {
				b.WriteByte('\n')
			}
			continue
		}
		if s[i] == '/' && i+1 < len(s) && s[i+1] == '*' {
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			if i+1 < len(s) {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func readQuoted(s string, i int) (string, int, bool) {
	if i >= len(s) || s[i] != '"' {
		return "", i, false
	}
	i++
	var b strings.Builder
	for i < len(s) {
		c := s[i]
		if c == '"' {
			return b.String(), i + 1, true
		}
		if c == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case '\\', '"':
				b.WriteByte(s[i])
			default:
				b.WriteByte(s[i])
			}
			i++
			continue
		}
		b.WriteByte(c)
		i++
	}
	return "", i, false
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func normLang(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "_", "-")
}

func pickCatalog(catalogs map[string]map[string]string, locale string) map[string]string {
	if len(catalogs) == 0 {
		return nil
	}
	want := normLang(locale)
	base := want
	if i := strings.IndexByte(want, '-'); i >= 0 {
		base = want[:i]
	}
	order := []string{want, base, "en", "en-us"}
	for _, key := range order {
		if key == "" {
			continue
		}
		if cat, ok := catalogs[key]; ok {
			return cat
		}
	}
	return nil
}

func translate(cat map[string]string, s string) string {
	if cat == nil || s == "" {
		return s
	}
	if v, ok := cat[s]; ok {
		return v
	}
	return s
}
