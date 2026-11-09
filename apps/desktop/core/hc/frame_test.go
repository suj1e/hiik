package hc

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestFrameGolden(t *testing.T) {
	got := marshalFrame(frame{channel: 1, kind: kindData, payload: []byte("hi")})
	want := []byte{
		0x01, 0x00, 0x00, 0x00, // channel 1, little endian
		0x00,                   // DATA
		0x02, 0x00, 0x00, 0x00, // length 2, little endian
		'h', 'i',
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("frame bytes = % x, want % x", got, want)
	}
}

type chunkReader struct {
	data []byte
	n    int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := r.n
	if n > len(p) {
		n = len(p)
	}
	if n > len(r.data) {
		n = len(r.data)
	}
	copy(p, r.data[:n])
	r.data = r.data[n:]
	return n, nil
}

func TestReadFramePartial(t *testing.T) {
	want := frame{channel: 7, kind: kindFacts, payload: []byte("abc")}
	encoded := marshalFrame(want)
	for _, chunk := range []int{1, 2, 3, 5, 8} {
		r := &chunkReader{data: encoded, n: chunk}
		got, err := readFrame(r)
		if err != nil {
			t.Fatalf("chunk %d: %v", chunk, err)
		}
		if got.channel != want.channel || got.kind != want.kind || !bytes.Equal(got.payload, want.payload) {
			t.Fatalf("chunk %d: got %+v, want %+v", chunk, got, want)
		}
	}
}

func TestReadFrameEnds(t *testing.T) {
	if _, err := readFrame(bytes.NewReader(nil)); !errors.Is(err, io.EOF) {
		t.Fatalf("empty: %v", err)
	}
	partial := marshalFrame(frame{channel: 1, kind: kindData, payload: []byte("xyz")})[:6]
	if _, err := readFrame(bytes.NewReader(partial)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("partial header: %v", err)
	}
	cut := marshalFrame(frame{channel: 1, kind: kindData, payload: []byte("xyz")})[:10]
	if _, err := readFrame(bytes.NewReader(cut)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("partial payload: %v", err)
	}
}

type shortWriter struct {
	buf bytes.Buffer
}

func (w *shortWriter) Write(p []byte) (int, error) {
	n := len(p)
	if n > 3 {
		n = 3
	}
	return w.buf.Write(p[:n])
}

func TestWriteFrameShortWrites(t *testing.T) {
	w := &shortWriter{}
	want := frame{channel: 2, kind: kindData, payload: []byte("hello")}
	if err := writeFrame(w, want); err != nil {
		t.Fatal(err)
	}
	got, err := readFrame(bytes.NewReader(w.buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.channel != want.channel || got.kind != want.kind || !bytes.Equal(got.payload, want.payload) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
