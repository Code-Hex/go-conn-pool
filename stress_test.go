package connpool

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"

	fuzz "github.com/google/gofuzz"
	"golang.org/x/net/nettest"
)

type testingLogger interface {
	Logf(format string, args ...interface{})
}

func TestStressTCPProxy(t *testing.T) {
	t.Parallel()

	echoListener := newEchoServer(t, "tcp")
	t.Cleanup(func() { echoListener.Close() })

	dialer := New()
	t.Cleanup(func() { dialer.Close() })

	f := fuzz.New()
	for i := 0; i < 10000; i++ {
		var want string
		f.Fuzz(&want)
		t.Run(want, func(t *testing.T) {
			conn, err := dialer.Dial("tcp", echoListener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()

			stressEqual(t, conn, want)
		})
	}
}

func stressEqual(t *testing.T, conn net.Conn, want string) {
	conn.SetDeadline(time.Now().Add(3 * time.Second))

	if _, err := conn.Write([]byte(want)); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, len(want))
	if _, err := io.ReadAtLeast(conn, buf, len(want)); err != nil {
		t.Fatal(err)
	}

	if got := string(buf); want != got {
		t.Errorf("want %q, but got %q", want, got)
	}
}

func newEchoServer(t testingLogger, network string) net.Listener {
	echoListener, err := nettest.NewLocalListener(network)
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			conn, err := echoListener.Accept()
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if err != nil {
				panic(err)
			}
			go func() {
				defer conn.Close()
				_, err := io.Copy(conn, conn)
				if err != nil {
					t.Logf("server Echo error: %+v\n", err)
				}
			}()
		}
	}()

	return echoListener
}
