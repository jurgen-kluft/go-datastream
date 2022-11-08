package datastream

import (
	"bytes"
	"encoding/binary"
)

type Block struct {
	This      Pointer
	Endian    binary.ByteOrder
	Alignment int
	Writer    *bytes.Buffer
	Pointers  []Pointer
}

func NewBlock(ptr Pointer, endian binary.ByteOrder, alignment int) *Block {
	return &Block{
		This:      ptr,
		Endian:    endian,
		Alignment: alignment,
		Writer:    new(bytes.Buffer),
		Pointers:  make([]Pointer, 0),
	}
}

func (d *Block) Write(p []byte) {
	d.Writer.Write(p)
}

func (d *Block) WriteString(s string) {
	strbytes := []byte(s)
	strlen := len(strbytes)
	d.WriteInt32(int32(strlen))
	d.WriteBytes(strbytes)
}

func (d *Block) WriteBytes(b []byte) {
	d.Writer.Write(b)
}

func (d *Block) WriteInt8(i int8) {
	binary.Write(d.Writer, d.Endian, i)
}

func (d *Block) WriteInt16(i int16) {
	// TODO make sure to align
	binary.Write(d.Writer, d.Endian, i)
}

func (d *Block) WriteInt32(i int32) {
	// TODO make sure to align
	binary.Write(d.Writer, d.Endian, i)
}

func (d *Block) WriteInt64(i int64) {
	// TODO make sure to align
	binary.Write(d.Writer, d.Endian, i)
}

func (d *Block) WriteUInt8(i uint8) {
	binary.Write(d.Writer, d.Endian, i)
}

func (d *Block) WriteUInt16(i uint16) {
	// TODO make sure to align
	binary.Write(d.Writer, d.Endian, i)
}

func (d *Block) WriteUInt32(i uint32) {
	// TODO make sure to align
	binary.Write(d.Writer, d.Endian, i)
}

func (d *Block) WriteUInt64(i uint64) {
	// TODO make sure to align
	binary.Write(d.Writer, d.Endian, i)
}

func (d *Block) WriteFloat(f float32) {
	// TODO make sure to align
	binary.Write(d.Writer, d.Endian, f)
}

func (d *Block) WriteDouble(f float64) {
	// TODO make sure to align
	binary.Write(d.Writer, d.Endian, f)
}

func (d *Block) WritePtr(ptr Pointer) {
	// TODO make sure to align
	d.Pointers = append(d.Pointers, ptr)
	ptr.Offset = int64(d.Writer.Len())
	binary.Write(d.Writer, d.Endian, ptr.Offset)
}
