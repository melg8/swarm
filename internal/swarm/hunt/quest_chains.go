// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The class transfer quest chain data of M2: the executable form of
// the chain tables of docs/quest_protocol.md, every value read from
// the Mobius C1 quest scripts and their datapack pages (the Java
// sources of Q00406_PathOfTheElvenKnight,
// Q00407_PathOfTheElvenScout and ElfHumanFighterChange1, the
// spawn files GludioNPCs.xml / GludinVillageNPCs.xml / Others/
// 18_21.xml / 18_22.xml / 19_20.xml and the quest page htm files).
// The quest_chains_test.go pins the data against the npcdata name
// maps and the script facts.
//
// One naming correction the data round discovered: the mob the
// Q00407 script kills at cond 2 (addKillId 20053) is the Ol Mahum
// Patrol, NOT the Bugbear the research doc claimed - the npc stats
// (20000-20099.xml, npc 20053 name="Ol Mahum Patrol") and the
// spawn comments agree; the Bugbear is npc 20133 and no quest of
// this chain kills it.
//
// The accessors return fresh values (no shared state): a stage is
// a talk stage when its TalkNpc carries a template id and a kill
// stage when its Kill carries mobs (the zero sentinels of the
// townNpc convention).

// QuestNpc is one npc station of a quest chain: the display id of
// the npcdata space (the world tracker stores the NpcInfo template
// id as display id plus 1000000, so a scan looks up
// TemplateID+npcDisplayOffset) and the spawn position of the live
// stack (the trip planner walks to it).
type QuestNpc struct {
	TemplateID int32
	Name       string
	X          int32
	Y          int32
	Z          int32
}

// QuestKill is one farming stage of a quest: kill the mobs until
// the quest items reach the target count. The script drops the
// item straight into the killer's inventory (Quest.giveItems, the
// 2.5 s delayed ON_ATTACKABLE_KILL notification - the item lands
// seconds after the corpse, not on the ground).
type QuestKill struct {
	// Mobs are the npcdata display ids of the mobs whose kills
	// count (the addKillId set of the script, mapped through
	// CT0_to_C4_ids.txt).
	Mobs []int32
	// ItemIDs are the quest item ids the drops accumulate (the
	// classic item ids the inventory packets carry): one id for a
	// piece counter, several for the Q00407 torn letter set (the
	// sequential drops - one new piece per kill, in id order,
	// until all four exist).
	ItemIDs []int32
	// DropPercent is the per kill chance of the script (the
	// getRandom(10) < N gate; 100 for the unconditional drops).
	DropPercent int
	// Target is the total item count that advances the cond.
	Target int32
	// GroundX, GroundY center the spawn hull of the mobs (the
	// computed spawn bounding box center, the trip reference -
	// the hunt zone square generation of the M3 round refines it).
	GroundX int32
	GroundY int32
}

// QuestStage is one cond step of the chain: what the bot does while
// the quest journal reports Cond (the tracker QuestCond of the
// quest id).
type QuestStage struct {
	Cond int32
	// TalkNpc is the npc the stage talks to (the zero QuestNpc of
	// a pure kill stage): walking into the interaction distance
	// and opening the dialog (DriveDialog of the walker).
	TalkNpc QuestNpc
	// Links is the dialog route of the talk: non-empty when the
	// cond advance rides a bypass click of the npc's page (the
	// onEvent setCond branch - Kluto's "Ask about the favor",
	// Moretti's "Say you will begin the search"); empty when the
	// talk itself advances (the onTalk setCond branch - the server
	// moves the cond the moment the page opens).
	Links []DialogStep
	// Kill is the farming block of the stage (the zero QuestKill
	// of a pure talk stage).
	Kill QuestKill
}

// QuestChain is one first profession quest of the elven fighter:
// the journal progress the tracker observes (QuestCond of
// QuestID), the dialog route of the accept and the stage ladder
// from cond 1 to the exit.
type QuestChain struct {
	QuestID  int32
	Script   string
	Title    string
	MinLevel int32
	// ClassID is the starting class the script gates on (18,
	// ELVEN_FIGHTER).
	ClassID int32
	Start   QuestNpc
	// Accept is the dialog route from the start npc's first page
	// to the quest start (the links of the CREATED pages).
	Accept []DialogStep
	// Stages run from cond 1 to the final cond whose talk ends the
	// quest (the reward lands, exitQuest removes the registered
	// quest items).
	Stages []QuestStage
	// ProofItem is the item that survives the quest exit (not a
	// registered quest item) - the class change consumes it.
	ProofItem int32
	RewardXP  int64
	RewardSP  int64
}

