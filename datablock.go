package datastream

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
)

// TODO going through bytes.Buffer and binary.Write is not the most efficient way to do this.

type Block struct {
	This      Pointer
	Size      int64
	Endian    binary.ByteOrder
	Alignment int
	PutBuffer []byte
	Writer    *bytes.Buffer
	Pointers  []Pointer
}

func NewBlock(ptr Pointer, endian binary.ByteOrder, alignment int) *Block {
	return &Block{
		This:      ptr,
		Size:      0,
		Endian:    endian,
		Alignment: alignment,
		PutBuffer: make([]byte, 16),
		Writer:    new(bytes.Buffer),
		Pointers:  make([]Pointer, 0),
	}
}

func (d *Block) WriteTo(w io.Writer) {
	w.Write(d.Writer.Bytes())
}

func (d *Block) Finalize(fn func(ptr Pointer) Pointer) {
	for i, _ := range d.Pointers {
		d.Pointers[i] = fn(d.Pointers[i])
	}
}

func Align(value int64, alignment int) int64 {
	return (value + int64(alignment-1)) &^ int64(alignment-1)
}

func (d *Block) Close() {
	d.Size = int64(d.Writer.Len())
	d.Size = Align(d.Size, d.Alignment)
}

// Align a block
func (d *Block) Align(alignment int) {
	// Write out '0' bytes to align the block
	pos := d.Writer.Len()
	gap := Align(int64(pos), alignment) - int64(pos)
	for i := int64(0); i < gap; i++ {
		d.PutBuffer[i] = 0
	}
	d.WriteBytes(d.PutBuffer[:gap])
}

func (d *Block) Write(p []byte) {
	d.Writer.Write(p)
}

func (d *Block) WriteString(s string) {
	strbytes := []byte(s)
	strlen := len(strbytes)
	d.Align(4)
	d.Endian.PutUint32(d.PutBuffer, uint32(strlen))
	d.WriteBytes(d.PutBuffer[:4])
	d.WriteBytes(strbytes)
}

func (d *Block) WriteBytes(b []byte) {
	d.Writer.Write(b)
}

func (d *Block) WriteInt8(i int8) {
	d.Writer.WriteByte(byte(i))
}

func (d *Block) WriteInt16(i int16) {
	d.Align(2)
	d.Endian.PutUint16(d.PutBuffer, uint16(i))
	d.WriteBytes(d.PutBuffer[:2])
}

func (d *Block) WriteInt32(i int32) {
	d.Align(4)
	d.Endian.PutUint32(d.PutBuffer, uint32(i))
	d.WriteBytes(d.PutBuffer[:4])
}

func (d *Block) WriteInt64(i int64) {
	d.Align(8)
	d.Endian.PutUint64(d.PutBuffer, uint64(i))
	d.WriteBytes(d.PutBuffer[:8])
}

func (d *Block) WriteUInt8(i uint8) {
	d.Writer.WriteByte(byte(i))
}

func (d *Block) WriteUInt16(i uint16) {
	d.Align(2)
	d.Endian.PutUint16(d.PutBuffer, i)
	d.WriteBytes(d.PutBuffer[:2])
}

func (d *Block) WriteUInt32(i uint32) {
	d.Align(4)
	d.Endian.PutUint32(d.PutBuffer, i)
	d.WriteBytes(d.PutBuffer[:4])
}

func (d *Block) WriteUInt64(i uint64) {
	d.Align(8)
	d.Endian.PutUint64(d.PutBuffer, i)
	d.WriteBytes(d.PutBuffer[:8])
}

func (d *Block) WriteFloat(f float32) {
	d.WriteUInt32(math.Float32bits(f))
}

func (d *Block) WriteDouble(f float64) {
	d.WriteUInt64(math.Float64bits(f))
}

func (d *Block) WritePtr(ptr Pointer) {
	d.Align(8)
	d.Pointers = append(d.Pointers, ptr)
	ptr.Offset = int64(d.Writer.Len())
	d.Endian.PutUint64(d.PutBuffer, uint64(ptr.Offset))
	d.WriteBytes(d.PutBuffer[:8])
}
