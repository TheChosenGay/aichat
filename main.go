package main

import (
	"log/slog"
	"os"

	"github.com/TheChosenGay/aichat/api"
	"github.com/TheChosenGay/aichat/service"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		slog.Error("Failed to load .env file, error: %v", err)
		return
	}

	userServicePort := os.Getenv("USER_SERVICE_LISTEN_PORT")
	slog.Info("User service port", "port", userServicePort)
	// user server
	// userDbStore := store.NewUserDbStore(db)
	// userRedisStore := store.NewUserRedisStore(redis)
	userSrv := service.NewUserService(nil, nil)
	us := api.NewUserServer(userSrv, api.UserServerOpt{
		ListenPort: userServicePort,
	})

	if err := us.Run(); err != nil {
		slog.Error("Failed to run user server", "error", err)
		return
	}
}
