package sandbox

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// GenerateUniqueFlag generates a deterministic unique HMAC flag for a (baseFlag, studentID) pair.
func GenerateUniqueFlag(baseFlag, studentID string) string {
	mac := hmac.New(sha256.New, []byte(baseFlag))
	mac.Write([]byte(studentID))
	res := hex.EncodeToString(mac.Sum(nil))
	if len(res) > 16 {
		return res[:16]
	}
	return res
}
