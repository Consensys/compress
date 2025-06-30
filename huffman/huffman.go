package huffman

import (
	"container/heap"
	"io"
	"math"
	"sort"

	"github.com/icza/bitio"
)

// node represents a node in the Huffman tree.
type node struct {
	symbol    int   // symbol (0-511)
	frequency int   // frequency of the symbol
	left      *node // left child
	right     *node // right child
	depth     int   // depth from treeRoot root
}

// PriorityQueue implements a min-heap for Nodes.
type PriorityQueue []*node

func (pq *PriorityQueue) Len() int { return len(*pq) }
func (pq *PriorityQueue) Less(i, j int) bool {
	return (*pq)[i].frequency < (*pq)[j].frequency
}
func (pq *PriorityQueue) Swap(i, j int) { (*pq)[i], (*pq)[j] = (*pq)[j], (*pq)[i] }

// Push adds an element to the priority queue.
func (pq *PriorityQueue) Push(x interface{}) {
	*pq = append(*pq, x.(*node))
}

// Pop removes and returns the smallest element from the priority queue.
func (pq *PriorityQueue) Pop() interface{} {
	n := len(*pq)
	item := (*pq)[n-1]
	*pq = (*pq)[:n-1]
	return item
}

// generateCodeLengths generates the code lengths for each symbol.
func (n *node) generateCodeLengths(nbSymbols int) []int {
	codeLengths := make([]int, nbSymbols)

	// Helper function to traverse the tree and calculate code lengths.
	var traverse func(node *node, depth int)
	traverse = func(node *node, depth int) {
		if node == nil {
			return
		}
		// If it's a leaf node, record the code length.
		if node.left == nil && node.right == nil {
			if deficit := node.symbol - len(codeLengths) + 1; deficit > 0 {
				codeLengths = append(codeLengths, make([]int, deficit)...)
			}
			codeLengths[node.symbol] = depth
			return
		}
		// Traverse left and right children.
		traverse(node.left, depth+1)
		traverse(node.right, depth+1)
	}

	// Start traversal from the root.
	traverse(n, 0)
	return codeLengths
}

type symbolCode struct {
	encoding uint16
	length   uint8
}

// Code represents a prefix code.
type Code []symbolCode
type Encoder struct {
	w *bitio.Writer
	c *Code
}

// Write implements the spirit of [io.Writer], while allowing for
// a symbol set larger than 256.
func (e Encoder) Write(p []int) (n int, err error) {
	for n = range p {
		code := (*e.c)[p[n]]
		if err = e.w.WriteBits(uint64(code.encoding), code.length); err != nil {
			return
		}
	}
	return len(p), nil
}

func (c *Code) WriteTo(w io.Writer) (n int64, err error) {
	//TODO implement me
	panic("implement me")
}

func (c *Code) ReadFrom(r io.Reader) (n int64, err error) {
	//TODO implement me
	panic("implement me")
}

// NewCodeFromSymbolFrequencies builds an encoder based on the given symbol frequencies.
// Any frequency of 0 will be considered 1. A negative frequency will cause a panic
func NewCodeFromSymbolFrequencies(frequencies []int) *Code {
	// Create a priority queue and populate it with leaf nodes.
	pq := &PriorityQueue{}
	heap.Init(pq)
	for symbol, freq := range frequencies {
		if freq == 0 {
			freq = 1
		} else if freq < 0 {
			panic("negative frequency")
		}
		heap.Push(pq, &node{symbol: symbol, frequency: freq})
	}

	// Build the tree by merging the two smallest nodes until one node remains.
	for pq.Len() > 1 {
		// Remove the two nodes with the smallest frequencies.
		left := heap.Pop(pq).(*node)
		right := heap.Pop(pq).(*node)

		// Create a new internal node with these two as children.
		parent := &node{
			symbol:    -1, // Internal nodes don't represent symbols.
			frequency: left.frequency + right.frequency,
			left:      left,
			right:     right,
		}

		// Add the new node back to the priority queue.
		heap.Push(pq, parent)
	}

	return NewCodeFromCodeLengths((*pq)[0].generateCodeLengths(len(frequencies)))
}

func NewCodeFromCodeLengths(codeLengths []int) *Code {

	sorted := make([]int, len(codeLengths))
	for i := range sorted {
		sorted[i] = i
	}

	// sort the symbols first by code length, then by the symbol itself
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		cLA, cLB := codeLengths[a], codeLengths[b]
		if cLA < cLB {
			return true
		}
		if cLA > cLB {
			return false
		}
		return a < b
	})

	code := make(Code, len(codeLengths))
	lastCode := symbolCode{encoding: uint16(math.MaxUint16), length: 0} // max = -1
	for _, symb := range sorted {
		length := codeLengths[symb]
		if length > 16 {
			panic("code length too large")
		}
		lastCode.encoding++
		lastCode.encoding <<= length - int(lastCode.length)
		lastCode.length = uint8(length)
		code[symb] = lastCode
	}

	return &code
}

// NewEncoder creates an [Encoder] from a prefix [Code] and a [bitio.Writer].
// The [Encoder] will not own the writer. Interleaved writes are permitted.
// The [Code] is not duplicated, so any modifications will be reflected in future writes.
func NewEncoder(c *Code, w *bitio.Writer) *Encoder {
	return &Encoder{c: c, w: w}
}

type Decoder struct {
	treeRoot node
	r        *bitio.Reader
}

// Read implements the spirit of [io.Reader], while allowing for
// a symbol set larger than 256.
func (d *Decoder) Read(p []int) (n int, err error) {
	for n = range p {
		cur := &d.treeRoot
		for cur.symbol == -1 {
			var b uint64
			if b, err = d.r.ReadBits(1); err != nil {
				return
			}
			if b == 0 {
				cur = cur.left
			} else {
				cur = cur.right
			}
		}
		p[n] = cur.symbol
	}
	return len(p), nil
}

// NewDecoder creates a [Decoder] from a prefix [Code] and a [bitio.Reader].
// The [Decoder] will not own the reader. Interleaved reads are permitted,
// The [Code] is processed into a prefix tree, so modifications will NOT be reflected in future reads.
func NewDecoder(c *Code, r *bitio.Reader) *Decoder {
	// turn the code into a tree
	d := &Decoder{treeRoot: node{symbol: -1}, r: r}
	for symb, sc := range *c {
		parent := &d.treeRoot
		for i := range sc.length {
			curBit := (sc.encoding >> (sc.length - 1 - i)) & 1
			if parent.left == nil || parent.right == nil {
				if parent.left != nil || parent.right != nil {
					panic("bad treeRoot") // will never happen
				}
				parent.left = &node{symbol: -1}
				parent.right = &node{symbol: -1}
				if curBit == 0 {
					parent = parent.left
				} else {
					parent = parent.right
				}
			}
		}
		if parent.left != nil || parent.right != nil {
			panic("bad code - not a prefix code")
		}
		if parent.symbol != -1 {
			panic("bad code - repeated encoding")
		}
		parent.symbol = symb
	}

	return d
}
