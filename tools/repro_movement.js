#!/usr/bin/env node
/*
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
*/

// Movement position harness for the web map.
//
// It loads the real internal/swarm/webserver/web/map.js (unchanged) into
// a sandboxed context with a virtual clock and answers the question "how
// exactly do the drawn character and mob positions follow the server".
//
// Three modes:
//
//   node tools/repro_movement.js [--frames] [--verbose]
//       Simulation mode. A faithful simulation of the Mobius C1 server
//       movement semantics (100 ms game ticks, collision radius stop
//       rule, 1 s broadcast throttle, MoveToPawn chase re-issue) drives
//       the map and every frame is compared against the simulated server
//       truth. Metrics per move: the maximum position error (drawn vs
//       server, world units), the error at the arrival delivery, the
//       largest frame speed spike (frame speed as a multiple of the unit
//       speed) and the largest backwards step of the drawn marker along
//       its walk line (the visible jump back). One scenario re-issues
//       the mob MoveToPawn position samples with a stale moveAt, the
//       state bug shape the map must absorb. --frames prints the frame
//       log of every tenth frame. Exit 1 when a threshold breaks.
//
//   node tools/repro_movement.js --record <seconds> [outfile] [url] [bot]
//       Records the live SSE snapshot stream of a running bot to a JSON
//       file for offline analysis.
//
//   node tools/repro_movement.js --replay <file> [--frames]
//       Replays a recording through the real map at 60 fps virtual time.
//       Logs the drawn position of every moving unit frame by frame
//       (--frames prints every frame, the default logs spikes) and
//       reports speed spikes, DRAWN-BACK steps (the marker moving
//       against its walk line) and TARGET-BACK re-anchors (a late
//       position sample putting the projected target behind the drawn
//       marker, the server side lag the state tracker must prevent).
//       Exit 1 on spikes > 2.5x, a drawn step back or a target
//       re-anchor behind by more than 50 u.
//
// Mobius semantics reproduced (verified in the server sources):
// - game ticks are 100 ms; a creature advances speed * ticks per tick
//   and the speed is re-read every tick (buffs apply immediately);
// - it counts as arrived once one tick step covers the remaining
//   distance minus the collision radius, snaps to the exact destination
//   and broadcasts a zero distance MoveToLocation (forced);
// - non forced MoveToLocation broadcasts are throttled to one per
//   second, so a re-issued move inside the window stays invisible;
// - chasing creatures re-issue their move about once per second.
//
// Exit code 0 = movement rendering is correct, 1 = bug reproduced.

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const MAP_JS = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web", "map.js");

// Harness parameters (calibrated from the Mobius sources and the C1 npc
// stats: collision radii 5-15, chase re-issue ~1 s).
const HARNESS = {
    parseDelayMs: 1,       // game server -> bot packet processing
    transportMs: 4,        // bot SSE write -> browser event delivery
    pollPeriodMs: 300,     // webserver event stream poll period
    frameMs: 1000 / 60,    // requestAnimationFrame cadence
    idleAfterArrivalMs: 400,
    movesPerScenario: 6,
    // The drawn position may deviate from the server truth by up to one
    // game tick of movement (the stepwise server position vs the linear
    // inter tick rendering) plus one tick of phase slack.
    maxErrFactor: 0.25,
    maxErrAbsolute: 12,
    maxSpikeRatio: 2.5,
    // The drawn marker must never travel backwards along its walk
    // segment by more than sub pixel noise.
    maxBackUnits: 0.5,
    // A re-anchor of the same destination that puts the projected
    // target farther back than this means the position sample lagged
    // the movement by a large fraction of a second (the state bug).
    maxTargetBackUnits: 50,
    // The map renders a discontinuity as a teleport (map.js).
    teleportUnits: 400
};

// deterministic pseudo random numbers so runs are comparable
function makeRandom(seed) {
    let state = seed >>> 0;
    return function next() {
        state = (state * 1664525 + 1013904223) >>> 0;
        return state / 0x100000000;
    };
}

// element stubs good enough for the parts of map.js we drive
function makeElementStub() {
    return {
        classList: { contains: () => true },
        addEventListener: () => {},
        style: {},
        getBoundingClientRect: () => ({
            width: 800, height: 600, left: 0, top: 0
        })
    };
}

// loadMapJs creates a fresh sandboxed context with a virtual clock and
// returns { MapView, clock, rafQueue } for driving.
function loadMapJs() {
    const rafQueue = [];
    const clock = { nowMs: 0 };
    const sandbox = {
        Math,
        JSON,
        Date: { now: () => clock.nowMs },
        performance: { now: () => clock.nowMs },
        requestAnimationFrame: (cb) => {
            rafQueue.push({ cb, dueAt: clock.nowMs + HARNESS.frameMs });
            return rafQueue.length;
        },
        document: { getElementById: () => makeElementStub() },
        window: { addEventListener: () => {} },
        getComputedStyle: () => ({ getPropertyValue: () => "" }),
        setTimeout: () => 0,
        clearTimeout: () => {}
    };
    vm.createContext(sandbox);
    vm.runInContext(fs.readFileSync(MAP_JS, "utf8"), sandbox,
        { filename: "map.js" });
    vm.runInContext("globalThis.__MapView = MapView;", sandbox);

    return { MapView: sandbox.__MapView, clock, rafQueue };
}

