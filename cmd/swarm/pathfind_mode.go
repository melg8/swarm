// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "context"
    "errors"
    "fmt"
    "log"
    "net/http"
    "os"
    "os/signal"
    "strconv"
    "strings"
    "syscall"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/melg8/swarm/internal/swarm/webserver"
)

// runPathfindTest serves the bot less map pathfinding test UI: the geodata
// engine is loaded from the configured or auto detected directory and the
// web interface answers path requests until the process is stopped.
func runPathfindTest(cfg config) {
    dir := cfg.geodataDir
    if dir == "" {
        dir = detectGeodataDir()
    }
    engine := pathfind.NewEngine(dir)
    engine.SetMaxPassableHeight(uint16(cfg.maxPassable))
    // The capsule clearance keeps the test UI routes away from the
    // walls like the live bot plans them (docs/pathfinding.md).
    engine.SetCapsuleClearance(pathfind.DefaultCollisionRadius)

    stats := engine.Stats()
    if stats.HasData {
        log.Printf("Pathfind test: %d geodata region files in %s",
            stats.RegionFiles, stats.Dir)
    } else {
        log.Println("Pathfind test: no geodata files found in " + stats.Dir +
            ", pass -geodata with the game server data/geodata directory")
    }

    web := startWebInterface(cfg, nil, engine, nil)
    if web == nil {
        log.Println("Pathfind test needs the web interface, " +
            "pass a -web address")

        return
    }

    log.Println("Pathfind test UI is ready")

    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer stop()
    <-ctx.Done()
    shutdownWebInterface(web)
    log.Println("Pathfind test finished")
}

// navmeshShowFlag is the -show-navmesh flag value. It implements
// IsBoolFlag so the bare -show-navmesh works (the every tile stitched
// world), while -show-navmesh=21_19,22_19 names the tiles to open.
type navmeshShowFlag struct {
    tiles   []navmesh.RegionKey
    enabled bool
}

// String renders the flag state for the -help listing.
func (f *navmeshShowFlag) String() string {
    if !f.enabled {
        return "false"
    }
    if len(f.tiles) == 0 {
        return "true"
    }
    keys := make([]string, 0, len(f.tiles))
    for _, key := range f.tiles {
        keys = append(keys, strconv.Itoa(int(key.Col))+"_"+
            strconv.Itoa(int(key.Row)))
    }

    return strings.Join(keys, ",")
}

// Set parses the flag value: true/false toggle the mode, anything
// else is the comma separated col_row tile list.
func (f *navmeshShowFlag) Set(value string) error {
    switch value {
    case "true", "":
        f.enabled = true
        f.tiles = nil

        return nil
    case "false":
        f.enabled = false
        f.tiles = nil

        return nil
    }
    specs := strings.Split(value, ",")
    tiles := make([]navmesh.RegionKey, 0, len(specs))
    for _, spec := range specs {
        colText, rowText, found := strings.Cut(strings.TrimSpace(spec), "_")
        if !found {
            return fmt.Errorf("bad tile %q, expected col_row like 21_19", spec)
        }
        col, err := strconv.Atoi(colText)
        if err != nil || col < -32768 || col > 32767 {
            return fmt.Errorf("bad tile column %q: %w", colText, err)
        }
        row, err := strconv.Atoi(rowText)
        if err != nil || row < -32768 || row > 32767 {
            return fmt.Errorf("bad tile row %q: %w", rowText, err)
        }
        // The bounds checks above pin the int16 conversion range.
        col16, row16 := int16(col), int16(row) //nolint:gosec // guarded
        tiles = append(tiles, navmesh.RegionKey{Col: col16, Row: row16})
    }
    f.enabled = true
    f.tiles = tiles

    return nil
}

// IsBoolFlag lets flag.Parse accept the bare -show-navmesh form.
func (f *navmeshShowFlag) IsBoolFlag() bool {
    return true
}

// navmeshViewerInitial resolves and logs the initial tile selection
// of the viewer: the flag named tiles that exist, every tile when the
// flag named none.
func navmeshViewerInitial(mesh *navmesh.Mesh, dir string,
    requested []navmesh.RegionKey, tileFiles int, tileDir string,
) []navmesh.RegionKey {
    initial := requested
    if len(initial) > 0 {
        initial = intersectTiles(mesh, initial)
        if len(initial) == 0 {
            log.Println("Navmesh viewer: none of the requested tiles " +
                "exist in " + dir + ", opening every tile instead")
            initial = nil
        }
    }
    if len(initial) == 0 {
        log.Printf("Navmesh viewer: %d tiles stitched in %s",
            tileFiles, tileDir)
    } else {
        log.Printf("Navmesh viewer: %d of %d tiles selected in %s",
            len(initial), tileFiles, tileDir)
    }

    return initial
}

