#!/usr/bin/env node
/*
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
*/

// Reproduction harness for the web map movement interpolation.
//
// It loads the real internal/swarm/webserver/web/map.js (unchanged) into a
// sandboxed context, feeds it snapshots produced by a faithful simulation
// of the Mobius C1 movement semantics, and measures how well the drawn
// position matches the server arrival:
//
// - progress at arrival: how much of the packet path the drawn unit had
//   covered when the server arrival packet reached the web map (the
//   reported bug shows ~7/8 = 0.875 followed by a fast drag);
// - end of path jump: the largest frame to frame displacement right after
//   the arrival, as a multiple of the normal per frame speed.
//
// Mobius semantics reproduced here (see docs/development_log.md):
// - game ticks are 100 ms (GameTimeTaskManager.TICKS_PER_SECOND = 10);
// - the creature advances speed * ticks / 10 world units per tick;
// - it counts as arrived once the traveled distance covers
//   distance - collisionRadius (Creature.updatePosition), the position
//   is snapped to the exact destination and a zero distance
//   MoveToLocation is broadcast (StopMove ends chases the same way);
// - chasing creatures re-broadcast MoveToPawn at most once per second.
//
// Usage: node tools/repro_movement.js [--verbose]
// Exit code 0 = movement rendering is correct, 1 = bug reproduced.

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const MAP_JS = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web", "map.js");

