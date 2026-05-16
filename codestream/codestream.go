package codestream

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"unicode/utf8"

	"github.com/jurgen-kluft/go-datastream/datastream"
)

const (
	stringHeaderSize = 8
	arrayHeaderSize  = 8
	mapHeaderSize    = 16
	ptrOffsetSize    = 4
	maxU16Value      = 1<<16 - 1
	maxU32Value      = 1<<32 - 1
)

var (
	errNeedPointer   = errors.New("codestream: destination must be a non-nil pointer")
	errUnsupported   = errors.New("codestream: unsupported type")
	errOverflow      = errors.New("codestream: value exceeds format limits")
	errUnexported    = errors.New("codestream: unexported struct field is not supported")
	errCycleDetected = errors.New("codestream: cyclic pointer graph is not supported")
)

type layoutInfo struct {
	size  int
	align int
}

type encoder struct {
	stream *datastream.Stream
	stack  map[uintptr]bool
	layout map[reflect.Type]layoutInfo
}

type decoder struct {
	data   []byte
	layout map[reflect.Type]layoutInfo
}

func WriteToStream(w io.Writer, data interface{}) error {
	if data == nil {
		return errNeedPointer
	}

	v := reflect.ValueOf(data)
	if !v.IsValid() {
		return errNeedPointer
	}

	for v.Kind() == reflect.Interface {
		if v.IsNil() {
			return errNeedPointer
		}
		v = v.Elem()
	}

	if v.Kind() == reflect.Ptr && v.IsNil() {
		v = reflect.New(v.Type().Elem()).Elem()
	}
	for v.IsValid() && v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if !v.IsValid() {
		return errNeedPointer
	}

	layout, err := layoutOfType(v.Type())
	if err != nil {
		return err
	}

	stream := datastream.NewStream(datastream.SizeOfPointer32, binary.LittleEndian)
	stream.Alignment = layout.align
	stream.Current.Alignment = layout.align

	enc := &encoder{
		stream: stream,
		stack:  make(map[uintptr]bool, 32),
		layout: make(map[reflect.Type]layoutInfo, 64),
	}

	if err := enc.writeValue(v); err != nil {
		return err
	}

	_, err = stream.Write(w)
	return err
}

func ReadFromStream(r io.Reader, data interface{}) error {
	if data == nil {
		return errNeedPointer
	}

	dst := reflect.ValueOf(data)
	if !dst.IsValid() || dst.Kind() != reflect.Ptr || dst.IsNil() {
		return errNeedPointer
	}

	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	dec := &decoder{
		data:   raw,
		layout: make(map[reflect.Type]layoutInfo),
	}

	return dec.readValue(dst.Elem(), 0)
}

func layoutOfType(t reflect.Type) (layoutInfo, error) {
	switch t.Kind() {
	case reflect.Bool, reflect.Int8, reflect.Uint8:
		return layoutInfo{size: 1, align: 1}, nil
	case reflect.Int16, reflect.Uint16:
		return layoutInfo{size: 2, align: 2}, nil
	case reflect.Int32, reflect.Uint32, reflect.Float32:
		return layoutInfo{size: 4, align: 4}, nil
	case reflect.Int, reflect.Uint:
		return layoutInfo{size: 4, align: 4}, nil
	case reflect.Int64, reflect.Uint64, reflect.Float64:
		return layoutInfo{size: 8, align: 8}, nil
	case reflect.String:
		return layoutInfo{size: stringHeaderSize, align: 4}, nil
	case reflect.Ptr:
		return layoutInfo{size: ptrOffsetSize, align: 4}, nil
	case reflect.Slice, reflect.Array:
		return layoutInfo{size: arrayHeaderSize, align: 4}, nil
	case reflect.Map:
		return layoutInfo{size: mapHeaderSize, align: 4}, nil
	case reflect.Struct:
		return structLayout(t)
	default:
		return layoutInfo{}, fmt.Errorf("%w: %s", errUnsupported, t.String())
	}
}

