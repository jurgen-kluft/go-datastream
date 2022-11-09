package datastream

import (
	"encoding/binary"
	"os"
	"sort"
)

type Stream struct {
	Endian        binary.ByteOrder
	DataFilename  string
	RelocFilename string
	Alignment     int
	Root          *Block
	Current       *Block
	Stack         []*Block
	Blocks        []*Block
}

type Pointer struct {
	Index  int
	Offset int64
}

func NewStream(endian binary.ByteOrder, dataFilename string, relocFilename string) *Stream {
	str := &Stream{
		Endian:        endian,
		DataFilename:  dataFilename,
		RelocFilename: relocFilename,
		Alignment:     8, // our largest type is 8 bytes
		Current:       nil,
		Root:          nil,
		Blocks:        make([]*Block, 0),
	}

	str.OpenBlock()
	str.Root = str.Blocks[0]
	str.Current = str.Blocks[0]
	return str
}

func (d *Stream) OpenBlock() Pointer {
	b := NewBlock(Pointer{Index: len(d.Blocks), Offset: -1}, d.Endian, d.Alignment)
	d.Blocks = append(d.Blocks, b)
	return b.This
}

func (d *Stream) CloseBlock() {
	d.Current.close()
	d.Current = d.Stack[len(d.Stack)-1]
	d.Stack = d.Stack[:len(d.Stack)-1]
}

func (d *Stream) WriteString(s string) {
	d.Current.writeString(s)
}

func (d *Stream) WriteInt8(i int8) {
	d.Current.writeInt8(i)
}

func (d *Stream) WriteInt16(i int16) {
	d.Current.writeInt16(i)
}

func (d *Stream) WriteInt32(i int32) {
	d.Current.writeInt32(i)
}

func (d *Stream) WriteInt64(i int64) {
	d.Current.writeInt64(i)
}

func (d *Stream) WriteUInt8(i uint8) {
	d.Current.writeUInt8(i)
}

func (d *Stream) WriteUInt16(i uint16) {
	d.Current.writeUInt16(i)
}

func (d *Stream) WriteUInt32(i uint32) {
	d.Current.writeUInt32(i)
}

func (d *Stream) WriteUInt64(i uint64) {
	d.Current.writeUInt64(i)
}

func (d *Stream) WriteFloat(f float32) {
	d.Current.writeFloat(f)
}

func (d *Stream) WriteDouble(f float64) {
	d.Current.writeDouble(f)
}

func (d *Stream) WritePtr(ptr Pointer) {
	d.Current.writePtr(ptr)
}

// Finalize will write out a finalized stream to disk
func (d *Stream) Finalize() error {

	// resolve all the pointers
	pointers := make(map[int]int64)
	goffset := int64(0) // global offset
	for _, block := range d.Blocks {
		block.This.Offset = goffset
		block.finalize(goffset, pointers)
		goffset += block.Size
	}

	// write all blocks
	datafile, err := os.Create(d.DataFilename)
	defer datafile.Close()
	if err != nil {
		return err
	}

	for _, block := range d.Blocks {
		block.writeTo(datafile)
	}

	// write all pointers
	relocfile, err := os.Create(d.RelocFilename)
	defer relocfile.Close()
	if err != nil {
		return err
	}

	ptroffsets := make([]int64, len(pointers))
	for _, o := range pointers {
		ptroffsets = append(ptroffsets, o)
	}

	// sort the ptroffsets
	sort.Slice(ptroffsets, func(i, j int) bool {
		return ptroffsets[i] < ptroffsets[j]
	})

	for _, o := range ptroffsets {
		binary.Write(relocfile, d.Endian, o)
	}

	return nil
}
