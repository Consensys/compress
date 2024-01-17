package main

import (
	"bytes"
	"errors"
	"github.com/icza/bitio"
)

// this app creates a readable csv version of a compressed file
func main() {

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
}
