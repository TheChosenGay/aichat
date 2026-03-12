package ws

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/TheChosenGay/aichat/gateway"
	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

type WsConn struct {
	id   string
	conn *websocket.Conn

	onMessage gateway.ConnMessageCallback
	onClose   gateway.ConnCloseCallback
	onConnect gateway.ConnConnectCallback

	closeOnce sync.Once
	closeCh   chan struct{}
	writeCh   chan []byte
}

var _ gateway.Conn = (*WsConn)(nil)

func NewWsConn(id string, conn *websocket.Conn, onConnect gateway.ConnConnectCallback, onClose gateway.ConnCloseCallback, onMessage gateway.ConnMessageCallback) *WsConn {
	return &WsConn{
		id:        id,
		onConnect: onConnect,
		onClose:   onClose,
		onMessage: onMessage,
		conn:      conn,
		closeOnce: sync.Once{},
		closeCh:   make(chan struct{}),
		writeCh:   make(chan []byte, 20),
	}
}

func (c *WsConn) Id() string {
	return c.id
}

func (c *WsConn) Push(data []byte) error {
	select {
	case c.writeCh <- data:
		return nil
	case <-c.closeCh:
		return errors.New("connection closed")
	}
}

func (c *WsConn) Close() error {
	c.closeOnce.Do(func() {
		// close 只被执行一次
		close(c.closeCh)
		c.onClose(c.id)
		c.conn.Close()
	})
	return nil
}

func (c *WsConn) Start() {
	c.Read()
	c.Write()
	// 链接建立后，执行初始化
	c.onConnect(c.id)
}

func (c *WsConn) Read() {
	go func() {
		// 设置初始读超时
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		// 收到pong时续期
		c.conn.SetPongHandler(func(string) error {
			c.conn.SetReadDeadline(time.Now().Add(pongWait))
			slog.Info("receive pong message from ", c.Id())
			return nil
		})

		for {
			select {
			case <-c.closeCh:
				return
			default:
				_, message, err := c.conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						slog.Error("read message error", "error", err.Error())
					}
					c.Close()
					return
				}
				c.onMessage(message)
			}
		}
	}()
}

func (c *WsConn) Write() {
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-c.closeCh:
				return
			case message := <-c.writeCh:
				c.conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
					slog.Error("write message error", "error", err.Error())
					c.Close()
					return
				}
			case <-ticker.C:
				c.conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					slog.Error("write ping message error", "error", err.Error())
					c.Close()
					return
				}
			}
		}
	}()
}
