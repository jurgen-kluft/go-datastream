package codestream

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"

	"github.com/jurgen-kluft/go-datastream/datastream"
)

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

type testBank struct {
	Name string
	USD  float64
}

type testStruct struct {
	Name       string
	Age        int
	Weird      uint8
	Scores     []int32
	Friends    []string
	Bank       *testBank
	Attributes map[string]int
}

func TestCodeStreamReplacesContextForEachOperation(t *testing.T) {
	codeStream := NewCodeStream(Options{})
	initialContext := codeStream.Context()
	if initialContext != nil || codeStream.HasErrors() || len(codeStream.Issues()) != 0 {
		t.Fatalf("initial diagnostics = %v", codeStream.Issues())
	}

	if codeStream.WriteStream(&bytes.Buffer{}, nil) {
		t.Fatal("WriteStream accepted nil data")
	}
	failedContext := codeStream.Context()
	if failedContext == nil || !codeStream.HasErrors() {
		t.Fatalf("failed diagnostics = %v", codeStream.Issues())
	}

	if !codeStream.WriteStream(&bytes.Buffer{}, uint32(7)) {
		t.Fatalf("successful WriteStream failed: %v", codeStream.Issues())
	}
	if codeStream.Context() == failedContext || codeStream.HasErrors() || len(codeStream.Issues()) != 0 {
		t.Fatalf("successful operation retained diagnostics: %v", codeStream.Issues())
	}
}

func TestCodeStreamWriterErrorReturnsFalse(t *testing.T) {
	codeStream := NewCodeStream(Options{})
	if codeStream.WriteStream(errorWriter{}, uint32(7)) {
		t.Fatal("WriteStream succeeded with a failing writer")
	}
	if !codeStream.HasErrors() {
		t.Fatalf("issues = %v, want writer error", codeStream.Issues())
	}
}

func TestCodeStreamVerboseInformation(t *testing.T) {
	codeStream := NewCodeStream(Options{Verbose: true})
	if !codeStream.WriteStream(&bytes.Buffer{}, uint32(7)) {
		t.Fatalf("WriteStream failed: %v", codeStream.Issues())
	}
	for _, issue := range codeStream.Issues() {
		if issue.IssueType == datastream.IssueTypeInformation {
			return
		}
	}
	t.Fatalf("issues = %v, want information", codeStream.Issues())
}

func TestCodeStreamRoundTrip(t *testing.T) {
	original := &testStruct{
		Name:    "alice",
		Age:     33,
		Weird:   7,
		Scores:  []int32{10, 20, 30},
		Friends: []string{"bob", "carol"},
		Bank:    &testBank{Name: "main", USD: 123.5},
		Attributes: map[string]int{
			"beta":  2,
			"alpha": 1,
		},
	}

	var buf bytes.Buffer
	codeStream := NewCodeStream(Options{})
	if !codeStream.WriteStream(&buf, original) {
		t.Fatalf("WriteStream failed: %v", codeStream.Issues())
	}

	var decoded testStruct
	if !codeStream.ReadStream(bytes.NewReader(buf.Bytes()), &decoded) {
		t.Fatalf("ReadStream failed: %v", codeStream.Issues())
	}

	if !reflect.DeepEqual(original, &decoded) {
		t.Fatalf("round trip mismatch\noriginal: %#v\ndecoded: %#v", original, &decoded)
	}
}

func TestCodeStreamDeterministicMapEncoding(t *testing.T) {
	value := &testStruct{
		Name:  "same",
		Age:   12,
		Weird: 1,
		Attributes: map[string]int{
			"z": 26,
			"a": 1,
			"m": 13,
		},
	}

	var first bytes.Buffer
	codeStream := NewCodeStream(Options{})
	if !codeStream.WriteStream(&first, value) {
		t.Fatalf("first WriteStream failed: %v", codeStream.Issues())
	}

	var second bytes.Buffer
	if !codeStream.WriteStream(&second, value) {
		t.Fatalf("second WriteStream failed: %v", codeStream.Issues())
	}

	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("expected deterministic encoding, got different byte streams")
	}
}

func TestCodeStreamContainerHeadersArePointerFirst(t *testing.T) {
	type layoutFixture struct {
		Values []uint32
		Lookup map[uint16]uint32
		Suffix uint32
	}

	value := layoutFixture{
		Values: []uint32{0x11223344, 0x55667788},
		Lookup: map[uint16]uint32{
			7: 9,
		},
		Suffix: 13,
	}
	var buffer bytes.Buffer
	codeStream := NewCodeStream(Options{})
	if !codeStream.WriteStream(&buffer, value) {
		t.Fatalf("WriteStream failed: %v", codeStream.Issues())
	}
	data := buffer.Bytes()
	if len(data) < 20 {
		t.Fatalf("encoded size = %d, want at least 20", len(data))
	}

	valuesTarget := relativeTarget32(t, data, 0)
	if got := binary.LittleEndian.Uint32(data[4:8]); got != 2 {
		t.Fatalf("slice length = %d, want 2", got)
	}
	if got := binary.LittleEndian.Uint32(data[valuesTarget : valuesTarget+4]); got != 0x11223344 {
		t.Fatalf("first slice value = %#x", got)
	}

	keysTarget := relativeTarget32(t, data, 8)
	valuesMapTarget := relativeTarget32(t, data, 12)
	if got := binary.LittleEndian.Uint32(data[16:20]); got != 1 {
		t.Fatalf("map length = %d, want 1", got)
	}
	if got := binary.LittleEndian.Uint16(data[keysTarget : keysTarget+2]); got != 7 {
		t.Fatalf("map key = %d, want 7", got)
	}
	if got := binary.LittleEndian.Uint32(data[valuesMapTarget : valuesMapTarget+4]); got != 9 {
		t.Fatalf("map value = %d, want 9", got)
	}
	if got := binary.LittleEndian.Uint32(data[20:24]); got != 13 {
		t.Fatalf("suffix = %d, want 13", got)
	}
}

