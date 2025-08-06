package user

import (
	"encoding/base64"
	"errors"

	"github.com/google/uuid"
)

func GenerateToken() string {
	id := uuid.New().String()
	return base64.StdEncoding.EncodeToString([]byte(id))
}

func ValidateToken(tk string) error {
	dec, err := base64.StdEncoding.DecodeString(tk)
	if err != nil {
		return errors.New("invalid session")
	}

	_, err = uuid.Parse(string(dec))
	if err != nil {
		return errors.New("invalid session")
	}

	return nil
}
