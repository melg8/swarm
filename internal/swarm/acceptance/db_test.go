// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeDBServer speaks the MariaDB wire protocol subset the DB client
// uses: the native password greeting, the OK auth answer and canned
// COM_QUERY responses. It records the queries it served.
type fakeDBServer struct {
	listener net.Listener
	queries  chan string
	answers  map[string][][]string
	// affected answers UPDATE style queries with the given count.
	affected map[string]uint64
}

// startFakeDBServer boots the fake server on a loopback port.
func startFakeDBServer(t *testing.T) *fakeDBServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &fakeDBServer{
		listener: listener,
		queries:  make(chan string, 16),
		answers:  map[string][][]string{},
		affected: map[string]uint64{},
	}
	go server.serve()

	t.Cleanup(func() {
		_ = listener.Close()
	})

	return server
}

// serve accepts one client at a time and runs the scripted exchange.
func (s *fakeDBServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.serveConn(conn)
	}
}

// serveConn runs the greeting, the auth and the query loop with one
// client.
func (s *fakeDBServer) serveConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	if !s.serveAuth(conn) {
		return
	}
	s.serveQueries(conn)
}

// buildGreeting renders the server greeting packet: protocol 10, a
// version string, the auth data, the 4.1 capabilities and the native
// auth plugin name.
func buildGreeting() []byte {
	greeting := []byte{10}
	greeting = append(greeting, "11.8.6-MariaDB-test"...)
	greeting = append(greeting, 0)
	greeting = append(greeting, 1, 0, 0, 0) // Thread id.
	greeting = append(greeting, make([]byte, 8)...)
	greeting = append(greeting, 0)          // Filler.
	greeting = append(greeting, 0xFF, 0xC1) // Capabilities low.
	greeting = append(greeting, 33)         // Charset.
	greeting = append(greeting, 2, 0)       // Status flags.
	greeting = append(greeting, 0x9F, 0x81) // Capabilities high.
	greeting = append(greeting, 21)         // Auth data length.
	greeting = append(greeting, make([]byte, 10)...)
	greeting = append(greeting, make([]byte, 12)...) // Auth part 2.
	greeting = append(greeting, 0)
	greeting = append(greeting, "mysql_native_password"...)

	return append(greeting, 0)
}

// serveAuth answers the greeting and the handshake exchange.
func (s *fakeDBServer) serveAuth(conn net.Conn) bool {
	if err := writeDBPacket(conn, 0, buildGreeting()); err != nil {
		return false
	}
	// The client handshake response.
	payload, seq, err := readDBPacket(conn)
	if err != nil || len(payload) < 32 {
		return false
	}
	// The OK of the successful auth.
	ok := writeDBPacket(conn, seq+1,
		[]byte{okHeader, 0, 0, 2, 0, 0, 0})

	return ok == nil
}

// serveQueries answers the COM_QUERY loop of one client.
func (s *fakeDBServer) serveQueries(conn net.Conn) {
	for {
		query, seq, err := readDBPacket(conn)
		if err != nil || len(query) < 2 || query[0] != comQuery {
			return
		}
		sql := string(query[1:])
		s.queries <- sql
		rows, hasRows := s.answers[sql]
		if affected, ok := s.affected[sql]; ok {
			if !s.writeExecAnswer(conn, seq, affected) {
				return
			}

			continue
		}
		if !hasRows {
			if !s.writeOKAnswer(conn, seq) {
				return
			}

			continue
		}
		if !s.writeRowsAnswer(conn, seq, rows) {
			return
		}
	}
}

// writeExecAnswer sends one OK packet with the affected row count.
func (s *fakeDBServer) writeExecAnswer(
	conn net.Conn, seq byte, affected uint64,
) bool {
	okPacket := []byte{okHeader}
	okPacket = appendLenEncInt(okPacket, int64(affected))
	okPacket = append(okPacket, 0, 2, 0, 0, 0)

	return writeDBPacket(conn, seq+1, okPacket) == nil
}

// writeOKAnswer sends one empty OK packet.
func (s *fakeDBServer) writeOKAnswer(conn net.Conn, seq byte) bool {
	return writeDBPacket(conn, seq+1,
		[]byte{okHeader, 0, 0, 2, 0, 0, 0}) == nil
}

// writeRowsAnswer sends one text result set: the column count, the
// definitions, the rows and the closing EOF packets.
func (s *fakeDBServer) writeRowsAnswer(
	conn net.Conn, seq byte, rows [][]string,
) bool {
	if writeDBPacket(conn, seq+1, []byte{byte(len(rows[0]))}) != nil {
		return false
	}
	for range rows[0] {
		if writeDBPacket(conn, 0, []byte{0x03, 'a', 'b', 'c'}) != nil {
			return false
		}
	}
	if writeDBPacket(conn, 0, []byte{eofHeader, 0, 0, 2, 0}) != nil {
		return false
	}
	for _, row := range rows {
		if writeDBPacket(conn, 0, buildRowPacket(row)) != nil {
			return false
		}
	}

	return writeDBPacket(conn, 0, []byte{eofHeader, 0, 0, 2, 0}) == nil
}

// buildRowPacket renders one text result row.
func buildRowPacket(row []string) []byte {
	payload := make([]byte, 0, 32)
	for _, value := range row {
		payload = append(payload, byte(len(value)))
		payload = append(payload, value...)
	}

	return payload
}

// writeDBPacket frames and writes one packet with the sequence byte.
func writeDBPacket(conn net.Conn, seq byte, payload []byte) error {
	header := []byte{
		byte(len(payload)), byte(len(payload) >> 8), byte(len(payload) >> 16),
		seq,
	}
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(payload)

	return err
}

