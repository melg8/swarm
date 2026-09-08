// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// newPathfindTestServer builds a pathfind test server over a temp
// geodata directory with one flat walkable region file.
func newPathfindTestServer(t *testing.T, view *pathfind.Vec3) *Server {
	t.Helper()
	dir := t.TempDir()
	data := make([]byte, 0, 65536*3)
	for range 65536 {
		data = append(data, 0, 0, 0) // flat block at height 0
	}
	name := filepath.Join(dir, "22_22.l2j")
	require.NoError(t, os.WriteFile(name, data, 0o600))

	engine := pathfind.NewEngine(dir)

	return NewPathfindServer(engine, "127.0.0.1:0",
		log.New(io.Discard, "", 0), PathfindOptions{ViewCenter: view})
}

func TestPathfindConfigEndpoint(t *testing.T) {
	t.Run("default view falls back to the geodata center", func(t *testing.T) {
		server := newPathfindTestServer(t, nil)
		recorder := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
			http.MethodGet, "/api/config", nil))

		require.Equal(t, http.StatusOK, recorder.Code)

		var config configResponse
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &config))
		require.Equal(t, modePathfind, config.Mode)
		require.NotNil(t, config.Geodata)
		require.NotZero(t, config.MaxSteps)
		require.NotNil(t, config.Defaults)
	})

	t.Run("view override becomes the default center", func(t *testing.T) {
		view := pathfind.Vec3{X: 46000, Y: 51000, Z: -3400}
		server := newPathfindTestServer(t, &view)
		recorder := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
			http.MethodGet, "/api/config", nil))

		require.Equal(t, http.StatusOK, recorder.Code)

		var config configResponse
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &config))
		require.InDelta(t, 46000.0, config.Defaults.Center.X, 0.001)
		require.InDelta(t, 51000.0, config.Defaults.Center.Y, 0.001)
	})
}

func TestPathfindSearchEndpoint(t *testing.T) {
	server := newPathfindTestServer(t, nil)
	handler := server.httpServer.Handler

	t.Run("path found inside the flat region", func(t *testing.T) {
		body := `{"start":{"x":66000,"y":132000,"z":0},` +
			`"end":{"x":68000,"y":134000,"z":0}}`
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(
			http.MethodPost, "/api/pathfind", bytes.NewBufferString(body)))

		require.Equal(t, http.StatusOK, recorder.Code)

		var response pathfindResponse
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		require.True(t, response.Found)
		require.False(t, response.Aborted)
		require.Empty(t, response.Error)
		require.NotEmpty(t, response.Waypoints)
		require.NotEmpty(t, response.Raw)
		require.Greater(t, response.Length, 0.0)
		require.GreaterOrEqual(t, response.RegionsLoaded, 1)
		require.InDelta(t, 66000.0, response.Start.X, 0.001)
		require.InDelta(t, 134000.0, response.End.Y, 0.001)
	})

	t.Run("start outside the geodata answers an error", func(t *testing.T) {
		body := `{"start":{"x":0,"y":0,"z":0},` +
			`"end":{"x":68000,"y":134000,"z":0}}`
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(
			http.MethodPost, "/api/pathfind", bytes.NewBufferString(body)))

		require.Equal(t, http.StatusOK, recorder.Code)

		var response pathfindResponse
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		require.False(t, response.Found)
		require.NotEmpty(t, response.Error)
		require.Empty(t, response.Waypoints)
	})

	t.Run("invalid json body rejected", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(
			http.MethodPost, "/api/pathfind", bytes.NewBufferString("not json")))

		require.Equal(t, http.StatusBadRequest, recorder.Code)
	})
}

func TestToResponsePointsAndDownsample(t *testing.T) {
	world := []pathfind.Vec3{{X: 1, Y: 2, Z: 3}, {X: 4, Y: 5, Z: 6}}
	points := toResponsePoints(world)
	require.Len(t, points, 2)
	require.InDelta(t, 1.0, points[0].X, 0.001)
	require.InDelta(t, 6.0, points[1].Z, 0.001)

	// A short path stays untouched.
	short := []pathfindPoint{{X: 1}, {X: 2}}
	require.Len(t, downsample(short, 10), 2)

	// An oversized path keeps the shape and the last point.
	long := make([]pathfindPoint, 100)
	for i := range long {
		long[i] = pathfindPoint{X: float64(i)}
	}
	kept := downsample(long, 10)
	require.Len(t, kept, 11)
	require.InDelta(t, 0.0, kept[0].X, 0.001)
	require.InDelta(t, 99.0, kept[len(kept)-1].X, 0.001)
}

