// Copyright (c) the go-ruby-zlib/zlib authors
//
// SPDX-License-Identifier: BSD-3-Clause

package zlib

// Error is the base of the Zlib error family, mirroring Zlib::Error (a
// StandardError subclass in MRI). Every error this package returns wraps to
// Error via errors.Is, so a host can map the whole family to Zlib::Error and the
// leaf classes (StreamError / BufError / DataError / GzipFile::Error) to the
// matching Ruby exceptions.
type Error struct {
	// Class is the MRI exception class name a host should raise, e.g.
	// "Zlib::DataError"; it lets the binding pick the exact Ruby class.
	Class string
	// Msg is the message MRI uses for the condition.
	Msg string
	// wrapped is the underlying standard-library error, surfaced through Unwrap.
	wrapped error
}

// Error implements the error interface with MRI's message text.
func (e *Error) Error() string { return e.Msg }

// Unwrap exposes the underlying standard-library error for errors.Is/As.
func (e *Error) Unwrap() error { return e.wrapped }

// Is lets errors.Is(err, ErrXxx) match by MRI class name, so a caller can test
// errors.Is(err, ErrData) regardless of the wrapped library error.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Class == e.Class
}

// The sentinel leaf errors. They carry MRI's class name and default message;
// the wrap* helpers below clone them with the underlying library error attached
// so errors.Is still matches while errors.Unwrap reaches the cause.
var (
	// ErrStream is Zlib::StreamError — an invalid argument such as a bad
	// compression level.
	ErrStream = &Error{Class: "Zlib::StreamError", Msg: "stream error"}
	// ErrBuf is Zlib::BufError — a buffer/flush condition (e.g. a stream that
	// produced no progress).
	ErrBuf = &Error{Class: "Zlib::BufError", Msg: "buffer error"}
	// ErrData is Zlib::DataError — corrupt or invalid compressed input.
	ErrData = &Error{Class: "Zlib::DataError", Msg: "incorrect header check"}
	// ErrGzipFile is Zlib::GzipFile::Error — a malformed gzip stream.
	ErrGzipFile = &Error{Class: "Zlib::GzipFile::Error", Msg: "not in gzip format"}
)

// wrapData attaches cause to a fresh ErrData-classed error.
func wrapData(cause error) *Error {
	return &Error{Class: ErrData.Class, Msg: ErrData.Msg, wrapped: cause}
}

// wrapGzip attaches cause to a fresh ErrGzipFile-classed error.
func wrapGzip(cause error) *Error {
	return &Error{Class: ErrGzipFile.Class, Msg: ErrGzipFile.Msg, wrapped: cause}
}