// navmeshComparePack loads the -navmesh-compare tile pack of the
// dual pack view (issue #59): the second directory renders in pane B
// beside the primary one. An explicitly passed directory that holds
// no tiles answers ok=false and the mode stops - a silent single
// pack fallback would read as a typo'd pane B with no explanation.
func navmeshComparePack(dir string) (*navmesh.Mesh, bool) {
    mesh := navmesh.NewMesh(dir)
    stats := mesh.Stats()
    if stats.TileFiles == 0 {
        log.Println("Navmesh viewer: the -navmesh-compare " +
            "directory holds no tiles in " + dir)

        return nil, false
    }
    log.Printf("Navmesh viewer: the compare pack arms pane B: "+
        "%d tiles in %s", stats.TileFiles, stats.Dir)

    return mesh, true
}

func runNavmeshViewer(cfg config) {
    if cfg.webAddress == "" {
        log.Println("Navmesh viewer needs the web interface, " +
            "pass a -web address")

        return
    }
    dir := cfg.navmeshDir
    if dir == "" {
        dir = detectNavmeshDir()
    }
    if dir == "" {
        log.Println("Navmesh viewer found no tile directory, pass " +
            "-navmesh with the cmd/navmesh-build output")

        return
    }
    mesh := navmesh.NewMesh(dir)
    stats := mesh.Stats()
    if stats.TileFiles == 0 {
        log.Println("Navmesh viewer found no tiles in " + dir +
            ", build them with cmd/navmesh-build first")

        return
    }
    // The compare pack of the dual pack view (issue #59): the second
    // directory renders in pane B. An explicitly passed directory
    // that holds no tiles stops the mode - a silent single pack
    // fallback would read as a typo'd pane B with no explanation.
    var compare *navmesh.Mesh
    if cfg.navmeshCompareDir != "" {
        pack, ok := navmeshComparePack(cfg.navmeshCompareDir)
        if !ok {
            return
        }
        compare = pack
    }
    engine := newBotEngine(cfg)
    initial := navmeshViewerInitial(mesh, dir, cfg.navmeshShow.tiles,
        stats.TileFiles, stats.Dir)

    server := webserver.NewNavmeshServer(mesh, cfg.webAddress,
        log.Default(), webserver.NavmeshOptions{
            InitialTiles: initial,
            Engine:       engine,
            CompareMesh:  compare,
        })
    go func() {
        if err := server.ListenAndServe(); err != nil {
            if !errors.Is(err, http.ErrServerClosed) {
                log.Printf("Web interface stopped: %v", err)
            }
        }
    }()

    log.Println("Navmesh viewer UI is ready on http://" +
        server.Address())

    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer stop()
    <-ctx.Done()
    if err := server.Shutdown(context.Background()); err != nil {
        log.Printf("Web interface shutdown: %v", err)
    }
    log.Println("Navmesh viewer finished")
}

// intersectTiles keeps the requested tile keys that exist in the mesh
// directory (the flag may name tiles of another pack).
func intersectTiles(mesh *navmesh.Mesh, requested []navmesh.RegionKey,
) []navmesh.RegionKey {
    existing := make(map[navmesh.RegionKey]struct{})
    for _, key := range mesh.TileFiles() {
        existing[key] = struct{}{}
    }
    found := make([]navmesh.RegionKey, 0, len(requested))
    for _, key := range requested {
        if _, ok := existing[key]; ok {
            found = append(found, key)
        }
    }

    return found
}

// detectGeodataDir picks the first candidate directory that exists.
func detectGeodataDir() string {
    for _, candidate := range defaultGeodataCandidates {
        if info, err := os.Stat(candidate); err == nil && info.IsDir() {
            return candidate
        }
    }
    log.Println("No geodata directory found, pass -geodata explicitly")

    return defaultGeodataCandidates[0]
}

// detectNavmeshDir picks the first candidate directory that holds at
// least one tile file, "" when none does: the mesh is an accelerator
// over the geodata engine, not a dependency - an absent tile set
// keeps the navigator on the pure grid engine without a word of
// noise.
func detectNavmeshDir() string {
    for _, candidate := range defaultNavmeshCandidates {
        entries, err := os.ReadDir(candidate)
        if err != nil {
            continue
        }
        for _, entry := range entries {
            if strings.HasSuffix(entry.Name(), ".nm") {
                return candidate
            }
        }
    }

    return ""
}

// loadNavmesh builds the mesh over the navmesh tile directory when
// the tiles exist: the explicit -navmesh value wins, the autodetected
// data/navmesh follows, and a directory without tiles answers nil so
// the hunt loop installs the plain engine navigator.
func loadNavmesh(cfg config) *navmesh.Mesh {
    dir := cfg.navmeshDir
    if dir == "" {
        dir = detectNavmeshDir()
    }
    if dir == "" {
        return nil
    }
    mesh := navmesh.NewMesh(dir)
    stats := mesh.Stats()
    if stats.TileFiles == 0 {
        log.Println("No navmesh tiles found in " + dir +
            ", the hunt routes stay on the grid engine")

        return nil
    }
    log.Printf("Navmesh ready: %d tiles in %s, the long hunt routes "+
        "serve from the mesh", stats.TileFiles, stats.Dir)

    return mesh
}
