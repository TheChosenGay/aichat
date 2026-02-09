package utils

import (
	"errors"

	jwt "github.com/golang-jwt/jwt/v5"
)

func GenerateJwt(userId string, secret string) (string, error) {
	// Create a new token object, specifying signing method and the claims
	// you would like it to contain.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userId": userId,
	})
	// Sign and get the complete encoded token as a string using the secret
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func VerifyJwt(userId string, jwtToken string, secret string) (bool, error) {
	token, err := jwt.Parse(jwtToken, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return false, err
	}
	userIdClaim := token.Claims.(jwt.MapClaims)["userId"]
	if userIdClaim != userId {
		return false, errors.New("user id mismatch")
	}

	return true, nil
}
