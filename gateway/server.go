package gateway

type ServerOpt struct {
	ListenPort string
}

type Server interface {
	Run() error
}
