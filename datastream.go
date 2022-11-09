package datastream

import (
	"encoding/binary"
	"os"
)

type Stream struct {
	Endian    binary.ByteOrder
	Alignment int
	Root      *Block
	Current   *Block
	Blocks    []*Block
}
type Pointer struct {
	Index  int
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

func (d *Stream) NewBlock() *Block {
	b := NewBlock(Pointer{Index: len(d.Blocks), Offset: -1}, d.Endian, d.Alignment)
	d.Blocks = append(d.Blocks, b)
	return b
}

// Finalize, write out the stream to a file.
// To do that we need to 'resolve' all the pointers.
// Write the list of pointers into a separate file.
func (d *Stream) Finalize() error {
	// keep track of all unique pointers
	pointers := make(map[Pointer]bool)

	// resolve all the pointers
	offset := int64(0)
	for _, block := range d.Blocks {
		block.This.Offset = offset
		block.Finalize(func(ptr Pointer) Pointer {
			pointers[ptr] = true
			ptr.Offset += block.This.Offset
			return ptr
		})
		offset += block.Size
	}

	// write all blocks to a single file
	// write all pointers to a single file

	// open new file
	file, err := os.Create("datastream.bin")
	defer file.Close()
	if err != nil {
		return err
	}

	// write all blocks
	for _, block := range d.Blocks {
		block.WriteTo(file)
	}

	// write all pointers
	ptrs, err := os.Create("datastream.ptrs")
	defer ptrs.Close()
	if err != nil {
		return err
	}

	for ptr, _ := range pointers {
		binary.Write(ptrs, d.Endian, ptr.Offset)
	}

	return nil
}
