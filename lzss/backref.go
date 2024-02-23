package lzss

import (
	"fmt"
	"math"
	"math/bits"

	"github.com/icza/bitio"
)

const (
	MaxInputSize = 1 << 22 // 4Mb
	MaxDictSize  = 1 << 22 // 4Mb
)

const (
	SymbolDynamic     byte = 0xFF
	SymbolShort       byte = 0xFE
	maxBackrefLenLog2      = 8  // max length of a backref in bytes (1 << 8 = 256 bytes)
	shortAddrBits          = 14 // number of bits to encode the address in a short backref
)

type BackrefType struct {
	Delimiter      byte
	NbBitsAddress  uint8
	NbBitsLength   uint8
	NbBitsBackRef  uint8
	nbBytesBackRef int
	maxAddress     int
	maxLength      int
	dictLen        int
}

func NewShortBackrefType(level Level) (short BackrefType) {
	wordAlign := func(a int) uint8 {
		return (uint8(a) + uint8(level) - 1) / uint8(level) * uint8(level)
	}
	if level == NoCompression {
		wordAlign = func(a int) uint8 {
			return uint8(a)
		}
	}
	short = newBackRefType(SymbolShort, wordAlign(shortAddrBits), maxBackrefLenLog2, 0)
	return
}

func NewDynamicBackrefType(dictLen, addressableBytes int, level Level) (dynamic BackrefType) {
	wordAlign := func(a int) uint8 {
		return (uint8(a) + uint8(level) - 1) / uint8(level) * uint8(level)
	}
	if level == NoCompression {
		wordAlign = func(a int) uint8 {
			return uint8(a)
		}
	}
	bound := bits.Len(uint(addressableBytes + dictLen))
	return newBackRefType(SymbolDynamic, wordAlign(bound), maxBackrefLenLog2, dictLen)
}

func newBackRefType(symbol byte, nbBitsAddress, nbBitsLength uint8, dictLen int) BackrefType {
	return BackrefType{
		Delimiter:      symbol,
		NbBitsAddress:  nbBitsAddress,
		NbBitsLength:   nbBitsLength,
		NbBitsBackRef:  8 + nbBitsAddress + nbBitsLength,
		nbBytesBackRef: int(8+nbBitsAddress+nbBitsLength+7) / 8,
		maxAddress:     1 << nbBitsAddress,
		maxLength:      1 << nbBitsLength,
		dictLen:        dictLen,
	}
}

type backref struct {
	address int
	length  int
	bType   BackrefType
}

// Warning; writeTo and readFrom are not symmetrical

func (b *backref) writeTo(w writer, i int) {
	w.TryWriteByte(b.bType.Delimiter)
	w.TryWriteBits(uint64(b.length-1), b.bType.NbBitsLength)
	addrToWrite := (i + b.bType.dictLen) - b.address - 1
	w.TryWriteBits(uint64(addrToWrite), b.bType.NbBitsAddress)
}

func (b *backref) readFrom(r *bitio.Reader) error {
	n := r.TryReadBits(b.bType.NbBitsLength)
	b.length = int(n) + 1

	n = r.TryReadBits(b.bType.NbBitsAddress)
	b.address = int(n) + 1

	if r.TryError != nil {
		return r.TryError
	}

	if b.length <= 0 || b.address < 0 {
		return fmt.Errorf("invalid back reference: %v", b)
	}

	return nil
}

func (b *backref) savings() int {
	if b.length == -1 {
		return math.MinInt // -1 is a special value
	}
	return 8*b.length - int(b.bType.NbBitsBackRef)
}