// MobiusSim reproduces the server side movement of one creature,
// including the broadcast throttle: a non forced MoveToLocation is
// suppressed within one second of the previous broadcast, exactly like
// Creature.broadcastMoveToLocation does.
class MobiusSim {
    constructor(opts) {
        this.v = opts.speed;
        this.collision = opts.collision;
        this.tickMs = 100; // game ticks are always 100 ms
        this.x = opts.startX;
        this.y = opts.startY;
        this.destX = opts.startX;
        this.destY = opts.startY;
        this.moving = false;
        this.lastTickIndex = 0;
        this.lastBroadcastMs = -1e9;
    }

    // startMove re-points the AI at a destination from the current
    // position. The returned packet is null when the 1 s broadcast
    // throttle swallows it (the server silently changes course).
    startMove(destX, destY, nowMs) {
        this.destX = destX;
        this.destY = destY;
        if (!this.moving) {
            this.lastTickIndex = Math.floor(nowMs / this.tickMs);
        }
        this.moving = true;
        if (nowMs - this.lastBroadcastMs < 1000) {
            return null;
        }
        this.lastBroadcastMs = nowMs;

        return { type: "MoveToLocation", x: this.x, y: this.y, destX, destY };
    }

    // tick advances the server position exactly like
    // Creature.updatePosition: one game tick worth of distance per tick,
    // arrival once the step covers the remaining distance minus the
    // collision radius, then a snap to the exact destination and a
    // forced zero distance broadcast.
    tick(nowMs) {
        if (!this.moving) {
            return [];
        }
        const tickIndex = Math.floor(nowMs / this.tickMs);
        if (tickIndex <= this.lastTickIndex) {
            return [];
        }
        this.lastTickIndex = tickIndex;
        const packets = [];
        const dx = this.destX - this.x;
        const dy = this.destY - this.y;
        const dist = Math.hypot(dx, dy);
        const delta = Math.max(0.00001, dist - this.collision);
        const step = this.v / 10;
        if (delta <= 1 || step > delta) {
            this.x = this.destX;
            this.y = this.destY;
            this.moving = false;
            this.lastBroadcastMs = nowMs;
            packets.push({
                type: "MoveToLocation", x: this.x, y: this.y,
                destX: this.destX, destY: this.destY, arrival: true
            });
        } else {
            const frac = step / delta;
            this.x += dx * frac;
            this.y += dy * frac;
        }

        return packets;
    }

    // chaseMove starts a chase segment with the moveToPawn offset
    // semantics: the stop point sits `offset` units short of the target.
    chaseMove(targetX, targetY, offset, nowMs) {
        const dx = this.x - targetX;
        const dy = this.y - targetY;
        const dist = Math.hypot(dx, dy);
        const stop = Math.max(5, dist - offset + 5);
        const destX = this.x - dx / dist * stop;
        const destY = this.y - dy / dist * stop;
        const packet = this.startMove(destX, destY, nowMs);
        if (packet) {
            packet.type = "MoveToPawn";
            packet.targetX = targetX;
            packet.targetY = targetY;
            packet.distance = offset;
        }

        return packet;
    }

    // stopChase ends a chase in range like Creature.stopMove does.
    stopChase() {
        this.x = this.destX;
        this.y = this.destY;
        this.moving = false;

        return { type: "StopMove", x: this.x, y: this.y };
    }
}

// BotState fakes the Go state tracker for one observed npc object.
class BotState {
    constructor(objectId, templateId, collision) {
        this.obj = {
            objectId, templateId, kind: "npc", name: "sim", dead: false,
            moving: false, running: true, speed: 0,
            collisionRadius: collision,
            x: 0, y: 0, z: 0, heading: 0,
            destX: 0, destY: 0, destZ: 0, moveAtMs: 0
        };
    }

    apply(packet, nowMs, speed) {
        const o = this.obj;
        o.speed = speed;
        o.moveAtMs = nowMs;
        if (packet.type === "MoveToLocation") {
            o.x = packet.x;
            o.y = packet.y;
            o.destX = packet.destX;
            o.destY = packet.destY;
            o.moving = Math.abs(packet.destX - packet.x) > 1
                || Math.abs(packet.destY - packet.y) > 1;
        } else if (packet.type === "StopMove") {
            o.x = packet.x;
            o.y = packet.y;
            o.destX = packet.x;
            o.destY = packet.y;
            o.moving = false;
        } else if (packet.type === "MoveToPawn") {
            const dx = packet.x - packet.targetX;
            const dy = packet.y - packet.targetY;
            const dist = Math.hypot(dx, dy);
            o.x = packet.x;
            o.y = packet.y;
            o.destX = packet.targetX + dx / dist * packet.distance;
            o.destY = packet.targetY + dy / dist * packet.distance;
            o.moving = true;
        }
    }
}

// EventDriver runs the virtual clock: server ticks, SSE polls, packet
// delivery and requestAnimationFrame frames in time order.
class EventDriver {
    constructor(setClock) {
        this.events = [];
        this.time = 0;
        this.setClock = setClock;
    }

    at(whenMs, fn) {
        this.events.push({ at: whenMs, fn });
        this.events.sort((a, b) => a.at - b.at);
    }

    nextTime() {
        return this.events.length > 0 ? this.events[0].at : Infinity;
    }

