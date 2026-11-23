package sailpkg

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

const diffContext = 3

type lineOp struct {
	kind byte // ' ', '-', '+'
	text string
}

// Diff returns a unified diff from orig to edited. An empty result means the
// files are identical.
func Diff(path string, orig, edited []byte) ([]byte, error) {
	if bytes.Equal(orig, edited) {
		return nil, nil
	}
	a, aNL := splitLines(orig)
	b, bNL := splitLines(edited)
	script := editScript(a, b)
	var buf strings.Builder
	fmt.Fprintf(&buf, "--- a/%s\n+++ b/%s\n", path, path)
	for _, h := range hunks(script) {
		oldN, newN := 0, 0
		for _, op := range h.ops {
			switch op.kind {
			case ' ':
				oldN++
				newN++
			case '-':
				oldN++
			case '+':
				newN++
			}
		}
		fmt.Fprintf(&buf, "@@ -%d,%d +%d,%d @@\n", h.oldStart, oldN, h.newStart, newN)
		for _, op := range h.ops {
			buf.WriteByte(op.kind)
			buf.WriteString(op.text)
			buf.WriteByte('\n')
		}
	}
	if aNL != bNL {
		return nil, fmt.Errorf("%s: refusing to diff a change in the final newline", path)
	}
	return []byte(buf.String()), nil
}

// Apply applies one unified diff to orig.
func Apply(orig, patch []byte) ([]byte, error) {
	if len(bytes.TrimSpace(patch)) == 0 {
		return append([]byte(nil), orig...), nil
	}
	lines, nl := splitLines(orig)
	hs, err := parseHunks(patch)
	if err != nil {
		return nil, err
	}
	var out []string
	pos := 0
	for _, h := range hs {
		start := h.oldStart - 1
		if start < pos || start > len(lines) {
			return nil, fmt.Errorf("hunk at line %d is out of range", h.oldStart)
		}
		out = append(out, lines[pos:start]...)
		pos = start
		for _, op := range h.ops {
			switch op.kind {
			case ' ', '-':
				if pos >= len(lines) || lines[pos] != op.text {
					got := ""
					if pos < len(lines) {
						got = lines[pos]
					}
					return nil, fmt.Errorf("hunk at line %d does not match (%q)", h.oldStart, got)
				}
				if op.kind == ' ' {
					out = append(out, op.text)
				}
				pos++
			case '+':
				out = append(out, op.text)
			default:
				return nil, fmt.Errorf("bad diff line %q", op.kind)
			}
		}
	}
	out = append(out, lines[pos:]...)
	return joinLines(out, nl), nil
}

type hunk struct {
	oldStart int
	newStart int
	ops      []lineOp
}

func parseHunks(patch []byte) ([]hunk, error) {
	text := string(patch)
	if strings.HasSuffix(text, "\n") {
		text = text[:len(text)-1]
	}
	rows := strings.Split(text, "\n")
	var hs []hunk
	i := 0
	for i < len(rows) {
		row := rows[i]
		if strings.HasPrefix(row, "--- ") || strings.HasPrefix(row, "+++ ") {
			i++
			continue
		}
		if !strings.HasPrefix(row, "@@ ") {
			return nil, fmt.Errorf("expected hunk, got %q", row)
		}
		oldStart, _, err := parseHunkHeader(row)
		if err != nil {
			return nil, err
		}
		i++
		h := hunk{oldStart: oldStart}
		for i < len(rows) && !strings.HasPrefix(rows[i], "@@ ") {
			row = rows[i]
			i++
			if row == `\ No newline at end of file` {
				continue
			}
			if row == "" || (row[0] != ' ' && row[0] != '+' && row[0] != '-') {
				return nil, fmt.Errorf("bad diff line %q", row)
			}
			h.ops = append(h.ops, lineOp{kind: row[0], text: row[1:]})
		}
		if len(h.ops) == 0 {
			return nil, fmt.Errorf("empty hunk at line %d", oldStart)
		}
		hs = append(hs, h)
	}
	if len(hs) == 0 {
		return nil, fmt.Errorf("diff has no hunks")
	}
	return hs, nil
}

