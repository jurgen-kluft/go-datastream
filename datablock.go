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

func (d *Block) writeTo(w io.Writer) {
	w.Write(d.Writer.Bytes())
}

func (d *Block) finalize(offset int64, register map[int]int64) {
	// Register the pointers and update the stream
	stream := d.Writer.Bytes()
	for _, ptr := range d.Pointers {
		o := offset + ptr.Offset
		d.Endian.PutUint64(stream[ptr.Offset:(ptr.Offset+4)], uint64(o))
		register[ptr.Index] = o
	}
}

func align(value int64, alignment int) int64 {
	return (value + int64(alignment-1)) &^ int64(alignment-1)
}

func (d *Block) close() {
	d.Size = int64(d.Writer.Len())
	d.Size = align(d.Size, d.Alignment)
}

// Align a block
func (d *Block) align(alignment int) {
	// Write out '0' bytes to align the block
	pos := d.Writer.Len()
	gap := align(int64(pos), alignment) - int64(pos)
	for i := int64(0); i < gap; i++ {
		d.PutBuffer[i] = 0
	}
	d.writeBytes(d.PutBuffer[:gap])
}

func (d *Block) write(p []byte) {
	d.Writer.Write(p)
}

func (d *Block) writeString(s string) {
	strbytes := []byte(s)
	strlen := len(strbytes)
	d.align(4)
	d.Endian.PutUint32(d.PutBuffer, uint32(strlen))
	d.writeBytes(d.PutBuffer[:4])
	d.writeBytes(strbytes)
}

func (d *Block) writeBytes(b []byte) {
	d.Writer.Write(b)
}

func (d *Block) writeInt8(i int8) {
	d.Writer.WriteByte(byte(i))
}

func (d *Block) writeInt16(i int16) {
	d.align(2)
	d.Endian.PutUint16(d.PutBuffer, uint16(i))
	d.writeBytes(d.PutBuffer[:2])
}

func (d *Block) writeInt32(i int32) {
	d.align(4)
	d.Endian.PutUint32(d.PutBuffer, uint32(i))
	d.writeBytes(d.PutBuffer[:4])
}

func (d *Block) writeInt64(i int64) {
	d.align(8)
	d.Endian.PutUint64(d.PutBuffer, uint64(i))
	d.writeBytes(d.PutBuffer[:8])
}

func (d *Block) writeUInt8(i uint8) {
	d.Writer.WriteByte(byte(i))
}

func (d *Block) writeUInt16(i uint16) {
	d.align(2)
	d.Endian.PutUint16(d.PutBuffer, i)
	d.writeBytes(d.PutBuffer[:2])
}

func (d *Block) writeUInt32(i uint32) {
	d.align(4)
	d.Endian.PutUint32(d.PutBuffer, i)
	d.writeBytes(d.PutBuffer[:4])
}

func (d *Block) writeUInt64(i uint64) {
	d.align(8)
	d.Endian.PutUint64(d.PutBuffer, i)
	d.writeBytes(d.PutBuffer[:8])
}

func (d *Block) writeFloat(f float32) {
	d.writeUInt32(math.Float32bits(f))
}

func (d *Block) writeDouble(f float64) {
	d.writeUInt64(math.Float64bits(f))
}

func (d *Block) writePtr(ptr Pointer) {
	d.align(8)
	d.Pointers = append(d.Pointers, ptr)
	ptr.Offset = int64(d.Writer.Len())
	d.Endian.PutUint64(d.PutBuffer, uint64(ptr.Offset))
	d.writeBytes(d.PutBuffer[:8])
}
