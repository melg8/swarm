// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"context"
	"errors"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// testManager builds a manager with fake scenarios and no live
// dependencies (no proxy, no geodata, no database).
func testManager(t *testing.T, defs []TestDef) *Manager {
	t.Helper()
	manager := NewManager(ManagerDeps{
		Registry: state.NewRegistry(),
		Login:    "127.0.0.1:2106",
		Engine:   nil,
		Proxy:    nil,
		Logger:   log.New(os.Stderr, "test ", log.LstdFlags),
		DBConfig: DefaultDBConfig(),
	}, defs)
	t.Cleanup(func() {
		for _, test := range manager.tests {
			test.mu.Lock()
			cancel := test.cancel
			test.mu.Unlock()
			if cancel != nil {
				cancel()
			}
		}
	})

	return manager
}

// TestDefinitionsAreSane pins the shipped scenario list: unique ids,
// unique accounts, filled descriptions and timeouts.
func TestDefinitionsAreSane(t *testing.T) {
	defs := Definitions()
	require.NotEmpty(t, defs)
	ids := map[string]bool{}
	accounts := map[string]bool{}
	for _, def := range defs {
		require.NotEmpty(t, def.ID)
		require.NotEmpty(t, def.Title)
		require.NotEmpty(t, def.Description)
		require.NotEmpty(t, def.Account)
		require.NotNil(t, def.Scenario)
		require.Positive(t, def.Timeout)
		require.False(t, ids[def.ID], "duplicate id "+def.ID)
		require.False(t, accounts[def.Account],
			"duplicate account "+def.Account)
		ids[def.ID] = true
		accounts[def.Account] = true
	}
}

// TestDefinitionsUseTempAccounts pins the temp account contract: the
// scenarios own temp1 through temp7 and never collide with the
// -bots fleet accounts (test1, test2, ...).
func TestDefinitionsUseTempAccounts(t *testing.T) {
	for _, def := range Definitions() {
		switch def.Account {
		case farmAccount, lifeAccount, relayAccount, returnAccount,
			entryAccount, gearAccount, soakAccount:
		default:
			t.Fatalf("scenario %s owns the unexpected account %s",
				def.ID, def.Account)
		}
	}
}

// TestManagerRegistersTrackers pins the sidebar contract: the temp
// bots appear in the registry from construction on.
func TestManagerRegistersTrackers(t *testing.T) {
	manager := testManager(t, Definitions())
	ids := map[string]bool{}
	for _, info := range manager.registry.List() {
		ids[info.ID] = true
	}
	require.True(t, ids[farmAccount])
	require.True(t, ids[lifeAccount])
	require.True(t, ids[relayAccount])
	require.True(t, ids[returnAccount])
	require.True(t, ids[gearAccount])
	require.True(t, ids[soakAccount])
}

// TestStartRejectsUnknownIDs pins the api error path.
func TestStartRejectsUnknownIDs(t *testing.T) {
	manager := testManager(t, Definitions())
	require.Error(t, manager.Start("no-such-test"))
	require.Error(t, manager.StartAll("sideways"))
}

// TestScenarioLifecycle pins the pass and fail paths of a run.
func TestScenarioLifecycle(t *testing.T) {
	scenarioCalls := 0
	defs := []TestDef{{
		ID:          "always-passes",
		Title:       "always passes",
		Description: "a scenario that returns nil",
		Account:     "temp9",
		Timeout:     5 * time.Second,
		Scenario: func(_ context.Context, _ *Manager, _ *Test) error {
			scenarioCalls++

			return nil
		},
	}, {
		ID:          "always-fails",
		Title:       "always fails",
		Description: "a scenario that returns an error",
		Account:     "temp8",
		Timeout:     5 * time.Second,
		Scenario: func(_ context.Context, _ *Manager, _ *Test) error {
			return errors.New("the scenario broke")
		},
	}}
	manager := testManager(t, defs)

	require.NoError(t, manager.Start("always-passes"))
	waitStatus(t, manager, "always-passes", StatusPassed)
	require.NoError(t, manager.Start("always-fails"))
	waitStatus(t, manager, "always-fails", StatusFailed)
	view := manager.Tests()[1]
	require.Contains(t, view.FailReason, "the scenario broke")
	require.Equal(t, 1, scenarioCalls)
}