    // run advances until untilMs. After every event the frame pump runs
    // with a bound that keeps the virtual clock monotonic.
    run(untilMs, pumpFrames) {
        let guard = 0;
        while (this.time < untilMs && this.events.length > 0 && guard < 1e6) {
            guard++;
            const next = this.events.shift();
            if (next.at > untilMs) {
                this.events.unshift(next);
                break;
            }
            this.time = Math.max(this.time, next.at);
            this.setClock(this.time);
            next.fn(this.time);
            const bound = Math.min(this.nextTime() === Infinity
                ? untilMs : this.nextTime(), untilMs);
            pumpFrames(bound);
        }
        pumpFrames(untilMs);
        this.time = Math.max(this.time, untilMs);
        this.setClock(this.time);
    }
}

// Metrics collects the frame by frame comparison of the drawn position
// against the simulated server truth. The spike ratio compares the frame
// displacement with the unit speed over the ACTUAL frame interval, so
// sparse frame execution does not fake spikes. The back metric projects
// the frame displacement on the forward axis of the current movement
// segment: a negative projection is the marker stepping backwards along
// the walk line, the artifact the player sees as a jump back.
class Metrics {
    constructor() {
        this.maxErr = 0;
        this.maxSpike = 0;
        this.maxBack = 0;
        this.backFrames = 0;
        this.errAtArrival = null;
        this.last = null;
        this.lastTime = 0;
        this.lastSig = null;
        this.frames = 0;
    }

    observe(drawn, truth, speed, timeMs, atArrival, segment) {
        if (!drawn) {
            return;
        }
        const err = Math.hypot(drawn.x - truth.x, drawn.y - truth.y);
        if (err > this.maxErr) {
            this.maxErr = err;
        }
        const sig = segment ? segment.sig : null;
        if (this.last && timeMs > this.lastTime && sig !== null
            && sig === this.lastSig) {
            const stepX = drawn.x - this.last.x;
            const stepY = drawn.y - this.last.y;
            const delta = Math.hypot(stepX, stepY);
            const dtS = (timeMs - this.lastTime) / 1000;
            const frameSpeed = speed * dtS;
            if (frameSpeed > 0.01) {
                this.maxSpike = Math.max(this.maxSpike, delta / frameSpeed);
            }
            const len = Math.hypot(segment.dx, segment.dy);
            if (len > 1) {
                const rel = ((this.last.x - segment.ax) * segment.dx
                    + (this.last.y - segment.ay) * segment.dy) / len;
                if (rel >= 0 && rel <= len) {
                    const along = (stepX * segment.dx + stepY * segment.dy)
                        / len;
                    if (along < -1e-6) {
                        this.backFrames++;
                        this.maxBack = Math.max(this.maxBack, -along);
                    }
                }
            }
        }
        this.last = { x: drawn.x, y: drawn.y };
        this.lastTime = timeMs;
        this.lastSig = sig;
        this.frames++;
        if (atArrival) {
            this.errAtArrival = err;
        }
    }
}

// Scenario drives one movement pattern against a loaded MapView.
class Scenario {
    constructor(name, opts) {
        this.name = name;
        this.opts = opts;
        this.isSelf = opts.kind === "player";
        this.verbose = process.argv.includes("--verbose");
        this.frameLog = process.argv.includes("--frames");
        this.metrics = new Metrics();
        this.logCounter = 0;
    }

    run() {
        const loaded = loadMapJs();
        this.MapView = loaded.MapView;
        this.clock = loaded.clock;
        this.rafQueue = loaded.rafQueue;

        this.sim = new MobiusSim(this.opts);
        this.state = new BotState(this.opts.objectId,
            this.opts.templateId, this.opts.collision);
        this.random = makeRandom(this.opts.seed || 7);
        this.driver = new EventDriver((t) => { this.clock.nowMs = t; });

        // played character snapshot fields (fed by applySelfPacket)
        this.selfX = this.opts.startX;
        this.selfY = this.opts.startY;
        this.selfMoving = false;
        this.selfDestX = this.opts.startX;
        this.selfDestY = this.opts.startY;
        this.selfMoveAtMs = 0;
        this.selfSpeed = this.opts.speed;
        this.arrivalFlag = false;

        const snapshotObjects = () => (this.isSelf ? [] : [this.state.obj]);
        const characterSnapshot = () => ({
            objectId: 100, name: "self",
            x: this.selfX, y: this.selfY, z: 0, heading: 0,
            moving: this.selfMoving, speed: this.selfMoving
                ? this.selfSpeed : 0,
            collisionRadius: this.opts.collision,
            destX: this.selfDestX, destY: this.selfDestY,
            moveAtMs: this.selfMoveAtMs
        });

        // deliverSnapshot models the SSE poll: the snapshot is built at
        // the poll time and reaches the browser transportMs later. The
        // objects are copied like the Go Snapshot does - a live tracker
        // reference would mutate inside the stored snapshot and feed the
        // map state that was never sent.
        this.deliverSnapshot = (at) => {
            this.driver.at(at, (t) => {
                const snap = {
                    serverTimeMs: t,
                    character: characterSnapshot(),
                    objects: snapshotObjects().map((o) => ({ ...o }))
                };
                this.driver.at(t + HARNESS.transportMs, () => {
                    this.MapView.update(snap);
                });
            });
        };

        this.applyPacket = (packet, at) => {
            this.driver.at(at, (t) => {
                if (this.isSelf) {
                    this.applySelfPacket(packet, t);
                } else {
                    this.state.apply(packet, t, this.opts.speed);
                }
                if (packet.arrival) {
                    this.arrivalFlag = true;
                }
            });
        };

        this.scheduleTicks = (fromMs, toMs) => {
            let tickAt = Math.floor(fromMs / 100) * 100 + 100;
            while (tickAt <= toMs) {
                const at = tickAt;
                this.driver.at(at, (time) => {
                    for (const packet of this.sim.tick(time)) {
                        this.applyPacket(packet,
                            time + HARNESS.parseDelayMs);
                    }
                });
                tickAt += 100;
            }
        };

        // seed: a standing snapshot so the runtime entry exists
        this.state.obj.x = this.sim.x;
        this.state.obj.y = this.sim.y;
        this.state.obj.speed = this.opts.speed;
        this.deliverSnapshot(0);
        this.driver.run(10, (bound) => this.pumpFrames(bound));

        this.runPattern();
        this.settle(HARNESS.idleAfterArrivalMs);

        return this.evaluate();
    }

