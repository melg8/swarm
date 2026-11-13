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
    "time"

    "github.com/melg8/swarm/internal/swarm/hunt"
    "github.com/melg8/swarm/internal/swarm/proxy"
)

// The launch configuration file (the -config flag): the JSON file carries the
// shared launch parameters (login, web, geodata, proxy wiring) and the fleet
// composition (the bot types with their counts), so a deployment tunes its
// swarm by editing a file instead of a CLI flag line. An explicitly set flag
// always wins over the file, and a launch without -config keeps the plain flag
// defaults: the default configuration is the fallback, not a requirement.

// launchConfig is the parsed -config file. The zero values mean "not set in the
// file", so the matching flag default applies and a minimal file of just the
// bots array is complete.
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

// botSpec is one entry of the swarm composition: a bot type and how many bots
// of it the fleet launches. An empty type is the default fighter, so {"count":
// 3} reads as three fighters; the optional kite section tunes the archer fight
// only.
type botSpec struct {
    Type  string       `json:"type"`
    Count int          `json:"count"`
    Kite  *botKiteSpec `json:"kite"`
}

// botKiteSpec is the JSON shape of the per-spec kite overrides. The pointers
// distinguish "not set in the file" (the shipped tuning applies) from an
// explicit value, so a profile names exactly the knobs it tunes without a
// rebuild between measurement runs.
type botKiteSpec struct {
    Enabled       *bool    `json:"enabled"`
    RetreatRadius *float64 `json:"retreatRadius"`
    Step          *float64 `json:"step"`
    StepPeriod    *string  `json:"stepPeriod"`
    ReengageDelay *string  `json:"reengageDelay"`
}

// toParams reads the spec into the hunt params: unset fields keep
// DefaultKiteParams, set fields override. Durations arrive as Go duration
// strings (validate checked the parse).
func (s *botKiteSpec) toParams() hunt.KiteParams {
    p := hunt.DefaultKiteParams()
    if s.Enabled != nil {
        p.Enabled = *s.Enabled
    }
    if s.RetreatRadius != nil {
        p.RetreatRadius = *s.RetreatRadius
    }
    if s.Step != nil {
        p.Step = *s.Step
    }
    if s.StepPeriod != nil {
        if d, err := time.ParseDuration(*s.StepPeriod); err == nil {
            p.StepPeriod = d
        }
    }
    if s.ReengageDelay != nil {
        if d, err := time.ParseDuration(*s.ReengageDelay); err == nil {
            p.ReengageDelay = d
        }
    }

    return p
}

// botPlan is one expanded fleet slot: the type, the account (and character)
// name and the kite override, the whole per-bot identity. The account ladder
// walks the composition in order: two fighters then two archers over test1 make
// test1..test4.
type botPlan struct {
    Type    string
    Account string
    Kite    *botKiteSpec
}

// botTypeFighter is the melee type: the elven fighter of the hunt loop, the
// reading of an empty spec type.
const botTypeFighter = "fighter"

// botTypeArcher is the ranged type: the ranged weapon preference with kiting.
// The bow-aware radii and the lure profiles of the hunt loop serve a
// bow-equipped bot.
const botTypeArcher = "archer"

// implementedBotTypes lists the launchable bot types. A config naming anything
// else fails validation with this list in the message, so a config written for
// a newer swarm refuses loudly instead of silently degrading.
var implementedBotTypes = map[string]bool{
    botTypeFighter: true,
    botTypeArcher:  true,
}

// defaultLaunchConfig is the built-in default of the -config file: the flag
// defaults with three fighters and three archers (the ladder runs test1..test3
// as fighters and test4..test6 as archers). The shipped configs/swarm.json
// mirrors it; a launch without -config keeps the plain single fighter of the
// flag form.
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
        Bots: []botSpec{
            {Type: botTypeFighter, Count: 3, Kite: nil},
            {Type: botTypeArcher, Count: 3, Kite: nil},
        },
    }
}

// loadLaunchConfig reads and validates one launch configuration file. Unknown
// fields refuse the load (a typo'd key silently falling back to a default is
// the worst failure mode a config file can have), and the composition checks
// run before anything builds.
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

// validate checks the composition: every type implemented and every count
// positive. The shared parameters carry no constraint beyond the launch path
// itself (an unreachable login fails at connect time, exactly like the flag
// form).
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
        if err := validateKiteSpec(i, typ, spec.Kite); err != nil {
            return err
        }
    }

    return nil
}

// validateKiteSpec checks the optional kite section of one bot spec: an archer
// knob, the distances positive and the durations parsing as Go duration
// strings. A broken line refuses the launch with the spec index and the field
// name.
func validateKiteSpec(i int, typ string, kite *botKiteSpec) error {
    if kite == nil {
        return nil
    }
    if typ != botTypeArcher {
        return fmt.Errorf(
            "bots[%d] (%s): the kite section tunes the archer fight, "+
                "it does not apply to this type", i, typ)
    }
    if kite.RetreatRadius != nil && *kite.RetreatRadius <= 0 {
        return fmt.Errorf(
            "bots[%d] (archer): kite.retreatRadius must be positive, got %g",
            i, *kite.RetreatRadius)
    }
    if kite.Step != nil && *kite.Step <= 0 {
        return fmt.Errorf(
            "bots[%d] (archer): kite.step must be positive, got %g",
            i, *kite.Step)
    }
    for _, field := range []struct {
        name  string
        value *string
    }{
        {"kite.stepPeriod", kite.StepPeriod},
        {"kite.reengageDelay", kite.ReengageDelay},
    } {
        if field.value == nil {
            continue
        }
        if _, err := time.ParseDuration(*field.value); err != nil {
            return fmt.Errorf(
                "bots[%d] (archer): %s %q is not a duration (use the "+
                    "Go form: 3s, 500ms): %w",
                i, field.name, *field.value, err)
        }
    }

    return nil
}

// expand turns the composition into the flat fleet plan: the account ladder
// walks every slot of every spec in file order, so the accounts stay test1,
// test2... whatever the type mix is.
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
                Kite:    spec.Kite,
            })
        }
    }

    return plans
}

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

// applyLaunchConfig folds a loaded file into the flag configuration: an
// explicit flag wins, an empty file field keeps the flag default, and the file
// composition replaces the -bots count unless -bots itself was explicit.
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
        // The explicit -bots flag replaces the composition: the classic ladder
        // of N fighters, exactly like a launch without a config file.
        cfg.botPlans = classicFleetPlan(cfg.account, cfg.bots)

        return
    }
    // The composition expands over the resolved base account: an explicit
    // -account flag overrides the file value, and the ladder must follow it or
    // the single bot path and the fleet path would launch different sessions
    // off one config.
    resolved := lc
    resolved.Account = cfg.account
    cfg.botPlans = resolved.expand()
    cfg.bots = len(cfg.botPlans)
}

// foldStringFields folds the shared string parameters through one table: the
// flag name for the explicit-override check, the file value (empty means "not
// set") and the assignment. The bool flags stay in applyLaunchConfig: their
// false is indistinguishable from omitted, so only a true file value folds.
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

// classicFleetPlan builds the composition of the plain -bots flag: N fighters
// on the account ladder. Both launch paths read one plan shape.
func classicFleetPlan(base string, count int) []botPlan {
    plans := make([]botPlan, 0, count)
    for i := range count {
        plans = append(plans, botPlan{
            Type:    botTypeFighter,
            Account: fleetAccountName(base, i),
            Kite:    nil,
        })
    }

    return plans
}

// compositionText renders the plan for the launch log line ("2 fighter, 1
// archer").
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
