// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "bytes"
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "sort"
    "strings"

    "github.com/melg8/swarm/internal/swarm/proxy"
)

// The launch configuration file (the -config flag): the file driven
// way to describe the swarm. The JSON file carries the shared launch
// parameters (the login server, the web interface, the geodata and
// proxy wiring) and the fleet composition - the bot types with their
// counts - so a deployment tunes its swarm by editing a file instead
// of maintaining a CLI flag line. An explicitly set flag always wins
// over the file (the flag stays the emergency override of a broken
// config), and running without -config keeps the plain flag defaults
// of parseFlags: the default configuration is the fallback, not a
// requirement (owner issue #12).

// launchConfig is the parsed -config file: the shared launch
// parameters plus the swarm composition. The zero values mean "not
// set in the file" - the matching flag default applies then, so a
// minimal file of just the bots array is complete.
type launchConfig struct {
    Login      string    `json:"login"`
    Account    string    `json:"account"`
    Password   string    `json:"password"`
    Char       string    `json:"char"`
    Web        string    `json:"web"`
    Hunt       bool      `json:"hunt"`
    Geodata    string    `json:"geodata"`
    Navmesh    string    `json:"navmesh"`
    Proxy      bool      `json:"proxy"`
    ProxyLogin string    `json:"proxyLogin"`
    ProxyGame  string    `json:"proxyGame"`
    ProxyLog   string    `json:"proxyLog"`
    SessionDir string    `json:"sessionDir"`
    Bots       []botSpec `json:"bots"`
}

// botSpec is one entry of the swarm composition: a bot type and how
// many bots of that type the fleet launches. An empty type is the
// default fighter, so {"count": 3} reads as three fighters.
type botSpec struct {
    Type  string `json:"type"`
    Count int    `json:"count"`
}

// botPlan is one expanded fleet slot: the type the bot runs and the
// account (and character) name it connects under. The account ladder
// walks the whole composition in order: two fighters then two archers
// over the base test1 make test1, test2, test3, test4.
type botPlan struct {
    Type    string
    Account string
}

// botTypeFighter is the default (and today the only) bot type: the
// elven melee fighter of the current hunt loop. The empty type of a
// bot spec reads as this constant, so {"count": 3} means three
// fighters.
const botTypeFighter = "fighter"

// implementedBotTypes lists the bot types the fleet can launch today.
// The fighter is the melee hunt bot of the current hunt loop; the
// archer with its kiting behavior is issue #13 and joins this registry
// (and the docs) when it lands. A config naming anything else fails
// validation with this list in the message, so a config written for a
// newer swarm refuses loudly instead of silently degrading.
var implementedBotTypes = map[string]bool{
    botTypeFighter: true,
}

// defaultLaunchConfig is the built-in default of the -config file:
// exactly the flag defaults of parseFlags with a single fighter. The
// shipped configs/swarm.json mirrors it, and a launch without -config
// runs the same values - the file, the fallback and the flags agree
// by construction (the tests pin the three-way equality).
func defaultLaunchConfig() launchConfig {
    return launchConfig{
        Login:      defaultLoginAddress,
        Account:    defaultAccount,
        Password:   defaultPassword,
        Char:       defaultCharName,
        Web:        defaultWebAddress,
        Hunt:       false,
        Geodata:    "",
        Navmesh:    "",
        Proxy:      false,
        ProxyLogin: strings.Join(proxy.DefaultLoginAddresses(), ","),
        ProxyGame:  strings.Join(proxy.DefaultGameAddresses(), ","),
        ProxyLog:   defaultProxyLogPath,
        SessionDir: "logs",
        Bots:       []botSpec{{Type: botTypeFighter, Count: 1}},
    }
}

// loadLaunchConfig reads and validates one launch configuration
// file. Unknown fields refuse the load (a typo'd key silently
// falling back to a default is the worst failure mode a config file
// can have), and the composition checks run before the caller can
// build anything from the file.
func loadLaunchConfig(path string) (launchConfig, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return launchConfig{}, fmt.Errorf(
            "cannot read the launch config: %w", err)
    }
    var cfg launchConfig
    dec := json.NewDecoder(bytes.NewReader(data))
    dec.DisallowUnknownFields()
    if err := dec.Decode(&cfg); err != nil {
        return launchConfig{}, fmt.Errorf(
            "cannot parse the launch config %s: %w", path, err)
    }
    if err := cfg.validate(); err != nil {
        return launchConfig{}, fmt.Errorf(
            "invalid launch config %s: %w", path, err)
    }

    return cfg, nil
}

// validate checks the fleet composition of the file: every type must
// be an implemented bot type and every count positive. The shared
// parameters carry no constraint beyond the launch path itself (an
// unreachable login address fails at connect time with its own
// error, exactly like the flag form).
func (lc launchConfig) validate() error {
    if len(lc.Bots) == 0 {
        return errors.New(
            "the bots array is empty - name at least one bot type " +
                "with its count")
    }
    for i, spec := range lc.Bots {
        typ := spec.Type
        if typ == "" {
            typ = botTypeFighter
        }
        if !implementedBotTypes[typ] {
            return fmt.Errorf(
                "bots[%d]: unknown bot type %q (implemented: %s)",
                i, spec.Type, implementedTypeList())
        }
        if spec.Count < 1 {
            return fmt.Errorf(
                "bots[%d] (%s): the count must be at least 1, got %d",
                i, typ, spec.Count)
        }
    }

    return nil
}

