package lzss

import (
	"bytes"
	"fmt"
	"math/bits"

	"github.com/consensys/compress"

	"github.com/consensys/compress/lzss/internal/suffixarray"
	"github.com/icza/bitio"
)

type Compressor struct {
	outBuf        bytes.Buffer
	bw            *bitio.Writer // invariant: bw cache must always be empty
	nbSkippedBits uint8

	inBuf bytes.Buffer

	// some records of the previous state, used for reverting
	lastOutLen        int
	lastNbSkippedBits uint8
	lastInLen         int
	justBypassed      bool

	inputIndex *suffixarray.Index
	inputSa    [MaxInputSize]int32 // suffix array space.

	dictData        []byte
	dictIndex       *suffixarray.Index
	dictSa          [MaxDictSize]int32 // suffix array space.
	dictReservedIdx map[byte]int

	level         Level
	intendedLevel Level
}

type Level uint8

const (
	NoCompression Level = 0
	// BestCompression allows the compressor to produce a stream of bit-level granularity,
	// giving the compressor this freedom helps it achieve better compression ratios but
	// will impose a high number of constraints on the SNARK decompressor
	BestCompression Level = 1

	GoodCompression        Level = 2
	GoodSnarkDecompression Level = 4

	// BestSnarkDecompression forces the compressor to produce byte-aligned output.
	// It is convenient and efficient for the SNARK decompressor but can hurt the compression ratio significantly
	BestSnarkDecompression Level = 8
)

const (
	headerBitLen        = 24
	longBrAddressNbBits = 19
)

// NewCompressor returns a new compressor with the given dictionary
// The dictionary is an unstructured sequence of substrings that are expected to occur frequently in the data. It is not included in the compressed data and should thus be a-priori known to both the compressor and the decompressor.
// The level determines the bit alignment of the compressed data. The "higher" the level, the better the compression ratio but the more constraints on the decompressor.
func NewCompressor(dict []byte, level Level) (*Compressor, error) {
	dict = AugmentDict(dict)
	if len(dict) > MaxDictSize {
		return nil, fmt.Errorf("dict size must be <= %d", MaxDictSize)
	}
	c := &Compressor{
		dictData:        dict,
		dictReservedIdx: make(map[byte]int),
	}

	// TODO @gbotrel cleanup
	found := uint8(0)
	const mask uint8 = 0b111
	for i, b := range dict {
		if b == SymbolDict {
			found |= 0b001
			c.dictReservedIdx[SymbolDict] = i
		} else if b == SymbolShort {
			found |= 0b010
			c.dictReservedIdx[SymbolShort] = i
		} else if b == SymbolLong {
			found |= 0b100
			c.dictReservedIdx[SymbolLong] = i
		} else {
			continue
		}
		if found == mask {
			break
		}
	}

	c.outBuf.Grow(MaxInputSize)
	c.inBuf.Grow(1 << longBrAddressNbBits)
	c.bw = bitio.NewWriter(&c.outBuf)
	if level != NoCompression {
		// if we don't compress we don't need the dict.
		c.dictIndex = suffixarray.New(c.dictData, c.dictSa[:len(c.dictData)])
	}
	c.intendedLevel = level
	c.Reset()
	return c, nil
}

// AugmentDict ensures the dictionary contains the special symbols
func AugmentDict(dict []byte) []byte {

	found := uint8(0)
	const mask uint8 = 0b111
	for _, b := range dict {
		if b == SymbolDict {
			found |= 0b001
		} else if b == SymbolShort {
			found |= 0b010
		} else if b == SymbolLong {
			found |= 0b100
		} else {
			continue
		}
		if found == mask {
			return dict
		}
	}

	return append(dict, SymbolDict, SymbolShort, SymbolLong)
}

