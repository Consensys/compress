package lzss

import (
	"bytes"
	"errors"
	"io"

	"github.com/consensys/compress"
	"github.com/icza/bitio"
)

// Decompress decompresses the given data using the given dictionary
// the dictionary must be the same as the one used to compress the data
// Note that this is not a fail-safe decompressor, it will fail ungracefully if the data
// has a different format than the one expected
func Decompress(data, dict []byte) (d []byte, err error) {
	in := bitio.NewReader(bytes.NewReader(data))

	// parse header
	var header Header
	sizeHeader, err := header.ReadFrom(in)
	if err != nil {
		return
	}
	if header.Version != Version {
		return nil, errors.New("unsupported compressor version")
	}
	if header.Level == NoCompression {
		return data[sizeHeader:], nil
	}

	// init dict and backref types
	dict = AugmentDict(dict)
	shortBackRefType, longBackRefType, dictBackRefType := InitBackRefTypes(len(dict), header.Level)

	bDict := backref{bType: dictBackRefType}
	bShort := backref{bType: shortBackRefType}
	bLong := backref{bType: longBackRefType}

	var out bytes.Buffer
	out.Grow(len(data) * 7)

	// read byte per byte; if it's a backref, write the corresponding bytes
	// otherwise, write the byte as is
	s := in.TryReadByte()
	for in.TryError == nil {
		switch s {
		case SymbolShort:
			// short back ref
			bShort.readFrom(in)
			for i := 0; i < bShort.length; i++ {
				out.WriteByte(out.Bytes()[out.Len()-bShort.address])
			}
		case SymbolLong:
			// long back ref
			bLong.readFrom(in)
			for i := 0; i < bLong.length; i++ {
				out.WriteByte(out.Bytes()[out.Len()-bLong.address])
			}
		case SymbolDict:
			// dict back ref
			bDict.readFrom(in)
			out.Write(dict[bDict.address : bDict.address+bDict.length])
		default:
			out.WriteByte(s)
		}
		s = in.TryReadByte()
	}

	return out.Bytes(), nil
}

// ReadIntoStream reads the compressed data into a stream
// the stream is not padded with zeros as one obtained by a naive call to compress.NewStream may be
func ReadIntoStream(data, dict []byte, level Level) (compress.Stream, error) {

	out, err := compress.NewStream(data, uint8(level))
	if err != nil {
		return out, err
	}

	// now find out how much of the stream is padded zeros and remove them
	in := bitio.NewReader(bytes.NewReader(data))
	dict = AugmentDict(dict)
	var header Header
	if _, err := header.ReadFrom(in); err != nil {
		return out, err
	}
	shortBackRefType, longBackRefType, dictBackRefType := InitBackRefTypes(len(dict), level)

	// the main job of this function is to compute the right value for outLenBits
	// so we can remove the extra zeros at the end of out
	outLenBits := headerBitLen
	if header.Level == NoCompression {
		return out, nil
	}
	if header.Level != level {
		return out, errors.New("compression mode mismatch")
	}

	s := in.TryReadByte()
	for in.TryError == nil {
		var b *BackrefType
		switch s {
		case SymbolShort:
			b = &shortBackRefType
		case SymbolLong:
			b = &longBackRefType
		case SymbolDict:
			b = &dictBackRefType
		}
		if b == nil {
			outLenBits += 8
		} else {
			in.TryReadBits(b.NbBitsBackRef - 8)
			outLenBits += int(b.NbBitsBackRef)
		}
		s = in.TryReadByte()
	}
	if in.TryError != io.EOF {
		return out, in.TryError
	}

	return compress.Stream{
		D:       out.D[:outLenBits/int(level)],
		NbSymbs: out.NbSymbs,
	}, nil
}
