package compress

import (
	"bytes"
	"errors"
	"github.com/icza/bitio"
	"hash"
)

// todo @tabaie consider out-scoping NbSymbs > 256 and changing D's type to []byte
// todo @tabaie consider requiring NbSymbs to be a power of 2 and using BitsPerSymb instead of NbSymbs

// Stream is an inefficient data structure used for easy experimentation with compression algorithms.
type Stream struct {
	D       []int
	NbSymbs int
}

func (s *Stream) Len() int {
	return len(s.D)
}

func (s *Stream) RunLen(i int) int {
	runLen := 1
	for i+runLen < len(s.D) && s.D[i+runLen] == 0 {
		runLen++
	}
	return runLen
}

func (s *Stream) At(i int) int {
	return s.D[i]
}

func NewStream(in []byte, bitsPerSymbol uint8) (Stream, error) {
	var s Stream
	err := s.New(in, bitsPerSymbol)
	return s, err
}

// New reads a stream from a byte slice, with the number of bits per symbol specified. As opposed to ReadBytes, it attempts to exhaust the input.
func (s *Stream) New(in []byte, bitsPerSymbol uint8) error {
	n := len(in) * 8 / int(bitsPerSymbol)
	s.NbSymbs = 1 << bitsPerSymbol
	if cap(s.D) < n {
		s.D = make([]int, 0, n)
	}
	s.D = s.D[:0]
	_, err := s.Write(in)
	return err
}

// Write in accordance with the io.Writer interface
// todo api @tabaie reconcile which perspective the words "read" and "write" are used from. this one is in contrast with that of ReadBytes
func (s *Stream) Write(p []byte) (n int, err error) {
	bitsPerSymb := uint8(bitLen(s.NbSymbs))
	toRead := len(p) * 8 / int(bitsPerSymb)
	r := bitio.NewReader(bytes.NewReader(p))
	for i := 0; i < toRead; i++ {
		var x uint64
		if x, err = r.ReadBits(bitsPerSymb); err != nil {
			return (i*int(bitsPerSymb) + 7) / 8, err // counting the last partial byte
		}
		s.D = append(s.D, int(x))
	}
	return len(p), nil // counting the last partial byte
}

func (s *Stream) Reset() {
	s.D = s.D[:0]
}

func (s *Stream) BreakUp(nbSymbs int) Stream {
	newPerOld := log(s.NbSymbs, nbSymbs)
	d := make([]int, len(s.D)*newPerOld)

	for i := range s.D {
		v := s.D[i]
		for j := 0; j < newPerOld; j++ {
			d[(i+1)*newPerOld-j-1] = v % nbSymbs
			v /= nbSymbs
		}
	}

	return Stream{d, nbSymbs}
}

func (s *Stream) ToBytes(nbBits int) ([]byte, error) {
	res := make([]byte, StreamSerializedSize(len(s.D), bitLen(s.NbSymbs), nbBits))
	err := s.FillBytes(res, nbBits)
	return res, err
}

func StreamSerializedSize(nbWords, wordNbBits, nbBits int) int {
	wordsForLen := (31 + wordNbBits) / wordNbBits
	bytesPerElem := (nbBits + 7) / 8
	wordsPerElemHeadroom := (bytesPerElem*8 - nbBits + wordNbBits - 1) / wordNbBits
	wordsPerElem := (nbBits+wordNbBits-1)/wordNbBits - wordsPerElemHeadroom
	nbElems := (wordsForLen + nbWords + wordsPerElem - 1) / wordsPerElem
	return nbElems * bytesPerElem
}

type bytesWriter struct {
	i int
	b []byte
}

func (b *bytesWriter) Write(p []byte) (n int, err error) {
	if b.i+len(p) > len(b.b) {
		return 0, errors.New("not enough room in dst")
	}
	copy(b.b[b.i:], p)
	b.i += len(p)
	return len(p), nil
}

// FillBytes aligns the stream first according to "field elements" of length nbBits, and then aligns the field elements to bytes
func (s *Stream) FillBytes(dst []byte, nbBits int) error {
	bitsPerWord := bitLen(s.NbSymbs)

	if bitsPerWord >= nbBits {
		return errors.New("words do not fit in elements")
	}

	wordsForNb := (31 + bitsPerWord) / bitsPerWord
	wordsPerElem := (nbBits - 1) / bitsPerWord
	nbElems := (len(s.D) + wordsForNb + wordsPerElem - 1) / wordsPerElem
	bytesPerElem := (nbBits + 7) / 8
	headroomBitsPerElem := uint8(bytesPerElem*8 - bitsPerWord*wordsPerElem)

	if len(dst) < StreamSerializedSize(len(s.D), bitsPerWord, nbBits) {
		return errors.New("not enough room in dst")
	}

	num := make([]int, wordsForNb)
	{ // WriteNum type operation. TODO @tabaie refactor into its own function
		x := len(s.D)
		for i := wordsForNb - 1; i >= 0; i-- {
			num[i] = x % s.NbSymbs
			x /= s.NbSymbs
		}
		if x != 0 {
			return errors.New("writeNum overflow")
		}
	}

	dAt := func(i int) int64 {
		if i < 0 {
			return int64(num[i+wordsForNb])
		}
		return int64(s.D[i])
	}

	w := bitio.NewWriter(&bytesWriter{0, dst})

	for i := 0; i < nbElems; i++ {
		w.TryWriteBits(0, headroomBitsPerElem)
		for j := 0; j < wordsPerElem; j++ {
			absJ := i*wordsPerElem + j - wordsForNb
			if absJ >= len(s.D) {
				break
			}
			w.TryWriteBits(uint64(dAt(absJ)), uint8(bitsPerWord))
		}
	}
	w.TryAlign()
	return w.TryError
}

