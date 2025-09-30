package fromauthserver

import (
	"fmt"
	"reflect"
	"testing"
	"unsafe"
)

// PackedInitHeader - аналог структуры на Rust.
type PackedInitHeader struct {
	SessionID       int32
	ProtocolVersion int32
	RSAPublicKey    [128]byte
	GameGuard1      int32
	GameGuard2      int32
	GameGuard3      int32
	GameGuard4      int32
}

// InitPacketView - аналог структуры-представления из Rust.
type InitPacketView struct {
	Header      *PackedInitHeader
	BlowfishKey []byte
}

// dataForBenchmarkV2 - создает тестовый срез байт.
func dataForBenchmarkV2() []byte {
	data := make([]byte, 1024)
	for i := 0; i < 1024; i++ {
		data[i] = byte(i)
	}
	return data
}

// -----------------------------------------------------------------------------
// НОВАЯ, ОПТИМИЗИРОВАННАЯ ВЕРСИЯ (ZERO-ALLOC)
// -----------------------------------------------------------------------------

// parseInitPacketPackedInto заполняет существующую структуру InitPacketView, избегая аллокаций.
func parseInitPacketPackedInto(result *InitPacketView, data []byte) error {
	headerSize := int(unsafe.Sizeof(PackedInitHeader{}))
	if len(data) < headerSize {
		return fmt.Errorf("недостаточно данных для заголовка: нужно %d, есть %d", headerSize, len(data))
	}

	// Операция остается прежней: это просто "каст" указателя, копирования данных нет.
	result.Header = (*PackedInitHeader)(unsafe.Pointer(&data[0]))
	result.BlowfishKey = data[headerSize:]

	return nil
}

// BenchmarkParseInitPacketPackedNoAllocs - бенчмарк для версии без аллокаций.
func BenchmarkParseInitPacketPackedNoAllocs(b *testing.B) {
	data := dataForBenchmarkV2()
	// Создаем структуру ОДИН РАЗ на стеке, вне цикла.
	var packet InitPacketView
	var err error

	b.ReportAllocs() // Явно указываем, что нужно отслеживать аллокации.
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Передаем указатель на нашу структуру.
		// Новая память в цикле не выделяется.
		err = parseInitPacketPackedInto(&packet, data)
		if err != nil {
			b.Fatal(err)
		}
	}

	// Проверка, чтобы компилятор не выкинул код и чтобы убедиться в корректности.
	if packet.Header.SessionID != 50462976 {
		b.Fatalf("неверный session id: получили %d, ожидали 50462976", packet.Header.SessionID)
	}
}

// -----------------------------------------------------------------------------
// СТАРАЯ ВЕРСИЯ (С АЛЛОКАЦИЕЙ) - для сравнения
// -----------------------------------------------------------------------------

func parseInitPacketPackedWithAlloc(data []byte) (*InitPacketView, error) {
	headerSize := int(unsafe.Sizeof(PackedInitHeader{}))
	if len(data) < headerSize {
		return nil, fmt.Errorf("недостаточно данных для заголовка: нужно %d, есть %d", headerSize, len(data))
	}

	header := (*PackedInitHeader)(unsafe.Pointer(&data[0]))

	return &InitPacketView{ // Эта строка приводит к аллокации
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

	for i := 0; i < b.N; i++ {
		packet, err = parseInitPacketPackedWithAlloc(data)
		if err != nil {
			b.Fatal(err)
		}
	}

	if packet.Header.SessionID != 50462976 {
		b.Fatalf("неверный session id: получили %d, ожидали 50462976", packet.Header.SessionID)
	}
}

// TestStructSize - служебный тест для проверки корректности размера структуры.
func TestStructSize(t *testing.T) {
	expectedSize := 4 + 4 + 128 + 4 + 4 + 4 + 4
	actualSize := unsafe.Sizeof(PackedInitHeader{})

	if uintptr(expectedSize) != actualSize {
		t.Errorf("Размер структуры не совпадает! Ожидалось: %d, получено: %d", expectedSize, actualSize)
	}

	t.Logf("Размер структуры PackedInitHeader: %d байт", actualSize)
	headerType := reflect.TypeOf(PackedInitHeader{})
	for i := 0; i < headerType.NumField(); i++ {
		field := headerType.Field(i)
		t.Logf("Поле %s: смещение %d, размер %d", field.Name, field.Offset, field.Type.Size())
	}
}
