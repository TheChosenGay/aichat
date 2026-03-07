package main

import (
	"log/slog"
	"os"

	"github.com/TheChosenGay/aichat/api"
	"github.com/TheChosenGay/aichat/gateway"
	"github.com/TheChosenGay/aichat/gateway/ws"
	"github.com/TheChosenGay/aichat/service"
	"github.com/TheChosenGay/aichat/store"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		slog.Error("Failed to load .env file", "error", err)
		return
	}

	userServicePort := os.Getenv("USER_SERVICE_LISTEN_PORT")
	slog.Info("User service port", "port", userServicePort)
	db := store.NewMysqlInstance()
	// user server
	userDbStore := store.NewUserDbStore(db)
	userRedisStore := store.NewUserRedisStore(
		store.WithRedisClientName("user_service"),
		store.WithRedisDB(1),
		store.WithRedisAddr(os.Getenv("REDIS_ADDR")),
	)

	userSrv := service.NewUserService(userDbStore, userRedisStore)

	us := api.NewUserServer(userSrv, api.UserServerOpt{
		ListenPort: userServicePort,
	})
	wsServicePort := os.Getenv("GATEWAY_SERVICE_LISTEN_PORT")
	connManager := gateway.NewConnManager()
	msgStore := store.NewMessageDbStore(db)
	msgService := service.NewMessageService(msgStore, connManager)
	wsServer := ws.NewWsServer(&gateway.ServerOpt{
		ListenPort: wsServicePort,
	}, connManager, msgService)

	go func() {
		if err := wsServer.Run(); err != nil {
			slog.Error("Failed to run ws server", "error", err)
			return
		}
	}()

	if err := us.Run(); err != nil {
		slog.Error("Failed to run user server", "error", err)
		return
	}
}