func InitBackRefTypes(dictLen int, level Level) (short, long, dict BackrefType) {
	wordAlign := func(a int) uint8 {
		return (uint8(a) + uint8(level) - 1) / uint8(level) * uint8(level)
	}
	if level == NoCompression {
		wordAlign = func(a int) uint8 {
			return uint8(a)
		}
	}
	short = newBackRefType(SymbolShort, wordAlign(14), 8, false)
	long = newBackRefType(SymbolLong, wordAlign(longBrAddressNbBits), 8, false)
	dict = newBackRefType(SymbolDict, wordAlign(bits.Len(uint(dictLen))), 8, true)
	return
}

// The compressor cannot recover from a Write error. It must be Reset before writing again
func (compressor *Compressor) Write(d []byte) (n int, err error) {

	// reconstruct bit writer cache
	compressor.lastOutLen = compressor.outBuf.Len()
	lastByte := compressor.outBuf.Bytes()[compressor.outBuf.Len()-1]
	compressor.outBuf.Truncate(compressor.outBuf.Len() - 1)
	lastByte >>= compressor.nbSkippedBits
	if err = compressor.bw.WriteBits(uint64(lastByte), 8-compressor.nbSkippedBits); err != nil {
		return
	}

	compressor.lastNbSkippedBits = compressor.nbSkippedBits
	compressor.justBypassed = false
	if err = compressor.appendInput(d); err != nil {
		return
	}

	// write uncompressed data if compression is disabled
	if compressor.level == NoCompression {
		compressor.outBuf.Write(d)
		return len(d), nil
	}

	n, d = len(d), compressor.inBuf.Bytes()

	// initialize bit writer & backref types
	shortBackRefType, longBackRefType, dictBackRefType := InitBackRefTypes(len(compressor.dictData), compressor.level)

	// build the index
	compressor.inputIndex = suffixarray.New(d, compressor.inputSa[:len(d)])

	bDict := backref{bType: dictBackRefType, length: -1, address: -1}
	bShort := backref{bType: shortBackRefType, length: -1, address: -1}
	bLong := backref{bType: longBackRefType, length: -1, address: -1}

	fillBackrefs := func(i int, minLen int) bool {
		bDict.address, bDict.length = compressor.findBackRef(d, i, dictBackRefType, minLen)
		bShort.address, bShort.length = compressor.findBackRef(d, i, shortBackRefType, minLen)
		bLong.address, bLong.length = compressor.findBackRef(d, i, longBackRefType, minLen)
		return !(bDict.length == -1 && bShort.length == -1 && bLong.length == -1)
	}
	bestBackref := func() (backref, int) {
		if bDict.length != -1 && bDict.savings() > bShort.savings() && bDict.savings() > bLong.savings() {
			return bDict, bDict.savings()
		}
		if bShort.length != -1 && bShort.savings() > bLong.savings() {
			return bShort, bShort.savings()
		}
		return bLong, bLong.savings()
	}

	const minRepeatingBytes = 160
	for i := compressor.lastInLen; i < len(d); {
		// if we have a series of repeating bytes, we can have a special path for perf reasons.
		count := 0
		for i+count < len(d) && count <= 1<<8 && d[i] == d[i+count] {
			// no need to count after 1 << 8
			count++
		}
		if count >= minRepeatingBytes {
			// we have a series of repeating bytes
			// we write the symbol at i
			// and do a backref of length count-1 at i+1
			if i > 0 && d[i-1] == d[i] {
				// we don't need to write the symbol at i.
			} else {
				if !canEncodeSymbol(d[i]) {
					// we need to find a backref of len exactly 1. our dictionary should have it.
					bDict.address, bDict.length = compressor.dictReservedIdx[d[i]], 1
					bDict.writeTo(compressor.bw, i)
				} else {
					compressor.writeByte(d[i])
				}
				i++
				count--
			}

			if count <= shortBackRefType.maxLength {
				bShort.address = i - 1
				bShort.length = count
				bShort.writeTo(compressor.bw, i)
				i += count
				continue
			}
			if count > longBackRefType.maxLength {
				count = longBackRefType.maxLength
			}
			bLong.address = i - 1
			bLong.length = count
			bLong.writeTo(compressor.bw, i)
			i += count
			continue
		}

		if !canEncodeSymbol(d[i]) {
			// we must find a backref.
			if !fillBackrefs(i, 1) {
				// we didn't find a backref but can't write the symbol directly
				return i - compressor.lastInLen, fmt.Errorf("could not find a backref at index %d", i)
			}
			best, _ := bestBackref()
			best.writeTo(compressor.bw, i)
			i += best.length
			continue
		}
		if !fillBackrefs(i, -1) {
			// we didn't find a backref, let's write the symbol directly
			compressor.writeByte(d[i])
			i++
			continue
		}
		bestAtI, bestSavings := bestBackref() // todo @tabaie measure savings in bits not bytes

		if i+1 < len(d) {
			if fillBackrefs(i+1, bestAtI.length+1) {
				if newBest, newSavings := bestBackref(); newSavings > bestSavings {
					// we found a better backref at i+1
					compressor.writeByte(d[i])
					i++

					// then mark backref to be written at i+1
					bestSavings = newSavings
					bestAtI = newBest

					// can we find an even better backref at i+2 ?
					if canEncodeSymbol(d[i]) && i+1 < len(d) {
						if fillBackrefs(i+1, bestAtI.length+1) {
							// we found an even better backref
							if newBest, newSavings := bestBackref(); newSavings > bestSavings {
								compressor.writeByte(d[i])
								i++

								bestAtI = newBest
							}
						}
					}
				}
			} else if i+2 < len(d) && canEncodeSymbol(d[i+1]) {
				// maybe at i+2 ? (we already tried i+1)
				if fillBackrefs(i+2, bestAtI.length+2) {
					if newBest, newSavings := bestBackref(); newSavings > bestSavings {
						// we found a better backref
						// write the symbol at i
						compressor.writeByte(d[i])
						i++
						compressor.writeByte(d[i])
						i++

						// then emit the backref at i+2
						bestAtI = newBest
					}
				}
			}
		}

		bestAtI.writeTo(compressor.bw, i)
		i += bestAtI.length
	}

	if err = compressor.bw.TryError; err != nil {
		return
	}

	compressor.nbSkippedBits, err = compressor.bw.Align()
	return
}

