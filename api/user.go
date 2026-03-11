package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/TheChosenGay/aichat/middleware"
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

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=32"`
}

type CreateRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=32"`
	Name     string `json:"name" validate:"required,min=3,max=32"`
	Sex      bool   `json:"sex" validate:"required,boolean"`
}

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

// MARK - RegisterHandler
func (u *UserServer) RegisterHandler(mx *mux.Router) {
	mx.HandleFunc("/user/create", u.createUserHandler).Methods("POST")
	mx.HandleFunc("/user/login", u.loginHandler).Methods("POST")
	mx.HandleFunc("/user/logout", middleware.JwtMiddleware(u.logoutHandler)).Methods("POST")
	mx.HandleFunc("/user/list/{limit}", u.listUserHandler).Methods("GET")
}

// post address:port/user/create?email=xxx&password=xxx&name=xxx&birthAt=xxxx&sex=1|0
func (u *UserServer) createUserHandler(w http.ResponseWriter, r *http.Request) {
	req := &CreateRequest{}
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		WriteToJson(w, map[string]any{
			"code":  1,
			"error": err.Error(),
		})
		return
	}
	email := req.Email
	password := req.Password
	name := req.Name
	sex := req.Sex

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

// post address:port/user/login?id=xxx&password=xxx
func (u *UserServer) loginHandler(w http.ResponseWriter, r *http.Request) {
	req := &LoginRequest{}
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		WriteToJson(w, map[string]any{
			"code":  1,
			"error": err.Error(),
		})
		return
	}
	email := req.Email
	password := req.Password

	if email == "" || password == "" {
		http.Error(w, service.NewError(service.ErrServiceUser, service.ErrUserLogin, service.ErrParamInvalid).String(), http.StatusBadRequest)
		return
	}
	jwtToken, err := u.userService.LoginByEmail(email, password)
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

func (u *UserServer) logoutHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value(middleware.UserIdKey).(string)
	if err := u.userService.Logout(userId); err != nil {
		WriteToJson(w, map[string]any{
			"code": 1,
		})
		return
	}
	if err := WriteToJson(w, map[string]any{
		"code":   0,
		"logout": true,
	}); err != nil {
		slog.Error("Failed to write to json", "error", err.Error())
		return
	}
}

// get address:port/user/list/{limit}
func (u *UserServer) listUserHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	limit, err := strconv.Atoi(vars["limit"])
	if err != nil {
		http.Error(w, service.NewError(service.ErrServiceUser, service.ErrUserList, err).String(), http.StatusBadRequest)
		return
	}
	users, err := u.userService.ListUsers(limit)
	if err != nil {
		http.Error(w, service.NewError(service.ErrServiceUser, service.ErrUserList, err).String(), http.StatusInternalServerError)
		return
	}

	if err := WriteToJson(w, map[string]any{
		"code":  0,
		"users": users,
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
