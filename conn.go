package connpool

import (
	"net"
	"sync"
	"time"
)

// now returns the current time; it's overridden in tests.
var now = time.Now

type cacheKey struct {
	network string
	address string
}

// incomparable is a zero-width, non-comparable type. Adding it to a struct
// makes that struct also non-comparable, and generally doesn't add
// any size (as long as it's first).
type incomparable [0]func()

type Conn struct {
	dialer *Dialer

	rawConn   net.Conn
	cacheKey  cacheKey
	closech   chan struct{}
	createdAt time.Time

	// guarded by db.mu
	inUse      bool
	returnedAt time.Time

	// guarded by self
	errCaused bool
	mu        sync.Mutex
}

func (d *Dialer) newConn(rc net.Conn, key cacheKey) *Conn {
	return &Conn{
		dialer:    d,
		rawConn:   rc,
		cacheKey:  key,
		closech:   make(chan struct{}),
		createdAt: now(),
		inUse:     true,
	}
}

func (c *Conn) checkErr(err error) {
	if err != nil {
		c.errCaused = true
	}
}

func (c *Conn) isBroken() bool {
	return c.errCaused
}

func (c *Conn) expired(timeout time.Duration) bool {
	if timeout <= 0 {
		return false
	}
	return c.createdAt.Add(timeout).Before(now())
}

func (c *Conn) explicitClose() {
	c.rawConn.Close()
}

type (
	closeWriter interface {
		CloseWrite() error
	}
	closeReader interface {
		CloseRead() error
	}
)

var _ interface {
	net.Conn
	closeReader
	closeWriter
} = (*Conn)(nil)

func (c *Conn) CloseWrite() (err error) {
	defer c.checkErr(err)
	if v, ok := c.rawConn.(closeWriter); ok {
		return v.CloseWrite()
	}
	return nil
}

func (c *Conn) CloseRead() (err error) {
	defer c.checkErr(err)
	if v, ok := c.rawConn.(closeReader); ok {
		return v.CloseRead()
	}
	return nil
}

func (c *Conn) Read(b []byte) (_ int, err error) {
	defer c.checkErr(err)
	return c.rawConn.Read(b)
}

func (c *Conn) Write(b []byte) (_ int, err error) {
	defer c.checkErr(err)
	return c.rawConn.Write(b)
}

func (c *Conn) Close() (err error) {
	defer c.checkErr(err)

	c.dialer.mu.Lock()
	defer c.dialer.mu.Unlock()
	if !c.dialer.putCacheConnLocked(c) {
		return c.rawConn.Close()
	}

	return nil
}

func (c *Conn) LocalAddr() net.Addr {
	return c.rawConn.LocalAddr()
}

func (c *Conn) RemoteAddr() net.Addr {
	return c.rawConn.RemoteAddr()
}

func (c *Conn) SetDeadline(t time.Time) (err error) {
	defer c.checkErr(err)
	return c.rawConn.SetDeadline(t)
}

func (c *Conn) SetReadDeadline(t time.Time) (err error) {
	defer c.checkErr(err)
	return c.rawConn.SetReadDeadline(t)
}

func (c *Conn) SetWriteDeadline(t time.Time) (err error) {
	defer c.checkErr(err)
	return c.rawConn.SetWriteDeadline(t)
}
