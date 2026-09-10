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
2. **Game server** (emulated): `ProtocolVersion`/`KeyPacket` with the
   **static Mobius C1 session key** (`94 35 00 00 a1 6c 54 87`, exactly
   what the real `GameClient.CRYPT_KEY` ships). The real C1 client must
   stay compatible with a hardcoded key - that is why Mobius never
   rotates it - so the proxy must not either: a random per connection
   key desynchronized the real client and made its encrypted
   `AuthLogin` undecodable. The client's cipher chain starts from the
   proxy key, the bot's chain from the real server key - the two chains
   are independent, which is what makes the proxy a true MITM instead
   of a byte pipe. The client's `AuthLogin` (any keys) is parsed
   leniently (both the null terminated and the length prefixed utf16
   layouts, and a fully unreadable packet still continues - the account
   name is cosmetic) -> a `CharSelectionInfo` with exactly **one
   character**: the character played by the selected bot (the web UI
   selection, see below). The appearance fields come from the recorded
   real char list of the session, the vitals, the position and the
   paperdoll from the live tracker.
3. **Character select**: the proxy answers with the *recorded*
   `CharSelected` packet of the bot session - byte identical to what
   the real server sent the bot, except the position, vitals and
   progression fields, which are rewritten from the live tracker (see
   "The live self state" below). A client reconnecting after the bot
   walked away spawns where the character actually stands, not at the
   stale login-time coordinates.
4. **Enter world**: the client receives the *recorded* server packet
   stream of the bot session (everything after the bot's
   `CharSelected`: the UserInfo, the inventory, the known list, every
   movement and fight since the bot entered - the replay) and then the
   live feed continues seamlessly. The client processes the backlog in
   a fast forward burst and converges on the current world state. The
   recorded packets describing the played character itself are patched
   to the live state (see below); the world packets of other objects
   replay unchanged.
5. **In world**: every server packet is relayed to the client, every
   client packet (movement, attacks, actions, chat, logout) is
   forwarded to the real game server through the bot session - the
   server cannot tell the difference. The hunt loop of the bot keeps
   running: autonomous actions and user actions interleave on the same
   character. A bot initiated logout does NOT end this phase: the
   client is held through the bot relogin and restarted onto the
   replacement (see "The bot relogin handoff" below). The one exception
   is the keepalive: the `RequestNetPing` of the client is answered by
   the proxy itself and never reaches the server (see "The client
   keepalive" below).

Character creation and deletion are refused by the emulation (the
client manages exactly the one served character). The login phase
packets never reach the real server, so the bot account is untouched.

## The client keepalive (the ping feedback loop)

The C1 client sends `RequestNetPing` (0xA8) as its keepalive and its
scheduler re-arms on every `NetPing` answer (0xEC) it receives. In the
proxy topology both halves of that round trip exist separately - the
bot session pings the real server on its own 25 s cycle, and the
client's pings would transit through the bot session - so relaying the
server's answers to the client closes a feedback loop: one relayed
answer makes the client re-ping, the transit reaches the server
through the bot session, the new answer is recorded and relayed again,
and the loop accelerates without bound. That was the reported packet
explosion: the WebUI counter of the attached character grew by ~1
million packets in under 30 seconds (the flood is pure `0xEC`/`0xA8`
round trips) and the recorder entries behind it loaded the memory.

The fix severs the loop on both sides while keeping the client's
keepalive semantics intact:

- the client's `RequestNetPing` is answered by the proxy itself
  (locally, no server round trip, no recording) with a synthesized
  `NetPing` carrying the game time harvested from the last real
  answer of the bot session - the field the client uses for its
  clock cosmetics;
- the `NetPing` answers of the bot session are filtered out of the
  replay and the live feed (they are per-connection keepalive
  artifacts, they describe no world state), and their game time is
  harvested on the way;
- a client held for the bot relogin gets its keepalive answered too -
  the held connection must not time out on the client side.

The regression harness `TestProxyPacketGrowthRepro` drives a fake
client with the answer driven ping behavior (one new ping per received
answer) through the attach and the WebUI switch phases against the
live stack: before the fix the attached bot's inbound rate climbed
from single digits to 8000+ packets per second within seconds, after
the fix both phases stay at the ambient world rate.

## The live self state (reconnection correctness)

The replay answers one hard question: *where is the character right
now?* The recorded stream holds the world as the bot saw it, and for
every object except the played character the answer stays valid. For
the character itself the recorded place is the login-time spot, which
is wrong the moment the bot moves - a reconnecting client spawned
there, ran against the server-side geometry and crashed. Three pieces
fix it, all fed by the live state tracker (which the bot session keeps
current through every UserInfo, movement and teleport packet):

- **The char list paperdoll** (the selection screen equipment): the
  `CharSelectionInfo` paperdoll tables are built from the tracker -
  the slot object ids from the last UserInfo broadcast (the server
  refreshes the block on every equip and unequip) and the item ids
  resolved through the tracked inventory. Before the fix the tables
  were zeroed, so the selection screen rendered a naked character.
- **The char selected answer**: the recorded `CharSelected` packet is
  patched with the live position, HP/MP, SP/EXP and level (binary
  patch on a copy - the byte layout is scanned, not reserialized, so
  everything unpatched stays byte identical to the real server).
- **The replayed UserInfo**: every `UserInfo` of the played character
  is patched with the live position, vitals, level and progression.
  The stale *movement family* packets of the character itself
  (`MoveToLocation`, `MoveToPawn`, `StopMove`, `ValidateLocation`,
  `TeleportToLocation`) are dropped from the replay except the newest
  one - its coordinates are exactly where the tracker stands, because
  the tracker takes its position from that very packet. When the bot
  is mid-run at the reconnect, the client animates the same run; when
  it stands, the client places it at the same spot.

The live relay after the replay needs no patching: the real server
already answers with the current state, and the two views converge on
their own.

## The bot relogin handoff (the client survives the session cycle)

The relay answers the mirror question: *what happens to the client
when the BOT disconnects?* The hunt loop logs the character out when
the situation turns hopeless (the emergency logout), the supervisor
logs it back in seconds later - the user client did nothing and must
not be kicked to the login screen for a decision the bot made. The
relay handles the whole cycle in `streamSession`/`serveRelogin`:

- **The LeaveWorld suppression.** The `LeaveWorld` the real server
  answers to the bot's logout is dropped - both from the live feed
  and from the recorded history - because a client that processes it
  returns to the login screen on its own, which would break the
  hold. The suppression is conditional: a `LeaveWorld` that follows
  the client's OWN `Logout` packet is relayed normally (the classic
  flow must keep working - see below).
