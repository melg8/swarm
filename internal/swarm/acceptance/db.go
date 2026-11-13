// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package acceptance runs the user verifiable scenarios of the swarm
// from the live web interface: every test provisions its own temp
// character (the accounts temp1/temp2/temp3, the passwords equal the
// account names), drives it through the real game protocol and
// reports the checked conditions until the scenario passes or fails.
package acceptance

import (
    "errors"
    "fmt"
    "io"
    "net"
    "os"
    "strings"
    "time"
)

// Default database endpoints of the deployed Mobius stack: the game server
// talks to MariaDB as the passwordless root user, so the same channel serves
// the test character injection; the unix socket of the sandbox layout is tried
// first, it never depends on name resolution.
const (
    defaultDBAddress = "127.0.0.1:3306"
    defaultDBUser    = "root"
    defaultDBName    = "l2jmobiusc1"
    defaultDBSocket  = "~/mysql_tmp/mysql.sock"
)

// dbTimeout bounds the connect, the read and the write waits; the queries of
// the character reset are point lookups, a slow answer means the database is
// down.
const dbTimeout = 10 * time.Second

// Client capability flags of the handshake response (the MariaDB wire
// protocol): the 4.1 protocol framing plus the named auth plugin, so the empty
// password of the passwordless root account authenticates.
const (
    clientProtocol41    = 0x00000200
    clientPluginAuth    = 0x00080000
    clientConnectWithDB = 0x00000008
)

// maxPacketSize is the client limit announced in the handshake.
const maxPacketSize = 1<<24 - 1

// Packet header bytes, the COM_QUERY command byte and the auth plugin name.
const (
    okHeader         = 0x00
    errHeader        = 0xFF
    eofHeader        = 0xFE
    comQuery         = 0x03
    authPluginNative = "mysql_native_password"
)

// nullValue marks a NULL column of a text result row.
const nullValue = 0xFB

// DB is a one connection MariaDB wire client speaking exactly the subset the
// character reset needs - the 4.1 handshake with the mysql_native_password
// plugin, then COM_QUERY text results - so the runner injects the test
// characters without a driver dependency.
type DB struct {
    conn net.Conn
    // sequence is the packet sequence byte: it restarts per command
    // and increments per packet of one exchange.
    sequence byte
    // queryLog observes every executed statement (the run log mirrors them);
    // nil is free.
    queryLog func(statement string)
}

// SetQueryLog installs (or with nil removes) the statement observer. It is
// called between the injections, never while a statement is in flight, so the
// plain field read needs no lock.
func (db *DB) SetQueryLog(log func(statement string)) {
    db.queryLog = log
}

func (db *DB) note(sql string) {
    if db.queryLog != nil {
        db.queryLog(sql)
    }
}

type DBConfig struct {
    Address  string
    Socket   string
    User     string
    Password string
    Database string
}

// DefaultDBConfig returns the endpoint of the deployed stack: TCP
// 127.0.0.1:3306, root.
func DefaultDBConfig() DBConfig {
    return DBConfig{
        Address:  defaultDBAddress,
        Socket:   expandHome(defaultDBSocket),
        User:     defaultDBUser,
        Password: "",
        Database: defaultDBName,
    }
}

// dbDialer is the shared connection dialer of the wire client.
//
//nolint:exhaustruct_v5 // the zero defaults are intended
var dbDialer = &net.Dialer{Timeout: dbTimeout}

// expandHome resolves a leading ~ of a path against the home directory, so the
// sandbox socket path reads like the tools scripts that define it.
func expandHome(path string) string {
    if !strings.HasPrefix(path, "~") {
        return path
    }
    home, err := os.UserHomeDir()
    if err != nil || home == "" {
        return path[1:]
    }

    return home + path[1:]
}

// ConnectDB opens the wire client against the configured endpoint: the unix
// socket first (it never depends on the name resolution), the TCP fallback
// second. Both end in the same handshake.
func ConnectDB(config DBConfig) (*DB, error) {
    var lastErr error
    for _, dial := range dbDialFuncs(config) {
        conn, err := dial()
        if err != nil {
            lastErr = err

            continue
        }
        db := &DB{conn: conn, sequence: 0, queryLog: nil}
        if err := db.handshake(config); err != nil {
            _ = conn.Close()
            lastErr = err

            continue
        }

        return db, nil
    }
    if lastErr == nil {
        lastErr = errors.New("no database endpoint configured")
    }

    return nil, fmt.Errorf("acceptance db connect: %w", lastErr)
}

// dbDialFuncs lists the dial closures of the configured endpoints in
// priority order.
func dbDialFuncs(config DBConfig) []func() (net.Conn, error) {
    dials := make([]func() (net.Conn, error), 0, 2)
    if config.Socket != "" {
        socket := config.Socket
        dials = append(dials, func() (net.Conn, error) {
            conn, err := dbDialer.Dial("unix", socket)
            if err != nil {
                return nil, fmt.Errorf("socket: %w", err)
            }

            return conn, nil
        })
    }
    if config.Address != "" {
        address := config.Address
        dials = append(dials, func() (net.Conn, error) {
            conn, err := dbDialer.Dial("tcp", address)
            if err != nil {
                return nil, fmt.Errorf("tcp: %w", err)
            }

            return conn, nil
        })
    }

    return dials
}

