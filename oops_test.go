package oops

import (
	"bytes"
	"errors"
	"io"
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
				output := make([]byte, len(testCase.input))
				start := time.Now()
				_, err := conn.Write(input)
				if err != nil {
					t.Error(err)
				}
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
				if testCase.expectedError == nil && string(input) != string(output) {
					t.Errorf("expected input %#v to equal output %#v", string(input), string(output))
				}
			}
		})
	}
}

func TestListenerWrite(t *testing.T) {
	for _, testCase := range []struct {
		Name          string
		Conditions    []Condition
		MinWait       time.Duration
		MaxWait       time.Duration
		WriteDeadline time.Duration
		Deadline      time.Duration
		NumRuns       int
		expectedError error
		input         []byte
	}{
		{
			Name:       "latency is injected",
			Conditions: []Condition{WriteLatency(100 * time.Millisecond)},
			MinWait:    100 * time.Millisecond,
			MaxWait:    200 * time.Millisecond,
			input:      []byte(`this is a test`),
		},
		{
			Name:          "write deadline is respected with unlimited latency",
			Conditions:    []Condition{WriteLatency()},
			MinWait:       100 * time.Millisecond,
			MaxWait:       200 * time.Millisecond,
			WriteDeadline: 100 * time.Millisecond,
			expectedError: os.ErrDeadlineExceeded,
			input:         []byte(`this is a test`),
		},
		{
			Name:          "write deadline is respected with high latency",
			Conditions:    []Condition{WriteLatency(1 * time.Second)},
			MinWait:       100 * time.Millisecond,
			MaxWait:       200 * time.Millisecond,
			WriteDeadline: 100 * time.Millisecond,
			expectedError: os.ErrDeadlineExceeded,
			input:         []byte(`this is a test`),
		},
		{
			Name:          "deadline is respected with unlimited latency",
			Conditions:    []Condition{WriteLatency()},
			MinWait:       100 * time.Millisecond,
			MaxWait:       200 * time.Millisecond,
			Deadline:      100 * time.Millisecond,
			expectedError: os.ErrDeadlineExceeded,
			input:         []byte(`this is a test`),
		},
		{
			Name:          "deadline is respected with high latency",
			Conditions:    []Condition{WriteLatency(1 * time.Second)},
			MinWait:       100 * time.Millisecond,
			MaxWait:       200 * time.Millisecond,
			Deadline:      100 * time.Millisecond,
			expectedError: os.ErrDeadlineExceeded,
			input:         []byte(`this is a test`),
		},
		{
			Name:       "multiple calls",
			Conditions: []Condition{WriteLatency(100 * time.Millisecond)},
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
				output := make([]byte, len(testCase.input))
				start := time.Now()
				if testCase.WriteDeadline != 0 {
					conn.SetWriteDeadline(time.Now().Add(testCase.WriteDeadline))
				}
				if testCase.Deadline != 0 {
					conn.SetDeadline(time.Now().Add(testCase.Deadline))
				}

				select {
				case <-func() chan struct{} {
					done := make(chan struct{})
					go func() {
						defer close(done)
						_, err = conn.Write(input)
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
				_, err = conn.Read(output)
				if err != nil && !errors.Is(err, io.EOF) {
					t.Error(err)
				}
				duration := time.Now().Sub(start)
				if duration < testCase.MinWait {
					t.Errorf("expected latency of at least %s but write returned after %s", testCase.MinWait, duration)
				}
				if duration > testCase.MaxWait {
					t.Errorf("expected write to return in less than %s but it took %s", testCase.MaxWait, duration)
				}
				if testCase.expectedError == nil && string(input) != string(output) {
					t.Errorf("expected input %#v to equal output %#v", string(input), string(output))
				}
			}
		})
	}
}

func TestDialerRead(t *testing.T) {
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
		dialer := func(string, string) (net.Conn, error) { return &mockConn{}, nil }
		t.Run(testCase.Name, func(t *testing.T) {
			conn, err := InjectDialer(dialer, testCase.Conditions...)("", "")
			if err != nil {
				t.Error(err)
			}
			defer conn.Close()
			for i := 0; i < testCase.NumRuns+1; i++ {
				input := testCase.input
				output := make([]byte, len(testCase.input))
				start := time.Now()
				_, err := conn.Write(input)
				if err != nil {
					t.Error(err)
				}
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
				if testCase.expectedError == nil && string(input) != string(output) {
					t.Errorf("expected input %#v to equal output %#v", string(input), string(output))
				}
			}
		})
	}
}

