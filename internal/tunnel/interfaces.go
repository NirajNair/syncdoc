package tunnel

type Tunnel interface {
	URL() string
	Close() error
}
