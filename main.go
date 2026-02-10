package main

import (
	"log/slog"
	"os"

	"github.com/TheChosenGay/aichat/api"
	"github.com/TheChosenGay/aichat/service"
	"github.com/TheChosenGay/aichat/store"
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
	userDbStore := store.NewUserDbStore(store.NewMysqlInstance())
	userRedisStore := store.NewUserRedisStore(
		store.WithRedisClientName("user_service"),
		store.WithRedisDB(1),
		store.WithRedisAddr(os.Getenv("REDIS_ADDR")),
	)

	userSrv := service.NewUserService(userDbStore, userRedisStore)

	us := api.NewUserServer(userSrv, api.UserServerOpt{
		ListenPort: userServicePort,
	})

	if err := us.Run(); err != nil {
		slog.Error("Failed to run user server", "error", err)
		return
	}
}
