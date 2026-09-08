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
| `-proxy-login` | `127.0.0.1:2107,127.0.0.2:2106` | login listen addresses (the first is mandatory, the rest are fallbacks) |
| `-proxy-game` | `127.0.0.1:7778,127.0.0.2:7778` | game listen addresses |
| `-proxy-log` | `proxy.log` | the client connection log file |

The proxy needs nothing else from the stack: the swarm bot keeps
connecting to the real `127.0.0.1:2106` / `:7777` as usual.

## Client setup (l2.ini)

The C1 client finds the auth server through the `[URL]` section of its
`l2.ini`. The repository ships a ready re-encrypted copy at
`data/client/l2.ini` (open-l2encdec, protocol 212 container): its
`ServerAddr=127.0.0.1` and `Port=2107` point at the proxy. Copy it over
the `l2.ini` of the client folder (backup the original first).

The classic clients hardcode the login port 2106 in the executable and
may ignore the ini port. For that case the proxy also listens on
`127.0.0.2:2106` (the whole `127.0.0.0/8` block is loopback): set
`ServerAddr=127.0.0.2` in the ini (keep `Port` as is) and make sure the
Mobius login server does not own `0.0.0.0:2106` - set
`LoginserverHostname = 127.0.0.1` in the login `Server.ini` and restart
it. The sandbox deployment applies that tweak automatically
(`tools/swarm_fast_deploy.sh`); the reference Windows deployment needs
the same one line change.

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
packet id and every close reason. When a login does not work, send
this file - it shows whether the client reached the proxy at all
(nothing in the file: the client never connected, check the l2.ini and
the port), which port it used, and how far the flow got.

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