- **The hold.** When the session's recorder closes (the bot session
  ended), the connection stays open, the relay polls for a
  replacement session of the same bot id (a fresh recorder, a fresh
  send path, the character back `StatusOnline`), and the client
  packets in between are swallowed: the character is offline and the
  world behind the client is frozen, so every action is meaningless -
  except the `Logout` itself: a user that wants out of the frozen
  world gets a synthesized `LeaveWorld` from the proxy (the server
  cannot answer, the character is gone) and the connection closes.
  The hold gives up after 2 minutes (a dead bot releases the client
  instead of holding a silent world forever).
- **The resync (the restart dance).** Once the replacement session
  is online with its `CharSelected` recorded, the client is taken
  through the restart dance (see "Switching bots" below): the
  synthesized `RestartResponse` + the char list pair moves the client
  to its own char select screen (the client tears its whole world
  down there - no ghost objects, no stale self pawn), and the auto
  select serves the `CharSelected` answer of the same character
  live-patched to the fresh place. The client then re-enters the world
  by itself and the ordinary replay path serves the enter world burst
  of the replacement session (the UserInfo, the inventory, the new
  known list) followed by its live feed, and the connection transits
  the client packets through the live bot link again. The result is
  exactly the view a fresh client would get - plus a brief char
  select screen the user never has to click.

A user initiated logout keeps the classic flow: the client's own
`Logout` packet transits to the real server, the `LeaveWorld` answer
is RELAYED (not suppressed - the suppression only covers bot
initiated logouts), the client returns to the login screen by
itself, and the session end then closes the connection instead of
holding it. The defensive path: a replacement session whose
`CharSelected` was never recorded cannot serve a dance (there is no
answer for the double click), so the hold keeps waiting until the
login handshake lands in the recorder - which is the production
order anyway: the `CharSelected` is recorded before the character
enters the world.

## Switching bots

Multiple bot sessions can run in one swarm process (the fleet). The bot
**selected in the web UI** is the one a connecting client attaches to -
clicking a bot in the sidebar marks it with a `proxy` chip and POSTs
`/api/proxy/select`. With no selection (or the web UI off) the first
registered session serves. `GET /api/proxy` reports the state.