// TestScenarioTimeout pins the timeout bound: a hanging scenario
// fails with the deadline instead of running forever.
func TestScenarioTimeout(t *testing.T) {
	defs := []TestDef{{
		ID:          "hangs",
		Title:       "hangs",
		Description: "a scenario that never returns",
		Account:     "temp7",
		Timeout:     150 * time.Millisecond,
		Scenario: func(ctx context.Context, _ *Manager, _ *Test) error {
			<-ctx.Done()

			return ctx.Err()
		},
	}}
	manager := testManager(t, defs)
	require.NoError(t, manager.Start("hangs"))
	view := waitStatus(t, manager, "hangs", StatusFailed)
	require.NotEmpty(t, view.FailReason)
}

// TestRestartReplacesTheRun pins the button semantics: starting a
// running scenario cancels it and the newer run owns the status.
func TestRestartReplacesTheRun(t *testing.T) {
	var mu sync.Mutex
	generations := 0
	defs := []TestDef{{
		ID:          "slow",
		Title:       "slow",
		Description: "a scenario that waits for the context",
		Account:     "temp6",
		Timeout:     5 * time.Second,
		Scenario: func(ctx context.Context, _ *Manager, _ *Test) error {
			mu.Lock()
			generations++
			mu.Unlock()
			select {
			case <-ctx.Done():
				return errors.New("replaced")
			case <-time.After(300 * time.Millisecond):
				return nil
			}
		},
	}}
	manager := testManager(t, defs)

	require.NoError(t, manager.Start("slow"))
	require.Equal(t, StatusRunning, waitRunning(manager, "slow"))
	require.NoError(t, manager.Start("slow"))
	view := waitStatus(t, manager, "slow", StatusPassed)
	require.Empty(t, view.FailReason)
	require.Equal(t, 2, generations)
}

