package datastream

import (
	"encoding/binary"
)

type Stream struct {
	Endian             binary.ByteOrder
	Alignment          int
	Root               *Block
	Current            *Block
	Blocks             []*Block
	GlobalPointerIndex int64
}
type Pointer struct {
	Index  int64
	Offset int64
}

func NewStream(endian binary.ByteOrder) *Stream {
	str := &Stream{
		Endian:    endian,
		Alignment: 8, // our largest type is 8 bytes
		Current:   nil,
		Root:      nil,
		Blocks:    make([]*Block, 0),
	}

	str.Root = str.NewBlock()
	str.Current = str.Root
	return str
}

func (d *Stream) NewPtr() Pointer {
	d.GlobalPointerIndex++
	return Pointer{
		Index:  d.GlobalPointerIndex,
		Offset: -1,
	}
}

func (d *Stream) NewBlock() *Block {
	b := NewBlock(d.NewPtr(), d.Endian, d.Alignment)
	d.Blocks = append(d.Blocks, b)
	return b
}

// Finalize, write out the stream to a file.
// To do that we need to 'resolve' all the pointers.
// Write the list of pointers into a separate file.
func (d *Stream) Finalize() {
	// Simulate the writing of the blocks so as to compute the size of the blocks as
	// well as the absolute offsets of the pointers.
}
