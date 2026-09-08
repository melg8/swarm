// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecorderRecordsAndWalks(t *testing.T) {
	t.Parallel()
	recorder := NewRecorder()

	recorder.Record([]byte{0x21, 0x01})
	recorder.Record([]byte{0x22, 0x02})
	recorder.Record([]byte{0x21, 0x03})

	require.Equal(t, int64(1), recorder.FirstPacketSeq(0x21))
	require.Equal(t, int64(2), recorder.FirstPacketSeq(0x22))
	require.Nil(t, recorder.Entry(99))

	seen := make([]byte, 0, 3)
	recorder.Walk(func(_ int64, payload []byte) bool {
		seen = append(seen, payload[0])

		return true
	})
	require.Equal(t, []byte{0x21, 0x22, 0x21}, seen)

	stopped := false
	recorder.Walk(func(_ int64, _ []byte) bool {
		stopped = true

		return false
	})
	require.True(t, stopped, "the walk must honor the stop")
}

func TestRecorderAttachSnapshotAndLive(t *testing.T) {
	t.Parallel()
	recorder := NewRecorder()
	recorder.Record([]byte{0x21, 0x01})
	recorder.Record([]byte{0x04, 0x02})

	// Attach after the first entry: the snapshot carries the rest and
	// the live subscription continues at the end of the snapshot.
	entries, sub := recorder.Attach(1)
	require.Len(t, entries, 1)
	require.Equal(t, byte(0x04), entries[0].payload[0])
	require.Equal(t, int64(2), entries[0].seq)

	recorder.Record([]byte{0x01, 0x03})
	select {
	case update := <-sub.ch:
		require.Equal(t, int64(3), update.seq)
		require.Equal(t, byte(0x01), update.payload[0])
	case <-time.After(time.Second):
		t.Fatal("the live update never arrived")
	}

	recorder.removeSubscriber(sub)
	recorder.Record([]byte{0x01, 0x04})
	select {
	case <-sub.ch:
		t.Fatal("a removed subscriber must not receive updates")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRecorderPoisonsSlowSubscribers(t *testing.T) {
	t.Parallel()
	recorder := NewRecorder()
	_, sub := recorder.Attach(0)

	// Overflow the subscriber queue: the recorder must poison it
	// instead of blocking, and the recorder must keep working.
	for range subscriberQueueSize + 2 {
		recorder.Record([]byte{0x01})
	}
	select {
	case <-sub.poison:
	default:
		t.Fatal("the slow subscriber must be poisoned")
	}

	stopped := false
	recorder.Walk(func(_ int64, _ []byte) bool {
		stopped = true

		return false
	})
	require.True(t, stopped, "the recorder must stay usable")
}

func TestRecorderCloseReleasesSubscribers(t *testing.T) {
	t.Parallel()
	recorder := NewRecorder()
	_, sub := recorder.Attach(0)

	recorder.Record([]byte{0x01})
	require.Len(t, sub.ch, 1)

	recorder.Close()
	select {
	case <-recorder.CloseSignal():
	default:
		t.Fatal("the close signal must fire")
	}

	// Records after the close are ignored.
	recorder.Record([]byte{0x02})
	require.True(t, recorder.Closed())
	_, sub2 := recorder.Attach(0)
	select {
	case <-sub2.ch:
		t.Fatal("a closed recorder must not deliver live updates")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRecorderTrimKeepsPrologueAndTail(t *testing.T) {
	t.Parallel()
	recorder := NewRecorder()

	// Fill the recorder past the cap with distinct one byte payloads.
	total := recorderPrologueEntries + recorderTailEntries + 100
	payloads := make([]byte, 0, total)
	for i := range total {
		recorder.Record([]byte{byte(i % 251)})
		payloads = append(payloads, byte(i%251))
	}

	// One byte entries stay far below the byte cap: the trim only fires
	// when the bytes pass the cap, so with tiny entries nothing is
	// dropped and the sequences stay dense.
	require.Equal(t, total, recorder.Stats().Entries)

	// Record enough bulk to cross the byte cap.
	bulk := make([]byte, recorderMaxBytes/2)
	recorder.Record(bulk)
	recorder.Record(bulk)

	stats := recorder.Stats()
	require.Less(t, stats.Entries, total+2, "the trim must have dropped entries")
	require.LessOrEqual(t, stats.Bytes, recorderMaxBytes+len(bulk))

	// The prologue (the enter world burst) survives the trim: the first
	// entries keep their sequence numbers.
	require.Equal(t, int64(1), recorder.FirstPacketSeq(0x00))
	require.Equal(t, byte(0), mustEntry(t, recorder, 1)[0])
}

// mustEntry returns the entry payload or fails the test.
func mustEntry(t *testing.T, recorder *Recorder, seq int64) []byte {
	t.Helper()
	payload := recorder.Entry(seq)
	require.NotNil(t, payload)

	return payload
}
