package api

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// These are wire diagnostics for the pinned jsonschema 4.26.0 / Python 3.12,
// not a general Python interpreter. In particular repr must preserve dictionary
// order, None/boolean spelling, quotes and code-point widths in error bodies.
func pythonString(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for _, r := range s {
		switch {
		case r == rune(quote) || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == ' ' || unicode.IsGraphic(r) && !unicode.IsSpace(r):
			b.WriteRune(r)
		case r < 256:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r <= 0xffff:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			fmt.Fprintf(&b, `\U%08x`, r)
		}
	}
	b.WriteByte(quote)
	return b.String()
}

func pythonRepr(value any) string {
	switch value := value.(type) {
	case nil:
		return "None"
	case bool:
		if value {
			return "True"
		}
		return "False"
	case string:
		return pythonString(value)
	case json.Number:
		s := string(value)
		if !strings.ContainsAny(s, ".eE") {
			if s == "-0" {
				return "0"
			}
			return s
		}
		n, err := strconv.ParseFloat(s, 64)
		if math.IsInf(n, 1) {
			return "inf"
		}
		if math.IsInf(n, -1) {
			return "-inf"
		}
		if err != nil && n == 0 {
			return s
		}
		format := byte('g')
		if n == 0 || n >= 1e-4 && n < 1e16 || n <= -1e-4 && n > -1e16 {
			format = 'f'
		}
		s = strconv.FormatFloat(n, format, -1, 64)
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}
		return s
	case jsonObject:
		parts := make([]string, len(value))
		for i, m := range value {
			parts[i] = pythonString(m.key) + ": " + pythonRepr(m.value)
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []any:
		parts := make([]string, len(value))
		for i, item := range value {
			parts[i] = pythonRepr(item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		panic("unsupported diagnostic JSON value")
	}
}

// jsonschema pretty-prints at width 72, with insertion order and indent=1.
// Format recursively by available columns; the output is then passed through
// the same newline flattening as Placement's WebOb JSON error formatter.
func pythonPretty(value any) string { return prettyJSON(value, 0, 0, 0) }

func prettyJSON(value any, indent, allowance, level int) string {
	repr := pythonRepr(value)
	if utf8.RuneCountInString(repr) <= 72-indent-allowance {
		return repr
	}
	switch value := value.(type) {
	case jsonObject:
		parts := make([]string, len(value))
		for i, member := range value {
			key := pythonString(member.key)
			a := 1
			if i == len(value)-1 {
				a = allowance + 1
			}
			parts[i] = key + ": " + prettyJSON(member.value, indent+1+utf8.RuneCountInString(key)+2, a, level+1)
		}
		return "{" + strings.Join(parts, ",\n"+strings.Repeat(" ", indent+1)) + "}"
	case []any:
		parts := make([]string, len(value))
		for i, item := range value {
			a := 1
			if i == len(value)-1 {
				a = allowance + 1
			}
			parts[i] = prettyJSON(item, indent+1, a, level+1)
		}
		return "[" + strings.Join(parts, ",\n"+strings.Repeat(" ", indent+1)) + "]"
	case string:
		return prettyString(value, indent, allowance, level+1)
	default:
		return repr
	}
}

func prettyString(s string, indent, allowance, level int) string {
	original := pythonString(s)
	if level == 1 {
		indent++
		allowance++
	}
	// Python splitlines(keepends=True), including Unicode line boundaries.
	var lines []string
	start := 0
	for pos, r := range s {
		if r == '\n' || r == '\r' || r == '\v' || r == '\f' || r == 0x1c || r == 0x1d || r == 0x1e || r == 0x85 || r == 0x2028 || r == 0x2029 {
			end := pos + utf8.RuneLen(r)
			if r == '\r' && end < len(s) && s[end] == '\n' {
				continue
			}
			lines = append(lines, s[start:end])
			start = end
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	var chunks []string
	for i, line := range lines {
		width := 72 - indent
		if i == len(lines)-1 {
			width -= allowance
		}
		if utf8.RuneCountInString(pythonString(line)) <= width {
			chunks = append(chunks, pythonString(line))
			continue
		}
		// Group each word with its following whitespace, like \S*\s*.
		var parts []string
		start, spaces := 0, false
		for pos, r := range line {
			if !unicode.IsSpace(r) && spaces {
				parts = append(parts, line[start:pos])
				start = pos
				spaces = false
			}
			if unicode.IsSpace(r) {
				spaces = true
			}
		}
		if start < len(line) {
			parts = append(parts, line[start:])
		}
		current := ""
		for j, part := range parts {
			width := 72 - indent
			if i == len(lines)-1 && j == len(parts)-1 {
				width -= allowance
			}
			if utf8.RuneCountInString(pythonString(current+part)) > width {
				if current != "" {
					chunks = append(chunks, pythonString(current))
				}
				current = part
			} else {
				current += part
			}
		}
		if current != "" {
			chunks = append(chunks, pythonString(current))
		}
	}
	if len(chunks) <= 1 {
		return original
	}
	result := strings.Join(chunks, "\n"+strings.Repeat(" ", indent))
	if level == 1 {
		result = "(" + result + ")"
	}
	return result
}
