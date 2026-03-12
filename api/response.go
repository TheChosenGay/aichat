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

func OK(w http.ResponseWriter, data any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(
		APIResponse{
			Code: int(APIRespCodeOK),
			Data: data,
			Msg:  "success",
		})
}

func Fail(w http.ResponseWriter, code APIRespCode, msg string) error {
	w.WriteHeader(int(code))
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(
		APIResponse{
			Code: int(code),
			Data: nil,
			Msg:  msg,
		})
}

func BadRequest(w http.ResponseWriter, msg string) error {
	return Fail(w, APIRespCodeBadRequest, msg)
}

func Unauthorized(w http.ResponseWriter, msg string) error {
	return Fail(w, APIRespCodeUnauthorized, msg)
}

func InternalError(w http.ResponseWriter, msg string) error {
	return Fail(w, APIRespCodeInternalError, msg)
}
