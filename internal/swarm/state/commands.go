// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

// CommandKind values of the manual web UI commands.
const (
	// CommandMove walks the character to a world point.
	CommandMove = "move"
	// CommandAttack runs to the object and attacks it.
	CommandAttack = "attack"
	// CommandPickup runs to the ground item and picks it up.
	CommandPickup = "pickup"
	// CommandUseItem uses an inventory item: equippable items toggle
	// their equipped state.
	CommandUseItem = "useItem"
	// CommandDrop drops an inventory item on the ground at the feet
	// of the character (the server accepts feet drops only).
	CommandDrop = "drop"
)

// Command is one manual command of the web interface, queued on the bot
// by the web server and consumed by the hunt loop of the session. The
// JSON tags match the POST /api/bots/{id}/commands request body.
type Command struct {
	Kind     string `json:"kind"`
	ObjectID int32  `json:"objectId"`
	Count    int32  `json:"count"`
	X        int32  `json:"x"`
	Y        int32  `json:"y"`
	Z        int32  `json:"z"`
}

// commandQueueCapacity bounds the queued manual commands: the queue is
// a tiny buffer for bursts, a bot that cannot keep up drops the oldest
// entries on overflow.
const commandQueueCapacity = 32

// PushCommand queues one manual command for the hunt loop. The call
// never blocks: when the queue is full the oldest command is dropped to
// make room (the newest manual intent wins).
func (b *Bot) PushCommand(cmd Command) {
	queue := b.commandQueue
	for {
		select {
		case queue <- cmd:
			return
		default:
		}
		// Full: drop the oldest and retry.
		select {
		case <-queue:
		default:
			// Another drainer emptied it, retry the push.
		}
	}
}

// Commands exposes the receive side of the command queue. The hunt
// loop drains it every tick.
func (b *Bot) Commands() <-chan Command {
	return b.commandQueue
}

// drainCommands empties the command queue without consuming (used on
// session resets so a reconnect never replays stale clicks).
func (b *Bot) drainCommands() {
	for {
		select {
		case <-b.commandQueue:
		default:
			return
		}
	}
}
