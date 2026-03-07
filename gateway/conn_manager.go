package gateway

import (
	"encoding/json"
	"errors"
	"sync"

	"github.com/TheChosenGay/aichat/types"
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
		return nil
	}

	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return conn.Push(data)
}
