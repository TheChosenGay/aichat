package api

import (
	"math"
	"net/http"
	"strconv"
)

func getCookieAndLimit(r *http.Request, cookieKey string, limitKey string, defaultLimit int) (int64, int, error) {
	cookie := int64(math.MaxInt64)
	if cookieStr := r.URL.Query().Get(cookieKey); cookieStr != "" {
		cookieVal, err := strconv.ParseInt(cookieStr, 10, 64)
		if err != nil {
			return 0, 0, err
		}
		cookie = cookieVal
	}
	limit := defaultLimit
	if limitStr := r.URL.Query().Get(limitKey); limitStr != "" {
		limitVal, err := strconv.Atoi(limitStr)
		if err != nil {
			return 0, 0, err
		}
		limit = limitVal
	}
	return cookie, limit, nil
}
