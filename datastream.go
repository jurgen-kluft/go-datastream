package datastream

import (
	"bytes"
	"crypto/sha1"
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

func (d *Stream) Align2() {
	d.Current.align(2)
}

func (d *Stream) Align4() {
	d.Current.align(4)
}

func (d *Stream) Align8() {
	d.Current.align(8)
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

	// Compute the total number of pointer offsets we may use, so we can preallocate the slice
	countPtrOffsets := 0
	for _, block := range d.Blocks {
		countPtrOffsets += len(block.Pointers)
	}
	ptroffsets := make([]int64, 0, countPtrOffsets)

	// deduplicate block then finalize all blocks, collecting all pointer offsets
	// Note: When we deduplicate a block, we need to update all pointers that point
	//       to it, which may cause more deduplications, so we need to

	for true {
		// resolve all the pointers, taking care of blocks that have been deduplicated
		pointers := make(map[int]int64)

		// go over all unique blocks and assign offsets, taking care of alignment
		globalOffset := int64(0) // global offset
		for _, block := range d.Blocks {
			if block.Canonical >= 0 {
				// skip blocks that are inactive due to deduplication
				continue
			}
			if offset, ok := pointers[block.This.Index]; ok {
				block.This.Offset = offset
				continue
			}
			// block alignment
			if globalOffset%int64(block.Alignment) != 0 {
				padding := int64(block.Alignment) - (globalOffset % int64(block.Alignment))
				globalOffset += padding
			}
			block.This.Offset = globalOffset
			pointers[block.This.Index] = globalOffset
			globalOffset += block.Size
		}

		// finalize all blocks
		ptroffsets = ptroffsets[:0] // reset the slice while keeping the capacity
		for _, block := range d.Blocks {
			if block.Canonical >= 0 {
				// skip blocks that are inactive due to deduplication
				continue
			}
			ptroffsets = block.finalize(block.This.Offset, pointers, ptroffsets)
		}

		// deduplicate blocks using SHA1
		dedupNum := 0
		dedupMap := make(map[[20]byte]*Block)
		hasher := sha1.New()
		for _, block := range d.Blocks {
			if block.Canonical >= 0 {
				// skip blocks that are inactive due to deduplication
				continue
			}
			hash := block.hash(hasher)
			if existing, ok := dedupMap[hash]; ok {
				block.Canonical = existing.This.Index
				dedupNum++
			} else {
				dedupMap[hash] = block
			}
		}
		if dedupNum == 0 {
			break
		}
	}

	// write all blocks to a buffer, taking care of alignment
	buffer := bytes.Buffer{}
	for _, block := range d.Blocks {
		if block.Canonical >= 0 {
			// skip blocks that are inactive due to deduplication
			continue
		}
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

	// write the full buffer to the data file
	datafile, err := os.Create(dataFilename)
	if err != nil {
		return err
	}
	defer datafile.Close()
	_, err = datafile.Write(buffer.Bytes())
	if err != nil {
		return err
	}

	// sort the ptroffsets
	sort.Slice(ptroffsets, func(i, j int) bool {
		return ptroffsets[i] < ptroffsets[j]
	})

	relocDataBuffer := make([]byte, (len(ptroffsets)+1)*8) // count + (8 bytes per offset)
	for i := 0; i <= len(ptroffsets); i++ {
		o := uint64(0)
		if i == 0 {
			o = uint64(len(ptroffsets)) // first 8 bytes is the count of offsets
		} else {
			o = uint64(ptroffsets[i-1])
		}
		// swap value according to endianness
		offset := relocDataBuffer[i*8 : (i+1)*8]
		if d.Endian == binary.BigEndian {
			binary.BigEndian.PutUint64(offset, o)
		} else {
			binary.LittleEndian.PutUint64(offset, o)
		}
	}

	// write all pointers to the reloc file
	relocfile, err := os.Create(relocFilename)
	if err != nil {
		return err
	}
	defer relocfile.Close()

	_, err = relocfile.Write(relocDataBuffer)
	if err != nil {
		return err
	}

	return nil
}
