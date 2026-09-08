<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# data/client — Lineage 2 C1 client assets

## l2.ini (proxy redirect)

The C1 client uses this `l2.ini` to find the authentication server. The
copy shipped here is the client's stock C1 `l2.ini` (Lineage2Ver212
container, Blowfish payload) re-encrypted by
[open-l2encdec](https://github.com/ritsuwastaken/open-l2encdec) with two
edits of the plaintext `[URL]` section:

| Setting      | Stock value  | This copy   | Meaning |
|--------------|--------------|-------------|---------|
| `ServerAddr` | `127.0.0.1`  | `127.0.0.1` | swarm proxy login server host (swarm and the client run on the same machine) |
| `Port`       | `7777`       | `2107`      | swarm proxy login port for client builds that honor the ini port (see the port notes) |

The swarm proxy (branch `feature/proxy-server`, run the bot with
`-proxy`) listens for client logins on `127.0.0.1:2107`, `127.0.0.1:2106`,
`127.0.0.2:2106` and `127.0.0.2:2107`, and for game connections on
`127.0.0.1:7778` and `127.0.0.2:7778`; the emulated login server answers with
a one entry server list pointing at the proxy game port, so the client
never needs to know the Mobius ports.

To install: copy this file over `l2.ini` in the client folder (backup
the original first).

### Port notes (read if the client cannot login)

The classic C1 executable **hardcodes the login port 2106**: the auth
socket dials `ServerAddr:2106` and ignores the ini `Port` line (an
Unreal Engine leftover - that is why the stock ini ships `Port=7777`
while the real auth server always answered on 2106). The `Port=2107`
edit of this copy only matters for client builds that honor the ini
port; the proxy therefore answers 2106 and 2107 on both `127.0.0.1` and
`127.0.0.2` (the whole 127.0.0.0/8 block is loopback).

`127.0.0.1:2106` belongs to the real Mobius login server by default, so
pick one of the two recipes of `docs/proxy.md` ("How the C1 client finds
the proxy"): either move the real login server to `127.0.0.3`
(`LoginserverHostname = 127.0.0.3` in the login `Server.ini` + swarm
`-login 127.0.0.3:2106`, this ini then works as is), or keep the real
login on a non-wildcard `127.0.0.1` and re-encrypt this ini with
`ServerAddr=127.0.0.2`. A login listener skipped with an access
permissions error on Windows means the port is either wildcard-owned or
reserved by Hyper-V/WinNAT (`netsh interface ipv4 show excludedportrange
protocol=tcp`, `net stop winnat` frees it) - the proxy log names the
remedies.

### Regenerating after edits

```bash
# decrypt
l2encdec -c decode -o l2-dec.ini l2.ini
# edit l2-dec.ini ([URL] ServerAddr / Port)
l2encdec -c encode -p 212 -o l2.ini l2-dec.ini
```

The `l2encdec` binary is built from open-l2encdec 1.3.11 (MIT) with
mbedtls 3.6.7, miniz 3.1.2 and avinal/blowfish via CMake; the pinned
upstream commit lives in `tools/install_l2encdec.sh`.

The decrypted plaintext contains private data of the client owner (the
[AutoLogOn] block) and is intentionally NOT committed.
