package datastream

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"strings"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) {
	return len(data) - 1, nil
}

func TestNewStream(t *testing.T) {
	ctx := NewContext()
	stream := NewStream(SizeOfPointer64, binary.LittleEndian, ctx)
	if stream.Endian != binary.LittleEndian {
		t.Error("Expected LittleEndian")
	}
	if stream.Alignment != 8 {
		t.Error("Expected alignment 8")
	}
	if stream.Current == nil {
		t.Error("Expected Current not nil")
	}
	if len(stream.Stack) != 1 {
		t.Error("Expected Stack len 1")
	}
	if len(stream.Blocks) != 1 {
		t.Error("Expected Blocks len 1")
	}
	ctx.AddWarning("shared")
	if !stream.HasWarnings() {
		t.Fatal("stream does not use the injected Context")
	}
}

func TestNewStreamRejectsNilContext(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewStream accepted a nil Context")
		}
	}()
	NewStream(SizeOfPointer32, binary.LittleEndian, nil)
}

func TestOpenCloseBlock(t *testing.T) {
	stream := NewStream(SizeOfPointer64, binary.LittleEndian, NewContext())
	initialCurrent := stream.Current
	stream.OpenBlock()
	if stream.Current == initialCurrent {
		t.Error("Expected Current to change")
	}
	if len(stream.Blocks) != 2 {
		t.Error("Expected Blocks len 2")
	}
	if len(stream.Stack) != 2 {
		t.Error("Expected Stack len 2")
	}
	stream.CloseBlock()
	if stream.Current != initialCurrent {
		t.Error("Expected Current back to initial")
	}
	if len(stream.Stack) != 1 {
		t.Error("Expected Stack len 1 after close")
	}
}

func TestWriteMethods(t *testing.T) {
	stream := NewStream(SizeOfPointer64, binary.LittleEndian, NewContext())
	stream.WriteI8(42)
	stream.Align2() // you need to align manually
	stream.WriteU16(0x1234)
	stream.Align4()
	str := "test"
	stream.WriteU32(uint32(len(str)))
	stream.WriteBytes([]byte(str))
	stream.WriteU8(0) // null terminator
	data := stream.Current.Writer.Bytes()
	// I8: 42
	// U16 align 2: pos=1, gap=1, write 0, then 34 12
	// String: align 4: pos=4, gap=0, len=4,0,0,0, t,e,s,t,0
	expected := []byte{42, 0, 0x34, 0x12, 4, 0, 0, 0, 't', 'e', 's', 't', 0}
	if !bytes.Equal(data, expected) {
		t.Errorf("Expected %v, got %v", expected, data)
	}
}

func TestFinalize(t *testing.T) {
	stream := NewStream(SizeOfPointer64, binary.LittleEndian, NewContext())
	stream.WriteI8(1)
	ptr := stream.OpenBlock()
	stream.WriteI8(2)
	stream.CloseBlock()
	stream.Align8()      // align before writing pointer
	stream.WritePtr(ptr) // ptr to the second block

	// Create temp files
	dataFile, err := os.CreateTemp("", "data")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(dataFile.Name())

	relocFile, err := os.CreateTemp("", "reloc")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(relocFile.Name())

	stream.Finalize()

	writer := bytes.NewBuffer(nil)
	pointerOffsets := stream.Write(writer)
	if stream.HasErrors() {
		t.Fatal(stream.Issues())
	}
	if len(pointerOffsets) != 1 {
		t.Fatalf("Expected 1 pointer offset, got %d", len(pointerOffsets))
	}
	if pointerOffsets[0] != 8 {
		t.Errorf("Expected pointer offset 8, got %d", pointerOffsets[0])
	}

	// Read data file
	data := writer.Bytes()

	// Block 0: 1, 7 bytes padding, 8-byte pointer to block 1
	// Block 1: 2
	// Total 17 bytes
	expectedData := make([]byte, 17)
	expectedData[0] = 1
	// The pointer field is at byte 8 and the target block starts at byte 16.
	binary.LittleEndian.PutUint64(expectedData[8:16], 8)
	expectedData[16] = 2
	if !bytes.Equal(data, expectedData) {
		t.Errorf("Expected data %v, got %v", expectedData, data)
	}

}