func (compressor *Compressor) Reset() {
	compressor.level = compressor.intendedLevel
	compressor.outBuf.Reset()
	header := Header{
		Version: Version,
		Level:   compressor.level,
	}
	if _, err := header.WriteTo(&compressor.outBuf); err != nil {
		panic(err)
	}
	compressor.inBuf.Reset()
	compressor.lastOutLen = compressor.outBuf.Len()
	compressor.lastNbSkippedBits = 0
	compressor.justBypassed = false
	compressor.nbSkippedBits = 0
	compressor.lastInLen = 0
}

// Len returns the number of bytes compressed so far (includes the header)
func (compressor *Compressor) Len() int {
	return compressor.outBuf.Len()
}

// Written returns the number of bytes written to the compressor
func (compressor *Compressor) Written() int {
	return compressor.inBuf.Len()
}

// Revert undoes the last call to Write
// between any two calls to Revert, a call to Reset or Write should be made
func (compressor *Compressor) Revert() error {
	if compressor.lastInLen == -1 {
		return fmt.Errorf("cannot revert twice in a row")
	}

	compressor.inBuf.Truncate(compressor.lastInLen)
	compressor.lastInLen = -1

	if compressor.justBypassed {
		in := compressor.inBuf.Bytes()
		compressor.Reset()
		_, err := compressor.Write(in) // recompress everything. inefficient but 1) gets a better compression ratio and 2) this is not a common case
		return err
	} else {
		compressor.outBuf.Truncate(compressor.lastOutLen)
		compressor.nbSkippedBits = compressor.lastNbSkippedBits
		return nil
	}
}

