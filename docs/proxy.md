# The C1 client proxy (MITM server)

The swarm process can act as a transparent proxy for a real Lineage 2
C1 client: the client connects to the swarm exactly like it would
connect to the Mobius servers, while the swarm keeps its own bot
session on the real server and relays everything between the two.
The design goal: with the bot farming, the user connects with the
game client, watches the automated gameplay from the character's own
perspective and can take over at any moment - the clicks of the user
flow to the real server through the bot session.

## Running

```bash
go run ./cmd/swarm -hunt -proxy -web 127.0.0.1:8080
```

Flags:

| Flag | Default | Meaning |
|------|---------|---------|
| `-proxy` | off | enables the client proxy |
| `-proxy-login` | `127.0.0.1:2107,127.0.0.1:2106,127.0.0.2:2106,127.0.0.2:2107` | login listen addresses (the first is mandatory, the rest are optional fallbacks) |
| `-proxy-game` | `127.0.0.1:7778,127.0.0.2:7778` | game listen addresses |
| `-proxy-log` | `proxy.log` | the client connection log file |

The proxy needs nothing else from the stack: the swarm bot keeps
connecting to the real `127.0.0.1:2106` / `:7777` as usual (or wherever
`-login` points it).

## How the C1 client finds the proxy (read this first)

The classic C1 executable **hardcodes the auth port 2106**: the auth
socket dials `ServerAddr:2106` and ignores the `[URL]` `Port` line of
`l2.ini` (an Unreal Engine leftover - the stock C1 `l2.ini` ships
`Port=7777` while the auth server always answered on 2106). The `Port`
edit of the shipped ini only matters for builds that honor it, so the
proxy answers every combination: `2106` and `2107` on both `127.0.0.1`
and `127.0.0.2`.

The catch: `127.0.0.1:2106` is the address of the **real** Mobius login
server, and the swarm bots need that server. Two proven recipes resolve
the conflict (both are one config line plus one flag or ini line):

**Recipe A (recommended, every client path works).** Move the real login
server to the third loopback address and let the proxy own
`127.0.0.1:2106`:

1. In the Mobius login `Server.ini`: `LoginserverHostname = 127.0.0.3`
   (a wildcard `*`/`0.0.0.0` bind blocks *all* loopback addresses on
   that port under Windows).
2. Run the swarm with `-login 127.0.0.3:2106` (the bots reach the real
   login server there).
3. Keep the shipped `l2.ini` (`ServerAddr=127.0.0.1`): a classic client
   lands on the proxy through the hardcoded 2106, a port-honoring
   build through 2107.

**Recipe B (no swarm flag change).** Keep the real login server on
`127.0.0.1:2106` (not wildcard!) and move the client to the second
loopback address:

1. In the Mobius login `Server.ini`: `LoginserverHostname = 127.0.0.1`
   (must not be `*`/`0.0.0.0`, otherwise the proxy cannot bind
   `127.0.0.2:2106` on Windows).
2. Edit `l2.ini`: `ServerAddr=127.0.0.2` (the classic client then dials
   `127.0.0.2:2106`, which the proxy answers).

## Client setup (l2.ini)

The C1 client finds the auth server through the `[URL]` section of its
`l2.ini`. The repository ships a ready re-encrypted copy at
`data/client/l2.ini` (open-l2encdec, protocol 212 container): its
`ServerAddr=127.0.0.1` and `Port=2107` pair with Recipe A above (and
with any client build that honors the ini port). Copy it over the
`l2.ini` of the client folder (backup the original first); for Recipe B
decrypt it, set `ServerAddr=127.0.0.2`, re-encrypt (see
`data/client/Readme.txt`).

When the swarm, the stack and the client run on one machine (the
reference setup), `127.0.0.1` works as is. A client on another machine
needs `-proxy-login 0.0.0.0:2107 -proxy-game 0.0.0.0:7778` and the
`ServerAddr` of its ini set to the swarm machine address.

## The protocol the client sees

