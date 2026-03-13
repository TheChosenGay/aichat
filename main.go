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
	// 配置 slog 显示文件名和行号
	slog.SetDefault(slog.New(
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}),
	))

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
	msgStore := store.NewMessageDbStore(db)
	roomStore := store.NewRoomDbStore(db)

	connManager := gateway.NewConnManager()
	userSrv := service.NewUserService(userDbStore, userRedisStore, connManager)
	roomSrv := service.NewRoomService(roomStore, userDbStore)

	apiServer := api.NewServer(
		&api.ServerOpt{
			ListenPort: userServicePort,
		},
		api.NewUserServer(userSrv, api.UserServerOpt{
			ListenPort: userServicePort,
		}),
		api.NewRoomServer(roomSrv),
	)

	wsServicePort := os.Getenv("GATEWAY_SERVICE_LISTEN_PORT")
	msgService := service.NewMessageService(msgStore, roomSrv, connManager, userSrv)
	wsServer := ws.NewWsServer(&gateway.ServerOpt{
		ListenPort: wsServicePort,
	}, connManager, msgService, userSrv)

	go func() {
		if err := wsServer.Run(); err != nil {
			slog.Error("Failed to run ws server", "error", err)
			return
		}
	}()

	if err := apiServer.Run(); err != nil {
		slog.Error("Failed to run user server", "error", err)
		return
	}
}