// ConsiderBypassing switches to NoCompression if we get significant expansion instead of compression
func (compressor *Compressor) ConsiderBypassing() (bypassed bool) {

	if compressor.outBuf.Len() > compressor.inBuf.Len()+headerBitLen/8 {
		// compression was not worth it
		compressor.level = NoCompression
		compressor.nbSkippedBits = 0
		compressor.lastOutLen = compressor.lastInLen + headerBitLen/8
		compressor.lastNbSkippedBits = 0
		compressor.justBypassed = true
		compressor.outBuf.Reset()
		header := Header{Version: Version, Level: NoCompression}
		if _, err := header.WriteTo(&compressor.outBuf); err != nil {
			panic(err)
		}
		if _, err := compressor.outBuf.Write(compressor.inBuf.Bytes()); err != nil {
			panic(err)
		}
		return true
	}
	return false
}

// Bytes returns the compressed data
func (compressor *Compressor) Bytes() []byte {
	return compressor.outBuf.Bytes()
}

// Stream returns a stream of the compressed data
func (compressor *Compressor) Stream() compress.Stream {
	wordNbBits := uint8(compressor.level)
	if wordNbBits == 0 {
		wordNbBits = 8
	}

	res, err := compress.NewStream(compressor.outBuf.Bytes(), wordNbBits)
	if err != nil {
		panic(err)
	}

	return compress.Stream{
		D:       res.D[:(res.Len()-int(compressor.lastNbSkippedBits))/int(wordNbBits)],
		NbSymbs: res.NbSymbs,
	}
}

// Compress compresses the given data; if hint is provided, the compressor will try to use it
// hint should be a subset of the data compressed by the same compressor
// For example, calling Compress([]byte{1, 2, 3, 4, 5}, compressed([]byte{1, 2, 3})) will
// result in much faster compression than calling Compress([]byte{1, 2, 3, 4, 5})
func (compressor *Compressor) Compress(d []byte) (c []byte, err error) {
	compressor.Reset()
	_, err = compressor.Write(d)
	return compressor.Bytes(), err
}

// canEncodeSymbol returns true if the symbol can be encoded directly
func canEncodeSymbol(b byte) bool {
	return b != SymbolDict && b != SymbolShort && b != SymbolLong
}

func (compressor *Compressor) writeByte(b byte) {
	if !canEncodeSymbol(b) {
		panic("cannot encode symbol")
	}
	compressor.bw.TryWriteByte(b)
}

// findBackRef attempts to find a backref in the window [i-brAddressRange, i+brLengthRange]
// if no backref is found, it returns -1, -1
// else returns the address and length of the backref
func (compressor *Compressor) findBackRef(data []byte, i int, bType BackrefType, minLength int) (addr, length int) {
	if minLength == -1 {
		minLength = bType.nbBytesBackRef
	}

	if i+minLength > len(data) {
		return -1, -1
	}

	windowStart := max(0, i-bType.maxAddress)
	maxRefLen := bType.maxLength

	if i+maxRefLen > len(data) {
		maxRefLen = len(data) - i
	}

	if minLength > maxRefLen {
		return -1, -1
	}

	if bType.dictOnly {
		return compressor.dictIndex.LookupLongest(data[i:i+maxRefLen], minLength, maxRefLen, 0, len(compressor.dictData))
	}

	return compressor.inputIndex.LookupLongest(data[i:i+maxRefLen], minLength, maxRefLen, windowStart, i)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (compressor *Compressor) appendInput(d []byte) error {
	if compressor.inBuf.Len()+len(d) > MaxInputSize {
		return fmt.Errorf("input size must be <= %d", MaxInputSize)
	}
	compressor.lastInLen = compressor.inBuf.Len()
	compressor.inBuf.Write(d)
	return nil
}
