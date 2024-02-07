package lzss

import (
	"math"

	"github.com/icza/bitio"
)

const (
	MaxInputSize = 1 << 21 // 2Mb
	MaxDictSize  = 1 << 22 // 4Mb
)

type BackrefType struct {
	Delimiter      byte
	NbBitsAddress  uint8
	NbBitsLength   uint8
	NbBitsBackRef  uint8
	nbBytesBackRef int
	maxAddress     int
	maxLength      int
	dictOnly       bool
}

func newBackRefType(symbol byte, nbBitsAddress, nbBitsLength uint8, dictOnly bool) BackrefType {
	return BackrefType{
		Delimiter:      symbol,
		NbBitsAddress:  nbBitsAddress,
		NbBitsLength:   nbBitsLength,
		NbBitsBackRef:  8 + nbBitsAddress + nbBitsLength,
		nbBytesBackRef: int(8+nbBitsAddress+nbBitsLength+7) / 8,
		maxAddress:     1 << nbBitsAddress,
		maxLength:      1 << nbBitsLength,
		dictOnly:       dictOnly,
	}
}

const (
	SymbolDict  = 0xFF
	SymbolShort = 0xFE
	SymbolLong  = 0xFD
)

type backref struct {
	address int
	length  int
	bType   BackrefType
}

// Warning; writeTo and readFrom are not symmetrical

func (b *backref) writeTo(w bitWriter, i int) {
	w.tryWriteByte(b.bType.Delimiter)
	w.tryWriteBits(uint64(b.length-1), b.bType.NbBitsLength)
	addrToWrite := b.address
	if !b.bType.dictOnly {
		addrToWrite = i - b.address - 1
	}
	w.tryWriteBits(uint64(addrToWrite), b.bType.NbBitsAddress)
}

func (b *backref) readFrom(r *bitio.Reader) {
	n := r.TryReadBits(b.bType.NbBitsLength)
	b.length = int(n) + 1

	n = r.TryReadBits(b.bType.NbBitsAddress)
	b.address = int(n)
	if !b.bType.dictOnly {
		b.address++
	}
}

func (b *backref) savings() int {
	if b.length == -1 {
		return math.MinInt // -1 is a special value
	}
	return 8*b.length - int(b.bType.NbBitsBackRef)
}