func TestCodeStream64BitContainersUseNaturalAlignment(t *testing.T) {
	type layoutFixture struct {
		Prefix uint32
		Text   string
		Values []uint16
		Lookup map[uint8]uint32
		Suffix uint32
	}

	value := layoutFixture{
		Prefix: 1,
		Text:   "hi",
		Values: []uint16{3, 5},
		Lookup: map[uint8]uint32{7: 11},
		Suffix: 13,
	}
	var buffer bytes.Buffer
	options := Options{PointerIs64Bit: true, Endian: binary.LittleEndian}
	codeStream := NewCodeStream(options)
	if !codeStream.WriteStream(&buffer, value) {
		t.Fatalf("WriteStream failed: %v", codeStream.Issues())
	}
	data := buffer.Bytes()
	if got := len(data); got < 68 {
		t.Fatalf("encoded size = %d, want at least 68", got)
	}
	if got := binary.LittleEndian.Uint16(data[16:18]); got != 2 {
		t.Fatalf("string byte length = %d, want 2", got)
	}
	if got := binary.LittleEndian.Uint32(data[32:36]); got != 2 {
		t.Fatalf("slice length = %d, want 2", got)
	}
	if got := binary.LittleEndian.Uint32(data[56:60]); got != 1 {
		t.Fatalf("map length = %d, want 1", got)
	}
	if got := binary.LittleEndian.Uint32(data[64:68]); got != 13 {
		t.Fatalf("suffix = %d, want 13", got)
	}

	var decoded layoutFixture
	if !codeStream.ReadStream(bytes.NewReader(data), &decoded) {
		t.Fatalf("ReadStream failed: %v", codeStream.Issues())
	}
	if !reflect.DeepEqual(decoded, value) {
		t.Fatalf("round trip mismatch: got %#v want %#v", decoded, value)
	}
}

func TestCodeStreamEmptyContainersEncodeNullHeaders(t *testing.T) {
	type emptyFixture struct {
		Values []uint32
		Lookup map[uint32]uint32
	}
	var buffer bytes.Buffer
	codeStream := NewCodeStream(Options{})
	if !codeStream.WriteStream(&buffer, emptyFixture{}) {
		t.Fatalf("WriteStream failed: %v", codeStream.Issues())
	}
	want := make([]byte, 20)
	if !bytes.Equal(buffer.Bytes(), want) {
		t.Fatalf("empty headers = %v, want %v", buffer.Bytes(), want)
	}
}

func TestCodeStreamFixedArrayRoundTripsThroughHeader(t *testing.T) {
	type arrayFixture struct {
		Values [3]uint16
	}
	want := arrayFixture{Values: [3]uint16{2, 4, 6}}
	var buffer bytes.Buffer
	codeStream := NewCodeStream(Options{})
	if !codeStream.WriteStream(&buffer, want) {
		t.Fatalf("WriteStream failed: %v", codeStream.Issues())
	}
	data := buffer.Bytes()
	if got := binary.LittleEndian.Uint32(data[4:8]); got != 3 {
		t.Fatalf("array length = %d, want 3", got)
	}
	target := relativeTarget32(t, data, 0)
	if target%2 != 0 {
		t.Fatalf("array target %d is not naturally aligned", target)
	}

	var got arrayFixture
	if !codeStream.ReadStream(bytes.NewReader(data), &got) {
		t.Fatalf("ReadStream failed: %v", codeStream.Issues())
	}
	if got != want {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}

func TestCodeStreamRejectsContradictoryContainerHeaders(t *testing.T) {
	t.Run("slice length without data", func(t *testing.T) {
		data := make([]byte, arrayHeaderSize32)
		binary.LittleEndian.PutUint32(data[4:8], 1)
		var value []uint32
		codeStream := NewCodeStream(Options{})
		if codeStream.ReadStream(bytes.NewReader(data), &value) {
			t.Fatal("ReadStream accepted a non-empty slice with a null data pointer")
		}
	})

	t.Run("map length without data", func(t *testing.T) {
		data := make([]byte, mapHeaderSize32)
		binary.LittleEndian.PutUint32(data[8:12], 1)
		var value map[uint32]uint32
		codeStream := NewCodeStream(Options{})
		if codeStream.ReadStream(bytes.NewReader(data), &value) {
			t.Fatal("ReadStream accepted a non-empty map with null data pointers")
		}
	})
}

func relativeTarget32(t *testing.T, data []byte, fieldOffset int) int {
	t.Helper()
	displacement := int32(binary.LittleEndian.Uint32(data[fieldOffset : fieldOffset+4]))
	if displacement == 0 {
		t.Fatalf("relative pointer at %d is null", fieldOffset)
	}
	target := fieldOffset + int(displacement)
	if target < 0 || target >= len(data) {
		t.Fatalf("relative pointer at %d targets %d outside %d bytes", fieldOffset, target, len(data))
	}
	return target
}