// handshake performs the 4.1 greeting exchange with the native auth.
func (db *DB) handshake(config DBConfig) error {
    if err := db.conn.SetDeadline(time.Now().Add(dbTimeout)); err != nil {
        return fmt.Errorf("deadline: %w", err)
    }
    greeting, err := db.readPacket()
    if err != nil {
        return fmt.Errorf("read greeting: %w", err)
    }
    plugin, err := parseGreetingPlugin(greeting)
    if err != nil {
        return err
    }
    if plugin != authPluginNative {
        // The deployed stack authenticates the passwordless root with the
        // classic native scramble; anything else means the endpoint is not the
        // expected server.
        return fmt.Errorf("auth plugin %q is not %q", plugin,
            authPluginNative)
    }
    if err := db.writePacket(handshakeResponse(config)); err != nil {
        return fmt.Errorf("send handshake response: %w", err)
    }
    // The empty password answer is one empty auth block; an auth switch may
    // come first.
    for {
        payload, err := db.readPacket()
        if err != nil {
            return fmt.Errorf("read auth answer: %w", err)
        }
        switch payload[0] {
        case okHeader:
            return nil
        case eofHeader:
            // An auth switch request: the native plugin answers empty for an
            // empty password.
            if err := db.writePacket(nil); err != nil {
                return fmt.Errorf("send auth switch answer: %w", err)
            }
        default:
            return db.protocolError(payload, "auth answer")
        }
    }
}

func handshakeResponse(config DBConfig) []byte {
    payload := make([]byte, 0, 96)
    payload = appendUint32(payload,
        clientProtocol41|clientPluginAuth|clientConnectWithDB)
    payload = appendUint32(payload, maxPacketSize)
    payload = append(payload, 33) // utf8_general_ci
    payload = append(payload, make([]byte, 23)...)
    payload = append(payload, config.User...)
    payload = append(payload, 0)
    payload = append(payload, byte(len(config.Password)))
    payload = append(payload, config.Password...)
    if config.Database != "" {
        // The default schema: without it every query answers "1046 No database
        // selected".
        payload = append(payload, config.Database...)
        payload = append(payload, 0)
    }
    payload = append(payload, authPluginNative...)
    payload = append(payload, 0)

    return payload
}

// Query runs one SQL statement and returns the text result rows; statements
// without a result set (UPDATE, DELETE, INSERT) answer an empty slice. Every
// value comes back as the raw column string, NULL as an empty string.
func (db *DB) Query(sql string) ([][]string, error) {
    if err := db.conn.SetDeadline(time.Now().Add(dbTimeout)); err != nil {
        return nil, fmt.Errorf("deadline: %w", err)
    }
    db.note(sql)
    if err := db.sendCommand(sql); err != nil {
        return nil, err
    }
    first, err := db.readPacket()
    if err != nil {
        return nil, fmt.Errorf("read query answer: %w", err)
    }
    switch first[0] {
    case okHeader:
        return nil, nil
    case errHeader:
        return nil, db.protocolError(first, "query")
    }
    columnCount, _, err := readLenEncInt(first, 0)
    if err != nil {
        return nil, fmt.Errorf("read column count: %w", err)
    }
    // Skip the column definitions and the closing EOF of the block.
    for range columnCount {
        if _, err := db.readPacket(); err != nil {
            return nil, fmt.Errorf("column definition: %w", err)
        }
    }
    if _, err := db.readPacket(); err != nil {
        return nil, fmt.Errorf("read column block end: %w", err)
    }
    var rows [][]string
    for {
        payload, err := db.readPacket()
        if err != nil {
            return nil, fmt.Errorf("read row: %w", err)
        }
        // The end of the row block is the EOF packet: the 0xFE lead with a
        // short body. A row may well start with 0xFE or 0x00 (a long length
        // prefix or an empty value), so the length is the only discriminator.
        if payload[0] == eofHeader && len(payload) < 9 {
            return rows, nil
        }
        row, err := parseTextRow(payload, int(columnCount))
        if err != nil {
            return nil, fmt.Errorf("parse row: %w", err)
        }
        rows = append(rows, row)
    }
}

func (db *DB) Exec(sql string) (int64, error) {
    if err := db.conn.SetDeadline(time.Now().Add(dbTimeout)); err != nil {
        return 0, fmt.Errorf("deadline: %w", err)
    }
    db.note(sql)
    if err := db.sendCommand(sql); err != nil {
        return 0, err
    }
    payload, err := db.readPacket()
    if err != nil {
        return 0, fmt.Errorf("read exec answer: %w", err)
    }
    switch payload[0] {
    case okHeader:
        affected, _, err := readLenEncInt(payload, 1)
        if err != nil {
            return 0, fmt.Errorf("read affected rows: %w", err)
        }

        return affected, nil
    case errHeader:
        return 0, db.protocolError(payload, "exec")
    default:
        return 0, db.protocolError(payload, "exec answer")
    }
}