func TestUnresolvedPointerCreatesWarningAtWritePtrCaller(t *testing.T) {
	stream := NewStream(SizeOfPointer32, binary.LittleEndian, NewContext())
	_, file, line, _ := runtime.Caller(0)
	stream.WritePtr(Pointer{Index: 99})

	var output bytes.Buffer
	offsets := stream.Write(&output)
	if stream.HasErrors() || !stream.HasWarnings() {
		t.Fatalf("issues = %v, want warning only", stream.Issues())
	}
	if len(offsets) != 1 || len(output.Bytes()) != 4 {
		t.Fatalf("offsets = %v, output size = %d", offsets, output.Len())
	}
	issues := stream.Issues()
	wantLocation := file + ":" + fmt.Sprint(line+1)
	if len(issues) != 1 || !strings.Contains(issues[0].Description, wantLocation) {
		t.Fatalf("issues = %v, want location %s", issues, wantLocation)
	}
}

func TestPointerOverflowCollectsErrorAndSuppressesOutput(t *testing.T) {
	stream := NewStream(SizeOfPointer32, binary.LittleEndian, NewContext())
	stream.Current.writePtr(stream.context, Pointer{Index: 1}, sourceLocation{file: "fixture.go", line: 10})
	stream.Finalize()
	stream.Blocks[0].Size = int64(math.MaxInt32) + 8
	stream.Blocks = append(stream.Blocks, NewBlock(Pointer{Index: 1, Offset: -1}, SizeOfPointer32, binary.LittleEndian, 4))
	stream.Blocks[1].Size = 1

	var output bytes.Buffer
	if offsets := stream.Write(&output); offsets != nil {
		t.Fatalf("offsets = %v, want nil", offsets)
	}
	if !stream.HasErrors() || output.Len() != 0 {
		t.Fatalf("issues = %v, output size = %d", stream.Issues(), output.Len())
	}
}

func TestStreamLifecycleErrorsDoNotPanic(t *testing.T) {
	stream := NewStream(SizeOfPointer32, binary.LittleEndian, NewContext())
	stream.CloseBlock()
	stream.Align(3)
	stream.OpenBlock()
	stream.Finalize()

	if !stream.HasErrors() {
		t.Fatalf("issues = %v, want lifecycle errors", stream.Issues())
	}
	var output bytes.Buffer
	if offsets := stream.Write(&output); offsets != nil || output.Len() != 0 {
		t.Fatalf("offsets = %v, output size = %d", offsets, output.Len())
	}
}

func TestVerboseInformation(t *testing.T) {
	stream := NewStream(SizeOfPointer32, binary.LittleEndian, NewContext())
	stream.SetVerbose(true)
	stream.WriteU8(1)
	stream.Align4()
	var output bytes.Buffer
	stream.Write(&output)

	foundInformation := false
	for _, issue := range stream.Issues() {
		foundInformation = foundInformation || issue.IssueType == IssueTypeInformation
	}
	if !foundInformation {
		t.Fatalf("issues = %v, want verbose information", stream.Issues())
	}
}

func TestPointersToDeduplicatedBlocksRemainResolved(t *testing.T) {
	stream := NewStream(SizeOfPointer32, binary.LittleEndian, NewContext())
	stream.SetVerbose(true)
	first := stream.OpenBlock()
	stream.WriteU8(7)
	stream.CloseBlock()
	second := stream.OpenBlock()
	stream.WriteU8(7)
	stream.CloseBlock()
	stream.WritePtr(first)
	stream.WritePtr(second)

	var output bytes.Buffer
	stream.Write(&output)
	if stream.HasErrors() || stream.HasWarnings() {
		t.Fatalf("issues = %v, want no pointer diagnostics", stream.Issues())
	}
	firstTarget := int64(int32(binary.LittleEndian.Uint32(output.Bytes()[0:4])))
	secondTarget := 4 + int64(int32(binary.LittleEndian.Uint32(output.Bytes()[4:8])))
	if firstTarget != secondTarget {
		t.Fatalf("pointer targets = %d and %d, want identical canonical target", firstTarget, secondTarget)
	}
	resolvedCount := 0
	for _, issue := range stream.Issues() {
		if issue.IssueType == IssueTypeInformation && strings.HasPrefix(issue.Description, "resolved pointer") {
			resolvedCount++
		}
	}
	if resolvedCount != 2 {
		t.Fatalf("resolved pointer information count = %d, want 2; issues = %v", resolvedCount, stream.Issues())
	}
}

func TestWriterFailureCreatesError(t *testing.T) {
	for name, writer := range map[string]io.Writer{
		"returned error": failingWriter{},
		"short write":    shortWriter{},
	} {
		t.Run(name, func(t *testing.T) {
			stream := NewStream(SizeOfPointer32, binary.LittleEndian, NewContext())
			stream.WriteU8(1)
			if offsets := stream.Write(writer); offsets != nil {
				t.Fatalf("offsets = %v, want nil", offsets)
			}
			if !stream.HasErrors() {
				t.Fatalf("issues = %v, want writer error", stream.Issues())
			}
		})
	}
}