    // pumpFrames executes queued rAF callbacks and measures every frame
    // against the server truth.
    pumpFrames(bound) {
        const queue = this.rafQueue;
        let guard = 0;
        while (queue.length > 0 && guard < 5000) {
            guard++;
            const due = queue[0].dueAt;
            if (due > bound) {
                break;
            }
            this.clock.nowMs = Math.max(this.clock.nowMs, due);
            const batch = queue.splice(0, queue.length)
                .filter((f) => f.dueAt <= bound);
            if (batch.length === 0) {
                break;
            }
            const before = this.drawnPos();
            for (const frame of batch) {
                frame.cb(this.clock.nowMs);
            }
            const after = this.drawnPos();
            if (after && this.movingNow()) {
                const atArrival = this.arrivalFlag;
                this.arrivalFlag = false;
                this.metrics.observe(after, this.truthPos(), this.opts.speed,
                    this.clock.nowMs, atArrival, this.segment());
                this.logCounter++;
                if (this.frameLog && this.logCounter % 10 === 0) {
                    const step = Math.hypot(after.x - (before
                        ? before.x : after.x), after.y - (before
                        ? before.y : after.y));
                    const key = this.isSelf ? "self" : this.opts.objectId;
                    const rt = this.MapView.runtime.get(key);
                    const seg = rt && rt.seg ? " segLen=" + rt.seg.len.toFixed(0)
                        + " durMs=" + rt.seg.durMs.toFixed(0)
                        + " startMs=" + Math.round(rt.seg.startMs) : " noSeg";
                    const view = this.isSelf
                        ? this.MapView.lastSnap.character
                        : this.MapView.lastSnap.objects.find(
                            (o) => o.objectId === this.opts.objectId);
                    const vinfo = view ? " viewX=" + Math.round(view.x)
                        + "," + Math.round(view.y)
                        + " viewDest=" + Math.round(view.destX) + ","
                        + Math.round(view.destY)
                        + " moveAt=" + view.moveAtMs
                        + " mv=" + view.moving
                        + " vSpd=" + view.speed
                        + " vCol=" + view.collisionRadius : " noView";
                    console.log("    t=" + Math.round(this.clock.nowMs)
                        + " drawn=" + Math.round(after.x) + ","
                        + Math.round(after.y)
                        + " truth=" + Math.round(this.sim.x) + ","
                        + Math.round(this.sim.y)
                        + " err=" + Math.hypot(after.x - this.sim.x,
                            after.y - this.sim.y).toFixed(1)
                        + " frameU=" + step.toFixed(2)
                        + seg + vinfo);
                }
            }
        }
    }

    drawnPos() {
        const key = this.isSelf ? "self" : this.opts.objectId;
        const rt = this.MapView.runtime.get(key);

        return rt ? { x: rt.drawX, y: rt.drawY } : null;
    }

    truthPos() {
        return { x: this.sim.x, y: this.sim.y };
    }

    movingNow() {
        return this.isSelf ? this.selfMoving : this.state.obj.moving;
    }

    // segment returns the forward axis of the movement segment the
    // map is rendering right now, plus a signature that changes when a
    // new movement packet re-anchors it. The back metric only compares
    // frames of one signature, so a legitimate turn to a new
    // destination is not counted as a backwards step.
    segment() {
        const v = this.isSelf ? {
            x: this.selfX, y: this.selfY,
            destX: this.selfDestX, destY: this.selfDestY,
            moveAtMs: this.selfMoveAtMs, moving: this.selfMoving,
            speed: this.selfSpeed
        } : this.state.obj;
        if (!v.moving || !(v.speed > 0)) {
            return null;
        }

        return {
            ax: v.x, ay: v.y,
            dx: v.destX - v.x, dy: v.destY - v.y,
            sig: v.x + "," + v.y + "," + v.destX + "," + v.destY + ","
                + v.moveAtMs
        };
    }

