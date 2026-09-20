package fromauthserver

import (
    "fmt"
    "reflect"
    "testing"
    "unsafe"
)

// PackedInitHeader mirrors the Rust struct.
type PackedInitHeader struct {
    SessionID       int32
    ProtocolVersion int32
    RSAPublicKey    [128]byte
    GameGuard1      int32
    GameGuard2      int32
    GameGuard3      int32
    GameGuard4      int32
}

// InitPacketView mirrors the Rust view struct.
type InitPacketView struct {
    Header      *PackedInitHeader
    BlowfishKey []byte
}

// dataForBenchmarkV2 builds the test byte slice.
func dataForBenchmarkV2() []byte {
    data := make([]byte, 1024)
    for i := range 1024 {
        data[i] = byte(i)
    }

    return data
}

// -----------------------------------------------------------------------------
// The new optimized version (zero alloc)
// -----------------------------------------------------------------------------

// parseInitPacketPackedInto fills the caller's InitPacketView
// without the allocations.
func parseInitPacketPackedInto(result *InitPacketView, data []byte) error {
    headerSize := int(unsafe.Sizeof(PackedInitHeader{}))
    if len(data) < headerSize {
        return fmt.Errorf("not enough data for the header: need %d, have %d",
            headerSize, len(data))
    }

    // The operation stays the same: a pointer cast, no data copy.
    result.Header = (*PackedInitHeader)(unsafe.Pointer(&data[0]))
    result.BlowfishKey = data[headerSize:]

    return nil
}

// BenchmarkParseInitPacketPackedNoAllocs benches the zero alloc
// version.
func BenchmarkParseInitPacketPackedNoAllocs(b *testing.B) {
    data := dataForBenchmarkV2()
    // The struct is built ONCE on the stack, outside the loop.
    var packet InitPacketView
    var err error

    b.ReportAllocs() // the allocation tracking is explicit.
    b.ResetTimer()

    for range b.N {
        // The pointer of the struct goes in; the loop allocates
        // no new memory.
        err = parseInitPacketPackedInto(&packet, data)
        if err != nil {
            b.Fatal(err)
        }
    }

    // The check keeps the compiler from eliding the work and
    // verifies the parse.
    if packet.Header.SessionID != 50462976 {
        b.Fatalf("wrong session id: got %d, want 50462976",
            packet.Header.SessionID)
    }
}

// -----------------------------------------------------------------------------
// The old version (with the allocation) - the comparison base.
// -----------------------------------------------------------------------------

func parseInitPacketPackedWithAlloc(data []byte) (*InitPacketView, error) {
    headerSize := int(unsafe.Sizeof(PackedInitHeader{}))
    if len(data) < headerSize {
        return nil, fmt.Errorf("not enough data for the header: need %d, have %d",
            headerSize, len(data))
    }

    header := (*PackedInitHeader)(unsafe.Pointer(&data[0]))

    return &InitPacketView{ // this line is the allocation
        Header:      header,
        BlowfishKey: data[headerSize:],
    }, nil
}

func BenchmarkParseInitPacketPackedWithAlloc(b *testing.B) {
    data := dataForBenchmarkV2()
    var packet *InitPacketView
    var err error

    b.ReportAllocs()
    b.ResetTimer()

    for range b.N {
        packet, err = parseInitPacketPackedWithAlloc(data)
        if err != nil {
            b.Fatal(err)
        }
    }

    if packet.Header.SessionID != 50462976 {
        b.Fatalf("wrong session id: got %d, want 50462976",
            packet.Header.SessionID)
    }
}

// TestStructSize checks the struct size of the packed header.
func TestStructSize(t *testing.T) {
    expectedSize := 4 + 4 + 128 + 4 + 4 + 4 + 4
    actualSize := unsafe.Sizeof(PackedInitHeader{})

    if uintptr(expectedSize) != actualSize {
        t.Errorf("struct size mismatch: want %d, got %d",
            expectedSize, actualSize)
    }

    t.Logf("the PackedInitHeader struct size: %d bytes", actualSize)
    headerType := reflect.TypeOf(PackedInitHeader{})
    for i := range headerType.NumField() {
        field := headerType.Field(i)
        t.Logf("field %s: offset %d, size %d", field.Name,
            field.Offset, field.Type.Size())
    }
}