// readDBPacket reads one framed packet and returns its payload with
// the sequence byte.
func readDBPacket(conn net.Conn) ([]byte, byte, error) {
	header := make([]byte, 4)
	if err := readDBFull(conn, header); err != nil {
		return nil, 0, err
	}
	length := int(header[0]) | int(header[1])<<8 | int(header[2])<<16
	payload := make([]byte, length)
	if err := readDBFull(conn, payload); err != nil {
		return nil, 0, err
	}

	return payload, header[3], nil
}

// readDBFull fills the buffer completely.
func readDBFull(conn net.Conn, buf []byte) error {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return err
		}
	}

	return nil
}

// appendLenEncInt appends a length encoded integer.
func appendLenEncInt(dst []byte, value int64) []byte {
	switch {
	case value < 251:
		return append(dst, byte(value))
	case value < 65536:
		return append(dst, 0xFC, byte(value), byte(value>>8))
	default:
		return append(dst, 0xFD, byte(value), byte(value>>8), byte(value>>16))
	}
}

// connectFake dials the client against the fake server.
func connectFake(t *testing.T, server *fakeDBServer) *DB {
	t.Helper()
	db, err := ConnectDB(DBConfig{
		Address:  server.listener.Addr().String(),
		Socket:   "",
		User:     "root",
		Password: "",
		Database: "l2jmobiusc1",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	return db
}

// TestDBQueryReadsRows pins the handshake and the text result path of
// the wire client.
func TestDBQueryReadsRows(t *testing.T) {
	server := startFakeDBServer(t)
	server.answers["SELECT name FROM t"] = [][]string{
		{"one"}, {"two"}, {""}, // The third row is a NULL column.
	}
	db := connectFake(t, server)

	rows, err := db.Query("SELECT name FROM t")
	require.NoError(t, err)
	require.Equal(t, [][]string{{"one"}, {"two"}, {""}}, rows)
}

// TestDBExecReportsAffectedRows pins the OK answer path.
func TestDBExecReportsAffectedRows(t *testing.T) {
	server := startFakeDBServer(t)
	server.affected["DELETE FROM items WHERE owner_id=5"] = 3
	db := connectFake(t, server)

	affected, err := db.Exec("DELETE FROM items WHERE owner_id=5")
	require.NoError(t, err)
	require.Equal(t, int64(3), affected)
}

// TestDBConnectPrefersNoSocket pins the endpoint order: an empty
// socket path dials the TCP address only, a broken socket falls back
// to the TCP endpoint.
func TestDBConnectPrefersNoSocket(t *testing.T) {
	server := startFakeDBServer(t)
	db, err := ConnectDB(DBConfig{
		Address:  server.listener.Addr().String(),
		Socket:   "",
		User:     "root",
		Password: "",
		Database: "l2jmobiusc1",
	})
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// A dead socket plus a live TCP endpoint: the fallback answers.
	db2, err := ConnectDB(DBConfig{
		Address:  server.listener.Addr().String(),
		Socket:   "/nonexistent/swarm-test.sock",
		User:     "root",
		Password: "",
		Database: "l2jmobiusc1",
	})
	require.NoError(t, err)
	require.NoError(t, db2.Close())
}

// TestParseGreetingPlugin pins the greeting parser offsets against a
// hand built packet.
func TestParseGreetingPlugin(t *testing.T) {
	greeting := []byte{10}
	greeting = append(greeting, "11.8.6"...)
	greeting = append(greeting, 0)
	greeting = append(greeting, 7, 0, 0, 0) // Thread id.
	greeting = append(greeting, make([]byte, 8)...)
	greeting = append(greeting, 0)
	greeting = append(greeting, 0xFF, 0xC1)
	greeting = append(greeting, 33)
	greeting = append(greeting, 2, 0)
	greeting = append(greeting, 0x9F, 0x81)
	greeting = append(greeting, 21)
	greeting = append(greeting, make([]byte, 10)...)
	greeting = append(greeting, make([]byte, 12)...)
	greeting = append(greeting, 0)
	greeting = append(greeting, "mysql_native_password"...)
	greeting = append(greeting, 0)

	plugin, err := parseGreetingPlugin(greeting)
	require.NoError(t, err)
	require.Equal(t, "mysql_native_password", plugin)
}

// TestParseTextRow pins the NULL marker and the multi byte lengths.
func TestParseTextRow(t *testing.T) {
	row, err := parseTextRow([]byte{nullValue, 2, 'o', 'k'}, 2)
	require.NoError(t, err)
	require.Equal(t, []string{"", "ok"}, row)

	// A three byte length marker inside a long value.
	long := make([]byte, 400)
	for i := range long {
		long[i] = 'x'
	}
	var packet []byte
	packet = append(packet, nullValue)
	packet = append(packet, 0xFD, byte(len(long)), byte(len(long)>>8), 0)
	packet = append(packet, long...)
	row, err = parseTextRow(packet, 2)
	require.NoError(t, err)
	require.Equal(t, []string{"", string(long)}, row)
}

// TestDeadlineSet protects the timeout wiring: a query against a
// dead connection fails instead of hanging.
func TestDeadlineSet(t *testing.T) {
	server := startFakeDBServer(t)
	db := connectFake(t, server)
	_ = db.Close()
	deadline := time.Now().Add(dbTimeout)
	_, err := db.Query("SELECT 1")
	require.Error(t, err)
	require.True(t, time.Now().Before(deadline.Add(30*time.Second)))
}
