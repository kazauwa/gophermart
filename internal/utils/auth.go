package utils

import (
	"errors"

	"github.com/alexedwards/argon2id"
	"golang.org/x/crypto/bcrypt"
)

func CheckPassword(hash, password string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err == nil {
		return true, nil
	}

	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return false, nil
	}

	return false, err
}

func GeneratePasswordHash(password string) (string, error) {
	// TODO: прокидывать параметры из конфига
	passwordHash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return "", err
	}
	return passwordHash, nil
}
