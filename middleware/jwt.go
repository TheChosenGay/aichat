package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/TheChosenGay/aichat/utils"
)

type HttpFunc func(w http.ResponseWriter, r *http.Request)

type contextKey string

const UserIdKey contextKey = "userId"

func JwtMiddleware(next HttpFunc) HttpFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			token = r.Header.Get("Authorization")
			token = strings.TrimPrefix(token, "Bearer ")
			if token == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		userId, err := utils.VerifyJwt(token)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), UserIdKey, userId)
		next(w, r.WithContext(ctx))
	}
}
