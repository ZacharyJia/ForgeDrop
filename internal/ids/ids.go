package ids

import (
	"crypto/rand"
	"encoding/base32"
)

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

func New() (string, error) {
	var b [20]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return encoding.EncodeToString(b[:]), nil
}
