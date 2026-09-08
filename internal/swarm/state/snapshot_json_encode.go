// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"encoding/json"
	"strconv"
	"time"
	"unicode/utf8"
)

// jsonHexDigits is the lowercase hex alphabet of the \u escapes.
const jsonHexDigits = "0123456789abcdef"

// jsonHTMLSafeSet mirrors the html safe set of encoding/json: the
// printable ASCII bytes a JSON string may carry verbatim while the
// HTML escaping of json.Marshal stays active (the quotes, the
// backslash, the angle brackets and the ampersand escape).
var jsonHTMLSafeSet = func() [utf8.RuneSelf]bool {
	var set [utf8.RuneSelf]bool
	for i := 0x20; i < utf8.RuneSelf; i++ {
		set[i] = true
	}
	set['"'] = false
	set['\\'] = false
	set['<'] = false
	set['>'] = false
	set['&'] = false

	return set
}()

// appendJSONString appends the JSON encoding of s - the exact bytes
// of the encoding/json string encoder with HTML escaping: the two
// mandatory escapes, the control bytes as \u00XX, the invalid UTF-8
// bytes as the replacement form of the running stdlib and the
// U+2028/U+2029 line break escapes (they are valid JSON but break
// JavaScript string literals).
func appendJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if jsonHTMLSafeSet[b] {
				i++

				continue
			}
			dst = append(dst, s[start:i]...)
			switch b {
			case '\\', '"':
				dst = append(dst, '\\', b)
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, '\\', 'u', '0', '0',
					jsonHexDigits[b>>4], jsonHexDigits[b&0xF])
			}
			i++
			start = i

			continue
		}
		c, size := utf8.DecodeRuneInString(s[i:])
		if c == utf8.RuneError && size == 1 {
			dst = append(dst, s[start:i]...)
			dst = append(dst, jsonInvalidUTF8Replacement...)
			i += size
			start = i

			continue
		}
		if c == '\u2028' || c == '\u2029' {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', jsonHexDigits[c&0xF])
			i += size
			start = i

			continue
		}
		i += size
	}
	dst = append(dst, s[start:]...)
	dst = append(dst, '"')

	return dst
}

// jsonInvalidUTF8Replacement is the byte sequence the running
// stdlib writes for one invalid UTF-8 byte inside a JSON string: the
// classic encoding/json appends the \ufffd escape sequence while the
// v2 backed encoder (GOEXPERIMENT=jsonv2 and the toolchains that
// ship it by default) appends the literal U+FFFD replacement rune.
// The init time probe mirrors json.Marshal of the same process so
// the hand rolled writer stays byte identical to the reflection
// encoder on every toolchain: the parity is pinned by the reflection
// tests and both replacement forms by
// TestAppendJSONStringInvalidUTF8Modes.
var jsonInvalidUTF8Replacement = func() []byte {
	probe, err := json.Marshal("\xff")
	if err == nil && string(probe) == "\"\uFFFD\"" {
		return []byte("\uFFFD")
	}

	return []byte(`\ufffd`)
}()

// appendJSONFloat appends the JSON encoding of f - the ES6 number
// to string conversion of encoding/json: the shortest round trip
// form, plain decimal notation inside 1e-6..1e21, scientific outside.
func appendJSONFloat(dst []byte, f float64) []byte {
	abs := 0.0
	if f > 0 {
		abs = f
	} else if f < 0 {
		abs = -f
	}
	format := byte('f')
	if abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		format = 'e'
	}
	dst = strconv.AppendFloat(dst, f, format, -1, 64)
	if format == 'e' {
		// The reflection encoder cleans e-09 up to e-9.
		n := len(dst)
		if n >= 4 && dst[n-4] == 'e' && dst[n-3] == '-' && dst[n-2] == '0' {
			dst[n-2] = dst[n-1]
			dst = dst[:n-1]
		}
	}

	return dst
}

// appendJSONTime appends the JSON encoding of t - the RFC 3339 with
// nanoseconds of the time.Time MarshalJSON (the fractional seconds
// lose their trailing zeros, the zero time prints as year 1).
func appendJSONTime(dst []byte, t time.Time) []byte {
	dst = append(dst, '"')
	dst = t.AppendFormat(dst, time.RFC3339Nano)
	dst = append(dst, '"')

	return dst
}
