package datastream

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
)

type Stream struct {
	Endian    binary.ByteOrder
	Alignment int
	Root      *Block
	Current   *Block
	Stack     []*Block
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

	root := NewBlock(Pointer{Index: len(str.Blocks), Offset: -1}, str.Endian, str.Alignment)
	str.Blocks = append(str.Blocks, root)
	str.Current = root
	str.Root = root
	str.Stack = append(str.Stack, str.Current)
	return str
}

func (d *Stream) OpenBlock() Pointer {
	b := NewBlock(Pointer{Index: len(d.Blocks), Offset: -1}, d.Endian, d.Alignment)
	d.Blocks = append(d.Blocks, b)
	d.Stack = append(d.Stack, d.Current)
	d.Current = b
	return b.This
}

func (d *Stream) CloseBlock() {
	d.Current.close()
	d.Stack = d.Stack[:len(d.Stack)-1]
	d.Current = d.Stack[len(d.Stack)-1]
}

func (d *Stream) WriteString(s string) {
	d.Current.writeString(s)
}

func (d *Stream) WriteI8(i int8) {
	d.Current.writeI8(i)
}

func (d *Stream) WriteI16(i int16) {
	d.Current.writeI16(i)
}

func (d *Stream) WriteI32(i int32) {
	d.Current.writeI32(i)
}

func (d *Stream) WriteI64(i int64) {
	d.Current.writeI64(i)
}

func (d *Stream) WriteU8(i uint8) {
	d.Current.writeU8(i)
}

func (d *Stream) WriteU16(i uint16) {
	d.Current.writeU16(i)
}

func (d *Stream) WriteU32(i uint32) {
	d.Current.writeU32(i)
}

func (d *Stream) WriteU64(i uint64) {
	d.Current.writeU64(i)
}

func (d *Stream) WriteFloat(f float32) {
	d.Current.writeF32(f)
}

func (d *Stream) WriteDouble(f float64) {
	d.Current.writeF64(f)
}

func (d *Stream) WritePtr(ptr Pointer) {
	d.Current.writePtr(ptr)
}

// Finalize will write out a finalized stream to disk
func (d *Stream) Finalize(dataFilename, relocFilename string) error {
	if len(d.Stack) > 1 {
		return fmt.Errorf("Cannot finalize stream with open blocks")
	}
	if d.Current != d.Root {
		return fmt.Errorf("Current block is not root")
	}
	d.Current.close()
	d.Stack = d.Stack[:0]

	// resolve all the pointers
	pointers := make(map[int]int64)
	goffset := int64(0) // global offset
	for _, block := range d.Blocks {
		// goffset must adhere to the alignment of the block
		if goffset%int64(block.Alignment) != 0 {
			padding := int64(block.Alignment) - (goffset % int64(block.Alignment))
			goffset += padding
		}
		block.This.Offset = goffset
		pointers[block.This.Index] = goffset
		goffset += block.Size
	}

	// finalize all blocks
	countPtrOffsets := 0
	for _, block := range d.Blocks {
		countPtrOffsets += len(block.Pointers)
	}

	ptroffsets := make([]int64, 0, countPtrOffsets)
	for _, block := range d.Blocks {
		ptroffsets = block.finalize(block.This.Offset, pointers, ptroffsets)
	}

	// write all blocks to a buffered writer
	buffer := bytes.Buffer{}
	for _, block := range d.Blocks {
		pos := buffer.Len()

		// check if pos adheres to block alignment, if not write padding
		if pos%block.Alignment != 0 {
			padding := block.Alignment - (pos % block.Alignment)
			for i := 0; i < padding; i++ {
				buffer.WriteByte(0)
			}
		}

		block.writeTo(&buffer)
	}

	// write the buffered data to the file
	// write all blocks
	datafile, err := os.Create(dataFilename)
	if err != nil {
		return err
	}
	defer datafile.Close()
	_, err = datafile.Write(buffer.Bytes())
	if err != nil {
		return err
	}

	// write all pointers
	relocfile, err := os.Create(relocFilename)
	if err != nil {
		return err
	}
	defer relocfile.Close()

	// sort the ptroffsets
	sort.Slice(ptroffsets, func(i, j int) bool {
		return ptroffsets[i] < ptroffsets[j]
	})

	for _, o := range ptroffsets {
		binary.Write(relocfile, d.Endian, o)
	}

	return nil
}
