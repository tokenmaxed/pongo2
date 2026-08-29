package pongo2

import "bytes"

// Meter observes and may bound work performed by one template execution.
//
// Iteration is called before each for-loop iteration. Charge is called before
// pongo2 retains n additional bytes in an internal construct buffer; a
// successful charge is paired with Release when those bytes are no longer
// retained. EnterMacro is called before each macro body; a successful entry is
// paired with LeaveMacro when the call returns.
//
// Returning an error from Iteration, Charge, or EnterMacro aborts execution.
// A failed Charge reserves no bytes, and a failed EnterMacro does not require a
// LeaveMacro call. The same Meter is shared by all child execution contexts.
type Meter interface {
	Iteration() error
	Charge(n int) error
	Release(n int)
	EnterMacro() error
	LeaveMacro()
}

// meteredBuffer accounts for the live contents of an internal rendering
// buffer. It deliberately charges content bytes rather than buffer capacity:
// capacity is an implementation detail, while every successful charge has an
// exact release when the construct returns.
type meteredBuffer struct {
	bytes.Buffer
	meter   Meter
	charged int
}

func newMeteredBuffer(meter Meter, capacity int) *meteredBuffer {
	buffer := &meteredBuffer{meter: meter}
	if capacity > 0 {
		buffer.Grow(capacity)
	}
	return buffer
}

func (buffer *meteredBuffer) Write(p []byte) (int, error) {
	if err := buffer.charge(len(p)); err != nil {
		return 0, err
	}
	n, err := buffer.Buffer.Write(p)
	buffer.commit(len(p), n)
	return n, err
}

func (buffer *meteredBuffer) WriteString(s string) (int, error) {
	if err := buffer.charge(len(s)); err != nil {
		return 0, err
	}
	n, err := buffer.Buffer.WriteString(s)
	buffer.commit(len(s), n)
	return n, err
}

func (buffer *meteredBuffer) charge(n int) error {
	if buffer.meter == nil || n == 0 {
		return nil
	}
	if err := buffer.meter.Charge(n); err != nil {
		return err
	}
	// Reserve before growing the bytes.Buffer so the construct's deferred
	// release still balances the charge if bytes.Buffer panics.
	buffer.charged += n
	return nil
}

func (buffer *meteredBuffer) commit(requested, written int) {
	if buffer.meter == nil || requested == 0 {
		return
	}
	if unwritten := requested - written; unwritten > 0 {
		buffer.charged -= unwritten
		buffer.meter.Release(unwritten)
	}
}

func (buffer *meteredBuffer) release() {
	if buffer.meter != nil && buffer.charged > 0 {
		buffer.meter.Release(buffer.charged)
		buffer.charged = 0
	}
}
