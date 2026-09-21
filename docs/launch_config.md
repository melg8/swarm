# Launch configuration

The launch configuration file (the `-config` flag) is the file driven
way to describe the swarm: the shared launch parameters and the fleet
composition - the bot types with their counts - so a deployment tunes
its swarm by editing a JSON file instead of maintaining a CLI flag
line (owner issue #12).

The shipped default lives at `configs/swarm.json` and mirrors the
built-in default exactly: the shared parameters at their flag
defaults with the default swarm composition of **three melee
warriors and three archers** (the owner picked the six bot default;
the account ladder walks the composition, so the fleet runs test1..
test3 as fighters and test4..test6 as archers). A launch without
`-config` keeps the plain single fighter of the flag form. The file /
default equality is pinned by `cmd/swarm/launch_config_test.go`.

## Running with a config

```bash
swarm_bot -config configs/swarm.json
swarm_bot -config myswarm.json -bots 5   # the explicit flag wins
swarm_bot                                # no file: the flag defaults
```

The precedence rules:

1. An **explicitly set flag** always wins over the file value (the
   emergency override of a broken config). A flag left at its default
   lets the file value through.
2. An **omitted (or empty) file field** keeps its flag default.
3. An explicit `-bots N` **replaces the composition whole**: the plain
   ladder of N fighters, exactly like a launch without a config.
4. A broken file refuses to launch: unknown fields, unknown bot
   types, non-positive counts and an empty composition all exit with
   an error naming the problem. A config must fail loudly, never
   silently degrade.

The mode switches (`-acceptance`, `-pathfind-test`, `-show-navmesh`,
the journal queries) are not part of the file: they stay flags and
dispatch before any bot session launches. The file parameters (the
login address, the account, the geodata wiring) still apply to the
mode they select.

## The file format

```json
{
    "login": "127.0.0.1:2106",
    "account": "test1",
    "password": "test",
    "char": "test1",
    "web": "127.0.0.1:8080",
    "hunt": false,
    "geodata": "",
    "navmesh": "",
    "proxy": false,
    "proxyLogin": "127.0.0.1:2107,127.0.0.1:2106,127.0.0.2:2106,127.0.0.2:2107",
    "proxyGame": "127.0.0.1:7778,127.0.0.2:7778",
    "proxyLog": "proxy.log",
    "sessionDir": "logs",
    "bots": [
        { "type": "fighter", "count": 3 },
        { "type": "archer", "count": 3 }
    ]
}
```

| Field | Flag | Meaning |
| --- | --- | --- |
| `login` | `-login` | login server address |
| `account` | `-account` | base account name; the fleet ladder derives test1, test2, ... from it |
| `password` | `-password` | account password (the server auto-creates accounts) |
| `char` | `-char` | base character name; a fleet bot's character equals its account name |
| `web` | `-web` | web interface address, empty disables it (see the precedence rule 2 - use the explicit `-web=""` flag to disable) |
| `hunt` | `-hunt` | the autonomous hunt loop of every bot |
| `geodata` | `-geodata` | geodata directory, empty autodetects the candidates |
| `navmesh` | `-navmesh` | navmesh tile directory, empty autodetects `data/navmesh` |
| `proxy` | `-proxy` | the MITM client proxy for real C1 clients |
| `proxyLogin` | `-proxy-login` | comma separated proxy login listen addresses |
| `proxyGame` | `-proxy-game` | comma separated proxy game listen addresses |
| `proxyLog` | `-proxy-log` | the proxy connection log file |
| `sessionDir` | `-session-dir` | session journal directory, empty disables the journal |
| `bots` | `-bots` | the fleet composition: an array of `{type, count}` entries |

## The composition

Every entry of the `bots` array names a bot type and how many bots of
that type the fleet launches. An empty `type` is the default
`fighter`, so `{"count": 3}` reads as three fighters.

The account ladder walks the whole composition in file order: two
fighters over the base `test1` make `test1`, `test2`; adding two more
bots of the next type continues with `test3`, `test4`. The ladder
semantics are the plain `-bots` rule (a numbered base continues its
own ladder, `temp2` runs `temp2`, `temp3`, ...).

The implemented bot types today:

| Type | Behavior |
| --- | --- |
| `fighter` | the elven melee fighter of the current hunt loop |
| `archer` | the ranged archetype: the ranged weapon preference with kiting and bow shots even at melee range. The bow gear plan is live (the weapon milestone buys the bow, the quiver restocks behind it, the arrows arm onto the left hand); the kiting lands in its own slices (#18-#20 of #13) |

A config naming an unimplemented type fails validation with the list
of implemented types in the error, so a config written for a newer
swarm refuses loudly instead of silently degrading.

A composition of exactly one bot runs the single bot path (no fleet
supervisor, no shared web sidebar); anything larger runs the fleet
mode exactly like `-bots N` does. The startup log reads the
composition back: `Starting swarm fleet of 6 bots (3 fighter,
3 archer)`.

## Where the code lives

`cmd/swarm/launch_config.go` carries the format, the loader, the
validation, the expansion and the flag fold; `cmd/swarm/main.go`
registers the `-config` flag and normalizes every launch (file or
plain flags) into one fleet plan the single bot path and the fleet
path share. The tests in `cmd/swarm/launch_config_test.go` pin the
format contract, the precedence rules and the three-way equality of
the built-in default, the shipped file and the flag defaults.