// Harness parameters (calibrated from the Mobius sources and the C1 npc
// stats: collision radii 5-15, random walk hops below MaxDriftRange=300).
const HARNESS = {
    parseDelayMs: 1,       // game server -> bot packet processing
    transportMs: 4,        // bot SSE write -> browser event delivery
    pollPeriodMs: 300,     // webserver event stream poll period
    frameMs: 1000 / 60,    // requestAnimationFrame cadence
    idleAfterArrivalMs: 400,
    movesPerScenario: 8,
    evaluatedFromMove: 2,  // first moves calibrate the arrival gap
    progressTolerance: 0.05,
    absoluteToleranceUnits: 5,
    maxJumpRatio: 2.5
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

// MobiusSim reproduces the server side movement of one creature.
class MobiusSim {
    constructor(opts) {
        this.v = opts.speed;
        this.collision = opts.collision;
        this.tickMs = 100; // game ticks are always 100 ms
        this.x = opts.startX;
        this.y = opts.startY;
        this.moving = false;
        this.moveStartTick = 0;
        this.lastTickIndex = 0;
        this.lastStepTick = 0;
    }

    startMove(destX, destY, nowMs) {
        this.destX = destX;
        this.destY = destY;
        this.moving = true;
        this.moveStartTick = Math.floor(nowMs / this.tickMs);
        this.lastTickIndex = this.moveStartTick;
        this.lastStepTick = this.moveStartTick;

        return { type: "MoveToLocation", x: this.x, y: this.y, destX, destY };
    }

    // tick advances the server position exactly like
    // Creature.updatePosition: one game tick worth of distance per tick,
    // compared against the REMAINING distance minus the collision radius
    // (the Java code recomputes dx/dy from the running xAccurate every
    // update and resets moveTimestamp after each one). Arrival happens
    // when one tick step covers the remaining distance, so the creature
    // effectively stops collision + step units short and the server snaps
    // it to the exact destination.
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
        // one game tick of movement since the last update
        const step = this.v * (tickIndex - this.lastStepTick) / 10;
        this.lastStepTick = tickIndex;
        if (delta <= 1 || step > delta) {
            this.x = this.destX;
            this.y = this.destY;
            this.moving = false;
            packets.push({
                type: "MoveToLocation", x: this.destX, y: this.destY,
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
        this.destX = this.x - dx / dist * stop;
        this.destY = this.y - dy / dist * stop;
        this.moving = true;
        this.moveStartTick = Math.floor(nowMs / this.tickMs);
        this.lastTickIndex = this.moveStartTick;
        this.lastStepTick = this.moveStartTick;

        return {
            type: "MoveToPawn", x: this.x, y: this.y,
            targetX, targetY, distance: offset
        };
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
    constructor(objectId, templateId) {
        this.version = 1;
        this.obj = {
            objectId, templateId, kind: "npc", name: "sim", dead: false,
            moving: false, running: true, speed: 0,
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
        this.version++;
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

// Scenario drives one movement pattern against a loaded MapView.
class Scenario {
    constructor(name, opts) {
        this.name = name;
        this.opts = opts;
        this.isSelf = opts.kind === "player";
        this.results = [];
        this.verbose = process.argv.includes("--verbose");
    }

    run() {
        const loaded = loadMapJs();
        this.MapView = loaded.MapView;
        this.clock = loaded.clock;
        this.rafQueue = loaded.rafQueue;

        const sim = new MobiusSim(this.opts);
        const state = new BotState(this.opts.objectId,
            this.opts.templateId);
        const random = makeRandom(this.opts.seed || 7);
        const driver = new EventDriver((t) => { this.clock.nowMs = t; });

        // played character snapshot fields (fed by applySelfPacket)
        this.selfX = this.opts.startX;
        this.selfY = this.opts.startY;
        this.selfMoving = false;
        this.selfDestX = this.opts.startX;
        this.selfDestY = this.opts.startY;
        this.selfMoveAtMs = 0;
        this.selfSpeed = this.opts.speed;

        const snapshotObjects = () => (this.isSelf ? [] : [state.obj]);
        const characterSnapshot = () => ({
            objectId: 100, name: "self",
            x: this.selfX, y: this.selfY, z: 0, heading: 0,
            moving: this.selfMoving, speed: this.selfMoving
                ? this.selfSpeed : 0,
            destX: this.selfDestX, destY: this.selfDestY,
            moveAtMs: this.selfMoveAtMs
        });

        // deliverSnapshot models the SSE poll: the snapshot is built at
        // the poll time (capturing the state at that moment, like
        // writeSnapshotEvent calls bot.Snapshot() inside the poll tick)
        // and reaches the browser transportMs later.
        const deliverSnapshot = (at) => {
            driver.at(at, (t) => {
                const snap = {
                    serverTimeMs: t,
                    character: characterSnapshot(),
                    objects: snapshotObjects()
                };
                driver.at(t + HARNESS.transportMs, (t2) => {
                    this.clock.nowMs = t2;
                    this.MapView.update(snap);
                });
            });
        };

        // the frame pump executes queued rAF callbacks without moving the
        // clock past the given bound.
        const pumpFrames = (bound) => {
            const queue = this.rafQueue;
            let guard = 0;
            while (queue.length > 0 && guard < 1000) {
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
                if (before && after) {
                    this.recordFrame(before, after);
                }
            }
        };

        this.drawnPos = () => {
            const key = this.isSelf ? "self" : this.opts.objectId;
            const rt = this.MapView.runtime.get(key);
            return rt ? { x: rt.drawX, y: rt.drawY } : null;
        };
        this.debugFrames = process.argv.includes("--debug-frames");
        this.frameLogCounter = 0;
        this.recordFrame = (before, after) => {
            const delta = Math.hypot(after.x - before.x,
                after.y - before.y);
            if (this.currentMove && this.currentMove.arrivalDelivered) {
                this.currentMove.maxJumpAfterArrival = Math.max(
                    this.currentMove.maxJumpAfterArrival, delta);
            }
            if (this.debugFrames && this.frameLogCounter++ % 30 === 0) {
                const key = this.isSelf ? "self" : this.opts.objectId;
                const distToDest = Math.hypot(after.x - this.selfDestX,
                    after.y - this.selfDestY);
                console.log("    frame clock=" + Math.round(this.clock.nowMs)
                    + " drawn=" + key + ":" + Math.round(after.x)
                    + "," + Math.round(after.y)
                    + " self(x,y)=(" + Math.round(this.selfX) + ","
                    + Math.round(this.selfY) + ")"
                    + " selfDest=(" + Math.round(this.selfDestX) + ","
                    + Math.round(this.selfDestY) + ")"
                    + " distToDest=" + Math.round(distToDest)
                    + " moveAt=" + this.selfMoveAtMs
                    + " moving=" + this.selfMoving
                    + " delta=" + delta.toFixed(1));
            }
        };
        // applySelfPacket feeds the character view like the Go tracker.
        this.applySelfPacket = (packet, nowMs, speed) => {
            this.selfSpeed = speed;
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
        };
        // applyPacket routes a server packet into the fake tracker.
        this.applyPacket = (packet, at) => {
            if (this.isSelf) {
                this.applySelfPacket(packet, at, sim.v);
            } else {
                state.apply(packet, at, sim.v);
            }
        };
        // scheduleTicks runs the sim ticks between from and to.
        this.scheduleTicks = (fromMs, toMs, onPacket) => {
            let tickAt = Math.floor(fromMs / 100) * 100 + 100;
            while (tickAt <= toMs) {
                const at = tickAt;
                driver.at(at, (time) => {
                    if (!sim.moving) {
                        return;
                    }
                    for (const packet of sim.tick(time)) {
                        onPacket(packet, time);
                    }
                });
                tickAt += 100;
            }
        };

        // seed: a standing snapshot so the runtime entry exists
        state.obj.x = sim.x;
        state.obj.y = sim.y;
        state.obj.speed = this.opts.speed;
        deliverSnapshot(0);
        driver.run(10, pumpFrames);

        for (let moveIndex = 0; moveIndex < HARNESS.movesPerScenario;
            moveIndex++) {
            const angle = random() * Math.PI * 2;
            const dist = this.opts.chase ? this.opts.chaseDistance
                : this.opts.minDist + random()
                    * (this.opts.maxDist - this.opts.minDist);
            const destX = sim.x + Math.cos(angle) * dist;
            const destY = sim.y + Math.sin(angle) * dist;
            const move = {
                index: moveIndex, dist,
                progressAtArrival: null, maxJumpAfterArrival: 0,
                arrivalDelivered: false
            };
            this.currentMove = move;

            if (this.opts.chase) {
                this.runChase(sim, driver, deliverSnapshot, pumpFrames,
                    move, destX, destY);
            } else {
                this.runWalk(sim, driver, deliverSnapshot, pumpFrames,
                    move, destX, destY);
            }
            // settle the visuals between moves
            driver.at(driver.time + HARNESS.idleAfterArrivalMs, () => {});
            driver.run(driver.time + HARNESS.idleAfterArrivalMs,
                pumpFrames);
            this.results.push(move);
        }

        return this.evaluate();
    }

    // runWalk simulates a plain MoveToLocation walk.
    runWalk(sim, driver, deliverSnapshot, pumpFrames, move,
        destX, destY) {
        const startPacket = sim.startMove(destX, destY, driver.time + 1);
        driver.at(driver.time + 1, (t) => {
            this.applyPacket(startPacket, t);
        });
        deliverSnapshot(driver.time + 2);

        // remember the segment for the progress measurement
        const finishMove = (arrivalTime) => {
            const pollAt = Math.ceil(arrivalTime / HARNESS.pollPeriodMs)
                * HARNESS.pollPeriodMs;
            deliverSnapshot(pollAt);
            driver.at(pollAt + HARNESS.transportMs, (t) => {
                move.arrivalDelivered = true;
                const pos = this.drawnPos();
                if (pos) {
                    const err = Math.hypot(pos.x - sim.destX,
                        pos.y - sim.destY);
                    move.progressAtArrival = 1 - err / move.dist;
                }
            });
        };
        this.scheduleTicks(driver.time + 1,
            driver.time + (move.dist / sim.v) * 1000 + 2000,
            (packet, time) => {
                driver.at(time + HARNESS.parseDelayMs, (t) => {
                    this.applyPacket(packet, t);
                    if (packet.arrival) {
                        finishMove(t);
                    }
                });
            });
        const deadline = driver.time + (move.dist / sim.v) * 1000 + 2000;
        driver.run(deadline, pumpFrames);
        // safety: report missing arrival as total miss
        if (!move.arrivalDelivered) {
            move.progressAtArrival = -1;
        }
    }

    // runChase simulates the played character chasing a target with
    // MoveToPawn re-broadcasts every second and a final StopMove.
    runChase(sim, driver, deliverSnapshot, pumpFrames, move,
        targetX, targetY) {
        const offset = this.opts.pawnOffset || 40;
        let guard = 0;
        while (guard < 60) {
            guard++;
            const packet = sim.chaseMove(targetX, targetY, offset,
                driver.time + 1);
            driver.at(driver.time + 1, (t) => {
                this.applyPacket(packet, t);
            });
            deliverSnapshot(driver.time + 2);

            const dist = Math.hypot(targetX - sim.x, targetY - sim.y);
            const segmentMs = 1000;
            this.scheduleTicks(driver.time + 1, driver.time + segmentMs,
                (pkt, time) => {
                    driver.at(time + HARNESS.parseDelayMs, (t) => {
                        this.applyPacket(pkt, t);
                    });
                });
            driver.run(driver.time + segmentMs, pumpFrames);
            if (dist < offset + 40) {
                break;
            }
        }
        const stop = sim.stopChase();
        driver.at(driver.time + 1, (t) => {
            this.applyPacket(stop, t);
        });
        const pollAt = Math.ceil((driver.time + 1) / HARNESS.pollPeriodMs)
            * HARNESS.pollPeriodMs;
        deliverSnapshot(pollAt);
        driver.at(pollAt + HARNESS.transportMs, (t) => {
            move.arrivalDelivered = true;
            const pos = this.drawnPos();
            if (pos) {
                const err = Math.hypot(pos.x - stop.x, pos.y - stop.y);
                move.progressAtArrival = 1 - err / move.dist;
            }
        });
        driver.run(pollAt + 1500, pumpFrames);
    }

    evaluate() {
        const failures = [];
        const lines = [];
        const perFrame = this.opts.speed * HARNESS.frameMs / 1000;
        for (const move of this.results) {
            const calibrated = move.index >= HARNESS.evaluatedFromMove;
            const tolerance = Math.max(
                HARNESS.absoluteToleranceUnits,
                HARNESS.progressTolerance * move.dist);
            const gapUnits = move.progressAtArrival === null
                ? Infinity : (1 - move.progressAtArrival) * move.dist;
            const jumpRatio = move.maxJumpAfterArrival / perFrame;
            const ok = !calibrated || (gapUnits <= tolerance
                && jumpRatio <= HARNESS.maxJumpRatio);
            if (calibrated && !ok) {
                failures.push(move);
            }
            lines.push("  move " + move.index
                + " dist=" + Math.round(move.dist)
                + " progress@arrival="
                + (move.progressAtArrival === null
                    ? "n/a" : move.progressAtArrival.toFixed(3))
                + " gap=" + (Number.isFinite(gapUnits)
                    ? Math.round(gapUnits) + "u" : "n/a")
                + " jump=" + jumpRatio.toFixed(2) + "x"
                + (calibrated ? (ok ? " [ok]" : " [FAIL]")
                    : " [warmup]"));
        }
        const passed = failures.length === 0;
        console.log((passed ? "PASS " : "FAIL ") + this.name
            + " (" + this.opts.speed + " u/s, collision "
            + this.opts.collision + ", "
            + (this.opts.chase ? "chase" : "walk") + ")");
        for (const line of lines) {
            if (this.verbose || !line.endsWith("[ok]")) {
                console.log(line);
            }
        }
        if (!passed) {
            console.log("  thresholds: gap <= "
                + HARNESS.absoluteToleranceUnits + " units or "
                + Math.round(HARNESS.progressTolerance * 100)
                + "% of distance, jump <= " + HARNESS.maxJumpRatio + "x");
        }

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
        })
    ];
}

function main() {
    let allPassed = true;
    for (const scenario of scenarios()) {
        const passed = scenario.run();
        allPassed = allPassed && passed;
    }
    console.log(allPassed
        ? "movement reproduction: PASS (no end of path teleport)"
        : "movement reproduction: FAIL (end of path teleport reproduced)");
    process.exitCode = allPassed ? 0 : 1;
}

main();
