// Copyright (c) the go-ruby-zlib/zlib authors
//
// SPDX-License-Identifier: BSD-3-Clause

package zlib

import (
	"bytes"
	"sync"
	"testing"

	kzlib "github.com/klauspost/compress/zlib"
)

// freshDeflate compresses data with a brand-new writer, the bytes a
// non-pooled implementation would emit. Pooled Deflate must equal this exactly.
func freshDeflate(t *testing.T, data []byte, level int) []byte {
	t.Helper()
	var b bytes.Buffer
	w, err := kzlib.NewWriterLevel(&b, level)
	if err != nil {
		t.Fatalf("NewWriterLevel(%d): %v", level, err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return append([]byte(nil), b.Bytes()...)
}

// TestDeflatePoolByteIdentical is the load-bearing guarantee behind pooling: a
// reused (Reset) writer emits bytes identical to a freshly constructed one, at
// every valid level, for both an empty and a repetitive payload. Each level is
// deflated several times so the second and later calls come back from the pool.
func TestDeflatePoolByteIdentical(t *testing.T) {
	payloads := [][]byte{
		nil,
		[]byte("The quick brown fox jumps over the lazy dog. "),
		bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 40),
		bytes.Repeat([]byte{0}, 4096),
	}
	for lvl := DefaultCompression; lvl <= BestCompression; lvl++ {
		for _, p := range payloads {
			want := freshDeflate(t, p, lvl)
			for rep := 0; rep < 4; rep++ {
				got, err := Deflate(p, lvl)
				if err != nil {
					t.Fatalf("Deflate(level=%d rep=%d): %v", lvl, rep, err)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("Deflate(level=%d rep=%d) not byte-identical to fresh:\n got %x\nwant %x", lvl, rep, got, want)
				}
			}
		}
	}
}

// TestInflatePoolReuseAfterError confirms a reader that hit a decode error is not
// returned to the pool poisoned: a bad stream errors, and the very next Inflate
// of a good stream still round-trips (it draws either that reader, Reset clean,
// or a fresh one, both valid).
func TestInflatePoolReuseAfterError(t *testing.T) {
	good, _ := Deflate([]byte("payload that round-trips"), DefaultCompression)
	for i := 0; i < 8; i++ {
		if _, err := Inflate([]byte("definitely not zlib")); err == nil {
			t.Fatal("Inflate(bad) = nil error, want error")
		}
		out, err := Inflate(good)
		if err != nil {
			t.Fatalf("Inflate(good) after bad: %v", err)
		}
		if string(out) != "payload that round-trips" {
			t.Fatalf("Inflate(good) = %q", out)
		}
	}
}

// TestPoolConcurrent hammers the pools from many goroutines under -race to prove
// the borrow/reset/return dance is data-race-free and each caller gets a correct,
// independent result.
func TestPoolConcurrent(t *testing.T) {
	payload := bytes.Repeat([]byte("concurrent zlib workload. "), 32)
	want := freshDeflate(t, payload, DefaultCompression)
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				comp, err := Deflate(payload, DefaultCompression)
				if err != nil || !bytes.Equal(comp, want) {
					t.Errorf("concurrent Deflate mismatch: err=%v equal=%v", err, bytes.Equal(comp, want))
					return
				}
				out, err := Inflate(comp)
				if err != nil || !bytes.Equal(out, payload) {
					t.Errorf("concurrent Inflate mismatch: err=%v equal=%v", err, bytes.Equal(out, payload))
					return
				}
			}
		}()
	}
	wg.Wait()
}

// TestDeflatePoolAvoidsPerCallAlloc guards the whole point of the change: pooled
// Deflate must not allocate a fresh compressor each call. A fresh writer costs
// well over a hundred allocations (its window and hash tables); pooled Deflate
// settles far below that once warm. The generous ceiling keeps the guard stable
// across Go and klauspost versions while still catching a regression that drops
// the pooling.
func TestDeflatePoolAvoidsPerCallAlloc(t *testing.T) {
	payload := bytes.Repeat([]byte("allocation regression guard. "), 40)
	// Warm the pool so the measured runs are all reuse.
	for i := 0; i < 4; i++ {
		if _, err := Deflate(payload, DefaultCompression); err != nil {
			t.Fatal(err)
		}
	}
	got := testing.AllocsPerRun(200, func() {
		if _, err := Deflate(payload, DefaultCompression); err != nil {
			t.Fatal(err)
		}
	})
	if got > 20 {
		t.Fatalf("pooled Deflate allocated %.0f objects/op, want <= 20 (pooling regressed?)", got)
	}
}
