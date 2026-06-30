// Copyright (c) the go-ruby-zlib/zlib authors
//
// SPDX-License-Identifier: BSD-3-Clause

package zlib

import (
	"bytes"
	"io"

	"github.com/klauspost/compress/zlib"
)

// Deflater is the streaming compressor, mirroring a Zlib::Deflate instance:
//
//	z := zlib.NewDeflater(zlib.BestCompression)
//	a, _ := z.Deflate([]byte("hello "), zlib.SyncFlush)
//	b, _ := z.Deflate([]byte("world"), zlib.NoFlush)
//	tail, _ := z.Finish()
//	stream := append(append(a, b...), tail...) // a valid zlib stream
//
// Bytes produced are accumulated internally; each Deflate / Finish call returns
// only the bytes that became available since the previous call (as MRI's
// Zlib::Deflate#deflate does). TotalIn / TotalOut / Adler / Finished track the
// stream as MRI's accessors do.
type Deflater struct {
	buf      bytes.Buffer // sink the zlib.Writer flushes into
	w        *zlib.Writer
	consumed int    // bytes of buf already returned to the caller
	totalIn  int64  // uncompressed bytes fed in
	adler    uint32 // running Adler-32 of the input (MRI's #adler)
	finished bool
}

// NewDeflater creates a streaming compressor at the given level, falling back to
// DefaultCompression when level is out of range. Use NewDeflaterLevel to detect an
// invalid level (which MRI reports as Zlib::StreamError) instead of defaulting it.
func NewDeflater(level int) *Deflater {
	d, err := NewDeflaterLevel(level)
	if err != nil {
		d, _ = NewDeflaterLevel(DefaultCompression)
	}
	return d
}

// NewDeflaterLevel creates a streaming compressor, returning ErrStream for an
// out-of-range level (MRI raises Zlib::StreamError from Zlib::Deflate.new).
func NewDeflaterLevel(level int) (*Deflater, error) {
	if !validLevel(level) {
		return nil, ErrStream
	}
	d := &Deflater{adler: adler32Identity}
	d.w, _ = zlib.NewWriterLevel(&d.buf, level) // level validated above
	return d, nil
}

// Deflate feeds data into the stream and returns the compressed bytes that became
// available, mirroring Zlib::Deflate#deflate(data, flush). flush is one of
// NoFlush / SyncFlush / FullFlush / Finish. With Finish the stream is closed (as
// Zlib::Deflate#deflate(data, Zlib::FINISH) does) and Finished becomes true.
func (d *Deflater) Deflate(data []byte, flush int) ([]byte, error) {
	if d.finished {
		return nil, ErrStream
	}
	if len(data) > 0 {
		// The sink is a bytes.Buffer, which never fails to grow.
		_, _ = d.w.Write(data)
		d.totalIn += int64(len(data))
		d.adler = Adler32(data, d.adler)
	}
	switch flush {
	case NoFlush:
		// Hold the bytes in the writer; nothing guaranteed available yet.
	case SyncFlush, FullFlush:
		_ = d.w.Flush() // bytes.Buffer sink never errors
	case Finish:
		_ = d.w.Close() // ditto
		d.finished = true
	default:
		return nil, ErrStream
	}
	return d.take(), nil
}

// Finish closes the stream and returns any remaining compressed bytes, mirroring
// Zlib::Deflate#finish. After Finish, Finished reports true and further Deflate
// calls return ErrStream. Calling Finish again on an already-finished Deflater is
// tolerated and returns an empty slice (no error), matching MRI, whose
// Zlib::Deflate#finish returns "" when re-invoked rather than raising.
func (d *Deflater) Finish() ([]byte, error) {
	if d.finished {
		// MRI tolerates re-finishing: the second #finish returns "".
		return []byte{}, nil
	}
	_ = d.w.Close() // bytes.Buffer sink never errors
	d.finished = true
	return d.take(), nil
}

// take returns the bytes accumulated in buf that have not yet been handed to the
// caller, advancing the consumed cursor.
func (d *Deflater) take() []byte {
	all := d.buf.Bytes()
	out := append([]byte(nil), all[d.consumed:]...)
	d.consumed = len(all)
	return out
}

// TotalIn reports the number of uncompressed bytes fed in (Zlib::Deflate#total_in).
func (d *Deflater) TotalIn() int64 { return d.totalIn }

// TotalOut reports the number of compressed bytes produced (Zlib::Deflate#total_out).
func (d *Deflater) TotalOut() int64 { return int64(d.buf.Len()) }

// Adler reports the running Adler-32 of the input (Zlib::Deflate#adler).
func (d *Deflater) Adler() uint32 { return d.adler }

// Finished reports whether the stream has been finished (Zlib::Deflate#finished?).
func (d *Deflater) Finished() bool { return d.finished }

// Inflater is the streaming decompressor, mirroring a Zlib::Inflate instance.
// inflate accumulates the compressed input and decodes as much as it can; the
// MRI streaming idiom Zlib::Inflate.new.inflate(stream) is supported by feeding a
// complete stream in one call.
type Inflater struct {
	in       bytes.Buffer // accumulated compressed input
	totalOut int64
	adler    uint32
	finished bool
}

// NewInflater creates a streaming decompressor (Zlib::Inflate.new).
func NewInflater() *Inflater {
	return &Inflater{adler: adler32Identity}
}

// Inflate feeds compressed data and returns the decompressed bytes decoded so
// far, mirroring Zlib::Inflate#inflate(data). The accumulated input must form a
// complete zlib stream by the time output is required; a complete stream (the
// common one-shot streaming use) decodes fully and marks the inflater finished.
// Corrupt input returns ErrData.
//
// Calling Inflate on an already-finished Inflater is tolerated and returns an
// empty slice (no error), matching MRI: once the stream end has been reached,
// Zlib::Inflate#inflate returns "" for any further input (including non-empty
// data) rather than raising.
func (inf *Inflater) Inflate(data []byte) ([]byte, error) {
	if inf.finished {
		// MRI tolerates inflate after the stream end: returns "".
		return []byte{}, nil
	}
	inf.in.Write(data)
	r, err := zlib.NewReader(bytes.NewReader(inf.in.Bytes()))
	if err != nil {
		// Header not yet complete: tolerate and wait for more input. A genuinely
		// invalid header surfaces on the next call once enough bytes arrive; an
		// outright bad magic with sufficient bytes is reported now.
		if inf.in.Len() >= 2 {
			return nil, wrapData(err)
		}
		return nil, nil
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, wrapData(err) // ReadAll surfaces body and checksum errors
	}
	_ = r.Close()
	inf.totalOut += int64(len(out))
	inf.adler = Adler32(out, inf.adler)
	inf.finished = true
	return out, nil
}

// Finish completes decompression, mirroring Zlib::Inflate#finish; for this
// one-shot streaming model it is a no-op that marks the inflater finished and
// returns no further bytes.
func (inf *Inflater) Finish() ([]byte, error) {
	inf.finished = true
	return nil, nil
}

// TotalOut reports the number of decompressed bytes produced (Zlib::Inflate#total_out).
func (inf *Inflater) TotalOut() int64 { return inf.totalOut }

// TotalIn reports the number of compressed bytes consumed (Zlib::Inflate#total_in).
func (inf *Inflater) TotalIn() int64 { return int64(inf.in.Len()) }

// Adler reports the running Adler-32 of the output (Zlib::Inflate#adler).
func (inf *Inflater) Adler() uint32 { return inf.adler }

// Finished reports whether the stream has been fully inflated (Zlib::Inflate#finished?).
func (inf *Inflater) Finished() bool { return inf.finished }