func TestBotConfigEndpointReportsBotMode(t *testing.T) {
	server, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/config", nil))

	require.Equal(t, http.StatusOK, recorder.Code)

	var config configResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &config))
	require.Equal(t, modeBot, config.Mode)
	require.Nil(t, config.Geodata)
	require.Nil(t, config.Defaults)
}

func TestServerAddress(t *testing.T) {
	server, _ := newTestServer(t)
	require.Equal(t, "127.0.0.1:0", server.Address())
}

// noFlushWriter drops the Flush method of the recorder: the event
// stream must refuse to start without a flusher.
type noFlushWriter struct {
	header http.Header
	body   bytes.Buffer
	code   int
}

func newNoFlushWriter() *noFlushWriter {
	return &noFlushWriter{header: http.Header{}}
}

// Header implements http.ResponseWriter.
func (w *noFlushWriter) Header() http.Header { return w.header }

// Write implements http.ResponseWriter.
func (w *noFlushWriter) Write(p []byte) (int, error) {
	return w.body.Write(p) //nolint:wrapcheck // test helper
}

// WriteHeader implements http.ResponseWriter.
func (w *noFlushWriter) WriteHeader(code int) { w.code = code }

func TestStreamEventsRequiresAFlusher(t *testing.T) {
	server, bot := newTestServer(t)
	writer := newNoFlushWriter()
	server.streamEvents(context.Background(), writer, bot)

	require.Equal(t, http.StatusInternalServerError, writer.code)
	require.Contains(t, writer.body.String(), "streaming unsupported")
}

func TestEventsStreamPollsVersionChanges(t *testing.T) {
	server, bot := newTestServer(t)

	recorder := newSyncRecorder()
	request := httptest.NewRequest(
		http.MethodGet, "/api/bots/test1/events", nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.httpServer.Handler.ServeHTTP(recorder, request)
	}()
	t.Cleanup(func() {
		server.shutdown()
		<-done
	})

	// The first snapshot arrives at once, the poll ticker delivers the
	// version change of the second state update.
	require.Eventually(t, func() bool {
		return bytes.Count([]byte(recorder.String()), []byte("event:")) == 1
	}, 2*time.Second, 20*time.Millisecond)

	bot.ApplyNpcInfo(state.NpcInfo{ObjectID: 9, Name: "Orc"})

	require.Eventually(t, func() bool {
		return bytes.Count([]byte(recorder.String()), []byte("event:")) == 2
	}, 2*time.Second, 100*time.Millisecond)
}

func TestWritePing(t *testing.T) {
	server, _ := newTestServer(t)
	recorder := httptest.NewRecorder()

	require.False(t, server.writePing(recorder, recorder))
	require.Equal(t, ": ping\n\n", recorder.Body.String())
}

// failingWriter simulates a dropped SSE connection.
type failingWriter struct {
	header http.Header
}

// Header implements http.ResponseWriter.
func (w *failingWriter) Header() http.Header { return w.header }

// Write implements http.ResponseWriter.
func (w *failingWriter) Write([]byte) (int, error) {
	return 0, errWriteFailed
}

// WriteHeader implements http.ResponseWriter.
func (w *failingWriter) WriteHeader(int) {}

var errWriteFailed = &writeError{}

type writeError struct{}

// Error implements the error interface.
func (*writeError) Error() string { return "write failed" }

func TestWritePingReportsWriteFailures(t *testing.T) {
	server, _ := newTestServer(t)
	writer := &failingWriter{}

	require.True(t, server.writePing(writer, recorderFlusher{}))
}

// recorderFlusher satisfies the flusher without touching anything.
type recorderFlusher struct{}

// Flush implements http.Flusher.
func (recorderFlusher) Flush() {}

func TestWriteSnapshotEventStreamsVersionChanges(t *testing.T) {
	_, bot := newTestServer(t)
	recorder := httptest.NewRecorder()
	lastVersion := bot.Version()
	stream := &sseStream{frame: nil, payload: nil}

	// An unchanged version writes nothing.
	writeSnapshotEvent(recorder, recorder, bot, &lastVersion, stream)
	require.Empty(t, recorder.Body.String())

	// A version change writes the snapshot event and moves the cursor.
	bot.ApplyNpcInfo(state.NpcInfo{ObjectID: 9, Name: "Orc"})
	writeSnapshotEvent(recorder, recorder, bot, &lastVersion, stream)
	require.Contains(t, recorder.Body.String(), "event: snapshot")
	require.Contains(t, recorder.Body.String(), `"id":"test1"`)
	require.Equal(t, bot.Version(), lastVersion)

	// The delivered version does not repeat.
	writeSnapshotEvent(recorder, recorder, bot, &lastVersion, stream)
	require.Equal(t, 1, bytes.Count(recorder.Body.Bytes(),
		[]byte("event: snapshot")))
}

