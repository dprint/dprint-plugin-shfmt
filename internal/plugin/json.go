package plugin

import (
	"strings"
	"unicode/utf8"
)

const hexDigits = "0123456789abcdef"

// appendJSONString appends the value as a JSON string. Invalid UTF-8 is
// replaced since JSON can't represent it.
func appendJSONString(dst []byte, value string) []byte {
	value = strings.ToValidUTF8(value, string(utf8.RuneError))
	dst = append(dst, '"')
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case c == '"' || c == '\\':
			dst = append(dst, '\\', c)
		case c == '\n':
			dst = append(dst, '\\', 'n')
		case c == '\r':
			dst = append(dst, '\\', 'r')
		case c == '\t':
			dst = append(dst, '\\', 't')
		case c < 0x20:
			dst = append(dst, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
		default:
			dst = append(dst, c)
		}
	}
	return append(dst, '"')
}

func appendJSONStringArray(dst []byte, values []string) []byte {
	dst = append(dst, '[')
	for i, value := range values {
		if i != 0 {
			dst = append(dst, ',')
		}
		dst = appendJSONString(dst, value)
	}
	return append(dst, ']')
}
