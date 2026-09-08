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
| `Port`       | `7777`       | `2107`      | swarm proxy login port (`-proxy-login` default) |

The swarm proxy (branch `feature/proxy-server`, run the bot with
`-proxy`) listens for client logins on `127.0.0.1:2107` and for game
connections on `127.0.0.1:7778`; the emulated login server answers with
a one entry server list pointing at the proxy game port, so the client
never needs to know the Mobius ports.

To install: copy this file over `l2.ini` in the client folder (backup
the original first).

### Port notes (read if the client cannot login)

The classic clients ship with the login port 2106 hardcoded in the
executable and may ignore the ini port. The proxy therefore binds a
second login listener on `127.0.0.2:2106` by default, so a client with
the hardcoded port also reaches it when `ServerAddr=127.0.0.2` is set
in the ini (the whole 127.0.0.0/8 block is loopback). For that listener
to bind, the Mobius login server must not own `0.0.0.0:2106`: set
`LoginserverHostname = 127.0.0.1` in the login `Server.ini` (the
sandbox deployment of `tools/swarm_fast_deploy.sh` applies this
automatically, see `docs/proxy.md` for the Windows deployment).

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
