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
	delimiter      byte
	NbBitsAddress  uint8
	nbBitsLength   uint8
	NbBitsBackRef  uint8
	nbBytesBackRef int
	maxAddress     int
	maxLength      int
	dictOnly       bool
}

func newBackRefType(symbol byte, nbBitsAddress, nbBitsLength uint8, dictOnly bool) BackrefType {
	return BackrefType{
		delimiter:      symbol,
		NbBitsAddress:  nbBitsAddress,
		nbBitsLength:   nbBitsLength,
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

func (b *backref) writeTo(w *bitio.Writer, i int) {
	w.TryWriteByte(b.bType.delimiter)
	w.TryWriteBits(uint64(b.length-1), b.bType.nbBitsLength)
	addrToWrite := b.address
	if !b.bType.dictOnly {
		addrToWrite = i - b.address - 1
	}
	w.TryWriteBits(uint64(addrToWrite), b.bType.NbBitsAddress)
}

func (b *backref) readFrom(r *bitio.Reader) {
	n := r.TryReadBits(b.bType.nbBitsLength)
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
	return b.length - b.bType.nbBytesBackRef
}