// ReadBytes first reads elements of length nbBits in a byte-aligned manner, and then reads the elements into the stream
func (s *Stream) ReadBytes(src []byte, nbBits int) error {
	bitsPerWord := bitLen(s.NbSymbs)

	if bitsPerWord >= nbBits {
		return errors.New("words do not fit in elements")
	}

	if s.NbSymbs != 1<<bitsPerWord {
		return errors.New("only powers of 2 currently supported for NbSymbs")
	}

	wordsForNb := (31 + bitsPerWord) / bitsPerWord
	wordsPerElem := (nbBits - 1) / bitsPerWord
	nbElems := (len(src)*8 + nbBits - 1) / nbBits
	bytesPerElem := (nbBits + 7) / 8
	headroomBitsPerElem := uint8(bytesPerElem*8 - bitsPerWord*wordsPerElem)

	w := bitio.NewReader(bytes.NewReader(src))
	sDLarge := s.D
	if len(s.D) < wordsForNb {
		s.D = make([]int, wordsForNb)
		sDLarge = s.D
	}

	for i := 0; i < nbElems && i < (len(s.D)+wordsPerElem-1)/wordsPerElem; i++ {
		if x := w.TryReadBits(headroomBitsPerElem); x != 0 {
			return errors.New("headroom bits not zero")
		}
		for j := 0; j < wordsPerElem; j++ {
			wordI := i*wordsPerElem + j

			if wordI == wordsForNb {
				_len := 0
				for k := 0; k < wordsForNb; k++ {
					_len *= s.NbSymbs
					_len += s.D[k]
				}
				s.D = sDLarge
				s.resize(_len)
			}

			if wordI >= wordsForNb {
				wordI -= wordsForNb
			}

			if wordI >= len(s.D) {
				continue
			}
			s.D[wordI] = int(w.TryReadBits(uint8(bitsPerWord)))
		}
	}

	if nbElems < (len(s.D)+wordsForNb+wordsPerElem-1)/wordsPerElem {
		return errors.New("insufficient bytes")
	}

	return w.TryError
}

func (s *Stream) resize(_len int) {
	if len(s.D) < _len {
		s.D = make([]int, _len)
	}
	s.D = s.D[:_len]
}

func log(x, base int) int {
	exp := 0
	for pow := 1; pow < x; pow *= base {
		exp++
	}
	return exp
}

func (s *Stream) Checksum(hsh hash.Hash, fieldBits int) ([]byte, error) {
	packed, err := s.ToBytes(fieldBits)
	if err != nil {
		return nil, err
	}
	fieldBytes := (fieldBits + 7) / 8
	for i := 0; i < len(packed); i += fieldBytes {
		hsh.Write(packed[i : i+fieldBytes])
	}

	return hsh.Sum(nil), err
}

func (s *Stream) WriteNum(r int, nbWords int) *Stream {
	for i := 0; i < nbWords; i++ {
		s.D = append(s.D, r%s.NbSymbs)
		r /= s.NbSymbs
	}
	if r != 0 {
		panic("overflow")
	}
	return s
}

func (s *Stream) ReadNum(start, nbWords int) int {
	res := 0
	for j := nbWords - 1; j >= 0; j-- {
		res *= s.NbSymbs
		res += s.D[start+j]
	}
	return res
}

func bitLen(n int) int {
	bitLen := 0
	for 1<<bitLen < n {
		bitLen++
	}
	return bitLen
}

// ContentToBytes writes the CONTENT of the stream to a byte slice, with no metadata about the size of the stream or the number of symbols.
// it mainly serves testing purposes so in case of a write error it panics.
func (s *Stream) ContentToBytes() []byte {
	bitsPerWord := bitLen(s.NbSymbs)

	nbBytes := (len(s.D)*bitsPerWord + 7) / 8
	bb := bytes.NewBuffer(make([]byte, 0, nbBytes))

	w := bitio.NewWriter(bb)
	for i := range s.D {
		w.TryWriteBits(uint64(s.D[i]), uint8(bitsPerWord))
	}
	if w.TryError != nil {
		panic(w.TryError)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}

	return bb.Bytes()
}

// Concat replaces the content of the current stream with the concatenation of the given streams.
func (s *Stream) Concat(a ...Stream) error {
	if len(a) == 0 {
		s.D = nil
		return nil
	}

	s.NbSymbs = a[0].NbSymbs
	_len := 0
	for _, v := range a {
		_len += len(v.D)

	}
	if cap(s.D) < _len {
		s.D = make([]int, 0, _len)
	}
	s.D = s.D[:0]
	for _, v := range a {
		if err := s.Append(v); err != nil {
			return err
		}
	}
	return nil
}

func (s *Stream) Append(a Stream) error {
	if a.NbSymbs != s.NbSymbs {
		return errors.New("streams must have the same number of symbols")
	}
	s.D = append(s.D, a.D...)
	return nil
}
