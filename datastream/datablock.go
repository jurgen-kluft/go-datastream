package datastream

import (
	"bytes"
	"encoding/binary"
	"hash"
	"io"
	"math"
)

// TODO going through bytes.Buffer and binary.Write is not the most efficient way to do this.

type Block struct {
	This           Pointer
	SizeOfPointer  SizeOfPointer
	Canonical      int
	Size           int64
	Endian         binary.ByteOrder
	Alignment      int
	PutBuffer      []byte
	Writer         *bytes.Buffer
	Pointers       []Pointer
	pointerSources map[int64]sourceLocation
}

type sourceLocation struct {
	file string
	line int
}

func NewBlock(ptr Pointer, sizeOfPointer SizeOfPointer, endian binary.ByteOrder, alignment int) *Block {
	return &Block{
		This:           ptr,
		SizeOfPointer:  sizeOfPointer,
		Canonical:      -1,
		Size:           0,
		Endian:         endian,
		Alignment:      alignment,
		PutBuffer:      make([]byte, 16),
		Writer:         new(bytes.Buffer),
		Pointers:       make([]Pointer, 0),
		pointerSources: make(map[int64]sourceLocation),
	}
}

func (d *Block) writeTo(ctx *Context, w io.Writer) bool {
	written, err := w.Write(d.Writer.Bytes())
	if err == nil && written != d.Writer.Len() {
		err = io.ErrShortWrite
	}
	if err != nil {
		ctx.AddError("writing block %d: %v", d.This.Index, err)
		return false
	}
	return true
}

func (d *Block) finalize(ctx *Context, offset int64, pointers map[int]int64, pointerOffsets []int64, emitIssues bool) []int64 {
	// Register the pointers and update the stream
	stream := d.Writer.Bytes()
	for _, ptr := range d.Pointers {
		pointerSize := int64(d.SizeOfPointer)
		if ptr.Offset < 0 || pointerSize <= 0 || ptr.Offset > int64(len(stream))-pointerSize {
			if emitIssues {
				ctx.AddError(
					"pointer field in block %d at offset %d is outside the block data",
					d.This.Index, ptr.Offset,
				)
			}
			continue
		}
		fieldOffset := offset + ptr.Offset
		targetOffset, ok := pointers[ptr.Index]
		if !ok {
			if emitIssues {
				location := d.pointerSources[ptr.Offset]
				ctx.AddWarning(
					"%s:%d: unresolved pointer from block %d at offset %d to block %d; writing zero displacement",
					location.file, location.line, d.This.Index, ptr.Offset, ptr.Index,
				)
			}
			targetOffset = fieldOffset
		}
		displacement := targetOffset - fieldOffset
		pointerOffsets = append(pointerOffsets, fieldOffset)
		if d.SizeOfPointer == SizeOfPointer32 {
			if displacement < math.MinInt32 || displacement > math.MaxInt32 {
				if emitIssues {
					location := d.pointerSources[ptr.Offset]
					ctx.AddError(
						"%s:%d: pointer displacement %d from block %d at offset %d exceeds int32 range",
						location.file, location.line, displacement, d.This.Index, ptr.Offset,
					)
				}
				d.Endian.PutUint32(stream[ptr.Offset:(ptr.Offset+4)], 0)
				continue
			}
			d.Endian.PutUint32(stream[ptr.Offset:(ptr.Offset+4)], uint32(int32(displacement)))
		} else {
			d.Endian.PutUint64(stream[ptr.Offset:(ptr.Offset+8)], uint64(displacement))
		}
		if ok && emitIssues {
			ctx.AddInformation(
				"resolved pointer from block %d at offset %d to block %d with displacement %d",
				d.This.Index, ptr.Offset, ptr.Index, displacement,
			)
		}
	}
	return pointerOffsets
}

func (d *Block) hash(hasher hash.Hash) [20]byte {
	hasher.Reset()
	hasher.Write(d.Writer.Bytes())
	var hash [20]byte
	copy(hash[:], hasher.Sum(nil))
	return hash
}

func align(value int64, alignment int) int64 {
	return (value + int64(alignment-1)) &^ int64(alignment-1)
}

func (d *Block) close(ctx *Context) {
	d.Size = int64(d.Writer.Len())
	ctx.AddInformation("closed block %d with size %d", d.This.Index, d.Size)
}

// Align a block
func (d *Block) align(ctx *Context, alignment int) {
	// Write out '0' bytes to align the block
	pos := d.Writer.Len()
	gap := align(int64(pos), alignment) - int64(pos)

	// If gap is 0, we're already aligned, so no need to write anything
	if gap == 0 {
		return
	}

	// Zero out PutBuffer to avoid writing garbage data
	for i := range d.PutBuffer {
		d.PutBuffer[i] = 0
	}

	for gap > 0 {
		chunk := int64(16)
		if chunk > gap {
			chunk = gap
		}
		d.writeBytes(d.PutBuffer[:chunk])
		gap -= chunk
	}
	ctx.AddInformation("added alignment padding to block %d at offset %d for alignment %d", d.This.Index, pos, alignment)
}

func (d *Block) writeBytes(b []byte) {
	d.Writer.Write(b)
}

func (d *Block) writeI8(i int8) {
	d.Writer.WriteByte(byte(i))
}

func (d *Block) writeI16(i int16) {
	d.Endian.PutUint16(d.PutBuffer, uint16(i))
	d.writeBytes(d.PutBuffer[:2])
}

func (d *Block) writeI32(i int32) {
	d.Endian.PutUint32(d.PutBuffer, uint32(i))
	d.writeBytes(d.PutBuffer[:4])
}

func (d *Block) writeI64(i int64) {
	d.Endian.PutUint64(d.PutBuffer, uint64(i))
	d.writeBytes(d.PutBuffer[:8])
}

func (d *Block) writeU8(i uint8) {
	d.Writer.WriteByte(byte(i))
}

func (d *Block) writeU16(i uint16) {
	d.Endian.PutUint16(d.PutBuffer, i)
	d.writeBytes(d.PutBuffer[:2])
}

func (d *Block) writeU32(i uint32) {
	d.Endian.PutUint32(d.PutBuffer, i)
	d.writeBytes(d.PutBuffer[:4])
}

func (d *Block) writeU64(i uint64) {
	d.Endian.PutUint64(d.PutBuffer, i)
	d.writeBytes(d.PutBuffer[:8])
}

func (d *Block) writeF32(f float32) {
	d.writeU32(math.Float32bits(f))
}

func (d *Block) writeF64(f float64) {
	d.writeU64(math.Float64bits(f))
}

func (d *Block) writePtr(ptr Pointer, source sourceLocation) {
	ptr.Offset = int64(d.Writer.Len())
	if ptr.Index >= 0 {
		d.Pointers = append(d.Pointers, ptr)
		d.pointerSources[ptr.Offset] = source
	}
	if d.SizeOfPointer == SizeOfPointer32 {
		d.Endian.PutUint32(d.PutBuffer, 0)
		d.writeBytes(d.PutBuffer[:4])
	} else {
		d.Endian.PutUint64(d.PutBuffer, 0)
		d.writeBytes(d.PutBuffer[:8])
	}
}
