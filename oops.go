package oops

import (
	"net"
	"os"
	"sync/atomic"
	"time"
)

// Dialer establishes a connection.
type Dialer func(network, addr string) (net.Conn, error)

// InjectDialer injects conditions into a listener.
func InjectDialer(f Dialer, conds ...Condition) Dialer {
	return func(network, addr string) (net.Conn, error) {
		for _, cond := range conds {
			f = cond.Dialer(f)
		}
		return f(network, addr)
	}
}

// InjectListener injects conditions into a listener.
func InjectListener(l net.Listener, conds ...Condition) net.Listener {
	for _, cond := range conds {
		l = cond.Listener(l)
	}
	return l
}

// Condition simulates an error scenario.
type Condition interface {
	Listener(net.Listener) net.Listener
	Dialer(Dialer) Dialer
}

// AcceptError causes an error on accept.  Ordering matters with this option as
// it short-circuits any options after this one.
func AcceptError(err error) Condition {
	return &decorator{
		listener: func(l *listener) *listener {
			l.accept = func() (net.Conn, error) {
				return nil, err
			}
			return l
		},
		dialer: func(d Dialer) Dialer {
			return func(network, addr string) (net.Conn, error) {
				return nil, err
			}
		},
	}
}

// AcceptLatency adds latency to listener accept requests until either timeout
// or underlying listener is closed. No duration hangs indefinitely until the
// underlying listener is closed.
func AcceptLatency(d ...time.Duration) Condition {
	var i uint64
	return &decorator{
		listener: func(l *listener) *listener {
			dl := newDeadline()
			doAccept := l.accept
			doClose := l.close
			l.accept = func() (net.Conn, error) {
				if len(d) > 0 {
					idx := int((atomic.AddUint64(&i, 1) - 1) % uint64(len(d)))
					dl.Wait(d[idx])
				} else {
					var blockForever chan struct{} = nil
					<-blockForever
				}
				return doAccept()
			}
			l.close = func() error {
				dl.Close()
				return doClose()
			}
			return l
		},
		dialer: func(dial Dialer) Dialer {
			return func(network, addr string) (net.Conn, error) {
				if len(d) > 0 {
					idx := int((atomic.AddUint64(&i, 1) - 1) % uint64(len(d)))
					timer := time.NewTimer(d[idx])
					defer timer.Stop()
					<-timer.C
				} else {
					var blockForever chan struct{} = nil
					<-blockForever
				}
				return dial(network, addr)
			}
		},
	}
}

// ReadLatency adds latency to each read from underlying connections. Reads
// will delay for timeout or until the underlying connection is closed. No
// duration hangs indefinitely until the underlying connection is closed or
// read deadline is exceeded.
func ReadLatency(d ...time.Duration) Condition {
	return newConn(func(cn *conn) *conn {
		doRead := cn.read
		doSetReadDeadline := cn.setReadDeadline
		doSetDeadline := cn.setDeadline
		doClose := cn.close
		dl := newDeadline()
		var i uint64
		cn.read = func(b []byte) (int, error) {
			var latency time.Duration
			if len(d) != 0 {
				idx := int((atomic.AddUint64(&i, 1) - 1) % uint64(len(d)))
				latency = d[idx]
			}
			err := dl.Wait(latency)
			if err != nil {
				return 0, err
			}
			return doRead(b)
		}
		cn.setReadDeadline = func(t time.Time) error {
			dl.Reset(t)
			return doSetReadDeadline(t)
		}
		cn.setDeadline = func(t time.Time) error {
			dl.Reset(t)
			return doSetDeadline(t)
		}
		cn.close = func() error {
			dl.Close()
			return doClose()
		}
		return cn
	})
}

// WriteLatency adds latency to each write from underlying connections. Writes
// will delay for timeout or until the underlying connection is closed. No
// duration hangs indefinitely until the underlying connection is closed or
// write deadline is exceeded.
func WriteLatency(d ...time.Duration) Condition {
	return newConn(func(cn *conn) *conn {
		doWrite := cn.write
		doSetWriteDeadline := cn.setWriteDeadline
		doSetDeadline := cn.setDeadline
		doClose := cn.close
		dl := newDeadline()
		var i uint64
		cn.write = func(b []byte) (int, error) {
			var latency time.Duration
			if len(d) != 0 {
				idx := int((atomic.AddUint64(&i, 1) - 1) % uint64(len(d)))
				latency = d[idx]
			}
			err := dl.Wait(latency)
			if err != nil {
				return 0, err
			}
			return doWrite(b)
		}
		cn.setWriteDeadline = func(t time.Time) error {
			dl.Reset(t)
			return doSetWriteDeadline(t)
		}
		cn.setDeadline = func(t time.Time) error {
			dl.Reset(t)
			return doSetDeadline(t)
		}
		cn.close = func() error {
			dl.Close()
			return doClose()
		}
		return cn
	})
}