    applySelfPacket(packet, nowMs) {
        this.selfSpeed = this.opts.speed;
        if (packet.type === "PositionOnly") {
            // The state bug shape: the anchor moves but the movement
            // state does not, so the projection overruns the server.
            this.selfX = packet.x;
            this.selfY = packet.y;

            return;
        }
        if (packet.type === "MoveToPawn") {
            const dx = packet.x - packet.targetX;
            const dy = packet.y - packet.targetY;
            const dist = Math.hypot(dx, dy);
            this.selfX = packet.x;
            this.selfY = packet.y;
            this.selfDestX = packet.targetX + dx / dist * packet.distance;
            this.selfDestY = packet.targetY + dy / dist * packet.distance;
            this.selfMoving = true;
            this.selfMoveAtMs = nowMs;
        } else if (packet.type === "MoveToLocation") {
            this.selfX = packet.x;
            this.selfY = packet.y;
            this.selfDestX = packet.destX;
            this.selfDestY = packet.destY;
            this.selfMoving = Math.abs(packet.destX - packet.x) > 1
                || Math.abs(packet.destY - packet.y) > 1;
            this.selfMoveAtMs = nowMs;
        } else if (packet.type === "StopMove") {
            this.selfX = packet.x;
            this.selfY = packet.y;
            this.selfDestX = packet.x;
            this.selfDestY = packet.y;
            this.selfMoving = false;
            this.selfMoveAtMs = nowMs;
        }
    }

    // walkTo starts one move and schedules its ticks until arrival.
    walkTo(destX, destY) {
        const packet = this.sim.startMove(destX, destY, this.driver.time + 1);
        if (packet) {
            this.applyPacket(packet, this.driver.time + 1
                + HARNESS.parseDelayMs);
        }
        this.deliverSnapshot(this.driver.time + 2);
        const budget = this.driver.time
            + (Math.hypot(destX - this.sim.x, destY - this.sim.y)
                / this.sim.v) * 1000 + 2000;
        this.scheduleTicks(this.driver.time + 1, budget);

        return budget;
    }

    settle(ms) {
        this.driver.at(this.driver.time + ms, () => {});
        this.driver.run(this.driver.time + ms,
            (bound) => this.pumpFrames(bound));
    }

    // runPattern runs the scenario specific movement pattern.
    runPattern() {
        if (this.opts.chase) {
            this.runChase();

            return;
        }
        if (this.opts.pickupHops) {
            this.runPickupHops();

            return;
        }
        if (this.opts.retarget) {
            this.runRetarget();

            return;
        }
        for (let i = 0; i < HARNESS.movesPerScenario; i++) {
            const angle = this.random() * Math.PI * 2;
            const dist = this.opts.minDist + this.random()
                * (this.opts.maxDist - this.opts.minDist);
            const budget = this.walkTo(this.sim.x + Math.cos(angle) * dist,
                this.sim.y + Math.sin(angle) * dist);
            this.driver.run(budget, (bound) => this.pumpFrames(bound));
            this.settle(HARNESS.idleAfterArrivalMs);
        }
    }

    // runChase simulates the played character chasing a target with
    // MoveToPawn re-broadcasts every second and a final StopMove.
    runChase() {
        const targetX = this.opts.startX + this.opts.chaseDistance;
        const targetY = this.opts.startY;
        const offset = this.opts.pawnOffset || 40;
        let guard = 0;
        while (guard < 60) {
            guard++;
            const packet = this.sim.chaseMove(targetX, targetY, offset,
                this.driver.time + 1);
            if (packet) {
                this.applyPacket(packet, this.driver.time + 1
                    + HARNESS.parseDelayMs);
            }
            this.deliverSnapshot(this.driver.time + 2);
            this.scheduleTicks(this.driver.time + 1, this.driver.time + 1000);
            if (this.opts.laggy) {
                // A mob chasing the player broadcasts MoveToPawn with
                // the player position sampled mid tick once per second.
                // The sample carries no movement state of the player.
                this.driver.at(this.driver.time + 400, (t) => {
                    this.applyLaggyPosition(t);
                });
            }
            this.driver.run(this.driver.time + 1000,
                (bound) => this.pumpFrames(bound));
            if (Math.hypot(targetX - this.sim.x, targetY - this.sim.y)
                < offset + 40) {
                break;
            }
        }
        const stop = this.sim.stopChase();
        this.applyPacket(stop, this.driver.time + 1);
        this.driver.run(this.driver.time + 1500,
            (bound) => this.pumpFrames(bound));
    }

    // applyLaggyPosition feeds a position-only self sample at the current
    // server truth: the anchor moves, the destination and moveAt stay.
    applyLaggyPosition(at) {
        if (!this.isSelf) {
            return;
        }
        this.selfX = this.sim.x;
        this.selfY = this.sim.y;
        this.deliverSnapshot(at + 1);
    }

    // runPickupHops mimics the loot phase: many short straight hops with
    // pauses, where the collision dominated stop rule matters most.
    runPickupHops() {
        for (let i = 0; i < 8; i++) {
            const angle = this.random() * Math.PI * 2;
            const dist = 40 + this.random() * 110;
            const budget = this.walkTo(this.sim.x + Math.cos(angle) * dist,
                this.sim.y + Math.sin(angle) * dist);
            this.driver.run(budget, (bound) => this.pumpFrames(bound));
            this.settle(200 + this.random() * 600);
        }
    }