func structLayout(t reflect.Type) (layoutInfo, error) {
	size := 0
	align := 1
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			return layoutInfo{}, fmt.Errorf("%w: %s.%s", errUnexported, t.String(), field.Name)
		}
		fieldLayout, err := layoutOfType(field.Type)
		if err != nil {
			return layoutInfo{}, err
		}
		size = alignUp(size, fieldLayout.align)
		size += fieldLayout.size
		if fieldLayout.align > align {
			align = fieldLayout.align
		}
	}
	size = alignUp(size, align)
	return layoutInfo{size: size, align: align}, nil
}

func alignUp(value int, alignment int) int {
	if alignment <= 1 {
		return value
	}
	rem := value % alignment
	if rem == 0 {
		return value
	}
	return value + alignment - rem
}

func (enc *encoder) layoutForType(t reflect.Type) (layoutInfo, error) {
	if info, ok := enc.layout[t]; ok {
		return info, nil
	}
	info, err := layoutOfType(t)
	if err != nil {
		return layoutInfo{}, err
	}
	enc.layout[t] = info
	return info, nil
}

func (dec *decoder) layoutForType(t reflect.Type) (layoutInfo, error) {
	if info, ok := dec.layout[t]; ok {
		return info, nil
	}
	info, err := layoutOfType(t)
	if err != nil {
		return layoutInfo{}, err
	}
	dec.layout[t] = info
	return info, nil
}

func derefRoot(v reflect.Value) reflect.Value {
	for v.IsValid() && v.Kind() == reflect.Interface {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return v
	}
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return reflect.New(v.Type().Elem()).Elem()
	}
	for v.IsValid() && v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	return v
}

func (enc *encoder) writeValue(v reflect.Value) error {
	v = derefRoot(v)
	if !v.IsValid() {
		return nil
	}

	layout, err := enc.layoutForType(v.Type())
	if err != nil {
		return err
	}
	enc.stream.Current.Alignment = layout.align
	return enc.writeInline(v)
}

func (enc *encoder) writeInline(v reflect.Value) error {
	if !v.IsValid() {
		return nil
	}

	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			enc.stream.WriteU8(1)
		} else {
			enc.stream.WriteU8(0)
		}
	case reflect.Int8:
		enc.stream.WriteI8(int8(v.Int()))
	case reflect.Uint8:
		enc.stream.WriteU8(uint8(v.Uint()))
	case reflect.Int16:
		enc.stream.WriteI16(int16(v.Int()))
	case reflect.Uint16:
		enc.stream.WriteU16(uint16(v.Uint()))
	case reflect.Int32:
		enc.stream.WriteI32(int32(v.Int()))
	case reflect.Uint32:
		enc.stream.WriteU32(uint32(v.Uint()))
	case reflect.Int:
		enc.stream.WriteI32(int32(v.Int()))
	case reflect.Uint:
		enc.stream.WriteU32(uint32(v.Uint()))
	case reflect.Int64:
		enc.stream.WriteI64(v.Int())
	case reflect.Uint64:
		enc.stream.WriteU64(v.Uint())
	case reflect.Float32:
		enc.stream.WriteFloat(float32(v.Float()))
	case reflect.Float64:
		enc.stream.WriteDouble(v.Float())
	case reflect.String:
		return enc.writeString(v.String())
	case reflect.Struct:
		return enc.writeStruct(v)
	case reflect.Ptr:
		return enc.writePointer(v)
	case reflect.Slice, reflect.Array:
		return enc.writeArrayLike(v)
	case reflect.Map:
		return enc.writeMap(v)
	default:
		return fmt.Errorf("%w: %s", errUnsupported, v.Type().String())
	}
	return nil
}

func (enc *encoder) writeStruct(v reflect.Value) error {
	t := v.Type()
	layout, err := enc.layoutForType(t)
	if err != nil {
		return err
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			return fmt.Errorf("%w: %s.%s", errUnexported, t.String(), field.Name)
		}
		fieldLayout, err := enc.layoutForType(field.Type)
		if err != nil {
			return err
		}
		enc.stream.Align(fieldLayout.align)
		if err := enc.writeInline(v.Field(i)); err != nil {
			return err
		}
	}

	enc.stream.Align(layout.align)
	return nil
}