func parseHunkHeader(row string) (oldStart, newStart int, err error) {
	// @@ -l,c +l,c @@
	if !strings.HasPrefix(row, "@@ ") || !strings.Contains(row, " @@") {
		return 0, 0, fmt.Errorf("bad hunk header %q", row)
	}
	body := strings.TrimPrefix(row, "@@ ")
	body = body[:strings.Index(body, " @@")]
	parts := strings.Fields(body)
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "-") || !strings.HasPrefix(parts[1], "+") {
		return 0, 0, fmt.Errorf("bad hunk header %q", row)
	}
	oldStart, err = atoiRange(parts[0][1:])
	if err != nil {
		return 0, 0, fmt.Errorf("bad hunk header %q", row)
	}
	newStart, err = atoiRange(parts[1][1:])
	if err != nil {
		return 0, 0, fmt.Errorf("bad hunk header %q", row)
	}
	return oldStart, newStart, nil
}

func atoiRange(s string) (int, error) {
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	return strconv.Atoi(s)
}

func splitLines(b []byte) ([]string, bool) {
	if len(b) == 0 {
		return nil, false
	}
	nl := b[len(b)-1] == '\n'
	s := string(b)
	if nl {
		s = s[:len(s)-1]
	}
	return strings.Split(s, "\n"), nl
}

func joinLines(lines []string, nl bool) []byte {
	if len(lines) == 0 {
		if nl {
			return []byte("\n")
		}
		return nil
	}
	s := strings.Join(lines, "\n")
	if nl {
		s += "\n"
	}
	return []byte(s)
}

func editScript(a, b []string) []lineOp {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var script []lineOp
	i, j := 0, 0
	for i < n && j < m {
		if a[i] == b[j] {
			script = append(script, lineOp{kind: ' ', text: a[i]})
			i++
			j++
			continue
		}
		if dp[i+1][j] >= dp[i][j+1] {
			script = append(script, lineOp{kind: '-', text: a[i]})
			i++
			continue
		}
		script = append(script, lineOp{kind: '+', text: b[j]})
		j++
	}
	for i < n {
		script = append(script, lineOp{kind: '-', text: a[i]})
		i++
	}
	for j < m {
		script = append(script, lineOp{kind: '+', text: b[j]})
		j++
	}
	return script
}

func hunks(script []lineOp) []hunk {
	var ranges [][2]int
	i := 0
	for i < len(script) {
		if script[i].kind == ' ' {
			i++
			continue
		}
		start := i - diffContext
		if start < 0 {
			start = 0
		}
		j := i
		for j < len(script) {
			if script[j].kind != ' ' {
				j++
				continue
			}
			k := j
			for k < len(script) && script[k].kind == ' ' {
				k++
			}
			eqs := k - j
			if k < len(script) && eqs <= diffContext*2 {
				j = k
				continue
			}
			if eqs > diffContext {
				j += diffContext
			} else {
				j = k
			}
			break
		}
		if len(ranges) > 0 && start <= ranges[len(ranges)-1][1] {
			ranges[len(ranges)-1][1] = j
		} else {
			ranges = append(ranges, [2]int{start, j})
		}
		i = j
	}
	out := make([]hunk, 0, len(ranges))
	for _, r := range ranges {
		oldStart, newStart := 1, 1
		for _, op := range script[:r[0]] {
			switch op.kind {
			case ' ':
				oldStart++
				newStart++
			case '-':
				oldStart++
			case '+':
				newStart++
			}
		}
		out = append(out, hunk{
			oldStart: oldStart,
			newStart: newStart,
			ops:      append([]lineOp(nil), script[r[0]:r[1]]...),
		})
	}
	return out
}