// waitRunning polls the test view until it reports running.
func waitRunning(manager *Manager, id string) string {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, view := range manager.Tests() {
			if view.ID == id {
				if view.Status == StatusRunning {
					return view.Status
				}

				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	return ""
}

// TestStartAllParallel pins the parallel mode: every scenario starts
// at once, each on its own account.
func TestStartAllParallel(t *testing.T) {
	defs := []TestDef{
		{
			ID: "a", Title: "a", Description: "a", Account: "temp5",
			Timeout: 5 * time.Second,
			Scenario: func(ctx context.Context, _ *Manager, _ *Test) error {
				<-ctx.Done()

				return nil
			},
		},
		{
			ID: "b", Title: "b", Description: "b", Account: "temp4",
			Timeout: 5 * time.Second,
			Scenario: func(ctx context.Context, _ *Manager, _ *Test) error {
				<-ctx.Done()

				return nil
			},
		},
	}
	manager := testManager(t, defs)
	require.NoError(t, manager.StartAll(ModeParallel))

	// Both scenarios run concurrently: the views report them running
	// at the same time.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		views := manager.Tests()
		if views[0].Status == StatusRunning &&
			views[1].Status == StatusRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.Equal(t, StatusRunning, manager.Tests()[0].Status)
	require.Equal(t, StatusRunning, manager.Tests()[1].Status)
}

// TestStartAllSequential pins the sequential mode: the scenarios run
// one after another in definition order - the second test stays idle
// until the first one passed.
func TestStartAllSequential(t *testing.T) {
	releaseA := make(chan struct{})
	defs := []TestDef{
		{
			ID: "a", Title: "a", Description: "a", Account: "temp5",
			Timeout: 5 * time.Second,
			Scenario: func(_ context.Context, _ *Manager,
				_ *Test,
			) error {
				<-releaseA

				return nil
			},
		},
		{
			ID: "b", Title: "b", Description: "b", Account: "temp4",
			Timeout: 5 * time.Second,
			Scenario: func(context.Context, *Manager, *Test) error {
				return nil
			},
		},
	}
	manager := testManager(t, defs)
	require.NoError(t, manager.StartAll(ModeSequential))

	waitStatus(t, manager, "a", StatusRunning)
	// The second test must not start while the first one runs.
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, StatusIdle, manager.Tests()[1].Status)

	close(releaseA)
	waitStatus(t, manager, "a", StatusPassed)
	waitStatus(t, manager, "b", StatusPassed)
}

// waitStatus polls the test view until the wanted status lands.
func waitStatus(
	t *testing.T, manager *Manager, id string, status string,
) TestView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, view := range manager.Tests() {
			if view.ID == id {
				if view.Status == status {
					return view
				}

				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("test %s never reached status %s", id, status)

	return TestView{}
}

// TestRunBlocksUntilPassed pins the CLI contract: Run launches a
// scenario and blocks until it reaches the terminal state, returning
// nil when the scenario passed.
func TestRunBlocksUntilPassed(t *testing.T) {
	defs := []TestDef{{
		ID: "passes", Title: "passes", Description: "passes",
		Account: "temp9", Timeout: 5 * time.Second,
		Scenario: func(context.Context, *Manager, *Test) error {
			return nil
		},
	}}
	manager := testManager(t, defs)

	require.NoError(t, manager.Run(context.Background(), "passes"))
	view := manager.Tests()[0]
	require.Equal(t, StatusPassed, view.Status)
}

// TestRunBlocksUntilFailed pins the CLI contract: a failing scenario
// makes Run return an error that wraps the fail reason.
func TestRunBlocksUntilFailed(t *testing.T) {
	defs := []TestDef{{
		ID: "fails", Title: "fails", Description: "fails",
		Account: "temp8", Timeout: 5 * time.Second,
		Scenario: func(context.Context, *Manager, *Test) error {
			return errors.New("the scenario broke")
		},
	}}
	manager := testManager(t, defs)

	err := manager.Run(context.Background(), "fails")
	require.Error(t, err)
	require.Contains(t, err.Error(), "the scenario broke")
	require.Equal(t, StatusFailed, manager.Tests()[0].Status)
}

// TestRunRejectsUnknownID pins the api error path of Run.
func TestRunRejectsUnknownID(t *testing.T) {
	manager := testManager(t, Definitions())
	err := manager.Run(context.Background(), "no-such-test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no-such-test")
}

// TestRunAllStopsAtFirstFailure pins the CLI contract: RunAll runs
// every scenario in definition order and stops at the first failing
// one, the rest stay unrun.
func TestRunAllStopsAtFirstFailure(t *testing.T) {
	calls := make(map[string]int)
	var mu sync.Mutex
	defs := []TestDef{
		{
			ID: "a", Title: "a", Description: "a", Account: "temp5",
			Timeout: 5 * time.Second,
			Scenario: func(context.Context, *Manager, *Test) error {
				mu.Lock()
				calls["a"]++
				mu.Unlock()

				return nil
			},
		},
		{
			ID: "b", Title: "b", Description: "b", Account: "temp4",
			Timeout: 5 * time.Second,
			Scenario: func(context.Context, *Manager, *Test) error {
				mu.Lock()
				calls["b"]++
				mu.Unlock()

				return errors.New("b broke")
			},
		},
		{
			ID: "c", Title: "c", Description: "c", Account: "temp3",
			Timeout: 5 * time.Second,
			Scenario: func(context.Context, *Manager, *Test) error {
				mu.Lock()
				calls["c"]++
				mu.Unlock()

				return nil
			},
		},
	}
	manager := testManager(t, defs)

	err := manager.RunAll(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "b broke")

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, calls["a"], "a ran once before b failed")
	require.Equal(t, 1, calls["b"], "b ran and failed")
	require.Equal(t, 0, calls["c"], "c stayed unrun after b failed")
}

// TestRunAllPassesEveryScenario pins the happy path: every scenario
// passing leaves RunAll returning nil.
func TestRunAllPassesEveryScenario(t *testing.T) {
	defs := []TestDef{
		{
			ID: "a", Title: "a", Description: "a", Account: "temp5",
			Timeout: 5 * time.Second,
			Scenario: func(context.Context, *Manager, *Test) error {
				return nil
			},
		},
		{
			ID: "b", Title: "b", Description: "b", Account: "temp4",
			Timeout: 5 * time.Second,
			Scenario: func(context.Context, *Manager, *Test) error {
				return nil
			},
		},
	}
	manager := testManager(t, defs)

	require.NoError(t, manager.RunAll(context.Background()))
	require.Equal(t, StatusPassed, manager.Tests()[0].Status)
	require.Equal(t, StatusPassed, manager.Tests()[1].Status)
}

// TestIDsReturnsDefinitionOrder pins the discovery contract of the
// CLI: the IDs come back in the registration order, the order the
// manager and the web UI show.
func TestIDsReturnsDefinitionOrder(t *testing.T) {
	defs := []TestDef{
		{
			ID:          "z",
			Title:       "z",
			Description: "z",
			Account:     "temp5",
			Timeout:     5 * time.Second,
			Scenario: func(context.Context, *Manager, *Test) error {
				return nil
			},
		},
		{
			ID:          "a",
			Title:       "a",
			Description: "a",
			Account:     "temp4",
			Timeout:     5 * time.Second,
			Scenario: func(context.Context, *Manager, *Test) error {
				return nil
			},
		},
	}
	manager := testManager(t, defs)
	require.Equal(t, []string{"z", "a"}, manager.IDs())
}
