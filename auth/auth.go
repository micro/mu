package auth

import (
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/google/uuid"
)

type Session struct {
	ID    string
	Token string
}

func GetSession(r *http.Request) (*Session, error) {
	c, err := r.Cookie("session")
	if err != nil {
		return nil, err
	}

	if c == nil {
		return nil, errors.New("session not found")
	}

	return ParseToken(c.Value)
}

func ParseToken(tk string) (*Session, error) {
	dec, err := base64.StdEncoding.DecodeString(tk)
	if err != nil {
		return nil, errors.New("invalid session")
	}

	id, err := uuid.Parse(string(dec))
	if err != nil {
		return nil, errors.New("invalid session")
	}

	return &Session{
		ID:    id.String(),
		Token: tk,
	}, nil

}

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