A selection change switches an **already connected** client through the
**restart flow - the official C1 mechanism** the in-game Restart button
rides: the relay wakes on the selection notification, resolves the
newly selected bot, and plays the exact packet pair the real server
answers a restart with (see `RequestRestart.handlePacket` of the Mobius
C1 server): `RestartResponse` followed by the `CharSelectionInfo` of
the new bot. The C1 client processes the `RestartResponse` by tearing
its whole world down and returning to the character select screen on
its own - and that teardown is what makes the switch correct: the
teleport + DeleteObject sweep approach of the earlier implementation
crashed the real client (a `UserInfo` carrying an object id the client
never spawned dereferences a missing pawn inside the
`UserInfoPacket` handler - the reported `General protection fault` when
switching between distant characters; a `UserInfo` of a nearby known
object left the client controlling its old pawn while the replayed
history of the new bot played every death and fight of its session -
the "monsters died at once" effect). A restart is the one mid-session
identity change the C1 client is designed to process.

To keep the switch hands-off, the proxy then plays the **double click
itself**: after 1.5 s (enough for the client to render the char select
screen) the auto select serves the live-patched `CharSelected` of the
new bot - the exact answer the user's own double click of the only
listed character would produce. The user's own click also still works
(it cancels the timer; both paths are idempotent). The client loads,
sends its `EnterWorld`, and the ordinary replay + live feed of the new
bot streams: the new character's position, appearance, race, class and
equipment all arrive through the enter world packets that exist for
exactly this purpose - without a reconnect.

Selecting the bot the client already watches changes nothing.

A selected bot that is not online yet (registered but still
connecting, or not registered at all) does not disconnect or stall
the client either: the relay keeps serving the current live feed and
polls for the target, switching the moment the target enters the
world - the client never needs a reconnect or a re-click (selecting
the same id twice is a no-op). A selection change that fires while
the client sits on the char select screen of a dance re-offers the
char list of the newest target, and the served `CharSelected` always
re-resolves the newest selection, so the client lands on the bot the
WebUI shows even if the user raced the dance with a click.

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
- **`game#N: auth login packet unreadable (len N, decrypted XX:...):
  continuing`**: the game `AuthLogin` of the client did not decode into
  an account name. The connection still continues (any account is
  accepted), but the hex dump tells the protocol state: bytes looking
  like `08 74 00 65 00 ...` mean the cipher is in sync and only the
  string layout differs, while pure noise means a cipher
  desynchronization (compare with `94 35 00 00 a1 6c 54 87` on the
  client side). The historical random-key bug produced exactly this
  signature before the static key fix. If the account line right after
  shows readable garbage, the login still works - the name is only
  used for logging.
- **`game#N: no bot session available for the client`**: the client
  made it through the full handshake, but no bot is online yet. Start
  the swarm with a bot that entered the world (the char list is built
  from its session).
- **`game#N: the bot session ... ended, holding the client for its
  relogin`**: not an error - the bot logged out (the emergency logout
  of the hunt loop) and the client is held. The follow up lines tell
  the outcome: `bot ... is back online, restarting the held client
  onto it` (the restart dance: the char select screen, the auto
  select, the replay of the new session), `the user
  logged out while held for the bot relogin` (the user left the
  frozen world by design) or `the bot ... did not return within 2m0s,
  releasing the held client` (the hold window expired - a supervisor
  stuck longer than two minutes is the thing to investigate).
- **`game#N: restart dance onto ... offered (auto select in ...)`**:
  not an error - a WebUI selection change (or a completed bot
  relogin) moved the client onto another character: the client
  received the `RestartResponse` + the char list of the target and
  the auto select timer is running. The next lines trace the entry:
  `auto selecting the offered character` (the proxy's own double
  click), then the ordinary `replaying ... then live` of the new
  bot. The dance of a bot switch logs as `selection switched from
  ... to ..., starting the restart dance`.
- **`game#N: auto selecting the offered character, serving the char
  selected answer`**: not an error - the hands-off part of every
  switch: the proxy served the `CharSelected` of the offered
  character without the user clicking anything.
- **`game#N: leave world suppressed, the client stays for the bot
  relogin`**: not an error - the real server answered the bot's logout
  with `LeaveWorld` and the proxy dropped it so the held client stays
  in the world. It appears once per bot logout of a connected client.

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
