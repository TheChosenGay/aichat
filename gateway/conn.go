package gateway

type ConnCloseCallback func(id string)
type ConnMessageCallback func(data []byte)
type ConnConnectCallback func(id string) // user id

// Conn define the behavior of a long-lived connection
type Conn interface {
	Id() string
	Push([]byte) error
	Close() error
}
