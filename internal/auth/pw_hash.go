package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	bytestr := []byte(password)
	hash, err := bcrypt.GenerateFromPassword(bytestr, 10)
	if err != nil {
		fmtErr := fmt.Errorf("Error generating hash from password:\n%v", err)
		return "", fmtErr
	}

	return string(hash), nil
}

func CheckPasswordHash(password, hash string) error {
	bytestr := []byte(password)
	bytehash := []byte(hash)
	err := bcrypt.CompareHashAndPassword(bytehash, bytestr)
	if err != nil {
		return err
	}

	return nil
}
