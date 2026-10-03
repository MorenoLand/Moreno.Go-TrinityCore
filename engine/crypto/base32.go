package crypto

import (
	"errors"
	"strings"
)

// Base32 mirrors Trinity::Encoding::Base32 (src/common/Encoding/Base32.cpp):
// standard base32 alphabet, case-insensitive decode, user-friendly aliases
// ('0'->'O', '1'->'l', '8'->'B'), '=' padding accepted and stripped, and
// trailing non-zero bits rejected as a decode error.

var base32Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

var errBase32Decode = errors.New("invalid base32 data")

func base32DecodeChar(c byte) (byte, bool) {
	switch c {
	case '0':
		return base32DecodeChar('O')
	case '1':
		return base32DecodeChar('l')
	case '8':
		return base32DecodeChar('B')
	}
	switch {
	case 'A' <= c && c <= 'Z':
		return c - 'A', true
	case 'a' <= c && c <= 'z':
		return c - 'a', true
	case '2' <= c && c <= '7':
		return c - '2' + 26, true
	}
	return 0, false
}

func Base32Encode(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	var out strings.Builder
	out.Grow((len(data)*8 + 4) / 5)
	var current uint32
	var bitsLeft uint
	for _, b := range data {
		current = (current << 8) | uint32(b)
		bitsLeft += 8
		for bitsLeft >= 5 {
			bitsLeft -= 5
			out.WriteByte(base32Alphabet[(current>>bitsLeft)&0x1f])
		}
	}
	if bitsLeft > 0 {
		out.WriteByte(base32Alphabet[(current<<(5-bitsLeft))&0x1f])
	}
	for out.Len()%8 != 0 {
		out.WriteByte('=')
	}
	return out.String()
}

func Base32Decode(data string) ([]byte, error) {
	out := make([]byte, 0, len(data)*5/8)
	var currentByte byte
	bitsLeft := 8
	i := 0
	for ; i < len(data); i++ {
		if data[i] == '=' {
			break
		}
		cur, ok := base32DecodeChar(data[i])
		if !ok {
			return nil, errBase32Decode
		}
		if bitsLeft > 5 {
			bitsLeft -= 5
			currentByte |= cur << bitsLeft
		} else {
			rem := 5 - bitsLeft
			currentByte |= cur >> rem
			out = append(out, currentByte)
			currentByte = (cur & ((1 << rem) - 1)) << (8 - rem)
			bitsLeft = 8 - rem
		}
	}
	if currentByte != 0 {
		return nil, errBase32Decode
	}
	for i < len(data) && data[i] == '=' && bitsLeft != 8 {
		if bitsLeft > 5 {
			bitsLeft -= 5
		} else {
			bitsLeft += 8 - 5
		}
		i++
	}
	if i != len(data) {
		return nil, errBase32Decode
	}
	return out, nil
}