// ClassChange is the profession change leg at the grand master: the
// dialog route from the master's static page to the change link.
type ClassChange struct {
	Npc QuestNpc
	// TargetClass is the resulting class id (19 Elven Knight, 22
	// Elven Scout) - the SelfClassID flip the M2 acceptance
	// checks.
	TargetClass int32
	// ProofItem is the quest proof the master takes (all copies).
	ProofItem int32
	MinLevel  int32
	// Route is the dialog walk from the static villagemaster page
	// (the "Listen to information about first class transfer" link
	// opens the script pages) to the change link whose bypass
	// command is `Script ElfHumanFighterChange1 <class id>` (the
	// -h form the walker strips).
	Route []DialogStep
}

// The zero sentinels of the stage halves (the townNpc convention:
// a stage is a talk stage when its TalkNpc carries a template id,
// a kill stage when its Kill carries mobs).
var (
	zeroQuestNpc  = QuestNpc{TemplateID: 0, Name: "", X: 0, Y: 0, Z: 0}
	zeroQuestKill = QuestKill{
		Mobs: nil, ItemIDs: nil, DropPercent: 0,
		Target: 0, GroundX: 0, GroundY: 0,
	}
)

// zeroQuestStage is the not-found sentinel of QuestStageByCond.
var zeroQuestStage = QuestStage{
	Cond: 0, TalkNpc: zeroQuestNpc, Links: nil, Kill: zeroQuestKill,
}

// questTalkStage builds a talk stage: the cond the stage starts
// at, the npc station and the optional dialog route (empty when
// the talk itself advances the cond).
func questTalkStage(
	cond int32, npc QuestNpc, links ...DialogStep,
) QuestStage {
	return QuestStage{Cond: cond, TalkNpc: npc, Links: links, Kill: zeroQuestKill}
}

// questKillStage builds a farming stage: the cond the stage starts
// at and the kill economy.
func questKillStage(cond int32, kill QuestKill) QuestStage {
	return QuestStage{Cond: cond, TalkNpc: zeroQuestNpc, Links: nil, Kill: kill}
}

// The class transfer npc stations of the live stack (the Gludio
// spawn file GludioNPCs.xml; Kluto of the Gludin spawn file).
// The constructors return fresh values - the chains share no state.
func soriusNPC() QuestNpc {
	return QuestNpc{
		TemplateID: 7327, Name: "Sorius", X: -13440, Y: 122643, Z: -3103,
	}
}

func klutoNPC() QuestNpc {
	return QuestNpc{
		TemplateID: 7317, Name: "Kluto", X: -83172, Y: 155483, Z: -3174,
	}
}

func reisaNPC() QuestNpc {
	return QuestNpc{
		TemplateID: 7328, Name: "Reisa", X: -13693, Y: 122583, Z: -3103,
	}
}

func morettiNPC() QuestNpc {
	return QuestNpc{
		TemplateID: 7337, Name: "Moretti", X: -11901, Y: 123798, Z: -3080,
	}
}

func priasNPC() QuestNpc {
	return QuestNpc{
		TemplateID: 7426, Name: "Prias", X: -9076, Y: 72969, Z: -3448,
	}
}

func rainsNPC() QuestNpc {
	return QuestNpc{
		TemplateID: 7288, Name: "Rains", X: -13579, Y: 123017, Z: -3103,
	}
}