    // runRetarget mimics the combat approach: the destination changes
    // every ~1.05 s (just past the broadcast throttle) before the unit
    // reaches the final point.
    runRetarget() {
        let destX = this.sim.x + 500;
        let destY = this.sim.y + this.random() * 200 - 100;
        for (let i = 0; i < 6; i++) {
            this.walkTo(destX, destY);
            this.driver.run(this.driver.time + 1050,
                (bound) => this.pumpFrames(bound));
            destX += 120 + this.random() * 120;
            destY += this.random() * 160 - 80;
        }
        const finalPacket = this.sim.startMove(destX, destY,
            this.driver.time + 1);
        if (finalPacket) {
            this.applyPacket(finalPacket, this.driver.time + 1
                + HARNESS.parseDelayMs);
        }
        this.deliverSnapshot(this.driver.time + 2);
        this.scheduleTicks(this.driver.time + 1, this.driver.time + 6000);
        this.driver.run(this.driver.time + 6000,
            (bound) => this.pumpFrames(bound));
    }

    evaluate() {
        const failures = [];
        const errFactor = this.opts.maxErrFactor || HARNESS.maxErrFactor;
        const maxErr = Math.max(HARNESS.maxErrAbsolute,
            errFactor * this.opts.speed);
        const errOk = this.metrics.maxErr <= maxErr;
        const spikeOk = this.metrics.maxSpike <= HARNESS.maxSpikeRatio;
        const backOk = this.metrics.maxBack <= HARNESS.maxBackUnits;
        const arrivalOk = this.metrics.errAtArrival === null
            || this.metrics.errAtArrival <= maxErr;
        if (!errOk) {
            failures.push("max error " + this.metrics.maxErr.toFixed(1)
                + " > " + maxErr.toFixed(1));
        }
        if (!spikeOk) {
            failures.push("max spike " + this.metrics.maxSpike.toFixed(2)
                + "x > " + HARNESS.maxSpikeRatio + "x");
        }
        if (!backOk) {
            failures.push("drawn back " + this.metrics.maxBack.toFixed(1)
                + "u > " + HARNESS.maxBackUnits + "u in "
                + this.metrics.backFrames + " frames");
        }
        if (!arrivalOk) {
            failures.push("arrival error "
                + this.metrics.errAtArrival.toFixed(1) + " > "
                + maxErr.toFixed(1));
        }
        const passed = failures.length === 0;
        console.log((passed ? "PASS " : "FAIL ") + this.name
            + " (" + this.opts.speed + " u/s, collision "
            + this.opts.collision + ")");
        console.log("  maxErr=" + this.metrics.maxErr.toFixed(1) + "u"
            + " err@arrival=" + (this.metrics.errAtArrival === null
                ? "n/a" : this.metrics.errAtArrival.toFixed(1) + "u")
            + " maxSpike=" + this.metrics.maxSpike.toFixed(2) + "x"
            + " maxBack=" + this.metrics.maxBack.toFixed(1) + "u"
            + " frames=" + this.metrics.frames
            + (passed ? "" : "  <- " + failures.join("; ")));

        return passed;
    }
}

// scenario definitions: each mirrors a real world pattern
function scenarios() {
    return [
        new Scenario("npc random walk", {
            kind: "npc", objectId: 5001, templateId: 1000001,
            speed: 60, collision: 10,
            startX: 45000, startY: 50000, minDist: 80, maxDist: 300,
            seed: 11
        }),
        new Scenario("npc long run", {
            kind: "npc", objectId: 5002, templateId: 1000002,
            speed: 120, collision: 15,
            startX: 45000, startY: 50000, minDist: 600, maxDist: 900,
            seed: 23
        }),
        new Scenario("played character walk", {
            kind: "player", objectId: 100, templateId: 0,
            speed: 165, collision: 9,
            startX: 45000, startY: 50000, minDist: 200, maxDist: 500,
            seed: 31
        }),
        new Scenario("played character chase", {
            kind: "player", objectId: 100, templateId: 0,
            speed: 165, collision: 9,
            startX: 45000, startY: 50000, chase: true,
            chaseDistance: 700, pawnOffset: 40, seed: 41
        }),
        new Scenario("played character chase, laggy samples", {
            kind: "player", objectId: 100, templateId: 0,
            speed: 165, collision: 9,
            startX: 45000, startY: 50000, chase: true, laggy: true,
            chaseDistance: 700, pawnOffset: 40, seed: 43,
            maxErrFactor: 0.6
        }),
        new Scenario("pickup hops", {
            kind: "player", objectId: 100, templateId: 0,
            speed: 165, collision: 9,
            startX: 45000, startY: 50000, pickupHops: true, seed: 53
        }),
        new Scenario("retarget approach", {
            kind: "player", objectId: 100, templateId: 0,
            speed: 165, collision: 9,
            startX: 45000, startY: 50000, retarget: true, seed: 61
        }),
        new Scenario("npc chase of the player", {
            kind: "npc", objectId: 5003, templateId: 1000003,
            speed: 120, collision: 12,
            startX: 45200, startY: 50000, chase: true,
            chaseDistance: 700, pawnOffset: 40, seed: 71
        })
    ];
}

// ---- live record / replay ----

// parseArgs splits the mode arguments off the flag list.
function parseArgs() {
    const args = process.argv.slice(2);
    const record = args.includes("--record") ? args[args.indexOf("--record")
        + 1] : null;
    const replay = args.includes("--replay")
        ? args[args.indexOf("--replay") + 1] : null;
    const positional = (name) => {
        const at = args.indexOf(name);

        return at >= 0 ? args[at + 1] : null;
    };

    return {
        record, replay,
        outfile: positional("--record") ? (args[args.indexOf("--record")
            + 2] || null) : null,
        url: positional("--record") ? (args[args.indexOf("--record") + 3]
            || "http://127.0.0.1:8080") : null,
        bot: positional("--record") ? (args[args.indexOf("--record") + 4]
            || null) : null,
        flags: new Set(args.filter((a) => a.startsWith("--")))
    };
}

