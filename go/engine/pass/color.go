package pass

import (
	"fmt"
	"strconv"
	"strings"
)

func parseColor(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "rgb(") || strings.HasPrefix(low, "rgba(") {
		return parseRGB(low)
	}
	if strings.HasPrefix(s, "#") {
		return parseHex(s)
	}
	return "", false
}

func parseRGB(s string) (string, bool) {
	open := strings.IndexByte(s, '(')
	close := strings.LastIndexByte(s, ')')
	if open < 0 || close < 0 || close < open {
		return "", false
	}
	parts := strings.Split(s[open+1:close], ",")
	if len(parts) < 3 {
		return "", false
	}
	var nums [3]int
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(strings.TrimSpace(parts[i]))
		if err != nil || n < 0 || n > 255 {
			return "", false
		}
		nums[i] = n
	}
	return fmt.Sprintf("#%02X%02X%02X", nums[0], nums[1], nums[2]), true
}

func parseHex(s string) (string, bool) {
	h := strings.TrimPrefix(s, "#")
	switch len(h) {
	case 3:
		return fmt.Sprintf("#%c%c%c%c%c%c", h[0], h[0], h[1], h[1], h[2], h[2]), true
	case 6:
		if _, err := strconv.ParseUint(h, 16, 32); err != nil {
			return "", false
		}
		return "#" + strings.ToUpper(h), true
	case 8:
		// #AARRGGBB, alpha first, as used by readers of real passes.
		if _, err := strconv.ParseUint(h, 16, 64); err != nil {
			return "", false
		}
		return "#" + strings.ToUpper(h[2:]), true
	default:
		return "", false
	}
}

func colorOrDefault(raw, fallback string, problems *[]string, name string) string {
	if raw == "" {
		return fallback
	}
	c, ok := parseColor(raw)
	if !ok {
		*problems = append(*problems, name+" color is unreadable")
		return fallback
	}
	return c
}
