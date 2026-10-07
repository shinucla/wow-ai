// Package codec encodes and decodes the WoWAI pixel-strip byte stream.
// Matches addon/WoWAI/Codec.lua and bridge/capture.ps1.
package codec

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	Magic1      = 0xC7
	Magic2      = 0x1A
	Bits        = 3
	MaxPayload  = 3200
	DefaultCell = 4
	CellsPerRow = 200
	MaxRows     = 48
)

var (
	ErrNoMagic   = errors.New("no magic")
	ErrTruncated = errors.New("truncated")
	ErrLength    = errors.New("length")
	ErrChecksum  = errors.New("checksum")
)

// Fletcher16 matches Codec.lua (mod 255).
func Fletcher16(bytes []byte, from, to int) (s1, s2 byte) {
	var a, b int
	for i := from; i <= to; i++ {
		a = (a + int(bytes[i])) % 255
		b = (b + a) % 255
	}
	return byte(a), byte(b)
}

// Encode packs id + payload into 3-bit cell values (0..7).
func Encode(id uint16, payload []byte) []byte {
	lenN := len(payload)
	bytes := make([]byte, 0, 8+lenN)
	bytes = append(bytes, Magic1, Magic2, byte(id>>8), byte(id), byte(lenN>>8), byte(lenN))
	bytes = append(bytes, payload...)
	s1, s2 := Fletcher16(bytes, 2, 5+lenN)
	bytes = append(bytes, s1, s2)

	base := 1 << Bits
	cells := make([]byte, 0, (len(bytes)*8+Bits-1)/Bits)
	acc, nbits := 0, 0
	for _, b := range bytes {
		acc = acc*256 + int(b)
		nbits += 8
		for nbits >= Bits {
			shift := nbits - Bits
			cells = append(cells, byte((acc>>shift)%base))
			nbits = shift
			acc %= 1 << nbits
		}
	}
	if nbits > 0 {
		cells = append(cells, byte((acc<<(Bits-nbits))%base))
	}
	return cells
}

// CellColor returns 0/1 RGB for a cell value.
func CellColor(v byte) (r, g, b float64) {
	if (v/4)%2 == 1 {
		r = 1
	}
	if (v/2)%2 == 1 {
		g = 1
	}
	if v%2 == 1 {
		b = 1
	}
	return
}

// Sample is one RGB triple from the center of a cell (0–255).
type Sample struct{ R, G, B uint8 }

// CellValue maps a sample to a 3-bit cell (channel ≥ 128 = on).
func CellValue(s Sample) byte {
	var v byte
	if s.R >= 128 {
		v += 4
	}
	if s.G >= 128 {
		v += 2
	}
	if s.B >= 128 {
		v += 1
	}
	return v
}

// Message is a decoded strip frame.
type Message struct {
	ID      uint16
	Payload []byte
}

// DecodeCells turns a row-major cell stream into a message.
func DecodeCells(cells []byte, cellsPerRow, maxRows int) (*Message, error) {
	if cellsPerRow <= 0 {
		cellsPerRow = CellsPerRow
	}
	if maxRows <= 0 {
		maxRows = MaxRows
	}
	total := cellsPerRow * maxRows
	if len(cells) > total {
		cells = cells[:total]
	}

	acc, nbits := 0, 0
	bytes := make([]byte, 0, 64)
	needed := 6

	for i := 0; i < len(cells); i++ {
		v := int(cells[i] & 7)
		acc = (acc << Bits) | v
		nbits += Bits
		for nbits >= 8 {
			b := byte((acc >> (nbits - 8)) & 0xFF)
			bytes = append(bytes, b)
			nbits -= 8
			acc &= (1 << nbits) - 1
			if len(bytes) == 2 {
				if bytes[0] != Magic1 || bytes[1] != Magic2 {
					return nil, ErrNoMagic
				}
			}
			if len(bytes) == 6 {
				length := int(bytes[4])*256 + int(bytes[5])
				needed = 8 + length
				maxBytes := total * Bits / 8
				if needed > maxBytes {
					return nil, ErrLength
				}
			}
			if len(bytes) >= needed {
				break
			}
		}
		if len(bytes) >= needed {
			break
		}
	}
	if len(bytes) < needed {
		return nil, ErrTruncated
	}
	length := int(bytes[4])*256 + int(bytes[5])
	s1, s2 := Fletcher16(bytes, 2, 5+length)
	if bytes[6+length] != s1 || bytes[7+length] != s2 {
		return nil, ErrChecksum
	}
	payload := make([]byte, length)
	copy(payload, bytes[6:6+length])
	id := binary.BigEndian.Uint16(bytes[2:4])
	return &Message{ID: id, Payload: payload}, nil
}

// DecodeSamples decodes from per-cell RGB samples in row-major order.
func DecodeSamples(samples []Sample, cellsPerRow, maxRows int) (*Message, error) {
	cells := make([]byte, len(samples))
	for i, s := range samples {
		cells[i] = CellValue(s)
	}
	return DecodeCells(cells, cellsPerRow, maxRows)
}

// FormatError returns a short reject reason for logging.
func FormatError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprint(err)
}
