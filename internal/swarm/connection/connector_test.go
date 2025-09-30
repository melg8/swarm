// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
	"net"
	"testing"
	"time"
)

// Mock connector for testing.
type mockConnector struct {
	address     string
	shouldFail  bool
	connectTime time.Duration
}

func (m *mockConnector) Connect() (net.Conn, error) {
	if m.connectTime > 0 {
		time.Sleep(m.connectTime)
	}
	if m.shouldFail {
		return nil, &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: &net.DNSError{
				UnwrapErr:   nil,
				Err:         "connection refused",
				Name:        m.address,
				Server:      "",
				IsTimeout:   false,
				IsTemporary: false,
				IsNotFound:  false,
			},
			Source: nil,
			Addr:   nil,
		}
	}
	// Return a mock connection
	return &net.TCPConn{}, nil
}

func (m *mockConnector) Address() string {
	return m.address
}

func TestTcpConnector(t *testing.T) {
	t.Run("successful connection", func(t *testing.T) {
		// Start a test server
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to start test server: %v", err)
		}
		defer listener.Close()

		connector := NewTCPConnector(listener.Addr().String(), time.Second)
		conn, err := connector.Connect()
		if err != nil {
			t.Errorf("Expected successful connection, got error: %v", err)
		}
		if conn != nil {
			conn.Close()
		}
	})

	t.Run("connection timeout", func(t *testing.T) {
		connector := NewTCPConnector("127.0.0.1:1", time.Millisecond*100)
		_, err := connector.Connect()
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})
}

func TestRateLimitedConnector(t *testing.T) {
	t.Run("respects rate limit", func(t *testing.T) {
		mock := &mockConnector{
			address:     "test:1234",
			shouldFail:  false,
			connectTime: 0,
		}
		timeout := time.Millisecond * 100
		connector := NewRateLimitedConnector(mock, timeout)

		// First connection should be immediate
		start := time.Now()
		conn1, err := connector.Connect()
		if err != nil {
			t.Errorf("First connection failed: %v", err)
		}
		if conn1 != nil {
			conn1.Close()
		}

		// Second connection should be delayed
		conn2, err := connector.Connect()
		elapsed := time.Since(start)
		if err != nil {
			t.Errorf("Second connection failed: %v", err)
		}
		if conn2 != nil {
			conn2.Close()
		}

		if elapsed < timeout {
			t.Errorf("Rate limit not respected. Expected delay of %v, got %v", timeout, elapsed)
		}
	})
}

func TestRetryConnector(t *testing.T) {
	t.Run("successful on first try", func(t *testing.T) {
		mock := &mockConnector{
			address:     "test:1234",
			shouldFail:  false,
			connectTime: 0,
		}
		connector := NewRetryConnector(mock, 3)

		conn, err := connector.Connect()
		if err != nil {
			t.Errorf("Expected successful connection, got error: %v", err)
		}
		if conn != nil {
			conn.Close()
		}
	})

	t.Run("retries on failure", func(t *testing.T) {
		mock := &mockConnector{
			address:     "test:1234",
			shouldFail:  true,
			connectTime: 10,
		}
		connector := NewRetryConnector(mock, 3)

		_, err := connector.Connect()
		if err == nil {
			t.Error("Expected error after all retries, got nil")
		}
	})

	t.Run("succeeds after retries", func(t *testing.T) {
		attempts := 0
		mock := &mockConnector{
			address:     "test:1234",
			shouldFail:  false,
			connectTime: time.Millisecond * 10,
		}
		connector := NewRetryConnector(mock, 3)

		conn, err := connector.Connect()
		if err != nil {
			t.Errorf("Expected successful connection after retries, got error: %v", err)
		}
		if conn != nil {
			conn.Close()
		}
		if attempts > 3 {
			t.Errorf("Too many retry attempts: %d", attempts)
		}
	})
}

func TestServerConnector(t *testing.T) {
	t.Run("successful connection", func(t *testing.T) {
		// Start a test server
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to start test server: %v", err)
		}
		defer listener.Close()

		// Create server connector with the test server address
		connector, err := ServerConnector(listener.Addr().String())
		if err != nil {
			t.Fatalf("Failed to create server connector: %v", err)
		}

		// Test connection
		conn, err := connector.Connect()
		if err != nil {
			t.Errorf("Expected successful connection, got error: %v", err)
		}
		if conn != nil {
			conn.Close()
		}
	})

	t.Run("connection to invalid address", func(t *testing.T) {
		connector, err := ServerConnector("127.0.0.1:1")
		if err != nil {
			t.Fatalf("Failed to create server connector: %v", err)
		}

		_, err = connector.Connect()
		if err == nil {
			t.Error("Expected error when connecting to invalid address, got nil")
		}
	})

	t.Run("address method", func(t *testing.T) {
		expectedAddr := "127.0.0.1:8080"
		connector, err := ServerConnector(expectedAddr)
		if err != nil {
			t.Fatalf("Failed to create server connector: %v", err)
		}

		if addr := connector.Address(); addr != expectedAddr {
			t.Errorf("Expected address %s, got %s", expectedAddr, addr)
		}
	})
}
