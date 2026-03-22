package main

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof" // 注册 pprof 路由到 DefaultServeMux
	"os"
	"runtime"

	"github.com/TheChosenGay/aichat/api"
	"github.com/TheChosenGay/aichat/gateway"
	"github.com/TheChosenGay/aichat/gateway/ws"
	"github.com/TheChosenGay/aichat/service"
	"github.com/TheChosenGay/aichat/service/router"
	"github.com/TheChosenGay/aichat/store"
	"github.com/TheChosenGay/aichat/store/cos"
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

	// 开启 block 和 mutex 采样，pprof 才能采集到阻塞和锁竞争数据
	runtime.SetBlockProfileRate(1)
	runtime.SetMutexProfileFraction(1)

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

	cosClient := cos.NewClient(
		os.Getenv("COS_BUCKET"),
		os.Getenv("COS_REGION"),
		os.Getenv("COS_SECRET_ID"),
		os.Getenv("COS_SECRET_KEY"),
	)

	connManager := gateway.NewConnManager()
	redisMsgRouter := router.NewRedisMsgRouter(connManager, userRedisStore)
	userSrv := service.NewUserService(userDbStore, userRedisStore, connManager, cosClient)
	roomSrv := service.NewRoomService(roomStore, userDbStore)

	conversationDbStore := store.NewConversationDbStore(db)
	conversationSrv := service.NewConversationService(conversationDbStore, userSrv)

	relationDbStore := store.NewRelationshipDbStore(db)
	relationSrv := service.NewRelationshipService(relationDbStore, userSrv)
	msgService := service.NewMessageService(msgStore, roomSrv, conversationSrv, redisMsgRouter, userSrv)
	apiServer := api.NewServer(
		&api.ServerOpt{
			ListenPort: userServicePort,
		},
		api.NewUserServer(userSrv, api.UserServerOpt{
			ListenPort: userServicePort,
		}),
		api.NewRoomServer(roomSrv),
		api.NewRelationServer(api.RelationServerOpt{
			ListenPort: userServicePort,
		}, relationSrv),
		api.NewConversationServer(conversationSrv),
		api.NewMessageServer(*msgService),
	)

	wsServicePort := os.Getenv("GATEWAY_SERVICE_LISTEN_PORT")
	wsServer := ws.NewWsServer(&gateway.ServerOpt{
		ListenPort: wsServicePort,
	}, connManager, msgService, userSrv, redisMsgRouter)

	go func() {
		if err := wsServer.Run(); err != nil {
			slog.Error("Failed to run ws server", "error", err)
			return
		}
	}()

	// pprof 独立端口，仅内部使用
	go func() {
		slog.Info("pprof server", "addr", ":6060")
		if err := http.ListenAndServe(":6060", nil); err != nil {
			slog.Error("pprof server failed", "error", err)
		}
	}()

	if err := apiServer.Run(); err != nil {
		slog.Error("Failed to run user server", "error", err)
		return
	}
}
