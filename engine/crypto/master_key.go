package crypto

import (
	"encoding/hex"
	"errors"
	"strings"
)

// ParseMasterKey converts the configured TOTP master secret (hex, as read
// from TOTPMasterSecret / MORENOCORE_TOTP_MASTER_SECRET) into the 16-byte
// AES key used to encrypt stored TOTP secrets. It mirrors the C++
// BigNumber::ToByteArray<AES::KEY_SIZE_BYTES> semantics used by
// cs_account.cpp's 2FA handlers (and the auth server's parseTOTPMasterSecret):
// the key is the little-endian encoding of the configured hex *number*
// (BigNumber::ToByteArray defaults to little-endian); over-long values are
// truncated to the low 16 bytes, short ones are zero-padded.
func ParseMasterKey(value string) ([AESKeySize]byte, error) {
	var key [AESKeySize]byte
	value = strings.TrimSpace(value)
	if value == "" {
		return key, nil
	}
	if strings.HasPrefix(strings.ToLower(value), "0x") {
		return key, errors.New("TOTP master secret must be hexadecimal without a 0x prefix")
	}
	if len(value)%2 != 0 {
		value = "0" + value
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return key, errors.New("TOTP master secret must be hexadecimal")
	}
	if len(decoded) > len(key) {
		decoded = decoded[len(decoded)-len(key):]
	}
	copy(key[len(key)-len(decoded):], decoded)
	for i, j := 0, len(key)-1; i < j; i, j = i+1, j-1 {
		key[i], key[j] = key[j], key[i]
	}
	return key, nil
}
