package oops

import (
	"bytes"
	"errors"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

type mockListener struct {
}

func (m *mockListener) Accept() (net.Conn, error) {
	return &mockConn{}, nil
}

func (m *mockListener) Addr() net.Addr {
	return nil
}

func (m *mockListener) Close() error {
	return nil
}

type mockConn struct {
	Buffer bytes.Buffer
	mu     sync.Mutex
}

func (m *mockConn) Read(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Buffer.Read(b)
}

func (m *mockConn) Write(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Buffer.Write(b)
}

func (m *mockConn) Close() error {
	return nil
}

func (m *mockConn) LocalAddr() net.Addr {
	return nil
}

func (m *mockConn) RemoteAddr() net.Addr {
	return nil
}

func (m *mockConn) SetDeadline(time.Time) error {
	return nil
}

func (m *mockConn) SetReadDeadline(time.Time) error {
	return nil
}

func (m *mockConn) SetWriteDeadline(time.Time) error {
	return nil
}

func TestListenerRead(t *testing.T) {
	for _, testCase := range []struct {
		Name          string
		Conditions    []Condition
		MinWait       time.Duration
		MaxWait       time.Duration
		ReadDeadline  time.Duration
		Deadline      time.Duration
		NumRuns       int
		expectedError error
		input         []byte
	}{
		{
			Name:       "latency is injected",
			Conditions: []Condition{ReadLatency(100 * time.Millisecond)},
			MinWait:    100 * time.Millisecond,
			MaxWait:    200 * time.Millisecond,
			input:      []byte(`this is a test`),
		},
		{
			Name:          "read deadline is respected with unlimited latency",
			Conditions:    []Condition{ReadLatency()},
			MinWait:       100 * time.Millisecond,
			MaxWait:       200 * time.Millisecond,
			ReadDeadline:  100 * time.Millisecond,
			expectedError: os.ErrDeadlineExceeded,
			input:         []byte(`this is a test`),
		},
		{
			Name:          "read deadline is respected with high latency",
			Conditions:    []Condition{ReadLatency(1 * time.Second)},
			MinWait:       100 * time.Millisecond,
			MaxWait:       200 * time.Millisecond,
			ReadDeadline:  100 * time.Millisecond,
			expectedError: os.ErrDeadlineExceeded,
			input:         []byte(`this is a test`),
		},
		{
			Name:          "deadline is respected with unlimited latency",
			Conditions:    []Condition{ReadLatency()},
			MinWait:       100 * time.Millisecond,
			MaxWait:       200 * time.Millisecond,
			Deadline:      100 * time.Millisecond,
			expectedError: os.ErrDeadlineExceeded,
			input:         []byte(`this is a test`),
		},
		{
			Name:          "deadline is respected with high latency",
			Conditions:    []Condition{ReadLatency(1 * time.Second)},
			MinWait:       100 * time.Millisecond,
			MaxWait:       200 * time.Millisecond,
			Deadline:      100 * time.Millisecond,
			expectedError: os.ErrDeadlineExceeded,
			input:         []byte(`this is a test`),
		},
		{
			Name:       "multiple calls",
			Conditions: []Condition{ReadLatency(100 * time.Millisecond)},
			MinWait:    100 * time.Millisecond,
			MaxWait:    200 * time.Millisecond,
			NumRuns:    2,
			input:      []byte(`this is a test`),
		},
	} {
		var ml mockListener
		t.Run(testCase.Name, func(t *testing.T) {
			conn, err := InjectListener(&ml, testCase.Conditions...).Accept()
			if err != nil {
				t.Error(err)
			}
			defer conn.Close()
			for i := 0; i < testCase.NumRuns+1; i++ {
				input := testCase.input
				go func() {
					_, err := conn.Write(input)
					if err != nil {
						t.Error(err)
					}
				}()
				output := make([]byte, len(testCase.input))
				start := time.Now()
				if testCase.ReadDeadline != 0 {
					conn.SetReadDeadline(time.Now().Add(testCase.ReadDeadline))
				}
				if testCase.Deadline != 0 {
					conn.SetDeadline(time.Now().Add(testCase.Deadline))
				}

				select {
				case <-func() chan struct{} {
					done := make(chan struct{})
					go func() {
						defer close(done)
						_, err = conn.Read(output)
					}()
					return done
				}():
				case <-time.After(time.Second):
				}
				if (err == nil) != (testCase.expectedError == nil) {
					if err == nil {
						t.Errorf("expected err to be %v but got %v", testCase.expectedError, err)
					}
				}
				if err != nil && !errors.Is(err, testCase.expectedError) {
					t.Errorf("expected err of type %T but got %T", testCase.expectedError, err)
				}
				duration := time.Now().Sub(start)
				if duration < testCase.MinWait {
					t.Errorf("expected latency of at least %s but read returned after %s", testCase.MinWait, duration)
				}
				if duration > testCase.MaxWait {
					t.Errorf("expected read to return in less than %s but it took %s", testCase.MaxWait, duration)
				}
			}
		})
	}
}
