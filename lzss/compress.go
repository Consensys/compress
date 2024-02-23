package lzss

import (
	"bytes"
	"fmt"

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
	dictReservedIdx map[byte]int       // stores the index of the reserved symbols in the dictionary

	level         Level
	intendedLevel Level
}

type Level uint8

const (
	NoCompression   Level = 0
	BestCompression Level = 1
)

// NewCompressor returns a new compressor with the given dictionary
// The dictionary is an unstructured sequence of substrings that are expected to occur frequently in the data. It is not included in the compressed data and should thus be a-priori known to both the compressor and the decompressor.
// The level determines the bit alignment of the compressed data. The "higher" the level, the better the compression ratio but the more constraints on the decompressor.
func NewCompressor(dict []byte) (*Compressor, error) {
	dict = AugmentDict(dict)
	if len(dict) > MaxDictSize {
		return nil, fmt.Errorf("dict size must be <= %d", MaxDictSize)
	}
	c := &Compressor{
		dictData:        dict,
		dictReservedIdx: make(map[byte]int),
	}

	// find the reserved symbols in the dictionary
	for i, b := range dict {
		if b == SymbolDynamic {
			c.dictReservedIdx[SymbolDynamic] = i
		} else if b == SymbolShort {
			c.dictReservedIdx[SymbolShort] = i
		} else {
			continue
		}
		if len(c.dictReservedIdx) == 2 {
			break
		}
	}

	c.outBuf.Grow(MaxInputSize)
	c.inBuf.Grow(1 << 19)
	c.bw = bitio.NewWriter(&c.outBuf)
	c.intendedLevel = BestCompression
	c.dictIndex = suffixarray.New(c.dictData, c.dictSa[:len(c.dictData)])
	c.Reset()
	return c, nil
}

// AugmentDict ensures the dictionary contains the special symbols
func AugmentDict(dict []byte) []byte {

	found := uint8(0)
	const mask uint8 = 0b110
	for _, b := range dict {
		if b == SymbolShort {
			found |= 0b010
		} else if b == SymbolDynamic {
			found |= 0b100
		} else {
			continue
		}
		if found == mask {
			return dict
		}
	}

	return append(dict, SymbolShort, SymbolDynamic)
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

	d = compressor.inBuf.Bytes()

	// build the index
	compressor.inputIndex = suffixarray.New(d, compressor.inputSa[:len(d)])

	n, err = compressor.write(compressor.bw, d, compressor.lastInLen, compressor.inputIndex)
	if err != nil {
		return
	}

	if err = compressor.bw.TryError; err != nil {
		return
	}

	compressor.nbSkippedBits, err = compressor.bw.Align()
	return
}

type writer interface {
	TryWriteBits(v uint64, nbBits uint8)
	TryWriteByte(b byte)
}