// ReadError causes the listener's connections to return an error on read.
// Ordering matters with this option as it short-circuits any options after
// this one.
func ReadError(err error) Condition {
	return newConn(func(cn *conn) *conn {
		cn.read = func(_ []byte) (int, error) {
			return 0, err
		}
		return cn
	})
}

type decorator struct {
	listener func(*listener) *listener
	dialer   func(Dialer) Dialer
}

func (d *decorator) Listener(l net.Listener) net.Listener {
	result := &listener{
		accept: l.Accept,
		close:  l.Close,
		addr:   l.Addr,
	}
	return d.listener(result)
}

func (d *decorator) Dialer(dial Dialer) Dialer {
	return d.dialer(dial)
}

type listener struct {
	accept func() (net.Conn, error)
	addr   func() net.Addr
	close  func() error
}

func (l *listener) Accept() (net.Conn, error) {
	return l.accept()
}

func (l *listener) Addr() net.Addr {
	return l.addr()
}

func (l *listener) Close() error {
	return l.close()
}

type conn struct {
	read             func([]byte) (int, error)
	write            func([]byte) (int, error)
	close            func() error
	localAddr        func() net.Addr
	remoteAddr       func() net.Addr
	setDeadline      func(time.Time) error
	setReadDeadline  func(time.Time) error
	setWriteDeadline func(time.Time) error
}

func (c *conn) Read(b []byte) (int, error) {
	return c.read(b)
}

func (c *conn) Write(b []byte) (int, error) {
	return c.write(b)
}

func (c *conn) Close() error {
	return c.close()
}

func (c *conn) LocalAddr() net.Addr {
	return c.localAddr()
}

func (c *conn) RemoteAddr() net.Addr {
	return c.remoteAddr()
}

func (c *conn) SetDeadline(t time.Time) error {
	return c.setDeadline(t)
}

func (c *conn) SetReadDeadline(t time.Time) error {
	return c.setReadDeadline(t)
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	return c.setWriteDeadline(t)
}

func newConn(f func(*conn) *conn) Condition {
	return &decorator{
		listener: func(l *listener) *listener {
			doAccept := l.accept
			l.accept = func() (net.Conn, error) {
				c, err := doAccept()
				if err != nil {
					return c, err
				}
				return f(&conn{
					read:             c.Read,
					write:            c.Write,
					close:            c.Close,
					localAddr:        c.LocalAddr,
					remoteAddr:       c.RemoteAddr,
					setDeadline:      c.SetDeadline,
					setReadDeadline:  c.SetReadDeadline,
					setWriteDeadline: c.SetWriteDeadline,
				}), nil
			}
			return l
		},
		dialer: func(d Dialer) Dialer {
			return func(network, addr string) (net.Conn, error) {
				c, err := d(network, addr)
				if err != nil {
					return c, err
				}
				return f(&conn{
					read:             c.Read,
					write:            c.Write,
					close:            c.Close,
					localAddr:        c.LocalAddr,
					remoteAddr:       c.RemoteAddr,
					setDeadline:      c.SetDeadline,
					setReadDeadline:  c.SetReadDeadline,
					setWriteDeadline: c.SetWriteDeadline,
				}), nil
			}
		},
	}
}

type deadline struct {
	errors chan error
	reset  chan time.Time
	done   chan struct{}
}

func newDeadline() *deadline {
	timer := time.NewTimer(1<<63 - 1)
	var timedOut bool
	errors := make(chan error)
	reset := make(chan time.Time)
	done := make(chan struct{})

	go func() {
		stopTimer := func() {
			if !timer.Stop() && !timedOut {
				<-timer.C
			}
		}
		defer stopTimer()
		defer close(errors)
		defer close(reset)
		var errGate chan error = nil

		for {
			select {
			case next := <-reset:
				stopTimer()
				errGate = nil
				to := next.Sub(time.Now())
				timer.Reset(to)
				timedOut = false
			case <-timer.C:
				timedOut = true
				errGate = errors
			case errGate <- os.ErrDeadlineExceeded:
			case <-done:
				return
			}
		}
	}()
	return &deadline{
		errors: errors,
		reset:  reset,
		done:   done,
	}
}

func (d *deadline) Wait(timeout time.Duration) error {
	var c <-chan time.Time = nil
	var timedOut bool
	if timeout != 0 {
		timer := time.NewTimer(timeout)
		c = timer.C
		defer func() {
			if !timer.Stop() && !timedOut {
				<-timer.C
			}
		}()
	}

	select {
	case err := <-d.errors:
		return err
	case <-c:
		timedOut = true
	case <-d.done:
	}
	return nil
}

func (d *deadline) Reset(t time.Time) {
	select {
	case d.reset <- t:
	case <-d.done:
	}
}

func (d *deadline) Close() {
	close(d.done)
}
