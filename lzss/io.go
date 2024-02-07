package lzss

import (
	"bytes"
	"github.com/icza/bitio"
	"io"
)

// writer aliases

type bitWriter interface {
	io.Writer
	startSession() error
	tryWriteBits(v uint64, nbBits uint8)
	tryWriteByte(b byte)
	tryError() error
	endSession() error
	reset()
	bytes() []byte
	len() int
	revert()
}

type writer struct { // standard output writer for the compressor; capable of reverting
	bb                bytes.Buffer
	bw                *bitio.Writer // invariant: bw cache must always be empty
	nbSkippedBits     uint8
	lastOutLen        int
	lastNbSkippedBits uint8
}

func (w *writer) startSession() error {
	w.lastOutLen = w.len()
	lastByte := w.bb.Bytes()[w.bb.Len()-1] // TODO change to   [w.lastOutLen-1]
	w.bb.Truncate(w.bb.Len() - 1)
	lastByte >>= w.nbSkippedBits
	w.lastNbSkippedBits = w.nbSkippedBits
	return w.bw.WriteBits(uint64(lastByte), 8-w.nbSkippedBits)
}

func (w *writer) Write(d []byte) (n int, err error) {
	return w.bb.Write(d)
}

func (w *writer) len() int {
	return w.bb.Len()
}

func (w *writer) tryWriteBits(v uint64, nbBits uint8) {
	w.bw.TryWriteBits(v, nbBits)
}

func (w *writer) tryWriteByte(b byte) {
	w.bw.TryWriteByte(b)
}

func (w *writer) tryError() error {
	return w.bw.TryError
}

func (w *writer) endSession() (err error) {
	w.nbSkippedBits, err = w.bw.Align()
	return
}

func (w *writer) reset() {
	w.bb.Reset()
	w.nbSkippedBits = 0
	w.lastOutLen = 0
	w.lastNbSkippedBits = 0
}

func (w *writer) bytes() []byte {
	return w.bb.Bytes()
}

func (w *writer) revert() {
	w.bb.Truncate(w.lastOutLen)
	w.nbSkippedBits = w.lastNbSkippedBits
}

func newBitWriter(size int) *writer {
	var res writer
	res.bb.Grow(size)
	res.bw = bitio.NewWriter(&res.bb)
	return &res
}
