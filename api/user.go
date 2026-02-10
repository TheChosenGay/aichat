package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/TheChosenGay/aichat/service"
	"github.com/TheChosenGay/aichat/types"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type UserServerOpt struct {
	ListenPort string
}

type UserServerOption func(*UserServerOpt)

type UserServer struct {
	opt         *UserServerOpt
	userService service.UserService
}

func NewUserServer(userSrv service.UserService, opt UserServerOpt, opts ...UserServerOption) *UserServer {
	u := &UserServer{
		userService: userSrv,
		opt:         &opt,
	}

	for _, o := range opts {
		o(u.opt)
	}

	return u
}

func (u *UserServer) Run() error {
	mx := mux.NewRouter()
	mx.HandleFunc("/user/create", u.createUserHandler).Methods("POST")
	mx.HandleFunc("/user/login", u.loginHandler).Methods("POST")

	slog.Info("User server listening on port", "port", u.opt.ListenPort)
	return http.ListenAndServe(u.opt.ListenPort, mx)
}

// post address:port/user/create?email=xxx&password=xxx&name=xxx&birthAt=xxxx&sex=1|0
func (u *UserServer) createUserHandler(w http.ResponseWriter, r *http.Request) {
	vars := r.URL.Query()
	email := vars.Get("email")
	password := vars.Get("password")
	name := vars.Get("name")
	sex := vars.Get("sex") == "1"
	uid := uuid.New().String()
	user := types.NewUser(uid, email, password, name, sex, 0, time.Now().UnixNano())

	slog.Info("create user", "id", uid)
	if err := validator.New().Struct(user); err != nil {
		slog.Error("Failed to validate user", "error", err.Error())
		http.Error(w, service.NewError(service.ErrServiceUser, service.ErrUserCreate, service.ErrParamInvalid).String(), http.StatusBadRequest)
		return
	}

	// save user to database
	if err := u.userService.CreateUser(user); err != nil {
		slog.Error("Failed to create user", "error", err.Error())
		http.Error(w, service.NewError(service.ErrServiceUser, service.ErrUserCreate, err).String(), http.StatusInternalServerError)
		return
	}

	if err := WriteToJson(w, map[string]any{
		"code": 0,
	}); err != nil {
		slog.Error("Failed to write to json", "error", err.Error())
		return
	}
}

func (u *UserServer) loginHandler(w http.ResponseWriter, r *http.Request) {
	vars := r.URL.Query()
	userId := vars.Get("id")
	password := vars.Get("password")
	if userId == "" || password == "" {
		http.Error(w, service.NewError(service.ErrServiceUser, service.ErrUserLogin, service.ErrParamInvalid).String(), http.StatusBadRequest)
		return
	}
	jwtToken, err := u.userService.LoginByPassword(userId, password)
	if err != nil {
		http.Error(w, service.NewError(service.ErrServiceUser, service.ErrUserLogin, err).String(), http.StatusInternalServerError)
		return
	}

	if err := WriteToJson(w, map[string]any{
		"code":     0,
		"jwtToken": jwtToken,
	}); err != nil {
		slog.Error("Failed to write to json", "error", err.Error())
		return
	}
}

func WriteToJson(w io.Writer, v any) error {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return err
	}
	return nil
}