// write compresses the data and writes it to the writer
// note that this is meant to be stateless and not modify the compressor object.
func (compressor *Compressor) write(w writer, d []byte, startIndex int, inputIndex *suffixarray.Index) (n int, err error) {
	dictLen := len(compressor.dictData)

	shortType := NewShortBackrefType()

	// we use a circular buffer to store the last 3 backrefs
	cb := newCircularBuffer()

	bestBackref := func(at int) (backref, int) {
		if b, ok := cb.best(at); ok {
			return b, b.savings()
		}

		bDynamic := backref{bType: NewDynamicBackrefType(dictLen, at), length: -1, address: -1}
		bShort := backref{bType: shortType, length: -1, address: -1}

		// we haven't computed the backref yet
		minLen := -1
		if !canEncodeSymbol(d[at]) {
			minLen = 1
		}

		bShort.address, bShort.length = findBackRef(d, at, shortType, minLen, inputIndex, compressor.dictIndex, dictLen)
		bDynamic.address, bDynamic.length = findBackRef(d, at, bDynamic.bType, minLen, inputIndex, compressor.dictIndex, dictLen)

		// we store the best backref in the circular buffer
		var bestAtI backref
		if bShort.length != -1 && bShort.savings() > bDynamic.savings() {
			bestAtI = bShort
		} else {
			bestAtI = bDynamic
		}

		cb.push(bestAtI, at)
		return bestAtI, bestAtI.savings()
	}

	const minRepeatingBytes = 160
	for i := startIndex; i < len(d); {
		// if we have a series of repeating bytes, we can do "RLE" using a short backref
		// note that since all our backref have max len of (1<<maxBackrefLenLog2)
		// we stop if we have a series of repeating bytes of length (1<<maxBackrefLenLog2)
		count := 0
		for i+count < len(d) && count < (1<<maxBackrefLenLog2) && d[i] == d[i+count] {
			count++
		}
		if count >= minRepeatingBytes {
			// we have a series of repeating bytes which would make a reasonable backref
			// let's use this path for perf reasons.

			// first, we need to ensure the previous byte is the same to have the start point for the backref

			// we write the symbol at i
			if !(i > 0 && d[i-1] == d[i]) {
				if !canEncodeSymbol(d[i]) {
					// if this is a reserved symbol, it should be in the dictionary
					// (this is a backref with len(1))
					bDict := backref{
						bType:   NewDynamicBackrefType(dictLen, i),
						address: compressor.dictReservedIdx[d[i]],
						length:  1,
					}
					bDict.writeTo(w, i)
				} else {
					w.TryWriteByte(d[i])
				}
				i++
				count--
				// we can now do a backref of length count-1 at i+1
			} // else --> we do a backref of length count at i

			bShort := backref{bType: shortType, address: i - 1, length: count}
			bDynamic := backref{bType: NewDynamicBackrefType(dictLen, i), address: dictLen + i - 1, length: count}
			if bShort.savings() > bDynamic.savings() {
				bShort.writeTo(w, i)
			} else {
				bDynamic.writeTo(w, i)
			}
			i += count
			continue
		}

		bestAtI, bestSavings := bestBackref(i)
		if !canEncodeSymbol(d[i]) {
			// at minima, we have a backref of length 1 in the dictionary
			bestAtI.writeTo(w, i)
			i += bestAtI.length
			continue
		}
		if bestSavings < 0 {
			// we didn't find a backref, let's write the symbol directly
			w.TryWriteByte(d[i])
			i++
			continue
		}

		// for the next few bytes, we will try to find a better backref
		if i+1 < len(d) {
			if _, newSavings := bestBackref(i + 1); newSavings > bestSavings+1 {
				// we found a better backref at i+1
				w.TryWriteByte(d[i])
				i++
				continue
			}
		}
		if i+2 < len(d) && canEncodeSymbol(d[i+1]) {
			// maybe at i+2 ? (we already tried i+1)
			if _, newSavings := bestBackref(i + 2); newSavings > bestSavings+2 {
				// we found a better backref
				// write the symbol at i and i+1
				w.TryWriteByte(d[i])
				w.TryWriteByte(d[i+1])
				i += 2
				continue
			}
		}

		bestAtI.writeTo(w, i)
		i += bestAtI.length
	}

	return len(d) - startIndex, nil
}

const circularBufferSize = 3

type circularBuffer struct {
	k           int
	keys        [circularBufferSize]int
	bestBackref [circularBufferSize]backref
}

func newCircularBuffer() *circularBuffer {
	return &circularBuffer{keys: [circularBufferSize]int{-1, -1, -1}}
}

func (cb *circularBuffer) push(b backref, at int) {
	cb.keys[cb.k] = at
	cb.bestBackref[cb.k] = b
	cb.k = (cb.k + 1) % circularBufferSize
}

