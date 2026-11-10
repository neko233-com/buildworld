package migration

import (
	"strconv"
	"strings"
	"unicode/utf16"
)

// parseWhenLiteral decodes Groovy's ordinary quoted string escapes without
// accepting executable GString interpolation. Shell parsing uses a different
// function because shell backslashes have to survive translation.
func parseWhenLiteral(source string, start int) (string, int, bool) {
	if start < 0 || start >= len(source) || (source[start] != '\'' && source[start] != '"') {
		return "", start, false
	}
	quote := source[start]
	var value strings.Builder
	for pos := start + 1; pos < len(source); pos++ {
		char := source[pos]
		if char == quote {
			return value.String(), pos + 1, true
		}
		if char == '\n' || char == '\r' || (char == '$' && quote == '"') {
			return "", start, false
		}
		if char != '\\' {
			value.WriteByte(char)
			continue
		}
		pos++
		if pos >= len(source) {
			return "", start, false
		}
		switch source[pos] {
		case '\\', '\'', '"':
			value.WriteByte(source[pos])
		case '$':
			value.WriteByte('$')
		case 'b':
			value.WriteByte('\b')
		case 'f':
			value.WriteByte('\f')
		case 'n':
			value.WriteByte('\n')
		case 'r':
			value.WriteByte('\r')
		case 't':
			value.WriteByte('\t')
		case 'u':
			if pos+4 >= len(source) {
				return "", start, false
			}
			code, err := strconv.ParseUint(source[pos+1:pos+5], 16, 16)
			if err != nil {
				return "", start, false
			}
			pos += 4
			r := rune(code)
			// Groovy can interpret a Unicode-escaped dollar as interpolation;
			// do not silently turn it into a static double-quoted condition.
			if quote == '"' && r == '$' {
				return "", start, false
			}
			if utf16.IsSurrogate(r) {
				if r < 0xD800 || r > 0xDBFF || pos+6 >= len(source) || source[pos+1:pos+3] != `\u` {
					return "", start, false
				}
				low, err := strconv.ParseUint(source[pos+3:pos+7], 16, 16)
				if err != nil || low < 0xDC00 || low > 0xDFFF {
					return "", start, false
				}
				r = utf16.DecodeRune(r, rune(low))
				pos += 6
			}
			value.WriteRune(r)
		default:
			return "", start, false
		}
	}
	return "", start, false
}