func TestDialerWrite(t *testing.T) {
	for _, testCase := range []struct {
		Name          string
		Conditions    []Condition
		MinWait       time.Duration
		MaxWait       time.Duration
		WriteDeadline time.Duration
		Deadline      time.Duration
		NumRuns       int
		expectedError error
		input         []byte
	}{
		{
			Name:       "latency is injected",
			Conditions: []Condition{WriteLatency(100 * time.Millisecond)},
			MinWait:    100 * time.Millisecond,
			MaxWait:    200 * time.Millisecond,
			input:      []byte(`this is a test`),
		},
		{
			Name:          "write deadline is respected with unlimited latency",
			Conditions:    []Condition{WriteLatency()},
			MinWait:       100 * time.Millisecond,
			MaxWait:       200 * time.Millisecond,
			WriteDeadline: 100 * time.Millisecond,
			expectedError: os.ErrDeadlineExceeded,
			input:         []byte(`this is a test`),
		},
		{
			Name:          "write deadline is respected with high latency",
			Conditions:    []Condition{WriteLatency(1 * time.Second)},
			MinWait:       100 * time.Millisecond,
			MaxWait:       200 * time.Millisecond,
			WriteDeadline: 100 * time.Millisecond,
			expectedError: os.ErrDeadlineExceeded,
			input:         []byte(`this is a test`),
		},
		{
			Name:          "deadline is respected with unlimited latency",
			Conditions:    []Condition{WriteLatency()},
			MinWait:       100 * time.Millisecond,
			MaxWait:       200 * time.Millisecond,
			Deadline:      100 * time.Millisecond,
			expectedError: os.ErrDeadlineExceeded,
			input:         []byte(`this is a test`),
		},
		{
			Name:          "deadline is respected with high latency",
			Conditions:    []Condition{WriteLatency(1 * time.Second)},
			MinWait:       100 * time.Millisecond,
			MaxWait:       200 * time.Millisecond,
			Deadline:      100 * time.Millisecond,
			expectedError: os.ErrDeadlineExceeded,
			input:         []byte(`this is a test`),
		},
		{
			Name:       "multiple calls",
			Conditions: []Condition{WriteLatency(100 * time.Millisecond)},
			MinWait:    100 * time.Millisecond,
			MaxWait:    200 * time.Millisecond,
			NumRuns:    2,
			input:      []byte(`this is a test`),
		},
	} {
		dialer := func(string, string) (net.Conn, error) { return &mockConn{}, nil }
		t.Run(testCase.Name, func(t *testing.T) {
			conn, err := InjectDialer(dialer, testCase.Conditions...)("", "")
			if err != nil {
				t.Error(err)
			}
			defer conn.Close()
			for i := 0; i < testCase.NumRuns+1; i++ {
				input := testCase.input
				output := make([]byte, len(testCase.input))
				start := time.Now()
				if testCase.WriteDeadline != 0 {
					conn.SetWriteDeadline(time.Now().Add(testCase.WriteDeadline))
				}
				if testCase.Deadline != 0 {
					conn.SetDeadline(time.Now().Add(testCase.Deadline))
				}

				select {
				case <-func() chan struct{} {
					done := make(chan struct{})
					go func() {
						defer close(done)
						_, err = conn.Write(input)
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
				_, err = conn.Read(output)
				if err != nil && !errors.Is(err, io.EOF) {
					t.Error(err)
				}
				duration := time.Now().Sub(start)
				if duration < testCase.MinWait {
					t.Errorf("expected latency of at least %s but write returned after %s", testCase.MinWait, duration)
				}
				if duration > testCase.MaxWait {
					t.Errorf("expected write to return in less than %s but it took %s", testCase.MaxWait, duration)
				}
				if testCase.expectedError == nil && string(input) != string(output) {
					t.Errorf("expected input %#v to equal output %#v", string(input), string(output))
				}
			}
		})
	}
}

func TestAcceptLatency(t *testing.T) {
	var ml mockListener
	l := InjectListener(&ml, AcceptLatency(100*time.Millisecond))
	start := time.Now()
	conn, err := l.Accept()
	if err != nil {
		t.Errorf("expected nil error but got %s", err)
	}
	defer conn.Close()
	duration := time.Now().Sub(start)
	if duration < 100*time.Millisecond {
		t.Errorf("expected latency of at least %s but accept returned after %s", 100*time.Millisecond, duration)
	}
	if duration > 200*time.Millisecond {
		t.Errorf("expected accept to return in less than %s but it took %s", 200*time.Millisecond, duration)
	}
}