func (enc *encoder) writePointer(v reflect.Value) error {
	if v.IsNil() {
		enc.stream.WriteU32(0)
		return nil
	}

	ptr := v.Pointer()
	if enc.stack[ptr] {
		return errCycleDetected
	}
	enc.stack[ptr] = true
	defer delete(enc.stack, ptr)

	childPtr := enc.stream.OpenBlock()
	childLayout, err := enc.layoutForType(v.Type().Elem())
	if err != nil {
		return err
	}
	enc.stream.Current.Alignment = childLayout.align
	if err := enc.writeInline(v.Elem()); err != nil {
		return err
	}
	enc.stream.CloseBlock()
	enc.stream.WritePtr(childPtr)
	return nil
}

func (enc *encoder) writeString(s string) error {
	if len(s) > maxU16Value {
		return fmt.Errorf("%w: string length %d", errOverflow, len(s))
	}
	runes := utf8.RuneCountInString(s)
	if runes > maxU16Value {
		return fmt.Errorf("%w: string rune count %d", errOverflow, runes)
	}

	childPtr := enc.stream.OpenBlock()
	enc.stream.Current.Alignment = 1
	enc.stream.WriteBytes([]byte(s))
	enc.stream.WriteU8(0)
	enc.stream.CloseBlock()

	enc.stream.Align4()
	enc.stream.WriteU16(uint16(len(s)))
	enc.stream.WriteU16(uint16(runes))
	enc.stream.WritePtr(childPtr)
	return nil
}

func (enc *encoder) writeArrayLike(v reflect.Value) error {
	t := v.Type()
	elemType := t.Elem()
	elemLayout, err := enc.layoutForType(elemType)
	if err != nil {
		return err
	}

	length := v.Len()
	if v.Kind() == reflect.Slice && v.IsNil() {
		length = 0
	}

	if length == 0 {
		enc.stream.Align4()
		enc.stream.WriteU32(0)
		enc.stream.WriteU32(0)
		return nil
	}
	if length > maxU32Value {
		return fmt.Errorf("%w: array length %d", errOverflow, length)
	}

	childPtr := enc.stream.OpenBlock()
	enc.stream.Current.Alignment = elemLayout.align
	for i := 0; i < length; i++ {
		enc.stream.Align(elemLayout.align)
		if err := enc.writeInline(v.Index(i)); err != nil {
			return err
		}
	}
	enc.stream.CloseBlock()

	enc.stream.Align4()
	enc.stream.WriteU32(uint32(length))
	enc.stream.WritePtr(childPtr)
	return nil
}

func (enc *encoder) writeMap(v reflect.Value) error {
	t := v.Type()
	keyLayout, err := enc.layoutForType(t.Key())
	if err != nil {
		return err
	}
	valueLayout, err := enc.layoutForType(t.Elem())
	if err != nil {
		return err
	}

	if v.IsNil() || v.Len() == 0 {
		enc.stream.Align4()
		enc.stream.WriteU32(0)
		enc.stream.WriteU32(0)
		enc.stream.WriteU32(0)
		return nil
	}

	if keyLayout.size > maxU16Value || valueLayout.size > maxU16Value {
		return fmt.Errorf("%w: map entry size exceeds uint16 range", errOverflow)
	}

	keys := v.MapKeys()
	sort.Slice(keys, func(i, j int) bool {
		left, _ := canonicalBytes(keys[i])
		right, _ := canonicalBytes(keys[j])
		return bytes.Compare(left, right) < 0
	})

	keyBlock := enc.stream.OpenBlock()
	enc.stream.Current.Alignment = keyLayout.align
	for _, key := range keys {
		enc.stream.Align(keyLayout.align)
		if err := enc.writeInline(key); err != nil {
			return err
		}
	}
	enc.stream.CloseBlock()

	valueBlock := enc.stream.OpenBlock()
	enc.stream.Current.Alignment = valueLayout.align
	for _, key := range keys {
		enc.stream.Align(valueLayout.align)
		if err := enc.writeInline(v.MapIndex(key)); err != nil {
			return err
		}
	}
	enc.stream.CloseBlock()

	enc.stream.Align4()
	enc.stream.WriteU32(uint32(v.Len()))
	enc.stream.WritePtr(keyBlock)
	enc.stream.WritePtr(valueBlock)
	return nil
}