// record connects to the bot SSE stream and stores every snapshot with
// its local receive time.
async function record(seconds, outfile, url, bot) {
    const base = url.replace(/\/$/, "");
    let id = bot;
    if (!id) {
        const response = await fetch(base + "/api/bots");
        const bots = await response.json();
        if (bots.length === 0) {
            console.error("no bots registered at " + base);
            process.exit(1);
        }
        id = bots[0].id;
    }
    console.log("recording bot " + id + " from " + base + " for "
        + seconds + "s");
    const response = await fetch(base + "/api/bots/" + id + "/events");
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    const samples = [];
    const started = Date.now();
    let buffer = "";
    while (Date.now() - started < seconds * 1000) {
        const chunk = await reader.read();
        if (chunk.done) {
            break;
        }
        buffer += decoder.decode(chunk.value, { stream: true });
        let boundary = buffer.indexOf("\n\n");
        while (boundary >= 0) {
            const block = buffer.slice(0, boundary);
            buffer = buffer.slice(boundary + 2);
            const dataLine = block.split("\n")
                .find((line) => line.startsWith("data: "));
            if (dataLine) {
                samples.push({
                    t: Date.now(),
                    snap: JSON.parse(dataLine.slice(6))
                });
            }
            boundary = buffer.indexOf("\n\n");
        }
    }
    fs.writeFileSync(outfile || "movement_recording.json", JSON.stringify({
        startedAt: started, bot: id, samples
    }));
    console.log("recorded " + samples.length + " snapshots to "
        + (outfile || "movement_recording.json"));
}

