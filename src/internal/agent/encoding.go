package agent

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

func normalizeEncoding(name string) encoding.Encoding {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "windows-1251", "cp1251", "win1251":
		return charmap.Windows1251
	case "cp866", "ibm866", "866":
		return charmap.CodePage866
	case "koi8-r", "koi8r":
		return charmap.KOI8R
	default:
		return nil
	}
}

func decodeEncoding(data []byte, name string) string {
	if encoding := normalizeEncoding(name); encoding != nil {
		decoded, err := encoding.NewDecoder().Bytes(data)
		if err == nil {
			return strings.TrimRight(string(decoded), "\r")
		}
	}
	trimmed := bytes.TrimRight(data, "\r")
	if utf8.Valid(trimmed) {
		return string(trimmed)
	}
	decoded, err := charmap.Windows1251.NewDecoder().Bytes(trimmed)
	if err != nil {
		return string(bytes.ToValidUTF8(trimmed, []byte("�")))
	}
	return strings.TrimRight(string(decoded), "\r")
}