func (cb *circularBuffer) best(at int) (backref, bool) {
	for i := 0; i < circularBufferSize; i++ {
		if cb.keys[i] == at {
			return cb.bestBackref[i], true
		}
	}
	return backref{}, false
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

// WrittenBytes returns the bytes written to the compressor
// This returns a pointer to the internal buffer, so it should not be modified
func (compressor *Compressor) WrittenBytes() []byte {
	return compressor.inBuf.Bytes()
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

	if compressor.outBuf.Len() > compressor.inBuf.Len()+HeaderSize {
		// compression was not worth it
		compressor.level = NoCompression
		compressor.nbSkippedBits = 0
		compressor.lastOutLen = compressor.lastInLen + HeaderSize
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

// Compress compresses the given data and returns the compressed data
func (compressor *Compressor) Compress(d []byte) (c []byte, err error) {
	compressor.Reset()
	_, err = compressor.Write(d)
	return compressor.Bytes(), err
}

// CompressedSize256k returns the size of the compressed data
// This is state less and thread-safe (but other methods are not)
// Max size of d is 256kB
func (compressor *Compressor) CompressedSize256k(d []byte) (size int, err error) {
	size = HeaderSize
	if compressor.level == NoCompression {
		size += len(d)
		return
	}
	const maxInputSize = 1 << 18 // 256kB
	if len(d) > maxInputSize {
		return 0, fmt.Errorf("input size must be <= %d", maxInputSize)
	}

	// build the index
	var indexSpace [maxInputSize]int32 // should be allocated on the stack.
	index := suffixarray.New(d, indexSpace[:len(d)])

	bw := &bitCounterWriter{}
	_, err = compressor.write(bw, d, 0, index)
	if err != nil {
		return
	}

	size += bw.Len()
	return
}

type bitCounterWriter struct {
	nbBits int
}

func (b *bitCounterWriter) TryWriteBits(_ uint64, nbBits uint8) {
	b.nbBits += int(nbBits)
}

func (b *bitCounterWriter) TryWriteByte(_ byte) {
	b.nbBits += 8
}

// Len returns the number of bytes written so far
// --> we round up nbBits to the next byte
func (b *bitCounterWriter) Len() int {
	return (b.nbBits + 7) / 8
}

// canEncodeSymbol returns true if the symbol can be encoded directly
func canEncodeSymbol(b byte) bool {
	return b != SymbolDynamic && b != SymbolShort
}

// findBackRef attempts to find a backref in the window [i-brAddressRange, i+brLengthRange]
// if no backref is found, it returns -1, -1
// else returns the address and length of the backref
func findBackRef(data []byte, i int, bType BackrefType, minLength int, dataIndex, dictIndex *suffixarray.Index, dictLen int) (addr, length int) {
	if minLength == -1 {
		minLength = bType.nbBytesBackRef
	}

	if i+minLength > len(data) {
		return -1, -1
	}

	windowStart := max(0, i-bType.maxAddress)
	maxLength := 1 << maxBackrefLenLog2
	if i+maxLength > len(data) {
		maxLength = len(data) - i
	}

	if minLength > maxLength {
		return -1, -1
	}

	// we look for data[i:i+maxLength) in the window data[windowStart:i)
	addr, length = dataIndex.LookupLongest(data[i:i+maxLength], minLength, maxLength, windowStart, i)
	if bType.Delimiter == SymbolDynamic {
		addr += dictLen
	}

	if length < maxLength && bType.Delimiter == SymbolDynamic {
		// we also check the dictionary and check if it's a better backref
		// we look for data[i:i+maxLength) in the dict[0:dictLen)
		dAddr, dLength := dictIndex.LookupLongest(data[i:i+maxLength], minLength, maxLength, 0, dictLen)
		if dLength > length {
			addr, length = dAddr, dLength
		}
	}

	return
}

func (compressor *Compressor) appendInput(d []byte) error {
	if compressor.inBuf.Len()+len(d) > MaxInputSize {
		return fmt.Errorf("input size must be <= %d", MaxInputSize)
	}
	compressor.lastInLen = compressor.inBuf.Len()
	compressor.inBuf.Write(d)
	return nil
}
