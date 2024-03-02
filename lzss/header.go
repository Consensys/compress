package lzss

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	// Version is the current release version of the compressor.
	Version    = 1
	HeaderSize = 3
)

// Header is the header of a compressed data.
// It contains the compressor release version and the compression level.
type Header struct {
	Version       uint16 // compressor release version
	NoCompression bool
}

func (s *Header) WriteTo(w io.Writer) (int64, error) {
	if err := binary.Write(w, binary.BigEndian, uint16(s.Version)); err != nil {
		return 0, err
	}

	if _, err := w.Write([]byte{ind(s.NoCompression)}); err != nil {
		return 2, err
	}

	return HeaderSize, nil
}

func (s *Header) ReadFrom(r io.Reader) (int64, error) {
	var b [HeaderSize]byte
	n, err := io.ReadFull(r, b[:])
	if err != nil {
		return int64(n), err
	}

	s.Version = binary.BigEndian.Uint16(b[:2])
	s.NoCompression, err = indInv(b[2])
	return int64(n), err
}

// ind indicator function
func ind(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// indInv is inverse to ind
func indInv(b byte) (bool, error) {
	if b == 0 {
		return false, nil
	}
	if b == 1 {
		return true, nil
	}
	return false, errors.New("expected 0 or 1")
}