// ElvenKnightChain is the Q00406 Path to an Elven Knight ladder:
// the skeleton hunt of the Ruins of Agony and the Ol Mahum camp
// second leg, read from the script.
//
// The registered quest items (removed at the exit): Sorius' Letter
// 1202, Kluto's Box 1203, Topaz Piece 1205, Emerald Piece 1206,
// Kluto's Memo 1276. The brooch 1204 survives.
//
// The mid-chain pages the talk stages answer (documentation of the
// server behavior): cond 1 -> 30327-07 (no pieces) / 30327-08;
// cond 3 -> 30317-01 with the favor link; cond 4 -> 30317-03 /
// 30317-04; cond 6 at Kluto -> 30317-06.
func ElvenKnightChain() QuestChain {
	return QuestChain{
		QuestID:  406,
		Script:   "Q00406_PathOfTheElvenKnight",
		Title:    "Path to an Elven Knight",
		MinLevel: 19,
		ClassID:  18,
		Start:    soriusNPC(),
		Accept: []DialogStep{
			{LinkText: "Say you want to be an Elven Knight"},
			{LinkText: "Challenge the test"},
		},
		Stages: []QuestStage{
			questKillStage(1, QuestKill{
				Mobs: []int32{35, 42, 45, 51, 54, 60},
				// Tracker Skeleton 17, Tracker Skeleton
				// Leader 18, Skeleton Scout 19, Skeleton
				// Bowman 20, Ruin Spartoi 21, Raging
				// Spartoi 22 - the Ruins of Agony west of
				// Gludio (spawns Others/18_21.xml, 208
				// entries).
				ItemIDs:     []int32{1205},
				DropPercent: 70,
				Target:      20,
				GroundX:     -49896,
				GroundY:     113960,
			}),
			// The talk itself advances: page 30327-09,
			// setCond(3), Sorius' Letter 1202.
			questTalkStage(2, soriusNPC()),
			// The link's event 30317-02.htm: setCond(4),
			// the letter taken, Kluto's Memo 1276 given.
			questTalkStage(3, klutoNPC(), DialogStep{
				LinkText: "Ask about the favor",
			}),
			questKillStage(4, QuestKill{
				Mobs: []int32{782},
				// Ol Mahum Novice 17 - the camps north of
				// Gludin (spawns Others/18_22.xml, 23
				// entries of the two quest species).
				ItemIDs:     []int32{1206},
				DropPercent: 50,
				Target:      20,
				GroundX:     -47398,
				GroundY:     145943,
			}),
			// The talk itself advances: page 30317-05,
			// setCond(6), all pieces taken, Kluto's Box
			// 1203 given.
			questTalkStage(5, klutoNPC()),
			// The closing talk: page 30327-10, the box and
			// the memo taken, the brooch 1204 given, 3200
			// xp + 2280 sp, exitQuest(true, true).
			questTalkStage(6, soriusNPC()),
		},
		ProofItem: 1204,
		RewardXP:  3200,
		RewardSP:  2280,
	}
}

// ElvenScoutChain is the Q00407 Path to an Elven Scout ladder: the
// Ol Mahum Patrol torn letter hunt, the Prias rescue south of the
// Neutral Zone and the return legs, read from the script.
//
// The registered quest items: Reisa's Letter 1207, the four torn
// letter pieces 1208-1211, Moretti's Herb 1212, Moretti's Letter
// 1214, Prias' Letter 1215, the Honorary Guard token 1216, the
// Rusted Key 1293. The recommendation 1217 survives.
//
// Guard Babenco (30334, the Gludio west entrance) holds the advice
// page 30334-01 at cond 2 (the directions to the campground) - the
// optional guide, not a chain station.
func ElvenScoutChain() QuestChain {
	return QuestChain{
		QuestID:  407,
		Script:   "Q00407_PathOfTheElvenScout",
		Title:    "Path to an Elven Scout",
		MinLevel: 19,
		ClassID:  18,
		Start:    reisaNPC(),
		Accept: []DialogStep{
			// The 30328-05 event does the whole start: startQuest
			// plus Reisa's Letter 1207 (unlike Q00406, whose
			// accept link chain needs two clicks).
			{LinkText: "Say you will accept the task"},
		},
		Stages: []QuestStage{
			// The second link's event 30337-03.htm:
			// setCond(2), the letter taken.
			questTalkStage(1, morettiNPC(),
				DialogStep{LinkText: "Listen to details"},
				DialogStep{LinkText: "Say you will begin the search"}),
			questKillStage(2, QuestKill{
				Mobs: []int32{53},
				// Ol Mahum Patrol 21 - the abandoned camp
				// between Gludio and Gludin (spawns
				// Others/18_22.xml around -49868 149683).
				// The drops are the four torn letter
				// pieces 1208-1211, one per kill in id
				// order (no chance gate, the first
				// missing piece drops): DropPercent 100.
				ItemIDs:     []int32{1208, 1209, 1210, 1211},
				DropPercent: 100,
				Target:      4,
				GroundX:     -47398,
				GroundY:     145943,
			}),
			// The talk itself advances: page 30337-06,
			// setCond(4), the pieces taken, the herb 1212
			// and Moretti's Letter 1214 given.
			questTalkStage(3, morettiNPC()),
			// The talk itself advances: page 30426-01,
			// setCond(5).
			questTalkStage(4, priasNPC()),
			questKillStage(5, QuestKill{
				Mobs: []int32{5031},
				// Ol Mahum Sentry 17 - the two guards of
				// Prias (spawns Others/19_20.xml at -8700
				// 72362, respawn 180 s).
				ItemIDs:     []int32{1293},
				DropPercent: 60,
				Target:      1,
				GroundX:     -8628,
				GroundY:     72347,
			}),
			// The talk itself advances: page 30426-02,
			// setCond(7), the key, the herb and the letter
			// taken, Prias' Letter 1215 given.
			questTalkStage(6, priasNPC()),
			// The talk itself advances: page 30337-07,
			// setCond(8), Prias' letter taken, the
			// Honorary Guard token 1216 given.
			questTalkStage(7, morettiNPC()),
			// The closing talk: page 30328-07, the token
			// taken, the recommendation 1217 given, 3200
			// xp + 1000 sp, exitQuest(true, true).
			questTalkStage(8, reisaNPC()),
		},
		ProofItem: 1217,
		RewardXP:  3200,
		RewardSP:  1000,
	}
}

