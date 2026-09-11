//go:build windows

package memmod

import (
	"testing"
	"unsafe"
)

func TestSectionsWithinSmallImage(t *testing.T) {
	const count = 2
	headerSize := unsafe.Sizeof(IMAGE_NT_HEADERS{})
	data := make([]byte, int(headerSize)+count*int(unsafe.Sizeof(IMAGE_SECTION_HEADER{})))
	header := (*IMAGE_NT_HEADERS)(unsafe.Pointer(&data[0]))
	header.FileHeader.SizeOfOptionalHeader = uint16(unsafe.Sizeof(header.OptionalHeader))
	header.FileHeader.NumberOfSections = count
	sections := header.Sections()
	if len(sections) != count || cap(sections) != count {
		t.Fatalf("section bounds: len=%d cap=%d", len(sections), cap(sections))
	}
	sections[1].VirtualAddress = 0x2000
	if header.Sections()[1].VirtualAddress != 0x2000 {
		t.Fatal("section view does not reference the source image")
	}
}
