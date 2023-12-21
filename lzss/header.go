package lzss

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
)

const (
	// Version is the current release version of the compressor.
	Version = 0
)

// Header is the header of a compressed data.
// It contains the compressor release version and the compression level.
type Header struct {
	Version byte   // compressor release version
	Level   Level  // compression level
	Extra   []byte // "extra data", max len == math.MaxUint16
}

func (s *Header) WriteTo(w io.Writer) (int64, error) {
	l := len(s.Extra)
	if l > math.MaxUint16 {
		return 0, errors.New("extra data too long")
	}

	if _, err := w.Write([]byte{s.Version, byte(s.Level)}); err != nil {
		return 0, err
	}

	if err := binary.Write(w, binary.LittleEndian, uint16(l)); err != nil {
		return 2, err
	}
	if l > 0 {
		if _, err := w.Write(s.Extra); err != nil {
			return 4, err
		}
	}

	return 4 + int64(l), nil
}

func (s *Header) ReadFrom(r io.Reader) (int64, error) {
	var b [4]byte
	n, err := io.ReadFull(r, b[:])
	if err != nil {
		return int64(n), err
	}

	s.Version = b[0]
	s.Level = Level(b[1])
	l := binary.LittleEndian.Uint16(b[2:])
	if l > 0 {
		s.Extra = make([]byte, l)
		n2, err := io.ReadFull(r, s.Extra)
		n += int(n2)
		if err != nil {
			return int64(n), err
		}
	}
	return int64(n), nil
}