// ElvenKnightClassChange is the Elven Fighter -> Elven Knight leg
// at Grand Master Rains (the class id 19, the Elven Knight Brooch
// proof of Q00406).
func ElvenKnightClassChange() ClassChange {
	return ClassChange{
		Npc:         rainsNPC(),
		TargetClass: 19,
		ProofItem:   1204,
		MinLevel:    20,
		Route: []DialogStep{
			// The static villagemaster page 30288.htm link
			// opens the script pages; 30288-11 offers the two
			// professions; 30288-12 carries the change link
			// (`bypass -h Script ElfHumanFighterChange1 19`);
			// the event takes the brooch, flips the class and
			// answers with 30288-35.htm.
			{LinkText: "Listen to information about first class transfer"},
			{LinkText: "Elven Knight"},
			{LinkText: "Change profession to an Elven Knight"},
		},
	}
}

// ElvenScoutClassChange is the Elven Fighter -> Elven Scout leg at
// Grand Master Rains (the class id 22, the Reisa's Recommendation
// proof of Q00407).
func ElvenScoutClassChange() ClassChange {
	return ClassChange{
		Npc:         rainsNPC(),
		TargetClass: 22,
		ProofItem:   1217,
		MinLevel:    20,
		Route: []DialogStep{
			// The 30288-15 page carries the scout change link
			// (`bypass -h Script ElfHumanFighterChange1 22`).
			{LinkText: "Listen to information about first class transfer"},
			{LinkText: "Elven Scout"},
			{LinkText: "Change profession to an Elven Scout"},
		},
	}
}

// QuestEntryLinks is the dialog prefix every quest npc conversation
// of this chain rides through: the first talk click of an npc
// without an ON_NPC_FIRST_TALK listener opens the npc's STATIC
// page (the trainer html of data/html/trainer/<npcid>.htm - live
// verified 2026-09-12: Sorius answers with his trainer page, not
// the quest page), whose "Quest" link (the bare `bypass Script`
// command) runs ScriptLink.showQuestWindow: a station of exactly
// one quest (every station of both chains) resolves the single
// option straight through Quest.notifyTalk to the quest page of
// the current cond (the live run of 2026-09-12: the "Quest" click
// at Sorius answers with 30327-01 directly - the choose window
// link list only appears for npcs offering several quests).
func QuestEntryLinks(_ QuestChain) []DialogStep {
	return []DialogStep{
		{LinkText: "Quest"},
	}
}

// QuestStageByCond returns the stage of the chain at a cond value
// (the quest journal the tracker reports): the stage the bot acts
// on next. False when the cond sits outside the ladder (the quest
// ended or never started).
func QuestStageByCond(chain QuestChain, cond int32) (QuestStage, bool) {
	for i := range chain.Stages {
		if chain.Stages[i].Cond == cond {
			return chain.Stages[i], true
		}
	}

	return zeroQuestStage, false
}
