package utils

import (
	"errors"
	"os"
	"time"

	"github.com/TheChosenGay/aichat/types"
	jwt "github.com/golang-jwt/jwt/v5"
)

const expireKey = "exp"

func GenerateJwt(user *types.User) (string, error) {
	// Create a new token object, specifying signing method and the claims
	// you would like it to contain.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userId":  user.Id,
		"email":   user.Email,
		expireKey: time.Now().Add(24 * time.Hour).Unix(),
	})
	// Sign and get the complete encoded token as a string using the secret
	secret := os.Getenv("JWT_SECRET")
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func VerifyJwt(jwtToken string) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	token, err := jwt.Parse(jwtToken, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return "", err
	}
	expireClaim := token.Claims.(jwt.MapClaims)[expireKey]
	if expireClaim == nil {
		return "", errors.New("expire claim not found")
	}

	userId := token.Claims.(jwt.MapClaims)["userId"]

	return userId.(string), nil
}
