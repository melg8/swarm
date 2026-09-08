// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"bytes"
	"context"
	"log"
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestListenSkipsBusyOptionalListeners verifies the listener family
// semantics: the first address is mandatory, a busy optional address is
// skipped with a logged reason and the bound address reports stay
// honest (only the successfully bound listeners appear).
func TestListenSkipsBusyOptionalListeners(t *testing.T) {
	// Occupy one address so the optional listener cannot bind it.
	occupier, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = occupier.Close() })
	busy := occupier.Addr().String()

	logBuffer := &bytes.Buffer{}
	logger := log.New(logBuffer, "", 0)
	server := NewServer(logger,
		WithLoginAddresses("127.0.0.1:0", busy, "127.0.0.2:0"),
		WithGameAddresses("127.0.0.1:0", busy))
	require.NoError(t, server.Listen())
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	require.Len(t, server.LoginAddrs(), 2, "the busy login listener must be skipped")
	require.Len(t, server.GameAddrs(), 1, "the busy game listener must be skipped")
	require.NotContains(t, server.LoginAddrs(), busy)
	require.NotContains(t, server.GameAddrs(), busy)

	require.Contains(t, logBuffer.String(), "Proxy optional listener "+busy+" skipped")
}

// TestListenFailsOnBusyMandatoryListener verifies that a bind failure of
// the first (mandatory) address of a family is returned as an error.
func TestListenFailsOnBusyMandatoryListener(t *testing.T) {
	occupier, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = occupier.Close() })
	busy := occupier.Addr().String()

	server := NewServer(
		log.New(&bytes.Buffer{}, "", 0),
		WithLoginAddresses(busy),
		WithGameAddresses("127.0.0.1:0"))
	err = server.Listen()
	require.Error(t, err)
	require.Contains(t, err.Error(), busy)
}

// TestGamePortStaysFamilyIsolated guards the listener bookkeeping: with
// several login listeners bound the advertised game port must come from
// the game family, not from an interleaved login listener (the flat
// listeners slice of the first implementation mixed the families).
func TestGamePortStaysFamilyIsolated(t *testing.T) {
	server := NewServer(
		log.New(&bytes.Buffer{}, "", 0),
		WithLoginAddresses("127.0.0.1:0", "127.0.0.2:0", "127.0.0.2:0"),
		WithGameAddresses("127.0.0.1:0"))
	require.NoError(t, server.Listen())
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	gameAddr := server.GameAddr()
	require.NotEmpty(t, gameAddr)
	_, gamePortStr, err := net.SplitHostPort(gameAddr)
	require.NoError(t, err)
	gamePort, err := strconv.ParseInt(gamePortStr, 10, 32)
	require.NoError(t, err)

	require.Equal(t, int32(gamePort), server.gamePort())
	for _, loginAddr := range server.LoginAddrs() {
		require.NotEqual(t, gameAddr, loginAddr,
			"the game primary must never be a login listener address")
	}
}

// TestDefaultLoginAddressesCoverEveryClientPath pins the default
// listener set: the ini port redirect (2107) and the hardcoded classic
// auth port (2106) on both loopback addresses the shipped l2.ini can
// point at.
func TestDefaultLoginAddressesCoverEveryClientPath(t *testing.T) {
	require.Equal(t, []string{
		"127.0.0.1:2107", "127.0.0.1:2106", "127.0.0.2:2106", "127.0.0.2:2107",
	}, DefaultLoginAddresses())
}

// TestLogBindHintNamesTheHardcodedPort verifies the Windows diagnostic:
// a skipped login listener on the hardcoded auth port 2106 explains the
// Mobius login server ownership and the reservation causes, while other
// ports and game listeners stay quiet.
func TestLogBindHintNamesTheHardcodedPort(t *testing.T) {
	logBuffer := &bytes.Buffer{}
	server := &Server{logger: log.New(logBuffer, "", 0)}

	server.logBindHint(true, "127.0.0.1:2106")
	hint := logBuffer.String()
	require.Contains(t, hint, "hardcode the login port 2106")
	require.Contains(t, hint, "LoginserverHostname=127.0.0.3")
	require.Contains(t, hint, "excludedportrange")

	logBuffer.Reset()
	server.logBindHint(true, "127.0.0.2:2107")
	require.Empty(t, logBuffer.String(),
		"a custom ini port has no generic remedy to explain")

	logBuffer.Reset()
	server.logBindHint(false, "127.0.0.1:7778")
	require.Empty(t, logBuffer.String(),
		"a busy game listener has no generic remedy to explain")
}