1. **Login server** (fully emulated, no contact with the real login
   server): `Init` (mirroring the real one, including the scrambled RSA
   modulus captured by the bot's login) -> the client's
   `RequestAuthLogin` with **any** account/password pair is accepted ->
   `LoginOk` -> `ServerList` with exactly one server pointing at the
   proxy game port -> `RequestServerLogin` -> `PlayOk` with locally
   generated keys. A `RequestGGAuth` (0x07) is answered like the legacy
   servers did.
2. **Game server** (emulated): `ProtocolVersion`/`KeyPacket` with a
   proxy key (the client's cipher chain starts from the proxy key, the
   bot's chain from the real server key - the two chains are
   independent, which is what makes the proxy a true MITM instead of a
   byte pipe) -> the client's `AuthLogin` (any keys) -> a
   `CharSelectionInfo` with exactly **one character**: the character
   played by the selected bot (the web UI selection, see below). The
   appearance fields come from the recorded real char list of the
   session, the vitals from the live tracker.
3. **Character select**: the proxy answers with the *recorded*
   `CharSelected` packet of the bot session - byte identical to what
   the real server sent the bot.
4. **Enter world**: the client receives the *recorded* server packet
   stream of the bot session (everything after the bot's
   `CharSelected`: the UserInfo, the inventory, the known list, every
   movement and fight since the bot entered - the replay) and then the
   live feed continues seamlessly. The client processes the backlog in
   a fast forward burst and converges on the current world state.
5. **In world**: every server packet is relayed to the client, every
   client packet (movement, attacks, actions, chat, logout) is
   forwarded to the real game server through the bot session - the
   server cannot tell the difference. The hunt loop of the bot keeps
   running: autonomous actions and user actions interleave on the same
   character.

Character creation and deletion are refused by the emulation (the
client manages exactly the one served character). The login phase
packets never reach the real server, so the bot account is untouched.

## Switching bots

Multiple bot sessions can run in one swarm process (the fleet). The bot
**selected in the web UI** is the one a connecting client attaches to -
clicking a bot in the sidebar marks it with a `proxy` chip and POSTs
`/api/proxy/select`. With no selection (or the web UI off) the first
registered session serves. The client switches bots by reconnecting
after changing the selection. `GET /api/proxy` reports the state.

## Packet transformation (the debug seam)

`proxy.Transformer` is the seam for the future server packet spoofing:
the relay decrypts every packet, hands it to the transformer and
re-encrypts the result on the direction specific chain. Because each
direction owns its chain, a transformer may freely rewrite, resize,
drop or inject packets. The current implementation is the transparent
passthrough (`PassthroughTransformer`).

## Debugging the client connection

Everything the proxy observes lands in `proxy.log` (separate from the
console output of the bot): every connection with its number
(`login#3`, `game#7`), every login attempt with the credentials used,
every state transition, the replay statistics, every client -> server
packet id and every close reason.

The file is the first diagnostic of a login failure and its content
maps the problem directly:

- **Nothing but the startup lines** (no `login#N: client connected`):
  the client never reached the proxy. With the classic hardcoded 2106
  this means the client dialed the real login server address instead -
  apply Recipe A or B above. Check who owns the port on Windows:
  `netstat -ano | findstr :2106`.
- **`Proxy optional listener 127.0.0.1:2106 skipped ... access
  permissions`**: either the real Mobius login server holds the port
  (its wildcard bind blocks the whole port on Windows - the recipes
  above) or Windows reserved the port range through Hyper-V/WinNAT.
  The reserved ranges are listed by
  `netsh interface ipv4 show excludedportrange protocol=tcp`; when 2106
  is inside one, `net stop winnat` (then `net start winnat` after the
  swarm bound its listeners) or a reboot frees it.
- **`login#N: client connected` followed by an immediate close**:
  the client reached the proxy but the handshake failed - the following
  lines carry the exact packet and reason (send the file).

The live verification harness of the whole path (against the deployed
stack, with a fake C1 client that walks the same protocol as the real
one):

```bash
tools/proxy_e2e.sh   # prints PROXY_E2E_OK
```

The real Windows client check follows the same recipe: start the stack,
start the swarm with `-proxy` (and `-hunt` for the observation mode),
copy `data/client/l2.ini` into the client, login with any credentials,
pick the offered character, enter the world.
