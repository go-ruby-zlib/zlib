// Copyright (c) the go-ruby-zlib/zlib authors
//
// SPDX-License-Identifier: BSD-3-Clause

package zlib

import (
	"bytes"
	"errors"
	"testing"
)

// TestDeflaterStream mirrors Zlib::Deflate.new(level).deflate(a, SYNC_FLUSH)
// .deflate(b).finish and checks the concatenation inflates back to a+b, plus the
// accessors.
func TestDeflaterStream(t *testing.T) {
	d := NewDeflater(BestCompression)
	p1, err := d.Deflate([]byte("hello "), SyncFlush)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := d.Deflate([]byte("world"), NoFlush)
	if err != nil {
		t.Fatal(err)
	}
	tail, err := d.Finish()
	if err != nil {
		t.Fatal(err)
	}
	stream := bytes.Join([][]byte{p1, p2, tail}, nil)
	out, err := Inflate(stream)
	if err != nil {
		t.Fatalf("inflate stream: %v", err)
	}
	if string(out) != "hello world" {
		t.Errorf("stream inflate = %q", out)
	}
	if d.TotalIn() != 11 {
		t.Errorf("TotalIn = %d, want 11", d.TotalIn())
	}
	if d.TotalOut() != int64(len(stream)) {
		t.Errorf("TotalOut = %d, want %d", d.TotalOut(), len(stream))
	}
	if d.Adler() != 436929629 {
		t.Errorf("Adler = %d, want 436929629", d.Adler())
	}
	if !d.Finished() {
		t.Error("Finished = false after Finish")
	}
}

// TestDeflaterFinishViaFlush exercises finishing through Deflate(_, FINISH).
func TestDeflaterFinishViaFlush(t *testing.T) {
	d := NewDeflater(DefaultCompression)
	out, err := d.Deflate([]byte("data"), Finish)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Finished() {
		t.Error("not finished after FINISH flush")
	}
	got, err := Inflate(out)
	if err != nil || string(got) != "data" {
		t.Fatalf("inflate = %q, %v", got, err)
	}
}

func TestDeflaterFullFlush(t *testing.T) {
	d := NewDeflater(BestSpeed)
	a, err := d.Deflate([]byte("chunk-one;"), FullFlush)
	if err != nil {
		t.Fatal(err)
	}
	b, err := d.Finish()
	if err != nil {
		t.Fatal(err)
	}
	out, err := Inflate(append(a, b...))
	if err != nil || string(out) != "chunk-one;" {
		t.Fatalf("inflate = %q, %v", out, err)
	}
}

func TestDeflaterErrors(t *testing.T) {
	// Unknown flush mode.
	d := NewDeflater(DefaultCompression)
	if _, err := d.Deflate([]byte("x"), 99); !errors.Is(err, ErrStream) {
		t.Errorf("bad flush err = %v, want ErrStream", err)
	}
	// After finishing, both Deflate and Finish reject further work.
	d2 := NewDeflater(DefaultCompression)
	if _, err := d2.Finish(); err != nil {
		t.Fatal(err)
	}
	if _, err := d2.Deflate([]byte("x"), NoFlush); !errors.Is(err, ErrStream) {
		t.Errorf("deflate after finish err = %v, want ErrStream", err)
	}
	if _, err := d2.Finish(); !errors.Is(err, ErrStream) {
		t.Errorf("double finish err = %v, want ErrStream", err)
	}
}

func TestNewDeflaterLevel(t *testing.T) {
	if _, err := NewDeflaterLevel(99); !errors.Is(err, ErrStream) {
		t.Errorf("NewDeflaterLevel(99) err = %v, want ErrStream", err)
	}
	// NewDeflater falls back to a default for a bad level rather than panicking.
	d := NewDeflater(99)
	out, err := d.Deflate([]byte("ok"), Finish)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := Inflate(out); string(got) != "ok" {
		t.Errorf("fallback deflater = %q", got)
	}
}

// TestInflaterStream mirrors Zlib::Inflate.new.inflate(stream).
func TestInflaterStream(t *testing.T) {
	comp, _ := Deflate([]byte("hello world"), DefaultCompression)
	inf := NewInflater()
	out, err := inf.Inflate(comp)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "hello world" {
		t.Errorf("inflater = %q", out)
	}
	if inf.TotalOut() != 11 {
		t.Errorf("TotalOut = %d, want 11", inf.TotalOut())
	}
	if inf.TotalIn() != int64(len(comp)) {
		t.Errorf("TotalIn = %d, want %d", inf.TotalIn(), len(comp))
	}
	if inf.Adler() != 436929629 {
		t.Errorf("Adler = %d", inf.Adler())
	}
	if !inf.Finished() {
		t.Error("not finished")
	}
	// Inflating again after finish is rejected.
	if _, err := inf.Inflate(comp); !errors.Is(err, ErrStream) {
		t.Errorf("inflate after finish err = %v, want ErrStream", err)
	}
}

// TestInflaterIncremental feeds the compressed stream in two pieces, exercising
// the "header not yet complete, wait for more" path (first byte alone) and the
// completion on the second feed.
func TestInflaterIncremental(t *testing.T) {
	comp, _ := Deflate([]byte("incremental data here"), BestCompression)
	inf := NewInflater()
	// One byte: header incomplete, no output, no error, not finished.
	out, err := inf.Inflate(comp[:1])
	if err != nil {
		t.Fatalf("first feed err = %v", err)
	}
	if len(out) != 0 || inf.Finished() {
		t.Errorf("first feed produced %q finished=%v", out, inf.Finished())
	}
	// Remainder completes the stream.
	out, err = inf.Inflate(comp[1:])
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "incremental data here" {
		t.Errorf("incremental result = %q", out)
	}
}

func TestInflaterBadData(t *testing.T) {
	inf := NewInflater()
	if _, err := inf.Inflate([]byte("garbage zlib bytes")); !errors.Is(err, ErrData) {
		t.Errorf("inflater bad data err = %v, want ErrData", err)
	}
	// Valid header but corrupt body via the streaming reader.
	good, _ := Deflate([]byte("body to corrupt"), BestCompression)
	bad := append([]byte(nil), good...)
	bad[len(bad)-1] ^= 0xff
	inf2 := NewInflater()
	if _, err := inf2.Inflate(bad); !errors.Is(err, ErrData) {
		t.Errorf("inflater corrupt err = %v, want ErrData", err)
	}
}

func TestInflaterFinish(t *testing.T) {
	inf := NewInflater()
	out, err := inf.Finish()
	if err != nil || out != nil {
		t.Errorf("Finish = %q, %v", out, err)
	}
	if !inf.Finished() {
		t.Error("not finished after Finish")
	}
}
