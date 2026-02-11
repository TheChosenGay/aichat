package middleware

import (
	"net/http"

	"github.com/TheChosenGay/aichat/store"
	"github.com/TheChosenGay/aichat/utils"
)

type HttpFunc func(w http.ResponseWriter, r *http.Request)

func JwtMiddleware(next HttpFunc, redisStore *store.UserRedisStore) HttpFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		email := r.Header.Get("Email")
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		_, secret, err := redisStore.GetJwt(email)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		valid, err := utils.VerifyJwt(email, token, secret)
		if err != nil || !valid {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
