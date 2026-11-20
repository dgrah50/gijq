package jsonview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"
)

const hexDigits = "0123456789abcdef"

// appendString appends s as a JSON string exactly as encoding/json does with
// SetEscapeHTML(false): only '"', '\\' and bytes < 0x20 are escaped, invalid
// UTF-8 becomes the literal escape `�`, and U+2028/U+2029 are escaped.
func appendString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if b >= 0x20 && b != '"' && b != '\\' {
				i++
				continue
			}
			dst = append(dst, s[start:i]...)
			switch b {
			case '\\', '"':
				dst = append(dst, '\\', b)
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, '\\', 'u', '0', '0', hexDigits[b>>4], hexDigits[b&0xF])
			}
			i++
			start = i
			continue
		}
		c, size := utf8.DecodeRuneInString(s[i:])
		if c == utf8.RuneError && size == 1 {
			dst = append(dst, s[start:i]...)
			dst = append(dst, `�`...)
			i += size
			start = i
			continue
		}
		if c == ' ' || c == ' ' {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hexDigits[c&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}

// appendFloat mirrors encoding/json's floatEncoder for float64. The caller
// must have checked that f is finite.
func appendFloat(dst []byte, f float64) []byte {
	abs := math.Abs(f)
	format := byte('f')
	if abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		format = 'e'
	}
	dst = strconv.AppendFloat(dst, f, format, -1, 64)
	if format == 'e' {
		// clean up e-09 to e-9
		n := len(dst)
		if n >= 4 && dst[n-4] == 'e' && dst[n-3] == '-' && dst[n-2] == '0' {
			dst[n-2] = dst[n-1]
			dst = dst[:n-1]
		}
	}
	return dst
}

// appendScalar appends the encoding of a non-container value, matching
// encoding/json except where gojq's own encoder (and jq) differ on purpose:
// NaN is `null` and infinities are clamped to +-MaxFloat64, so nothing ever
// forces a whole-value fallback. Invalid json.Number text is emitted verbatim
// (that is what %v would print for the node). A type gojq never produces is
// rendered by compact json.Marshal (no HTML escaping), or %v on error, on one
// line.
func appendScalar(dst []byte, v any) []byte {
	switch x := v.(type) {
	case nil:
		return append(dst, "null"...)
	case bool:
		if x {
			return append(dst, "true"...)
		}
		return append(dst, "false"...)
	case int:
		return strconv.AppendInt(dst, int64(x), 10)
	case float64:
		if math.IsNaN(x) {
			return append(dst, "null"...)
		}
		return appendFloat(dst, min(max(x, -math.MaxFloat64), math.MaxFloat64))
	case string:
		return appendString(dst, x)
	case *big.Int:
		if x == nil {
			return append(dst, "null"...)
		}
		return x.Append(dst, 10)
	case json.Number:
		if x == "" {
			return append(dst, '0')
		}
		return append(dst, x...)
	case []any: // nil slice
		return append(dst, "null"...)
	case map[string]any: // nil map
		return append(dst, "null"...)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return append(dst, strings.ReplaceAll(fmt.Sprintf("%v", v), "\n", `\n`)...)
	}
	return append(dst, bytes.TrimSuffix(buf.Bytes(), []byte("\n"))...)
}
