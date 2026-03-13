package api

import (
	"encoding/json"
	"net/http"
)

type APIRespCode int

const (
	APIRespCodeOK            APIRespCode = 0
	APIRespCodeBadRequest    APIRespCode = 400
	APIRespCodeUnauthorized  APIRespCode = 401
	APIRespCodeInternalError APIRespCode = 500
)

type APIResponse struct {
	Code int    `json:"code"`
	Data any    `json:"data"`
	Msg  string `json:"msg"`
}

func writeJSON(w http.ResponseWriter, httpStatus int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(v)
}

func OK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, APIResponse{
		Code: int(APIRespCodeOK),
		Data: data,
		Msg:  "success",
	})
}

func Fail(w http.ResponseWriter, code APIRespCode, msg string) {
	writeJSON(w, int(code), APIResponse{
		Code: int(code),
		Msg:  msg,
	})
}

func BadRequest(w http.ResponseWriter, msg string) {
	Fail(w, APIRespCodeBadRequest, msg)
}

func Unauthorized(w http.ResponseWriter, msg string) {
	Fail(w, APIRespCodeUnauthorized, msg)
}

func InternalError(w http.ResponseWriter, msg string) {
	Fail(w, APIRespCodeInternalError, msg)
}