func TestWriteJSONLogsEncodeFailures(t *testing.T) {
	loggerBuffer := &bytes.Buffer{}
	logger := log.New(loggerBuffer, "", 0)
	recorder := httptest.NewRecorder()

	writeJSON(recorder, logger, make(chan int))

	require.Empty(t, recorder.Body.String())
	require.Contains(t, loggerBuffer.String(), "Error encoding json response")
}

func TestGeodataTileCacheLRU(t *testing.T) {
	cache := newGeodataTileCache()
	key := geodataTileKey{mode: "height", level: 0, col: 1, row: 1}

	cache.put(key, []byte("first"))
	// A repeated put never overwrites a cached tile.
	cache.put(key, []byte("second"))
	tile, ok := cache.get(key)
	require.True(t, ok)
	require.Equal(t, []byte("first"), tile)

	// A miss answers false.
	_, ok = cache.get(geodataTileKey{mode: "walls", level: 0, col: 1, row: 1})
	require.False(t, ok)

	// The LRU evicts the least recently used tile: touching the first
	// key keeps it alive while the untouched second key goes first.
	second := geodataTileKey{mode: "height", level: 0, col: 2, row: 2}
	cache.put(second, []byte("second tile"))
	_, _ = cache.get(key)
	for i := range geodataTileCacheMax - 1 {
		cache.put(geodataTileKey{
			mode: "height", level: 1, col: 3, row: i,
		}, []byte("filler"))
	}

	_, ok = cache.get(second)
	require.False(t, ok, "the untouched key must be evicted first")
	_, ok = cache.get(key)
	require.True(t, ok, "the touched key survives the eviction")
}

func TestHandleGeodataTileCacheAndMissingRegion(t *testing.T) {
	server := newPathfindTestServer(t, nil)
	handler := server.httpServer.Handler

	// Two identical requests: the second one answers from the cache.
	for range 2 {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(
			http.MethodGet, "/api/geodata/tile/0/22_22.png?mode=height", nil))
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
		require.NotEmpty(t, recorder.Body.Bytes())
	}

	// A region the engine does not know answers 404.
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/geodata/tile/0/21_19.png?mode=height", nil))
	require.Equal(t, http.StatusNotFound, recorder.Code)

	for _, url := range []string{
		"/api/geodata/tile/0/abc_22.png",
		"/api/geodata/tile/0/70000_22.png",
		"/api/geodata/tile/0/22_70000.png",
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(
			http.MethodGet, url, nil))
		require.Equal(t, http.StatusBadRequest, recorder.Code, url)
	}
}

func TestIconsServeWithoutPack(t *testing.T) {
	// A working directory outside the repository finds no icon pack:
	// the detection walks up from the temp directory and answers empty.
	previous, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(t.TempDir()))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(previous))
	})
	require.Empty(t, detectIconsDir())

	server := newServer("127.0.0.1:0", log.New(io.Discard, "", 0))
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/icons/whatever.png", nil))

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestBotZoneCommandQueues(t *testing.T) {
	server, bot := newTestServer(t)
	body := `{"kind":"zone","count":1}`
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodPost, "/api/bots/test1/commands",
			bytes.NewBufferString(body)))

	require.Equal(t, http.StatusAccepted, recorder.Code)

	select {
	case cmd := <-bot.Commands():
		require.Equal(t, state.CommandZone, cmd.Kind)
		require.Equal(t, int32(1), cmd.Count)
	default:
		t.Fatal("the zone command must be queued")
	}
}

func TestDescribeCommandFallback(t *testing.T) {
	require.Equal(t, "user command: dance",
		describeCommand(commandRequest{Kind: "dance"}))
}

func TestServerShutdownClosesEventStreams(t *testing.T) {
	registry := state.NewRegistry()
	bot := state.NewBot("test1")
	bot.SetOnline("test1")
	registry.Add(bot)
	server := NewServer(registry, "127.0.0.1:0", log.New(io.Discard, "", 0))

	require.Equal(t, "127.0.0.1:0", server.Address())

	recorder := newSyncRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
			http.MethodGet, "/api/bots/test1/events", nil))
	}()

	require.Eventually(t, func() bool {
		return recorder.String() != ""
	}, 2*time.Second, 20*time.Millisecond)

	require.NoError(t, server.Shutdown(context.Background()))
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the shutdown must close the event stream")
	}
}
