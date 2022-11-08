package datastream

import (
	"encoding/binary"
)

type Stream struct {
	Endian             binary.ByteOrder
	Alignment          int
	Current            *Block
	Blocks             []*Block
	GlobalPointerIndex int64
}
type Pointer struct {
	Index  int64
	Offset int64
}

func NewStream(alignment int, endian binary.ByteOrder) *Stream {
	str := &Stream{
		Endian:    endian,
		Alignment: alignment,
		Current:   nil,
		Blocks:    make([]*Block, 0),
	}

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
