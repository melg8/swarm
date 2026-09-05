// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package crypt

import (
	"encoding/hex"
	"testing"

	toauthserver "github.com/melg8/swarm/internal/swarm/packets/to_auth_server"
)

// TestDebugDumpServerListBytes prints sealed packets for Java cross-check.
func TestDebugDumpServerListBytes(t *testing.T) {
	lc := NewLoginCrypt(MobiusAuthKey())

	authLogin := &toauthserver.RequestAuthLogin{Account: "test1", Password: "test"}
	wire1, err := lc.SealPacket(nil, authLogin)
	if err != nil {
		t.Fatal(err)
	}

	serverList := toauthserver.NewRequestServerList(0x11223344, 0x55667788)
	wire2, err := lc.SealPacket(nil, serverList)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("AUTH_LOGIN_WIRE=%s", hex.EncodeToString(wire1))
	t.Logf("SERVER_LIST_WIRE=%s", hex.EncodeToString(wire2))
}