// expand turns the composition into the flat fleet plan: the account
// ladder walks every slot of every spec in file order, so the fleet
// accounts stay test1, test2, test3... whatever the mix of types is
// (the ladder semantics of the plain -bots flag carry over).
func (lc launchConfig) expand() []botPlan {
    plans := make([]botPlan, 0, lc.totalBots())
    for _, spec := range lc.Bots {
        typ := spec.Type
        if typ == "" {
            typ = botTypeFighter
        }
        for range spec.Count {
            plans = append(plans, botPlan{
                Type:    typ,
                Account: fleetAccountName(lc.Account, len(plans)),
            })
        }
    }

    return plans
}

// totalBots sums the composition counts.
func (lc launchConfig) totalBots() int {
    total := 0
    for _, spec := range lc.Bots {
        total += spec.Count
    }

    return total
}

// implementedTypeList renders the sorted type registry for the
// validation messages.
func implementedTypeList() string {
    types := make([]string, 0, len(implementedBotTypes))
    for typ := range implementedBotTypes {
        types = append(types, typ)
    }
    sort.Strings(types)

    return strings.Join(types, ", ")
}

// applyLaunchConfig folds a loaded file into the flag configuration.
// An explicitly set flag wins over the file (the emergency override
// of a broken config), an empty file field means "keep the flag
// default", and the file composition replaces the -bots count unless
// -bots itself was explicit (then the flag form wins whole: the plain
// ladder of N fighters).
func applyLaunchConfig(cfg *config, lc launchConfig,
    explicit map[string]bool,
) {
    foldStringFields(cfg, lc, explicit)
    if !explicit["hunt"] && lc.Hunt {
        cfg.hunt = true
    }
    if !explicit["proxy"] && lc.Proxy {
        cfg.proxy = true
    }
    if explicit["bots"] {
        // The explicit -bots flag replaces the composition whole: the
        // classic ladder of N fighters, exactly like a launch without
        // a config file.
        cfg.botPlans = classicFleetPlan(cfg.account, cfg.bots)

        return
    }
    // The composition expands over the resolved base account: an
    // explicit -account flag overrides the file value, and the fleet
    // ladder must follow the flag (the plan and cfg.account can never
    // disagree or the single bot path and the fleet path would launch
    // different sessions off one config).
    resolved := lc
    resolved.Account = cfg.account
    cfg.botPlans = resolved.expand()
    cfg.bots = len(cfg.botPlans)
}

// foldStringFields folds the shared string parameters of the file
// into the flag configuration through one table: the flag name (for
// the explicit-override check), the file value (empty means "not set
// in the file") and the assignment. The bool flags stay in
// applyLaunchConfig - their false is indistinguishable from omitted,
// so only a true file value folds.
func foldStringFields(cfg *config, lc launchConfig,
    explicit map[string]bool,
) {
    folds := []struct {
        flag  string
        value string
        apply func(*config, string)
    }{
        {"login", lc.Login, func(c *config, v string) {
            c.loginAddress = v
        }},
        {"account", lc.Account, func(c *config, v string) {
            c.account = v
        }},
        {"password", lc.Password, func(c *config, v string) {
            c.password = v
        }},
        {"char", lc.Char, func(c *config, v string) {
            c.charName = v
        }},
        {"web", lc.Web, func(c *config, v string) {
            c.webAddress = v
        }},
        {"geodata", lc.Geodata, func(c *config, v string) {
            c.geodataDir = v
        }},
        {"navmesh", lc.Navmesh, func(c *config, v string) {
            c.navmeshDir = v
        }},
        {"proxy-login", lc.ProxyLogin, func(c *config, v string) {
            c.proxyLogin = v
        }},
        {"proxy-game", lc.ProxyGame, func(c *config, v string) {
            c.proxyGame = v
        }},
        {"proxy-log", lc.ProxyLog, func(c *config, v string) {
            c.proxyLog = v
        }},
        {"session-dir", lc.SessionDir, func(c *config, v string) {
            c.sessionDir = v
        }},
    }
    for _, fold := range folds {
        if !explicit[fold.flag] && fold.value != "" {
            fold.apply(cfg, fold.value)
        }
    }
}

// classicFleetPlan builds the composition of the plain -bots flag: N
// fighters on the account ladder. The launch paths read one plan
// whatever its origin (the flag or the file), so the fleet code
// carries a single shape.
func classicFleetPlan(base string, count int) []botPlan {
    plans := make([]botPlan, 0, count)
    for i := range count {
        plans = append(plans, botPlan{
            Type:    botTypeFighter,
            Account: fleetAccountName(base, i),
        })
    }

    return plans
}

// compositionText renders the plan for the launch log line: the head
// of "2 fighter, 1 archer" the operator reads back.
func compositionText(plans []botPlan) string {
    counts := map[string]int{}
    order := []string{}
    for _, plan := range plans {
        if _, seen := counts[plan.Type]; !seen {
            order = append(order, plan.Type)
        }
        counts[plan.Type]++
    }
    parts := make([]string, 0, len(order))
    for _, typ := range order {
        parts = append(parts, fmt.Sprintf("%d %s", counts[typ], typ))
    }

    return strings.Join(parts, ", ")
}
