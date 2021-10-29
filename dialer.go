package connpool

import (
	"context"
	"net"
	"sync"
	"time"
)

type Dialer struct {
	// If n <= 0, no idle connections are retained.
	// The default max idle connections is currently 2. This may change in a future release.
	maxIdleConns int

	maxLifetime time.Duration

	baseDialer *net.Dialer

	cache  map[cacheKey][]*Conn
	closed bool

	mu sync.Mutex
}

func New() *Dialer {
	d := &Dialer{
		maxIdleConns: 0,
		maxLifetime:  time.Second,
		cache:        make(map[cacheKey][]*Conn),
		closed:       false,
	}
	return d
}

func (d *Dialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

var (
	noDeadline   = time.Time{}
	aLongTimeAgo = time.Unix(1, 0)
)

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	key := cacheKey{
		network: network,
		address: address,
	}

	d.mu.Lock()
	cachedConn := d.getCacheConnLocked(key)
	if cachedConn != nil {
		d.mu.Unlock()
		return cachedConn, nil
	}
	d.mu.Unlock()

	rc, err := d.dialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}

	return d.newConn(rc, key), nil
}

func (d *Dialer) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if d.baseDialer != nil {
		return d.baseDialer.DialContext(ctx, network, address)
	}
	var netDialer net.Dialer
	return netDialer.DialContext(ctx, network, address)
}

func (d *Dialer) Close() error {
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()

	for _, conns := range d.cache {
		for _, conn := range conns {
			conn.Close()
		}
	}
	return nil
}

func (d *Dialer) putCacheConnLocked(conn *Conn) bool {
	if d.closed {
		return false
	}
	if conn.isBroken() {
		return false
	}
	conn.inUse = false
	d.cache[conn.cacheKey] = append(d.cache[conn.cacheKey], conn)
	return true
}

func (d *Dialer) getCacheConnLocked(key cacheKey) *Conn {
	conns, ok := d.cache[key]
	if !ok {
		return nil
	}

	for i, conn := range conns {
		if conn.expired(d.maxLifetime) || conn.isBroken() {
			conn.rawConn.Close()
			continue
		}

		copy(d.cache[key], conns[i:])
		conn.inUse = true
		return conn
	}

	return nil
}