func (db *DB) Close() error {
    return db.conn.Close()
}

// sendCommand frames one COM_QUERY statement and resets the sequence.
func (db *DB) sendCommand(sql string) error {
    db.sequence = 0
    request := append([]byte{comQuery}, sql...)

    return db.writePacket(request)
}

// Wire packet framing: the 3 byte little endian length prefix, the
// sequence byte, then the payload.

func (db *DB) writePacket(payload []byte) error {
    header := []byte{
        byte(len(payload)), byte(len(payload) >> 8), byte(len(payload) >> 16),
        db.sequence,
    }
    db.sequence++
    if _, err := db.conn.Write(header); err != nil {
        return err
    }
    if len(payload) == 0 {
        return nil
    }
    _, err := db.conn.Write(payload)

    return err
}

func (db *DB) readPacket() ([]byte, error) {
    header := make([]byte, 4)
    if _, err := io.ReadFull(db.conn, header); err != nil {
        return nil, err
    }
    length := int(header[0]) | int(header[1])<<8 | int(header[2])<<16
    db.sequence = header[3] + 1
    payload := make([]byte, length)
    if _, err := io.ReadFull(db.conn, payload); err != nil {
        return nil, err
    }

    return payload, nil
}

func (db *DB) protocolError(payload []byte, context string) error {
    if len(payload) >= 3 && payload[0] == errHeader {
        code := int(payload[1]) | int(payload[2])<<8
        message := payload[3:]
        // The sql state marker block ('#' plus five characters) follows the 4.1
        // errors.
        if len(message) > 6 && message[0] == '#' {
            message = message[6:]
        }

        return fmt.Errorf("db %s: %d %s", context, code, message)
    }

    return fmt.Errorf("db %s: unexpected payload % x", context, payload)
}

func parseGreetingPlugin(payload []byte) (string, error) {
    // int<1> protocol version.
    if len(payload) < 2 {
        return "", errors.New("short greeting packet")
    }
    pos := 1
    // string<NUL> server version.
    for pos < len(payload) && payload[pos] != 0 {
        pos++
    }
    pos++ // The terminating zero.
    // int<4> thread id, string[8] auth data part 1, int<1> filler,
    // int<2> capabilities low.
    pos += 4 + 8 + 1 + 2
    // int<1> charset, int<2> status flags, int<2> capabilities high.
    pos += 1 + 2 + 2
    if pos >= len(payload) {
        return "", errors.New("short greeting packet")
    }
    // int<1> auth plugin data length, zero when omitted (the classic 21 byte
    // default).
    dataLen := int(payload[pos])
    pos++
    // string[10] reserved.
    pos += 10
    if dataLen == 0 {
        dataLen = 21
    }
    part2 := dataLen - 8
    part2 = min(max(part2, 0), 13)
    pos += part2
    // string<NUL> the auth plugin name.
    end := pos
    for end < len(payload) && payload[end] != 0 {
        end++
    }

    return string(payload[pos:min(end, len(payload))]), nil
}

func parseTextRow(payload []byte, columns int) ([]string, error) {
    row := make([]string, 0, columns)
    pos := 0
    for len(row) < columns {
        if pos >= len(payload) {
            return nil, fmt.Errorf("row ends after %d of %d columns",
                len(row), columns)
        }
        if payload[pos] == nullValue {
            row = append(row, "")
            pos++

            continue
        }
        length, next, err := readLenEncInt(payload, pos)
        if err != nil {
            return nil, err
        }
        pos = next
        if pos+int(length) > len(payload) {
            return nil, errors.New("column value crosses the packet end")
        }
        row = append(row, string(payload[pos:pos+int(length)]))
        pos += int(length)
    }

    return row, nil
}

func readLenEncInt(payload []byte, pos int) (int64, int, error) {
    if pos >= len(payload) {
        return 0, 0, errors.New("length encoded integer past the end")
    }
    lead := payload[pos]
    switch {
    case lead < 0xFB:
        return int64(lead), pos + 1, nil
    case lead == 0xFC:
        if pos+3 > len(payload) {
            return 0, 0, errors.New("short 2 byte length")
        }

        return int64(payload[pos+1]) | int64(payload[pos+2])<<8, pos + 3, nil
    case lead == 0xFD:
        if pos+4 > len(payload) {
            return 0, 0, errors.New("short 3 byte length")
        }
        value := int64(payload[pos+1]) | int64(payload[pos+2])<<8 |
            int64(payload[pos+3])<<16

        return value, pos + 4, nil
    case lead == 0xFE:
        if pos+9 > len(payload) {
            return 0, 0, errors.New("short 8 byte length")
        }
        var value int64
        for i := range 8 {
            value |= int64(payload[pos+1+i]) << (8 * i)
        }

        return value, pos + 9, nil
    default:
        return 0, 0, fmt.Errorf("invalid length lead byte 0x%X", lead)
    }
}

func appendUint32(dst []byte, value uint32) []byte {
    return append(dst, byte(value), byte(value>>8), byte(value>>16),
        byte(value>>24))
}
