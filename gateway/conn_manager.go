package gateway

import (
	"errors"
	"sync"
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
