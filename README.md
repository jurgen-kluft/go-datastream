Introducing `data stream`
------------------

A data stream is a specialized data writer that can write 'blocks' of data and 'pointers' or 'references' to 'blocks'.
Once the stream is being finalized all 'blocks' and 'references' are resolved and you will get an additional stream of data that contain the offsets to the 'pointers'.

The main use-case for this library is to support writing C/C++ data structures to a stream and being able to read them directly into memory.

