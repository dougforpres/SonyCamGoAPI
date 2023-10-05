package winstruct

import (
	"bytes"
	"fmt"
	"log"
	"math"
	"reflect"
	"strconv"
	"syscall"
	"unsafe"
)

type winType struct {
	Field       string
	WinTypeName string
	Size        int
	FromBytes   func(b *bytes.Buffer, t target, options tagOptions)
	ToBytes     func(t target) []byte
}

type field struct {
	Name    string
	Type    winType
	Options tagOptions
	Value   reflect.Value
}

type meta struct {
	Size   int
	Fields []field
}

type target struct {
	Struct   reflect.Value
	Property reflect.Value
}

var winTypes = []winType{
	{
		WinTypeName: "WORD",
		Size:        2,
		FromBytes:   bytesToUint16,
		ToBytes:     uint16ToBytes,
	},
	{
		WinTypeName: "DWORD",
		Size:        4,
		FromBytes:   bytesToUint32,
		ToBytes:     uint32ToBytes,
	}, {
		WinTypeName: "DWORD32",
		Size:        4,
		FromBytes:   bytesToUint32,
		ToBytes:     uint32ToBytes,
	}, {
		WinTypeName: "DWORD64",
		Size:        8,
		FromBytes:   bytesToUint64,
		ToBytes:     uint64ToBytes,
	}, {
		WinTypeName: "QWORD",
		Size:        8,
		FromBytes:   bytesToUint64,
		ToBytes:     uint64ToBytes,
	}, {
		WinTypeName: "double",
		Size:        8,
		FromBytes:   bytesToFloat64,
		ToBytes:     float64ToBytes,
	}, {
		WinTypeName: "LPWSTR",
		Size:        8,
		FromBytes:   bytesToStringFromPointer,
		ToBytes:     stringFromPointerToBytes,
	}, {
		WinTypeName: "LPBYTE",
		Size:        8,
		FromBytes:   byteArrayPointerFromBytes,
		ToBytes:     bytesToByteArrayPointer,
	},
}

func bytesToUint(b *bytes.Buffer, s int) uint64 {
	var r uint64 = 0

	for i := 0; i < s; i++ {
		var by, _ = b.ReadByte()
		r = r | (uint64(by) << (8 * i))
	}

	return r
}

func uintToBytes(v uint64, s int) []byte {
	b := make([]byte, s)

	for i := 0; i < s; i++ {
		b[i] = byte(v & 0xff)
		v >>= 8
	}

	return b
}

func bytesToUint16(b *bytes.Buffer, t target, _ tagOptions) {
	t.Property.SetUint(bytesToUint(b, 2))
}

func uint16ToBytes(t target) []byte {
	v := t.Property.Uint()

	return uintToBytes(v, 2)
}

func bytesToUint32(b *bytes.Buffer, t target, _ tagOptions) {
	t.Property.SetUint(bytesToUint(b, 4))
}

func uint32ToBytes(t target) []byte {
	v := t.Property.Uint()

	return uintToBytes(v, 4)
}

func bytesToUint64(b *bytes.Buffer, t target, _ tagOptions) {
	t.Property.SetUint(bytesToUint(b, 8))
}

func uint64ToBytes(t target) []byte {
	v := t.Property.Uint()

	return uintToBytes(v, 8)
}

func bytesToFloat64(b *bytes.Buffer, t target, _ tagOptions) {
	t.Property.SetFloat(math.Float64frombits(bytesToUint(b, 8)))
}

func float64ToBytes(t target) []byte {
	v := t.Property.Float()

	return uintToBytes(math.Float64bits(v), 8)
}

func bytesToStringFromPointer(b *bytes.Buffer, t target, _ tagOptions) {
	ptr := uintptr(bytesToUint(b, 8))
	sptr := (*uint16)(unsafe.Pointer(ptr))

	if sptr != nil {
		t.Property.SetString(utf16PtrToString(sptr))
	}
}

func stringFromPointerToBytes(_ target) []byte {
	// Hack to just set to 0
	return uintToBytes(0, 8)
}

// utf16PtrToString is like UTF16ToString, but takes *uint16
// as a parameter instead of []uint16.
func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	//	fmt.Printf("input pointer size = %d\n", unsafe.Sizeof(p))
	end := unsafe.Pointer(p)
	n := 0
	for *(*uint16)(end) != 0 {
		//		fmt.Printf("char %d is %x\n", n, *(*uint16)(end))
		end = unsafe.Pointer(uintptr(end) + unsafe.Sizeof(*p))
		n++
	}
	return syscall.UTF16ToString(unsafe.Slice(p, n))
}