func canonicalBytes(v reflect.Value) ([]byte, error) {
	v = derefRoot(v)
	if !v.IsValid() {
		return nil, nil
	}

	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			return []byte{1}, nil
		}
		return []byte{0}, nil
	case reflect.Int8:
		return []byte{byte(int8(v.Int()))}, nil
	case reflect.Uint8:
		return []byte{byte(v.Uint())}, nil
	case reflect.Int16:
		buf := make([]byte, 2)
		binary.LittleEndian.PutUint16(buf, uint16(v.Int()))
		return buf, nil
	case reflect.Uint16:
		buf := make([]byte, 2)
		binary.LittleEndian.PutUint16(buf, uint16(v.Uint()))
		return buf, nil
	case reflect.Int32, reflect.Int:
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, uint32(v.Int()))
		return buf, nil
	case reflect.Uint32, reflect.Uint:
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, uint32(v.Uint()))
		return buf, nil
	case reflect.Int64:
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(v.Int()))
		return buf, nil
	case reflect.Uint64:
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, v.Uint())
		return buf, nil
	case reflect.Float32:
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, math.Float32bits(float32(v.Float())))
		return buf, nil
	case reflect.Float64:
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, math.Float64bits(v.Float()))
		return buf, nil
	case reflect.String:
		b := []byte(v.String())
		buf := make([]byte, 4+len(b))
		binary.LittleEndian.PutUint32(buf[:4], uint32(len(b)))
		copy(buf[4:], b)
		return buf, nil
	case reflect.Array, reflect.Slice:
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, uint32(v.Len()))
		for i := 0; i < v.Len(); i++ {
			chunk, err := canonicalBytes(v.Index(i))
			if err != nil {
				return nil, err
			}
			buf = append(buf, chunk...)
		}
		return buf, nil
	case reflect.Struct:
		var buf []byte
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if field.PkgPath != "" {
				return nil, fmt.Errorf("%w: %s.%s", errUnexported, v.Type().String(), field.Name)
			}
			chunk, err := canonicalBytes(v.Field(i))
			if err != nil {
				return nil, err
			}
			buf = append(buf, chunk...)
		}
		return buf, nil
	case reflect.Ptr:
		if v.IsNil() {
			return []byte{0}, nil
		}
		return canonicalBytes(v.Elem())
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupported, v.Type().String())
	}
}

func (dec *decoder) readValue(dst reflect.Value, offset int) error {
	if !dst.IsValid() {
		return nil
	}
	if dst.Kind() == reflect.Interface {
		return fmt.Errorf("%w: interface destination", errUnsupported)
	}

	switch dst.Kind() {
	case reflect.Bool:
		v, err := dec.readU8(offset)
		if err != nil {
			return err
		}
		dst.SetBool(v != 0)
	case reflect.Int8:
		v, err := dec.readU8(offset)
		if err != nil {
			return err
		}
		dst.SetInt(int64(int8(v)))
	case reflect.Uint8:
		v, err := dec.readU8(offset)
		if err != nil {
			return err
		}
		dst.SetUint(uint64(v))
	case reflect.Int16:
		v, err := dec.readU16(offset)
		if err != nil {
			return err
		}
		dst.SetInt(int64(int16(v)))
	case reflect.Uint16:
		v, err := dec.readU16(offset)
		if err != nil {
			return err
		}
		dst.SetUint(uint64(v))
	case reflect.Int32:
		v, err := dec.readU32(offset)
		if err != nil {
			return err
		}
		dst.SetInt(int64(int32(v)))
	case reflect.Uint32:
		v, err := dec.readU32(offset)
		if err != nil {
			return err
		}
		dst.SetUint(uint64(v))
	case reflect.Int:
		v, err := dec.readU32(offset)
		if err != nil {
			return err
		}
		dst.SetInt(int64(int32(v)))
	case reflect.Uint:
		v, err := dec.readU32(offset)
		if err != nil {
			return err
		}
		dst.SetUint(uint64(v))
	case reflect.Int64:
		v, err := dec.readU64(offset)
		if err != nil {
			return err
		}
		dst.SetInt(int64(v))
	case reflect.Uint64:
		v, err := dec.readU64(offset)
		if err != nil {
			return err
		}
		dst.SetUint(v)
	case reflect.Float32:
		v, err := dec.readU32(offset)
		if err != nil {
			return err
		}
		dst.SetFloat(float64(math.Float32frombits(v)))
	case reflect.Float64:
		v, err := dec.readU64(offset)
		if err != nil {
			return err
		}
		dst.SetFloat(math.Float64frombits(v))
	case reflect.String:
		return dec.readString(dst, offset)
	case reflect.Struct:
		return dec.readStruct(dst, offset)
	case reflect.Ptr:
		return dec.readPointer(dst, offset)
	case reflect.Slice:
		return dec.readSlice(dst, offset)
	case reflect.Array:
		return dec.readArray(dst, offset)
	case reflect.Map:
		return dec.readMap(dst, offset)
	default:
		return fmt.Errorf("%w: %s", errUnsupported, dst.Type().String())
	}
	return nil
}

