// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package session owns the persistent session journal of the bot: an
// append-only JSONL file in logs/ that records everything that happened
// from the application start (the event story of the tracker, periodic
// samples of the character state and the structured hunt events), and
// the compact agent-facing report rendered from it.
//
// The problem it solves: the state dump is a point-in-time snapshot and
// the tracker event log is a 512-entry ring, so a long-lived run on the
// user machine (8-24 hours) left no trace to analyze after the fact.
// The journal keeps the full story on disk; the web UI "session dump"
// button and the -session-report CLI render the compact report an
// agent reads to answer why the bot farmed little money, how many
// stalls it hit and what led to ineffective fights.
package session

import "time"

// Event kinds of the journal wire format (the "e" field of a record).
const (
	// kindBuild is the process identity line (the first record).
	kindBuild = "build"
	// kindStory is a mirrored tracker event line (the hunt decisions
	// and the game events: the story of the session in text form).
	kindStory = "story"
	// kindSample is the periodic character state sample (30 s).
	kindSample = "s"
	// kindKill is one killed mob with the fight duration and the
	// health the character ended the fight with.
	kindKill = "kill"
	// kindDeath is one character death.
	kindDeath = "death"
	// kindLevel is a level up mark.
	kindLevel = "level"
	// kindTripStart and kindTripEnd bracket a town trip.
	kindTripStart = "trip-start"
	kindTripEnd   = "trip-end"
	// kindBuy is a confirmed buy request batch with its cost.
	kindBuy = "buy"
	// kindSell is a sell batch offered to a merchant.
	kindSell = "sell"
	// kindZone is a hunting zone or spot switch.
	kindZone = "zone"
	// kindStall is a stagnation watch event (xp or position hold).
	kindStall = "stall"
	// kindRepath is a stuck-and-replanned walk leg.
	kindRepath = "repath"
	// kindConnect, kindLost and kindShutdown are the session
	// lifecycle marks of the supervisor.
	kindConnect  = "connect"
	kindLost     = "lost"
	kindShutdown = "shutdown"
)

// record is one JSONL line of the journal: a flat struct with short
// keys and omitempty fields so a kill line reads
// {"t":1730000000,"b":"test1","e":"kill","mob":"Kaboo Orc","lvl":4,
// "dur":8.2,"hp":87} - roughly 80 bytes. The flat shape keeps the
// offline reader a single json.Unmarshal per line, no kind switch.
//
// The fields overlap between kinds by design (dur is the fight seconds
// of a kill and the stall seconds of a stall); the aggregator reads the
// subset its kind defines.
type record struct {
	// T is the unix second of the event.
	T int64 `json:"t"`
	// B is the bot id (the account name; one journal file of a fleet
	// process carries every bot).
	B string `json:"b"`
	// E is the event kind.
	E string `json:"e"`
	// M is the story text of a story record.
	M string `json:"m,omitempty"`
	// Lv is the character level (sample, level, death).
	Lv int32 `json:"lv,omitempty"`
	// Xp is the cumulative experience of the sample (level table +
	// in-level exp, see state.CumulativeExp).
	Xp int64 `json:"xp,omitempty"`
	// Ad is the adena total of the sample.
	Ad int64 `json:"ad,omitempty"`
	// Hp is the character health percent (0-100).
	Hp float64 `json:"hp,omitempty"`
	// X and Y are the character position.
	X int32 `json:"x,omitempty"`
	Y int32 `json:"y,omitempty"`
	// Ph is the hunt phase name of the sample.
	Ph string `json:"ph,omitempty"`
	// Mob is the killed mob name.
	Mob string `json:"mob,omitempty"`
	// Lvl is the level of the killed mob.
	Lvl int32 `json:"lvl,omitempty"`
	// Dur is the fight seconds of a kill, the stall seconds of a
	// stall and the trip seconds of a trip end.
	Dur float64 `json:"dur,omitempty"`
	// R is the reason of a zone switch, trip or connect line.
	R string `json:"r,omitempty"`
	// N is the count of a sell batch or a repath attempt number.
	N int32 `json:"n,omitempty"`
	// Cost is the adena cost of a buy batch.
	Cost int64 `json:"cost,omitempty"`
	// Items is the itemized list of a buy batch.
	Items string `json:"items,omitempty"`
	// V is the build identity of the process.
	V string `json:"v,omitempty"`
}

// newRecord builds a zeroed record with the identity fields set. The
// callers then fill the fields their kind carries; the full literal
// here keeps the exhaustruct gate honest about the wire shape.
func newRecord(bot string, kind string, at time.Time) record {
	return record{
		T:     at.Unix(),
		B:     bot,
		E:     kind,
		M:     "",
		Lv:    0,
		Xp:    0,
		Ad:    0,
		Hp:    0,
		X:     0,
		Y:     0,
		Ph:    "",
		Mob:   "",
		Lvl:   0,
		Dur:   0,
		R:     "",
		N:     0,
		Cost:  0,
		Items: "",
		V:     "",
	}
}

// Sample is the periodic character state read the sampler publishes.
// The journal turns it into a kindSample record.
type Sample struct {
	Level  int32
	Exp    int64
	Adena  int64
	Health float64
	X      int32
	Y      int32
	Phase  string
}

// LiveView is the point-in-time state of one bot the live report
// header carries (the web server builds it from the tracker; the
// offline CLI passes nil).
type LiveView struct {
	ID          string
	Status      string
	Phase       string
	Level       int32
	ExpPercent  float64
	Health      float64
	Adena       int64
	X           int32
	Y           int32
	StartedUnix int64
}