// replay feeds a recording through the real map at 60 fps and reports
// the drawn marker stepping backwards along its walk segment
// (DRAWN-BACK), a target projection that retreats when a late position
// sample re-anchors the move (TARGET-BACK, the server side lag the
// state tracker has to prevent) and the frame speed spikes. A real
// teleport (the position jumps farther than the map teleport radius)
// is not a movement transition and is skipped by the back checks.
async function replay(file) {
    const data = JSON.parse(fs.readFileSync(file, "utf8"));
    const samples = data.samples;
    if (samples.length < 2) {
        console.error("recording has too few samples");
        process.exit(1);
    }
    const loaded = loadMapJs();
    const MapView = loaded.MapView;
    const clock = loaded.clock;
    const verbose = process.argv.includes("--frames");
    const from = samples[0].t;
    const to = samples[samples.length - 1].t;
    let sampleIndex = 0;
    let maxSpike = 0;
    let spikeCount = 0;
    let frames = 0;
    let drawnBackMax = 0;
    let drawnBackFrames = 0;
    let targetBackMax = 0;
    let targetBackCount = 0;
    const lastPos = new Map();
    const lastSig = new Map();
    const lastView = new Map();

    const viewOf = (snap, key) => {
        if (!snap) {
            return null;
        }
        if (key === "self") {
            return snap.character || null;
        }

        return (snap.objects || []).find((o) => String(o.objectId)
            === String(key)) || null;
    };
    const signature = (view) => view.x + "," + view.y + "," + view.destX
        + "," + view.destY + "," + view.moveAtMs;

    console.log("replaying " + samples.length + " snapshots, "
        + Math.round((to - from) / 1000) + "s");
    for (let t = from; t <= to; t += HARNESS.frameMs) {
        clock.nowMs = t;
        while (sampleIndex < samples.length && samples[sampleIndex].t <= t) {
            const snap = samples[sampleIndex].snap;
            const before = MapView.lastSnap;
            const nowBefore = clock.nowMs + MapView.clockOffsetMs;
            const keys = new Set(["self"]);
            for (const o of snap.objects || []) {
                keys.add(String(o.objectId));
            }
            for (const key of keys) {
                const prev = viewOf(before, key);
                const next = viewOf(snap, key);
                if (!prev || !next || !prev.moving || !next.moving
                    || !(next.speed > 0)) {
                    continue;
                }
                if (prev.destX !== next.destX || prev.destY !== next.destY) {
                    continue;
                }
                const jump = Math.hypot(next.x - prev.x, next.y - prev.y);
                if (jump > HARNESS.teleportUnits) {
                    continue;
                }
                const tp = MapView.projectTickwise(prev, key, nowBefore);
                const tn = MapView.projectTickwise(next, key, nowBefore);
                const ddx = tn.x - tp.x;
                const ddy = tn.y - tp.y;
                const fx = next.destX - next.x;
                const fy = next.destY - next.y;
                const flen = Math.hypot(fx, fy);
                if (flen <= 1) {
                    continue;
                }
                const along = (ddx * fx + ddy * fy) / flen;
                if (along < -HARNESS.maxBackUnits) {
                    targetBackCount++;
                    targetBackMax = Math.max(targetBackMax, -along);
                    console.log("TARGET-BACK t=" + Math.round(t) + " unit "
                        + key + " target retreated " + (-along).toFixed(1)
                        + "u (position jump " + jump.toFixed(0) + "u)");
                }
            }
            MapView.update(snap);
            sampleIndex++;
        }
        // drain the frame callbacks that are due
        let guard = 0;
        while (loaded.rafQueue.length > 0 && guard < 100
            && loaded.rafQueue[0].dueAt <= t + HARNESS.frameMs) {
            guard++;
            const frame = loaded.rafQueue.shift();
            clock.nowMs = Math.max(clock.nowMs, frame.dueAt);
            frame.cb(clock.nowMs);
        }
        clock.nowMs = t;
        frames++;
        const snap = MapView.lastSnap;
        if (!snap) {
            continue;
        }
        const views = [{ key: "self", view: snap.character, speed: snap.character ? snap.character.speed : 0 }]
            .concat((snap.objects || []).filter((o) => o.moving)
                .map((o) => ({ key: String(o.objectId), view: o, speed: o.speed })));
        for (const { key, view, speed } of views) {
            if (!view || !view.moving || !(speed > 0)) {
                lastPos.delete(key);
                lastSig.delete(key);

                continue;
            }
            const rt = MapView.runtime.get(key);
            if (!rt) {
                continue;
            }
            const sig = signature(view);
            const prev = lastPos.get(key);
            const prevSig = lastSig.get(key);
            lastPos.set(key, { x: rt.drawX, y: rt.drawY });
            lastSig.set(key, sig);
            if (!prev) {
                continue;
            }
            const stepX = rt.drawX - prev.x;
            const stepY = rt.drawY - prev.y;
            const delta = Math.hypot(stepX, stepY);
            const perFrame = speed * HARNESS.frameMs / 1000;
            const ratio = perFrame > 0.01 ? delta / perFrame : 0;
            if (ratio > maxSpike) {
                maxSpike = ratio;
            }
            if (ratio > HARNESS.maxSpikeRatio) {
                spikeCount++;
                console.log("SPIKE t=" + Math.round(t) + " unit " + key
                    + " moved " + delta.toFixed(1) + "u in one frame ("
                    + ratio.toFixed(2) + "x of " + speed + " u/s)");
            }
            if (prevSig === sig && delta > 0.01) {
                const fx = view.destX - view.x;
                const fy = view.destY - view.y;
                const flen = Math.hypot(fx, fy);
                // Outside the anchor-destination span the marker is
                // catching up after a retarget, not walking the line.
                const relX = rt.drawX - view.x;
                const relY = rt.drawY - view.y;
                const rel = flen > 1 ? (relX * fx + relY * fy) / flen : -1;
                if (flen > 1 && rel >= 0 && rel <= flen) {
                    const along = (stepX * fx + stepY * fy) / flen;
                    if (along < -HARNESS.maxBackUnits) {
                        drawnBackFrames++;
                        drawnBackMax = Math.max(drawnBackMax, -along);
                        console.log("DRAWN-BACK t=" + Math.round(t)
                            + " unit " + key + " stepped back "
                            + (-along).toFixed(2) + "u of "
                            + delta.toFixed(2) + "u (speed frame "
                            + perFrame.toFixed(2) + "u)");
                    }
                }
            }
            const prevView = lastView.get(key);
            lastView.set(key, view);
            if (verbose && frames % 10 === 0) {
                console.log("t=" + Math.round(t) + " unit " + key
                    + " drawn=" + Math.round(rt.drawX) + ","
                    + Math.round(rt.drawY)
                    + " seg=" + (view.destX - view.x).toFixed(0) + ","
                    + (view.destY - view.y).toFixed(0)
                    + " frameU=" + delta.toFixed(2)
                    + (prevView && prevView.moveAtMs !== view.moveAtMs
                        ? " reanchor" : ""));
            }
        }
    }
    console.log("frames=" + frames + " maxSpike=" + maxSpike.toFixed(2)
        + "x spikes>" + HARNESS.maxSpikeRatio + "x: " + spikeCount);
    console.log("drawnBackMax=" + drawnBackMax.toFixed(1)
        + "u frames=" + drawnBackFrames
        + " targetBackMax=" + targetBackMax.toFixed(1)
        + "u count=" + targetBackCount);
    const drawnOk = drawnBackMax <= HARNESS.maxBackUnits;
    const targetOk = targetBackMax <= HARNESS.maxTargetBackUnits;
    if (!drawnOk) {
        console.log("FAIL: drawn marker stepped back along the walk line");
    }
    if (!targetOk) {
        console.log("FAIL: a late position sample re-anchored the move "
            + "behind the projected target");
    }
    process.exitCode = maxSpike <= HARNESS.maxSpikeRatio
        && drawnOk && targetOk ? 0 : 1;
}

function main() {
    const args = parseArgs();
    if (args.record) {
        record(Number(args.record), args.outfile, args.url, args.bot);

        return;
    }
    if (args.replay) {
        replay(args.replay);

        return;
    }
    let allPassed = true;
    const only = process.argv.includes("--only")
        ? process.argv[process.argv.indexOf("--only") + 1] : null;
    for (const scenario of scenarios()) {
        if (only && !scenario.name.includes(only)) {
            continue;
        }
        const passed = scenario.run();
        allPassed = allPassed && passed;
    }
    console.log(allPassed
        ? "movement reproduction: PASS (drawn follows the server truth)"
        : "movement reproduction: FAIL (jerky movement reproduced)");
    process.exitCode = allPassed ? 0 : 1;
}

main();
