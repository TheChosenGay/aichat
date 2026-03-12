package api

import (
	"encoding/json"
	"io"
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

func OK(w io.Writer, data any) error {
	return json.NewEncoder(w).Encode(
		APIResponse{
			Code: int(APIRespCodeOK),
			Data: data,
			Msg:  "success",
		})
}

func Fail(w io.Writer, code APIRespCode, msg string) error {
	return json.NewEncoder(w).Encode(
		APIResponse{
			Code: int(code),
			Data: nil,
			Msg:  msg,
		})
}

func BadRequest(w io.Writer, msg string) error {
	return Fail(w, APIRespCodeBadRequest, msg)
}

func Unauthorized(w io.Writer, msg string) error {
	return Fail(w, APIRespCodeUnauthorized, msg)
}

func InternalError(w io.Writer, msg string) error {
	return Fail(w, APIRespCodeInternalError, msg)
}
