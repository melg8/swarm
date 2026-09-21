// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/melg8/swarm/internal/swarm/proxy"
    "github.com/stretchr/testify/require"
)

// writeConfigFile materializes one launch config JSON under a temp
// directory and returns its path.
func writeConfigFile(t *testing.T, content string) string {
    t.Helper()
    path := filepath.Join(t.TempDir(), "swarm.json")
    require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

    return path
}

// TestLaunchConfigLoadsTheComposition pins the happy path of the
// -config file: the bots array expands into the fleet plan (the types
// normalized, the account ladder walking the whole composition) and
// the shared parameters come along.
func TestLaunchConfigLoadsTheComposition(t *testing.T) {
    path := writeConfigFile(t, `{
        "login": "10.0.0.5:2106",
        "account": "bot7",
        "web": "0.0.0.0:8080",
        "hunt": true,
        "bots": [
            {"type": "fighter", "count": 2},
            {"count": 1}
        ]
    }`)
    lc, err := loadLaunchConfig(path)
    require.NoError(t, err)
    require.Equal(t, "10.0.0.5:2106", lc.Login)
    require.True(t, lc.Hunt)

    plan := lc.expand()
    require.Len(t, plan, 3)
    // The ladder walks the composition in order: the explicit type,
    // then the defaulted empty type (a fighter).
    require.Equal(t, botPlan{Type: "fighter", Account: "bot7"},
        plan[0])
    require.Equal(t, botPlan{Type: "fighter", Account: "bot8"},
        plan[1])
    require.Equal(t, botPlan{Type: "fighter", Account: "bot9"},
        plan[2])
}

// TestLaunchConfigRejectsBrokenFiles pins the validation contract:
// unknown fields, unknown bot types, non-positive counts, an empty
// composition and unparsable JSON all refuse the launch with an
// error naming the problem.
func TestLaunchConfigRejectsBrokenFiles(t *testing.T) {
    t.Run("the unknown field refuses", func(t *testing.T) {
        path := writeConfigFile(t,
            `{"lgn": "127.0.0.1:2106", "bots": [{"count": 1}]}`)
        _, err := loadLaunchConfig(path)
        require.ErrorContains(t, err, "unknown field")
    })

    t.Run("the unknown bot type refuses", func(t *testing.T) {
        path := writeConfigFile(t,
            `{"bots": [{"type": "archer", "count": 1}]}`)
        _, err := loadLaunchConfig(path)
        require.ErrorContains(t, err, "unknown bot type")
        require.ErrorContains(t, err, "fighter",
            "the message lists the implemented types")
    })

    t.Run("the zero count refuses", func(t *testing.T) {
        path := writeConfigFile(t,
            `{"bots": [{"type": "fighter", "count": 0}]}`)
        _, err := loadLaunchConfig(path)
        require.ErrorContains(t, err, "count must be at least 1")
    })

    t.Run("the empty composition refuses", func(t *testing.T) {
        path := writeConfigFile(t, `{"bots": []}`)
        _, err := loadLaunchConfig(path)
        require.ErrorContains(t, err, "bots array is empty")
    })

    t.Run("the missing file refuses", func(t *testing.T) {
        _, err := loadLaunchConfig(filepath.Join(
            t.TempDir(), "absent.json"))
        require.ErrorContains(t, err, "cannot read")
    })

    t.Run("the broken json refuses", func(t *testing.T) {
        path := writeConfigFile(t, `{"bots": [`)
        _, err := loadLaunchConfig(path)
        require.ErrorContains(t, err, "cannot parse")
    })
}

// applyFlags builds a parsed flag configuration the way parseFlags
// does for the fold tests: the flag defaults plus the explicit flag
// names the caller passes.
func applyFlags(t *testing.T, explicit []string) config {
    t.Helper()
    cfg := config{
        loginAddress: defaultLoginAddress,
        account:      defaultAccount,
        password:     defaultPassword,
        charName:     defaultCharName,
        webAddress:   defaultWebAddress,
        proxyLogin:   strings.Join(proxy.DefaultLoginAddresses(), ","),
        proxyGame:    strings.Join(proxy.DefaultGameAddresses(), ","),
        proxyLog:     defaultProxyLogPath,
        bots:         1,
    }
    explicitSet := map[string]bool{}
    for _, name := range explicit {
        explicitSet[name] = true
    }
    applyLaunchConfig(&cfg, fileConfigForFold(), explicitSet)

    return cfg
}

// fileConfigForFold is the fold fixture: a two fighter composition
// with a custom login and hunt, everything else left to the defaults.
func fileConfigForFold() launchConfig {
    return launchConfig{
        Login:    "10.0.0.5:2106",
        Account:  "cfgacc",
        Hunt:     true,
        Password: "cfgpass",
        Bots:     []botSpec{{Type: "fighter", Count: 2}},
    }
}

