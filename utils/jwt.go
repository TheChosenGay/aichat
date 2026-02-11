package utils

import (
	"errors"

	"github.com/TheChosenGay/aichat/types"
	jwt "github.com/golang-jwt/jwt/v5"
)

func GenerateJwt(user *types.User, secret string) (string, error) {
	// Create a new token object, specifying signing method and the claims
	// you would like it to contain.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userId": user.Id,
		"email":  user.Email,
	})
	// Sign and get the complete encoded token as a string using the secret
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func VerifyJwt(email string, jwtToken string, secret string) (bool, error) {
	token, err := jwt.Parse(jwtToken, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return false, err
	}
	emailClaim := token.Claims.(jwt.MapClaims)["email"]
	if emailClaim != email {
		return false, errors.New("user id mismatch")
	}

	return true, nil
}