func (dec *decoder) readStruct(dst reflect.Value, offset int) error {
	t := dst.Type()
	pos := offset
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			return fmt.Errorf("%w: %s.%s", errUnexported, t.String(), field.Name)
		}
		fieldLayout, err := dec.layoutForType(field.Type)
		if err != nil {
			return err
		}
		pos = alignUp(pos, fieldLayout.align)
		if err := dec.readValue(dst.Field(i), pos); err != nil {
			return fmt.Errorf("%s.%s: %w", t.String(), field.Name, err)
		}
		pos += fieldLayout.size
	}
	return nil
}

func (dec *decoder) readPointer(dst reflect.Value, offset int) error {
	off, err := dec.readU32(offset)
	if err != nil {
		return err
	}
	if off == 0 {
		dst.Set(reflect.Zero(dst.Type()))
		return nil
	}
	child := reflect.New(dst.Type().Elem())
	if err := dec.readValue(child.Elem(), int(off)); err != nil {
		return err
	}
	dst.Set(child)
	return nil
}

func (dec *decoder) readString(dst reflect.Value, offset int) error {
	byteLen, runeLen, dataOff, err := dec.readStringHeader(offset)
	if err != nil {
		return err
	}
	if byteLen == 0 && dataOff == 0 {
		dst.SetString("")
		return nil
	}
	if dataOff == 0 {
		return fmt.Errorf("%w: string data offset is zero", errOverflow)
	}
	data, err := dec.readBytes(int(dataOff), int(byteLen)+1)
	if err != nil {
		return err
	}
	if len(data) == 0 || data[len(data)-1] != 0 {
		return fmt.Errorf("%w: string is not null terminated", errOverflow)
	}
	value := string(data[:len(data)-1])
	if utf8.RuneCountInString(value) != int(runeLen) {
		return fmt.Errorf("%w: string character length mismatch", errOverflow)
	}
	dst.SetString(value)
	return nil
}

func (dec *decoder) readSlice(dst reflect.Value, offset int) error {
	length, dataOff, err := dec.readArrayHeader(offset)
	if err != nil {
		return err
	}
	if length == 0 || dataOff == 0 {
		dst.Set(reflect.MakeSlice(dst.Type(), 0, 0))
		return nil
	}
	dst.Set(reflect.MakeSlice(dst.Type(), int(length), int(length)))
	return dec.readArrayData(dst, int(dataOff), int(length))
}

func (dec *decoder) readArray(dst reflect.Value, offset int) error {
	length, dataOff, err := dec.readArrayHeader(offset)
	if err != nil {
		return err
	}
	if length == 0 || dataOff == 0 {
		if dst.Len() != 0 {
			return fmt.Errorf("%w: array length mismatch", errOverflow)
		}
		return nil
	}
	if int(length) != dst.Len() {
		return fmt.Errorf("%w: array length mismatch", errOverflow)
	}
	return dec.readArrayData(dst, int(dataOff), int(length))
}

func (dec *decoder) readArrayData(dst reflect.Value, offset int, length int) error {
	elemType := dst.Type().Elem()
	elemLayout, err := dec.layoutForType(elemType)
	if err != nil {
		return err
	}
	pos := offset
	for i := 0; i < length; i++ {
		pos = alignUp(pos, elemLayout.align)
		if err := dec.readValue(dst.Index(i), pos); err != nil {
			return fmt.Errorf("%s[%d]: %w", dst.Type().String(), i, err)
		}
		pos += elemLayout.size
	}
	return nil
}

