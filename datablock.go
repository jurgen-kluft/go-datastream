package datastream

import (
	"bytes"
	"encoding/binary"
)

type DataBlock struct {
	Endian    binary.ByteOrder
	Writer    *bytes.Buffer
	Alignment int
	Pointers  []*DataPtr
}

func NewDataBlock(alignment int, endian binary.ByteOrder) *DataBlock {
	return &DataBlock{
		Endian:    endian,
		Writer:    new(bytes.Buffer),
		Alignment: alignment,
	}
}

func (d *DataBlock) Write(p []byte) {
	d.Writer.Write(p)
}

func (d *DataBlock) WriteString(s string) {
	strbytes := []byte(s)
	strlen := len(strbytes)
	d.WriteInt32(int32(strlen))
	d.WriteBytes(strbytes)
}

func (d *DataBlock) WriteBytes(b []byte) {
	d.Writer.Write(b)
}

func (d *DataBlock) WriteInt8(i int8) {
	binary.Write(d.Writer, d.Endian, i)
}

func (d *DataBlock) WriteInt16(i int16) {
	// TODO make sure to align
	binary.Write(d.Writer, d.Endian, i)
}

func (d *DataBlock) WriteInt32(i int32) {
	// TODO make sure to align
	binary.Write(d.Writer, d.Endian, i)
}

func (d *DataBlock) WriteInt64(i int64) {
	// TODO make sure to align
	binary.Write(d.Writer, d.Endian, i)
}

func (d *DataBlock) WriteUInt8(i uint8) {
	binary.Write(d.Writer, d.Endian, i)
}

func (d *DataBlock) WriteUInt16(i uint16) {
	// TODO make sure to align
	binary.Write(d.Writer, d.Endian, i)
}

func (d *DataBlock) WriteUInt32(i uint32) {
	// TODO make sure to align
	binary.Write(d.Writer, d.Endian, i)
}

func (d *DataBlock) WriteUInt64(i uint64) {
	// TODO make sure to align
	binary.Write(d.Writer, d.Endian, i)
}

func (d *DataBlock) WriteFloat(f float32) {
	// TODO make sure to align
	binary.Write(d.Writer, d.Endian, f)
}

func (d *DataBlock) WriteDouble(f float64) {
	// TODO make sure to align
	binary.Write(d.Writer, d.Endian, f)
}

func (d *DataBlock) WritePtr(ptr *DataPtr) {
	// TODO make sure to align
	d.Pointers = append(d.Pointers, ptr)
	binary.Write(d.Writer, d.Endian, ptr.Offset)
}
