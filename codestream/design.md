I would like the following reader and writer for a binary format that mimics C/C++ data structures, including strings, pointers, arrays and simple key-value maps. The reader and writer should be build on top of the datastream package. The datastream package provides a way to read and write binary data with support for pointers and relocations, which is essential for implementing a format that mimics C/C++ data structures. When the binary serialization encounters a pointer, string, array or map, it will write the data to a separate block and write a pointer to that block in the main data stream. The reader will then be able to read the main data stream and follow the pointers to read the actual data from the blocks. The datastream package already does deduplication of blocks using SHA1 hashing, which will help to reduce the size of the binary data when there are duplicate strings, arrays or maps.

The code stream package should use reflection to automatically serialize and deserialize Go data structures into the binary format. 
Since we will mainly use this to write a main Golang datastructure, which will contain pointers and references to other data structures, operations are owned by a CodeStream:

	stream := NewCodeStream(options)
	ok := stream.WriteStream(w, data)

For the reader:

	ok := stream.ReadStream(r, data)

Each operation creates a fresh diagnostic Context. A false result means errors were collected. Warnings and verbose information remain available through the CodeStream but do not fail the operation.

As you can see, both the writer and reader should follow a deterministic method of writing and reading the data, so that the same data structure will always produce the same binary output, and the reader will be able to read it back correctly. The writer will need to keep track of all the pointers, strings, arrays and maps that it encounters during serialization, and write them to separate blocks in the binary format. The reader will need to read the main data stream first, and then follow the pointers to read the actual data from the blocks.

Noteworthy design decisions and constraints for the code stream package include:

- The final data file size must be < 2GB, so we can use 32-bit offsets for pointers and block locations.
- The binary format should be platform-independent, so we will use little-endian encoding for all multi-byte values.
- The binary format will NOT include metadata about the types of the data, so the reader will need to know the expected structure of the data in order to read it correctly. This means that the reader and writer must be used together with a predefined data structure, and the reader must be aware of the layout of the data in order to read it correctly.

So if we have the following data structure in Golang:

type MyStruct struct {
	Name string
	Age int
	Weird uint8
	Scores []int32
	Friends []string
	Bank *MyBank
	Attributes map[string]int
}

type MyBank struct {
	Name string
	USD float64
}

A string is stored with the relative pointer first:

type string_t struct // alignment should be 4 bytes
{
	DataOffset int32 // signed displacement from this field to the null-terminated UTF-8 data
	ByteLength uint16 // number of bytes in the UTF-8 encoded string, excluding the null terminator
	CharLength uint16 // number of Unicode code points
}

An array or slice will be stored as the following fixed structure in the binary format:

type array_t struct  // alignment should be 4 bytes
{
	DataOffset int32 // signed displacement from this field to the contiguous element data
	Length uint32    // number of elements in the array
}

A key-value map will be stored as the following fixed structure in the binary format:

type map_t struct // size 12, alignment 4
{
	KeyArrayDataOffset int32   // signed displacement to contiguous keys
	ValueArrayDataOffset int32 // signed displacement to contiguous values
	Length uint32              // number of key-value pairs in the map
}

When 64-bit pointers are selected, each pointer field is 8 bytes and naturally aligned to 8 bytes. String and array headers are 16 bytes, and map headers are 24 bytes, including trailing padding.

So the above MyStruct would be serialized as follows:

Note: DataBlock for MyStruct (alignment of DataBlock depends on the the largest type in the struct, which is string_t with alignment of 4 bytes, so the DataBlock alignment for MyStruct should be set to 4 bytes):

[string_t for Name]     (size = 8 bytes)
[int32 for Age]         (size = 4 bytes)
[uint8 for Weird]       (size = 1 byte)
[PADDING of 3 bytes to align to 4 bytes for the next field which is a uint32 offset for the Scores array]
[array_t for Scores]    (size = 8 bytes)
[array_t for Friends]   (size = 8 bytes)
[uint32 offset for Bank pointer] (size = 4 bytes)
[map_t for Attributes]  (size = 12 bytes)

Note: DataBlock for Bank struct (alignment of DataBlock depends on the the largest type in the struct, which is float64 with alignment of 8 bytes, so the DataBlock alignment for Bank struct should be set to 8 bytes):

[string data for Name] (size = 8 bytes)
[float64 for USD] (size = 8 bytes)
