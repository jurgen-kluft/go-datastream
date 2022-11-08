package datastream

import (
	"encoding/binary"
)

type DataStream struct {
	Endian    binary.ByteOrder
	Alignment int
	Current   *DataBlock
	Blocks    []*DataBlock
}

func NewDataStream(alignment int, endian binary.ByteOrder) *DataStream {
	return &DataStream{
		Endian:    endian,
		Alignment: alignment,
		Current:   NewDataBlock(alignment, endian),
	}
}

func (d *DataStream) OpenBlock() *DataPtr {
	// TODO Implement !
	return nil
}