func (dec *decoder) readMap(dst reflect.Value, offset int) error {
	length, keyOff, valueOff, err := dec.readMapHeader(offset)
	if err != nil {
		return err
	}
	if length == 0 || keyOff == 0 || valueOff == 0 {
		dst.Set(reflect.MakeMap(dst.Type()))
		return nil
	}

	keys := reflect.MakeSlice(reflect.SliceOf(dst.Type().Key()), int(length), int(length))
	if err := dec.readArrayData(keys, int(keyOff), int(length)); err != nil {
		return err
	}
	values := reflect.MakeSlice(reflect.SliceOf(dst.Type().Elem()), int(length), int(length))
	if err := dec.readArrayData(values, int(valueOff), int(length)); err != nil {
		return err
	}

	dst.Set(reflect.MakeMapWithSize(dst.Type(), int(length)))
	for i := 0; i < int(length); i++ {
		dst.SetMapIndex(keys.Index(i), values.Index(i))
	}
	return nil
}

func (dec *decoder) readArrayHeader(offset int) (length uint32, dataOff uint32, err error) {
	if offset < 0 || offset+arrayHeaderSize > len(dec.data) {
		return 0, 0, io.ErrUnexpectedEOF
	}
	length = binary.LittleEndian.Uint32(dec.data[offset+0 : offset+4])
	dataOff = binary.LittleEndian.Uint32(dec.data[offset+4 : offset+8])
	return length, dataOff, nil
}

func (dec *decoder) readStringHeader(offset int) (byteLen uint16, charLen uint16, dataOff uint32, err error) {
	if offset < 0 || offset+stringHeaderSize > len(dec.data) {
		return 0, 0, 0, io.ErrUnexpectedEOF
	}
	byteLen = binary.LittleEndian.Uint16(dec.data[offset : offset+2])
	charLen = binary.LittleEndian.Uint16(dec.data[offset+2 : offset+4])
	dataOff = binary.LittleEndian.Uint32(dec.data[offset+4 : offset+8])
	return byteLen, charLen, dataOff, nil
}

func (dec *decoder) readMapHeader(offset int) (length uint32, keyOff uint32, valueOff uint32, err error) {
	if offset < 0 || offset+mapHeaderSize > len(dec.data) {
		return 0, 0, 0, io.ErrUnexpectedEOF
	}
	length = binary.LittleEndian.Uint32(dec.data[offset+0 : offset+4])
	keyOff = binary.LittleEndian.Uint32(dec.data[offset+4 : offset+8])
	valueOff = binary.LittleEndian.Uint32(dec.data[offset+8 : offset+12])
	return length, keyOff, valueOff, nil
}

func (dec *decoder) readU8(offset int) (uint8, error) {
	if offset < 0 || offset+1 > len(dec.data) {
		return 0, io.ErrUnexpectedEOF
	}
	return dec.data[offset], nil
}

func (dec *decoder) readU16(offset int) (uint16, error) {
	if offset < 0 || offset+2 > len(dec.data) {
		return 0, io.ErrUnexpectedEOF
	}
	return binary.LittleEndian.Uint16(dec.data[offset : offset+2]), nil
}

func (dec *decoder) readU32(offset int) (uint32, error) {
	if offset < 0 || offset+4 > len(dec.data) {
		return 0, io.ErrUnexpectedEOF
	}
	return binary.LittleEndian.Uint32(dec.data[offset : offset+4]), nil
}

func (dec *decoder) readU64(offset int) (uint64, error) {
	if offset < 0 || offset+8 > len(dec.data) {
		return 0, io.ErrUnexpectedEOF
	}
	return binary.LittleEndian.Uint64(dec.data[offset : offset+8]), nil
}

func (dec *decoder) readBytes(offset int, size int) ([]byte, error) {
	if offset < 0 || size < 0 || offset+size > len(dec.data) {
		return nil, io.ErrUnexpectedEOF
	}
	return dec.data[offset : offset+size], nil
}