// TestApplyLaunchConfigFoldsTheFile pins the precedence rules: the
// file values apply over the flag defaults, an explicitly set flag
// wins over the file, and the composition replaces the -bots count
// (the plain -bots flag replaces the composition whole).
func TestApplyLaunchConfigFoldsTheFile(t *testing.T) {
    t.Run("the file folds over the defaults", func(t *testing.T) {
        cfg := applyFlags(t, nil)
        require.Equal(t, "10.0.0.5:2106", cfg.loginAddress)
        require.Equal(t, "cfgacc", cfg.account)
        require.Equal(t, "cfgpass", cfg.password)
        require.True(t, cfg.hunt)
        require.Equal(t, defaultWebAddress, cfg.webAddress,
            "an omitted file field keeps the flag default")
        require.Equal(t, 2, cfg.bots)
        require.Len(t, cfg.botPlans, 2)
        require.Equal(t, botPlan{Type: "fighter", Account: "cfgacc"},
            cfg.botPlans[0])
        require.Equal(t, botPlan{Type: "fighter", Account: "cfgacc2"},
            cfg.botPlans[1])
    })

    t.Run("the explicit flag wins over the file", func(t *testing.T) {
        cfg := applyFlags(t, []string{"login", "hunt"})
        require.Equal(t, defaultLoginAddress, cfg.loginAddress,
            "the explicit -login keeps its own value")
        require.False(t, cfg.hunt,
            "an explicit -hunt=false holds against the file")
        require.Equal(t, "cfgacc", cfg.account,
            "the untouched fields still fold")
    })

    t.Run("the explicit account rebases the ladder", func(t *testing.T) {
        cfg := applyFlags(t, []string{"account"})
        require.Equal(t, defaultAccount, cfg.account)
        require.Equal(t, botPlan{Type: "fighter", Account: defaultAccount},
            cfg.botPlans[0],
            "the ladder follows the resolved account, never the file one")
        require.Equal(t, botPlan{Type: "fighter", Account: "test2"},
            cfg.botPlans[1])
    })

    t.Run("the explicit bots flag replaces the composition",
        func(t *testing.T) {
            cfg := applyFlags(t, []string{"bots"})
            cfg.bots = 3
            applyLaunchConfig(&cfg, fileConfigForFold(),
                map[string]bool{"bots": true})
            require.Len(t, cfg.botPlans, 3)
            for _, plan := range cfg.botPlans {
                require.Equal(t, "fighter", plan.Type)
            }
        })
}

// TestDefaultLaunchConfigMatchesTheFlagDefaults pins the three-way
// agreement the issue asks for: the built-in default config, the
// shipped configs/swarm.json and the plain flag defaults describe the
// same launch, so "run without a config" and "run the default config"
// can never drift apart.
func TestDefaultLaunchConfigMatchesTheFlagDefaults(t *testing.T) {
    def := defaultLaunchConfig()
    require.Equal(t, defaultLoginAddress, def.Login)
    require.Equal(t, defaultAccount, def.Account)
    require.Equal(t, defaultPassword, def.Password)
    require.Equal(t, defaultCharName, def.Char)
    require.Equal(t, defaultWebAddress, def.Web)
    require.False(t, def.Hunt)
    require.Equal(t, "logs", def.SessionDir)
    require.Equal(t, defaultProxyLogPath, def.ProxyLog)
    require.Equal(t, []botSpec{{Type: "fighter", Count: 1}}, def.Bots)

    // The shipped file parses, validates and expands to the same
    // single fighter as the built-in default.
    lc, err := loadLaunchConfig(filepath.Join("..", "..",
        "configs", "swarm.json"))
    require.NoError(t, err)
    require.Equal(t, def, lc,
        "configs/swarm.json mirrors the built-in default")
    require.Equal(t, []botPlan{
        {Type: "fighter", Account: defaultAccount},
    }, lc.expand())
}

// TestClassicFleetPlanMatchesTheBotsFlag pins the normalization: a
// launch without a config file builds the same plan the old -bots
// ladder produced, so the fleet behavior of the flag form is frozen.
func TestClassicFleetPlanMatchesTheBotsFlag(t *testing.T) {
    plan := classicFleetPlan("test1", 3)
    require.Equal(t, []botPlan{
        {Type: "fighter", Account: "test1"},
        {Type: "fighter", Account: "test2"},
        {Type: "fighter", Account: "test3"},
    }, plan)

    // A numbered base continues its own ladder (the fleetAccountName
    // rule the flag form documents).
    numbered := classicFleetPlan("temp2", 2)
    require.Equal(t, []botPlan{
        {Type: "fighter", Account: "temp2"},
        {Type: "fighter", Account: "temp3"},
    }, numbered)
}

// TestCompositionTextRendersThePlan pins the launch log line: the
// counts per type in the plan order, the summary the operator reads
// back at startup.
func TestCompositionTextRendersThePlan(t *testing.T) {
    require.Equal(t, "1 fighter",
        compositionText([]botPlan{{Type: "fighter", Account: "a"}}))
    require.Equal(t, "2 fighter, 1 archer",
        compositionText([]botPlan{
            {Type: "fighter", Account: "a"},
            {Type: "fighter", Account: "b"},
            {Type: "archer", Account: "c"},
        }))
}
