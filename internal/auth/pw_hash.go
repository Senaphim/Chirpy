package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const TokenAccess string = "chirpy"

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

func MakeJWT(userId uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	claims := &jwt.RegisteredClaims{
		Issuer:    TokenAccess,
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)),
		Subject:   userId.String(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	siginingKey := []byte(tokenSecret)
	ss, err := token.SignedString(siginingKey)
	if err != nil {
		fmtErr := fmt.Errorf("Error signing key:\n%v", err)
		return "", fmtErr
	}

	return ss, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	claims := jwt.RegisteredClaims{}

	token, err := jwt.ParseWithClaims(
		tokenString,
		&claims,
		func(t *jwt.Token) (any, error) { return []byte(tokenSecret), nil },
	)
	if err != nil {
		fmtErr := fmt.Errorf("Error parsing token:\n%v", err)
		return uuid.Nil, fmtErr
	}

	userIdStr, err := token.Claims.GetSubject()
	if err != nil {
		fmtErr := fmt.Errorf("Error getting user id from token:\n%v", err)
		return uuid.Nil, fmtErr
	}

	issuer, err := token.Claims.GetIssuer()
	if err != nil {
		fmtErr := fmt.Errorf("Error getting issuer from token:\n%v", err)
		return uuid.Nil, fmtErr
	}
	if issuer != TokenAccess {
		return uuid.Nil, errors.New("Invalid issuer")
	}

	expiry, err := token.Claims.GetExpirationTime()
	if err != nil {
		fmtErr := fmt.Errorf("Error getting expiry time from token:\n%v", err)
		return uuid.Nil, fmtErr
	}
	if time.Now().UTC().After(expiry.Time) {
		return uuid.Nil, errors.New("Expired token")
	}

	id, err := uuid.Parse(userIdStr)
	if err != nil {
		fmtErr := fmt.Errorf("Error parsing user ID:\n%v", err)
		return uuid.Nil, fmtErr
	}
	return id, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	authorisation := headers.Get("Authorization")
	if authorisation == "" {
		return "", errors.New("No authorisation")
	}

	authorisationToken, found := strings.CutPrefix(strings.TrimSpace(authorisation), "Bearer ")
	if !found {
		return "", errors.New("Malformed bearer header")
	}

	return authorisationToken, nil
}
