package providerallowlist

import (
	"bytes"
	"encoding/binary"
	"unicode/utf16"
	"unicode/utf8"
)

// recoverPresenceEncoding mirrors JSON encoding detection only to retain an
// opt-in declaration. Its output is never used for acceptance or execution.
// Malformed suffixes retain the decoded prefix, as Python's replacement reader
// does; the original image and original decoder error remain authoritative.
func recoverPresenceEncoding(data []byte) ([]byte, bool) {
	var order binary.ByteOrder = binary.BigEndian
	width, skip := 0, 0
	switch {
	case bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}):
		return data[3:], true
	case bytes.HasPrefix(data, []byte{0, 0, 0xfe, 0xff}):
		width, skip = 4, 4
	case bytes.HasPrefix(data, []byte{0xff, 0xfe, 0, 0}):
		width, skip, order = 4, 4, binary.LittleEndian
	case bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		width, skip = 2, 2
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}):
		width, skip, order = 2, 2, binary.LittleEndian
	case len(data) >= 4 && data[0] == 0 && data[1] == 0 && data[2] == 0:
		width = 4
	case len(data) >= 4 && data[1] == 0 && data[2] == 0 && data[3] == 0:
		width, order = 4, binary.LittleEndian
	case len(data) >= 2 && data[0] == 0:
		width = 2
	case len(data) >= 2 && data[1] == 0:
		width, order = 2, binary.LittleEndian
	default:
		return nil, false
	}
	data = data[skip:]
	var points []rune
	if width == 2 {
		units := make([]uint16, 0, len(data)/2)
		for len(data) >= 2 {
			units = append(units, order.Uint16(data))
			data = data[2:]
		}
		points = utf16.Decode(units)
	} else {
		for len(data) >= 4 {
			point := rune(order.Uint32(data))
			if !utf8.ValidRune(point) {
				point = utf8.RuneError
			}
			points = append(points, point)
			data = data[4:]
		}
	}
	if len(data) != 0 {
		points = append(points, utf8.RuneError)
	}
	return []byte(string(points)), true
}