func byteArrayPointerFromBytes(b *bytes.Buffer, t target, options tagOptions) {
	// We need to read the pointer regardless
	ptr := uintptr(bytesToUint(b, 8))

	if ptr == 0 {
		return
	}

	// options must contain either:
	// 1. integer size of array
	// 2. Name of field (already parsed) to use as length (must be a number)
	size, err := strconv.Atoi(string(options))

	if err != nil {
		// See if there is a named field that matches options

		prop := t.Struct.FieldByName(string(options))

		if prop == (reflect.Value{}) {
			panic(fmt.Sprintf("Unable to find field with name >%s< in type >%s<", string(options), t.Struct.Type().Name()))
		}

		// Value needs to be an int/uint
		if prop.CanInt() {
			size = int(prop.Int())
		} else if prop.CanUint() {
			size = int(prop.Uint())
		} else {
			panic(fmt.Sprintf("Field >%s< in type >%s< must be int/uint for size to work", string(options), t.Struct.Type().Name()))
		}

		// Now we can read the data in... as long as size > 0
		if size == 0 {
			return
		}

		var data bytes.Buffer

		for i := ptr; i < ptr+uintptr(size); i++ {
			p := unsafe.Pointer(i)
			data.WriteByte(*(*byte)(p))
		}

		t.Property.SetBytes(data.Bytes())
	}
}

func bytesToByteArrayPointer(_ target) []byte {
	return uintToBytes(0, 8)
}

func getWinType(winTypeName string) winType {
	for _, s := range winTypes {
		if s.WinTypeName == winTypeName {
			return s
		}
	}

	panic(fmt.Sprintf("Cannot find type >%s< in mapped types", winTypeName))
}

func getMeta(v any) meta {
	var t = reflect.TypeOf(v)

	meta := meta{}

	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		tag := sf.Tag.Get("windows")
		if tag == "-" {
			continue
		}
		winTypeName, options := parseTag(tag)
		winType := getWinType(winTypeName)
		meta.Size += winType.Size
		meta.Fields = append(meta.Fields, field{Name: sf.Name, Type: winType, Options: options})
	}

	return meta
}

func getReflectionData(v any) (reflect.Value, reflect.Value) {
	targetObj := reflect.ValueOf(v)
	ref := targetObj

	if ref.Kind() == reflect.Ptr {
		ref = reflect.Indirect(ref)
	}

	if ref.Kind() == reflect.Interface {
		ref = ref.Elem()
	}

	// should double-check we now have a struct (could still be anything)
	if ref.Kind() != reflect.Struct {
		log.Fatal("unexpected type")
	}

	return targetObj, ref
}

func Marshal(v any) *bytes.Buffer {
	targetObj, ref := getReflectionData(v)
	meta := getMeta(targetObj.Elem().Interface())

	var b bytes.Buffer

	for i := 0; i < len(meta.Fields); i++ {
		fi := meta.Fields[i]

		if fi.Type.ToBytes == nil {
			panic(fmt.Sprintf("No marshaler for type >%s<", fi.Type.WinTypeName))
		}

		prop := ref.FieldByName(fi.Name)

		t := target{Struct: ref, Property: prop}
		b.Write(fi.Type.ToBytes(t))
	}

	return &b
}

func Unmarshal(b *bytes.Buffer, v any) {
	targetObj, ref := getReflectionData(v)
	meta := getMeta(targetObj.Elem().Interface())

	if b.Len() < meta.Size {
		panic(fmt.Sprintf("Required size is %d bytes, input is too small: %d", meta.Size, b.Len()))
	}

	for i := 0; i < len(meta.Fields); i++ {
		fi := meta.Fields[i]

		if fi.Type.FromBytes == nil {
			panic(fmt.Sprintf("No unmarshaler for type >%s<", fi.Type.WinTypeName))
		}

		prop := ref.FieldByName(fi.Name)
		t := target{Struct: ref, Property: prop}

		fi.Type.FromBytes(b, t, fi.Options)
	}
}

func NewByteBuffer(v any) *bytes.Buffer {
	return bytes.NewBuffer(make([]byte, Size(v)))
}

func Size(v any) int {
	targetObj := reflect.ValueOf(v)
	meta := getMeta(targetObj.Elem().Interface())

	return meta.Size
}
