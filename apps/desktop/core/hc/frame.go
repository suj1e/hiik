package hc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Frame header and stream kinds. See contracts/hc.openrpc.json and the
// protocol notes in docs/subsystems/core.md.
const (
	frameHeaderSize = 9
	maxPayload      = 16 << 20
)

// Stream frame kinds.
const (
	kindData uint8 = iota
	kindCredit
	kindClose
	kindResize
	kindFocus
	kindFacts
)

// frame is one framed-mode message: a channel, a kind and a payload.
type frame struct {
	channel uint32
	kind    uint8
	payload []byte
}

// marshalFrame returns the wire bytes of f.
func marshalFrame(f frame) []byte {
	out := make([]byte, frameHeaderSize+len(f.payload))
	binary.LittleEndian.PutUint32(out[0:4], f.channel)
	out[4] = f.kind
	binary.LittleEndian.PutUint32(out[5:9], uint32(len(f.payload)))
	copy(out[frameHeaderSize:], f.payload)
	return out
}

// writeFrame writes one frame to w.
func writeFrame(w io.Writer, f frame) error {
	return writeAll(w, marshalFrame(f))
}

// readFrame reads one frame from r. A clean end between frames returns
// io.EOF; a short header or payload returns io.ErrUnexpectedEOF.
func readFrame(r io.Reader) (frame, error) {
	var h [frameHeaderSize]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return frame{}, err
	}
	length := binary.LittleEndian.Uint32(h[5:9])
	if length > maxPayload {
		return frame{}, fmt.Errorf("hc: frame of %d bytes exceeds %d", length, maxPayload)
	}
	f := frame{
		channel: binary.LittleEndian.Uint32(h[0:4]),
		kind:    h[4],
		payload: make([]byte, length),
	}
	if _, err := io.ReadFull(r, f.payload); err != nil {
		if errors.Is(err, io.EOF) {
			return frame{}, io.ErrUnexpectedEOF
		}
		return frame{}, err
	}
	return f, nil
}

// writeAll writes all of b, even when w returns a short write with no error.
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if n > 0 {
			b = b[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
