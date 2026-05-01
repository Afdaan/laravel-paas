// ===========================================
// UID Utility
// ===========================================
// Handles bidirectional encoding of integer IDs
// and generation of random secure UIDs.
// ===========================================
package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

// customAlphabet is our "secret" Base62 alphabet
const customAlphabet = "P6v7Vw8Wx9Xy0YzZaAbBcCdDeEfFgGhHiIjJkKlLmMnNoOpPqQrRsStTuU12345"

// EncodeUID converts a uint ID to a secure string using a salt
// Legacy: Used for backfilling existing projects to keep old URLs valid.
func EncodeUID(id uint, salt string) string {
	if id == 0 {
		return ""
	}

	var mask uint32
	for i := 0; i < len(salt); i++ {
		mask = mask*31 + uint32(salt[i])
	}

	scrambled := uint64(id) ^ uint64(mask)
	scrambled += 1000000 

	var builder strings.Builder
	base := uint64(len(customAlphabet))
	
	n := scrambled
	for n > 0 {
		builder.WriteByte(customAlphabet[n%base])
		n = n / base
	}

	return builder.String()
}

// DecodeUID converts a secure string back to a uint ID using the same salt
// Legacy: Keeping for backward compatibility if needed.
func DecodeUID(uid string, salt string) (uint, error) {
	if uid == "" {
		return 0, fmt.Errorf("empty uid")
	}

	var mask uint32
	for i := 0; i < len(salt); i++ {
		mask = mask*31 + uint32(salt[i])
	}

	var scrambled uint64
	base := uint64(len(customAlphabet))
	
	alphabetIdx := make(map[byte]uint64)
	for i := 0; i < len(customAlphabet); i++ {
		alphabetIdx[customAlphabet[i]] = uint64(i)
	}

	multiplier := uint64(1)
	for i := 0; i < len(uid); i++ {
		val, ok := alphabetIdx[uid[i]]
		if !ok {
			return 0, fmt.Errorf("invalid character in uid")
		}
		scrambled += val * multiplier
		multiplier *= base
	}

	if scrambled < 1000000 {
		return 0, fmt.Errorf("invalid uid (too small)")
	}
	
	id := uint( (scrambled - 1000000) ^ uint64(mask) )
	return id, nil
}

// GenerateRandomUID creates a secure, non-predictable random string.
// Used for new projects ("Gaya #1").
func GenerateRandomUID() string {
	length := 10
	var sb strings.Builder
	for i := 0; i < length; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(customAlphabet))))
		sb.WriteByte(customAlphabet[n.Int64()])
	}
	return sb.String()
}
