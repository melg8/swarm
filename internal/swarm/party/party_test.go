// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package party

import (
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// writeConfig writes the JSON body into a temp file and returns its path.
func writeConfig(t *testing.T, body string) string {
    t.Helper()
    path := filepath.Join(t.TempDir(), "parties.json")
    require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

    return path
}

// TestLoadConfigReadsTheGrouping pins the parse of the shipped shape.
func TestLoadConfigReadsTheGrouping(t *testing.T) {
    path := writeConfig(t, `{"groups":[{"leader":"test1",
        "members":["test1","test2","test3"]}]}`)

    config, err := LoadConfig(path)
    require.NoError(t, err)
    require.Len(t, config.Groups, 1)
    require.Equal(t, "test1", config.Groups[0].Leader)
    require.Equal(t, []string{"test1", "test2", "test3"},
        config.Groups[0].Members)
    require.Equal(t, "test1[test1,test2,test3]", config.Describe())
}

// TestLoadConfigRejectsBrokenGroupings pins every validation rule.
func TestLoadConfigRejectsBrokenGroupings(t *testing.T) {
    cases := map[string]string{
        "no groups": `{"groups":[]}`,
        "no leader": `{"groups":[{"leader":"","members":["test1"]}]}`,
        "leader not listed": `{"groups":[{"leader":"test1",
            "members":["test2"]}]}`,
        "empty member": `{"groups":[{"leader":"test1",
            "members":["test1",""]}]}`,
        "shared account": `{"groups":[
            {"leader":"test1","members":["test1","test2"]},
            {"leader":"test3","members":["test3","test2"]}]}`,
        "over the cap": `{"groups":[{"leader":"a",
            "members":["a","b","c","d","e","f","g","h","i","j"]}]}`,
    }
    for name, body := range cases {
        t.Run(name, func(t *testing.T) {
            _, err := LoadConfig(writeConfig(t, body))
            require.Error(t, err)
        })
    }
}

// TestLoadConfigFailsOnAMissingFile pins the read error.
func TestLoadConfigFailsOnAMissingFile(t *testing.T) {
    _, err := LoadConfig(filepath.Join(t.TempDir(), "absent.json"))
    require.Error(t, err)
}

// TestHubSharesTheGroupState pins the hub contract: the group lookup, the
// publish of the placement and the leader state read.
func TestHubSharesTheGroupState(t *testing.T) {
    config, err := LoadConfig(writeConfig(t, `{"groups":[{"leader":"test1",
        "members":["test1","test2","test3"]}]}`))
    require.NoError(t, err)
    hub := NewHub(config)

    leader, members, ok := hub.GroupOf("test2")
    require.True(t, ok)
    require.Equal(t, "test1", leader)
    require.Equal(t, []string{"test1", "test2", "test3"}, members)

    _, _, ok = hub.GroupOf("stranger")
    require.False(t, ok)

    now := time.Unix(1_700_000_000, 0)
    hub.Publish(MemberState{
        Account: "test1", ObjectID: 42, TargetID: 77,
        X: 1000, Y: 2000, Z: -3000, Alive: true, At: now,
    })

    state, ok := hub.Member("test1")
    require.True(t, ok)
    require.Equal(t, int32(42), state.ObjectID)
    require.Equal(t, int32(77), state.TargetID)
    require.Equal(t, now, state.At)

    state, ok = hub.LeaderOf("test3")
    require.True(t, ok)
    require.Equal(t, int32(42), state.ObjectID)

    _, ok = hub.LeaderOf("stranger")
    require.False(t, ok)
}

// TestHubGroupSliceIsACopy pins the isolation of the returned member list.
func TestHubGroupSliceIsACopy(t *testing.T) {
    config, err := LoadConfig(writeConfig(t, `{"groups":[{"leader":"test1",
        "members":["test1","test2"]}]}`))
    require.NoError(t, err)
    hub := NewHub(config)

    _, members, ok := hub.GroupOf("test1")
    require.True(t, ok)
    members[0] = "mutated"

    _, fresh, ok := hub.GroupOf("test1")
    require.True(t, ok)
    require.Equal(t, "test1", fresh[0])
}

// TestHubSharesTheCoordination pins the command channel: the leader publishes
// the shared target and the followers read it through their group.
func TestHubSharesTheCoordination(t *testing.T) {
    config, err := LoadConfig(writeConfig(t, `{"groups":[{"leader":"test1",
        "members":["test1","test2","test3"]}]}`))
    require.NoError(t, err)
    hub := NewHub(config)

    now := time.Unix(1_700_000_000, 0)
    hub.PublishCoordination("test1", Coordination{
        Leader: "test1", First: "test2", FirstAt: now,
        TargetID: 500, X: 1000, Y: 2000, Z: -3500, At: now,
    })

    command, ok := hub.CoordinationOf("test3")
    require.True(t, ok)
    require.Equal(t, "test2", command.First)
    require.Equal(t, int32(500), command.TargetID)
    require.Equal(t, now, command.At)

    _, ok = hub.CoordinationOf("stranger")
    require.False(t, ok)
}

// TestHubSlotsAreStable pins the formation index: every member keeps the
// index it holds in the configuration, so the offsets never swap mid walk.
func TestHubSlotsAreStable(t *testing.T) {
    config, err := LoadConfig(writeConfig(t, `{"groups":[{"leader":"test1",
        "members":["test1","test2","test3"]}]}`))
    require.NoError(t, err)
    hub := NewHub(config)

    slot, ok := hub.SlotOf("test1")
    require.True(t, ok)
    require.Zero(t, slot)
    slot, ok = hub.SlotOf("test2")
    require.True(t, ok)
    require.Equal(t, 1, slot)
    slot, ok = hub.SlotOf("test3")
    require.True(t, ok)
    require.Equal(t, 2, slot)
    _, ok = hub.SlotOf("stranger")
    require.False(t, ok)
}
