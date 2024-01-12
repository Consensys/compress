package compress

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash"
	"math/big"

	"github.com/icza/bitio"
)

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
	d := make([]int, len(in)*8/int(bitsPerSymbol))
	r := bitio.NewReader(bytes.NewReader(in))
	for i := range d {
		if n, err := r.ReadBits(bitsPerSymbol); err != nil {
			return Stream{}, err
		} else {
			d[i] = int(n)
		}
	}
	return Stream{d, 1 << int(bitsPerSymbol)}, nil
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

// todo @tabaie too many copy pastes in the next three funcs

func (s *Stream) Pack(nbBits int) []*big.Int {
	wordLen := bitLen(s.NbSymbs)
	wordsPerElem := (nbBits - 1) / wordLen

	var radix big.Int
	radix.Lsh(big.NewInt(1), uint(wordLen))

	packed := make([]*big.Int, (len(s.D)+wordsPerElem-1)/wordsPerElem)
	for i := range packed {
		packed[i] = new(big.Int)
		for j := wordsPerElem - 1; j >= 0; j-- {
			absJ := i*wordsPerElem + j
			if absJ >= len(s.D) {
				continue
			}
			packed[i].Mul(packed[i], &radix).Add(packed[i], big.NewInt(int64(s.D[absJ])))
		}
	}
	return packed
}

func (s *Stream) FillBytes(bytes []byte, nbBits int) error {
	bitsPerWord := bitLen(s.NbSymbs)
	wordsPerElem := (nbBits - 1) / bitsPerWord
	bytesPerElem := (nbBits + 7) / 8

	if len(bytes) < (len(s.D)*bitsPerWord+7)/8+4 {
		return errors.New("not enough room in bytes")
	}

	binary.BigEndian.PutUint32(bytes[:4], uint32(len(s.D)))
	bytes = bytes[4:]

	var radix, elem big.Int // todo @tabaie all this big.Int business seems unnecessary. try using bitio instead?
	radix.Lsh(big.NewInt(1), uint(bitsPerWord))

	for i := 0; i < len(bytes) && i*wordsPerElem < len(s.D); i += bytesPerElem {
		elem.SetInt64(0)
		for j := wordsPerElem - 1; j >= 0; j-- {
			absJ := i*wordsPerElem + j
			if absJ >= len(s.D) {
				continue
			}
			elem.Mul(&elem, &radix).Add(&elem, big.NewInt(int64(s.D[absJ])))
		}
		elem.FillBytes(bytes[i : i+bytesPerElem])
	}
	return nil
}

func (s *Stream) ReadBytes(byts []byte, nbBits int) error {
	bitsPerWord := bitLen(s.NbSymbs)

	if s.NbSymbs != 1<<bitsPerWord {
		return errors.New("only powers of 2 currently supported for NbSymbs")
	}

	s.resize(int(binary.BigEndian.Uint32(byts[:4])))
	byts = byts[4:]

	wordsPerElem := (nbBits - 1) / bitsPerWord
	bytesPerElem := (nbBits + 7) / 8
	nbElems := (len(s.D) + wordsPerElem - 1) / wordsPerElem

	if len(byts) < nbElems*bytesPerElem {
		return errors.New("not enough bytes")
	}

	w := bitio.NewReader(bytes.NewReader(byts))

	for i := 0; i < nbElems; i++ {
		w.TryReadBits(uint8(8*bytesPerElem - bitsPerWord*wordsPerElem))
		if i+1 == nbElems {
			wordsToRead := len(s.D) - i*wordsPerElem
			w.TryReadBits(uint8((wordsPerElem - wordsToRead) * bitsPerWord)) // skip unused bits
		}
		for j := 0; j < wordsPerElem; j++ {
			wordI := i*wordsPerElem + j
			if wordI >= len(s.D) {
				break
			}
			s.D[wordI] = int(w.TryReadBits(uint8(bitsPerWord)))
		}
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

func (s *Stream) Checksum(hsh hash.Hash, fieldBits int) []byte {
	packed := s.Pack(fieldBits)
	fieldBytes := (fieldBits + 7) / 8
	byts := make([]byte, fieldBytes)
	for _, w := range packed {
		w.FillBytes(byts)
		hsh.Write(byts)
	}

	length := make([]byte, fieldBytes)
	big.NewInt(int64(s.Len())).FillBytes(length)
	hsh.Write(length)

	return hsh.Sum(nil)
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

func (s *Stream) Marshal() []byte {
	wordLen := bitLen(s.NbSymbs)

	nbBytes := (len(s.D)*wordLen + 7) / 8
	encodeLen := false
	if s.NbSymbs <= 128 {
		nbBytes++
		encodeLen = true
	}
	bb := bytes.NewBuffer(make([]byte, 0, nbBytes))

	w := bitio.NewWriter(bb)
	for i := range s.D {
		if err := w.WriteBits(uint64(s.D[i]), uint8(wordLen)); err != nil {
			panic(err)
		}
	}
	if err := w.Close(); err != nil {
		panic(err)
	}

	if encodeLen {
		nbWordsInLastByte := len(s.D) - ((nbBytes-2)*8+wordLen-1)/wordLen
		bb.WriteByte(byte(nbWordsInLastByte))
	}

	return bb.Bytes()
}

func (s *Stream) Unmarshal(b []byte) *Stream {
	wordLen := bitLen(s.NbSymbs)

	var nbWords int
	if s.NbSymbs <= 128 {
		nbWordsNotEntirelyInLastByte := ((len(b)-2)*8 + wordLen - 1) / wordLen
		nbWords = nbWordsNotEntirelyInLastByte + int(b[len(b)-1])
		b = b[:len(b)-1]
	} else {
		nbWords = (len(b) * 8) / wordLen
	}

	if cap(s.D) < nbWords {
		s.D = make([]int, nbWords)
	}
	s.D = s.D[:nbWords]

	r := bitio.NewReader(bytes.NewReader(b))
	for i := range s.D {
		if n, err := r.ReadBits(uint8(wordLen)); err != nil {
			panic(err)
		} else {
			s.D[i] = int(n)
		}
	}

	return s
}
