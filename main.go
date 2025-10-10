package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/consensys/compress/lzss"
)

var (
	flagDecompress = flag.Bool("d", false, "decompress")
	flagIn         = flag.String("i", "", "input file (required)")
	flagOut        = flag.String("o", "", "output file")
	flagNoOut      = flag.Bool("no_out", false, "no output")
	flagReport     = flag.Bool("r", false, "report compression ratio")
	flagDict       = flag.String("dict", "", "compression dictionary")
	flagVersion    = flag.Bool("version", false, "report executable version")
)

const (
	extension = ".linzip"
	version   = "0.3.0"
)

func quitF(format string, args ...interface{}) {
	if _, err := fmt.Fprintf(os.Stderr, format, args...); err != nil {
		panic(err)
	}
	os.Exit(1)
}

func assertNoError(err error) {
	if err != nil {
		quitF("%v\n", err)
	}
}

func main() {
	flag.Parse()

	if *flagVersion {
		fmt.Println("linzip v" + version)
		os.Exit(0)
	}

	if *flagIn == "" {
		quitF("no input file specified\n")
	}

	in, err := os.ReadFile(*flagIn)
	assertNoError(err)

	var (
		dict, out  []byte
		lenC, lenD int
	)
	if *flagDict != "" {
		dict, err = os.ReadFile(*flagDict)
		assertNoError(err)
	}

	if *flagOut != "" && *flagNoOut {
		quitF("options -no_out and -o are mutually exclusive\n")
	}

	if *flagOut == "" { // construct a file name from the input name
		if *flagDecompress {
			if strings.HasSuffix(*flagIn, extension) {
				*flagOut = (*flagIn)[:len(*flagIn)-len(extension)]
			} else {
				*flagOut = *flagIn + ".decompressed"
			}
		} else {
			*flagOut = *flagIn + extension
		}
	}

	if *flagDecompress {
		out, err = lzss.Decompress(in, dict)
		assertNoError(err)
		lenC, lenD = len(in), len(out)
	} else {
		c, err := lzss.NewCompressor(dict)
		assertNoError(err)
		out, err = c.Compress(in)
		assertNoError(err)
		lenC, lenD = len(out), len(in)
	}

	if *flagNoOut {
		*flagOut = ""
	} else {
		assertNoError(os.WriteFile(*flagOut, out, 0600))
	}

	if *flagReport {
		ratioPct := lenC * 100 / lenD
		fmt.Printf("%dB -> %dB compression ratio %d.%02d\n", lenC, lenD, ratioPct/100, ratioPct%100)
	}
}
