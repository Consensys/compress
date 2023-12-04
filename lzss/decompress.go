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
	var out bytes.Buffer
	out.Grow(len(data)*6 + len(dict))
	in := bitio.NewReader(bytes.NewReader(data))

	var settings settings
	if err = settings.readFrom(in); err != nil {
		return
	}
	if settings.version != 0 {
		return nil, errors.New("unsupported compressor version")
	}
	if settings.level == NoCompression {
		return data[2:], nil
	}

	dict = AugmentDict(dict)
	shortBackRefType, longBackRefType, dictBackRefType := InitBackRefTypes(len(dict), settings.level)

	bDict := backref{bType: dictBackRefType}
	bShort := backref{bType: shortBackRefType}
	bLong := backref{bType: longBackRefType}

	// read until startAt and write bytes as is

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
	var settings settings
	if err := settings.readFrom(in); err != nil {
		return out, err
	}
	shortBackRefType, longBackRefType, dictBackRefType := InitBackRefTypes(len(dict), level)

	// the main job of this function is to compute the right value for outLenBits
	// so we can remove the extra zeros at the end of out
	outLenBits := settings.bitLen()
	if settings.level == NoCompression {
		return out, nil
	}
	if settings.level != level {
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
