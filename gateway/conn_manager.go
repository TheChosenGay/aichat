package gateway

import (
	"encoding/json"
	"errors"
	"log/slog"
	"sync"

	"github.com/TheChosenGay/aichat/types"
	"golang.org/x/sync/errgroup"
)

type ConnManager struct {
	mx    sync.Mutex
	conns map[string]Conn
}

func NewConnManager() *ConnManager {
	return &ConnManager{
		conns: make(map[string]Conn),
	}
}

func (c *ConnManager) AddConn(conn Conn) error {
	c.mx.Lock()
	defer c.mx.Unlock()
	c.conns[conn.Id()] = conn
	return nil
}

// remove 就是remove，不要做不必要的事情
func (c *ConnManager) RemoveConn(id string) error {
	c.mx.Lock()
	defer c.mx.Unlock()
	delete(c.conns, id)
	return nil
}

func (c *ConnManager) GetConn(id string) (Conn, error) {
	c.mx.Lock()
	defer c.mx.Unlock()
	conn, ok := c.conns[id]
	if !ok {
		return nil, errors.New("conn not found")
	}
	return conn, nil
}

// MARK: - Message Router
func (c *ConnManager) Route(message *types.Message) error {
	conn, err := c.GetConn(message.ToId)
	if err != nil {
		// 用户不在线，目前先静默处理
		slog.Info("user not online", "toId", message.ToId)
		return nil
	}

	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return conn.Push(data)
}

func (c *ConnManager) RouteGroup(message *types.Message, memberIds []string) error {

	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	g := errgroup.Group{}
	g.SetLimit(40)
	for _, memberId := range memberIds {
		g.Go(func() error {
			if memberId == message.FromId {
				return nil
			}

			conn, err := c.GetConn(memberId)
			if err != nil {
				// 用户不在线，目前静默处理
				return nil
			}
			// 如果失败了，目前静默处理
			conn.Push(data)
			return nil
		})
	}
	return g.Wait()
}

// MARK: - Session Cleaner
func (c *ConnManager) Clean(userId string) error {
	conn, err := c.GetConn(userId)
	if err != nil {
		return err
	}
	return conn.Close() // 注意锁的粒度问题
}
