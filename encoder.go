package amf

import (
	"errors"
	"io"
	"math"
	"reflect"
	"strconv"
	"unicode"
)

type Encoder struct {
	writer       io.Writer
	stringCache  map[string]int
	objectCache  map[uintptr]int
	reservStruct bool
}

func (encoder *Encoder) getFieldName(f reflect.StructField) string {
	chars := []rune(f.Name)
	if unicode.IsLower(chars[0]) {
		return ""
	}

	name := f.Tag.Get("amf.name")
	if name != "" {
		return name
	}

	if !encoder.reservStruct {
		chars[0] = unicode.ToLower(chars[0])
		return string(chars)
	}

	return f.Name
}

func (encoder *Encoder) encodeBool(value bool) error {

	buffer := make([]byte, 1)
	if value {
		buffer[0] = TRUE_MARKER
	} else {
		buffer[0] = FALSE_MARKER
	}

	return encoder.writeBytes(buffer)
}

func (encoder *Encoder) encodeNull() error {

	return encoder.writeMarker(NULL_MARKER)
}

func (encoder *Encoder) encodeUint(value uint64) error {

	if value >= 0x20000000 {
		if value <= 0xffffffff {
			return encoder.encodeFloat(float64(value))
		}

		return encoder.encodeString(strconv.Uitoa64(value))
	}

	err := encoder.writeMarker(INTEGER_MARKER)
	if err != nil {
		return err
	}

	return encoder.writeU29(uint32(value))
}

func (encoder *Encoder) encodeInt(value int64) error {

	if value < -0xfffffff {
		if value > -0x7fffffff {
			return encoder.encodeFloat(float64(value))
		}
		return encoder.encodeString(strconv.Itoa64(value))
	}

	if value >= 0x20000000 {
		if value < 0x7fffffff {	
			return encoder.encodeFloat(float64(value))	
		}
		return encoder.encodeString(strconv.Itoa64(value))
	}

	err := encoder.writeMarker(INTEGER_MARKER)
	if err != nil {
		return err
	}

	return encoder.writeU29(uint32(value))
}

func (encoder *Encoder) encodeFloat(value float64) error {

	buffer := make([]byte, 9)

	buffer[0] = DOUBLE_MARKER
	intValue := math.Float64bits(float64(value))

	buffer[1] = byte((intValue >> 56) & 0xff)
	buffer[2] = byte((intValue >> 48) & 0xff)
	buffer[3] = byte((intValue >> 40) & 0xff)
	buffer[4] = byte((intValue >> 32) & 0xff)
	buffer[5] = byte((intValue >> 24) & 0xff)
	buffer[6] = byte((intValue >> 16) & 0xff)
	buffer[7] = byte((intValue >> 8) & 0xff)
	buffer[8] = byte(intValue & 0xff)

	return encoder.writeBytes(buffer)
}

func (encoder *Encoder) encodeString(value string) error {

	err := encoder.writeMarker(STRING_MARKER)
	if err != nil {
		return err
	}

	return encoder.writeString(value)
}

func (encoder *Encoder) encodeMap(value reflect.Value) error {

	err := encoder.writeMarker(OBJECT_MARKER)
	if err != nil {
		return err
	}

	index, ok := encoder.objectCache[value.Pointer()]
	if ok {
		index <<= 1
		encoder.writeU29(uint32(index << 1))
		return nil
	}

	err = encoder.writeMarker(0x0b)
	if err != nil {
		return err
	}

	err = encoder.writeString("")
	if err != nil {
		return err
	}

	keys := value.MapKeys()
	for i := 0; i < len(keys); i++ {
		key := keys[i]
		if key.Kind() != reflect.String {
			return errors.New("only string key allowed in map")
		}

		err = encoder.writeString(key.String())
		if err != nil {
			return err
		}

		v := value.MapIndex(key)
		if v.Kind() == reflect.Struct {
			v = v.Addr()
		}

		err = encoder.Encode(v.Interface())
		if err != nil {
			return err
		}
	}

	return encoder.writeString("")
}

func (encoder *Encoder) encodeStruct(value reflect.Value) error {

	err := encoder.writeMarker(OBJECT_MARKER)
	if err != nil {
		return err
	}

	index, ok := encoder.objectCache[value.Pointer()]
	if ok {
		index <<= 1
		encoder.writeU29(uint32(index << 1))
		return nil
	}

	err = encoder.writeMarker(0x0b)
	if err != nil {
		return err
	}

	err = encoder.writeString("")
	if err != nil {
		return err
	}

	v := reflect.Indirect(value)
	t := v.Type()
	switch t.Kind() {
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			key := encoder.getFieldName(f)
			if key == "" {
				continue
			}

			err = encoder.writeString(key)
			if err != nil {
				return err
			}

			fv := v.FieldByName(f.Name)
			if fv.Kind() == reflect.Struct {
				fv = fv.Addr()
			}

			err = encoder.Encode(fv.Interface())
			if err != nil {
				return err
			}
		}
	default:
		panic("not a struct")
	}

	return encoder.writeString("")
}

func (encoder *Encoder) encodeSlice(value reflect.Value) error {

	err := encoder.writeMarker(ARRAY_MARKER)
	if err != nil {
		return err
	}

	index, ok := encoder.objectCache[value.Pointer()]
	if ok {
		index <<= 1
		encoder.writeU29(uint32(index << 1))
		return nil
	}

	err = encoder.writeU29((uint32(value.Len()) << 1) | 0x01)
	if err != nil {
		return err
	}

	//FIXME 这里未实现ECMA数组

	err = encoder.writeString("")
	if err != nil {
		return err
	}

	for i := 0; i < value.Len(); i++ {

		v := value.Index(i)
		if v.Kind() == reflect.Struct {
			v = v.Addr()
		}
		err = encoder.Encode(v.Interface())
		if err != nil {
			return nil
		}
	}

	return nil
}

func (encoder *Encoder) Encode(value AMFAny) error {

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Map:
		return encoder.encodeMap(v)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return encoder.encodeUint(v.Uint())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return encoder.encodeInt(v.Int())
	case reflect.String:
		return encoder.encodeString(v.String())
	case reflect.Array:
		v = v.Slice(0, v.Len())
		return encoder.encodeSlice(v)
	case reflect.Slice:
		return encoder.encodeSlice(v)
	case reflect.Float64, reflect.Float32:
		return encoder.encodeFloat(v.Float())
	case reflect.Ptr:
		if v.IsNil() {
			return encoder.encodeNull()
		}
		vv := reflect.Indirect(v)
		if vv.Kind() == reflect.Struct {
			return encoder.encodeStruct(v)
		}
		return encoder.Encode(vv.Interface())
	}

	return errors.New("unsupported type:" + v.Type().String())
}

func (encoder *Encoder) writeString(value string) error {

	index, ok := encoder.stringCache[value]
	if ok {
		encoder.writeU29(uint32(index << 1))
		return nil
	}

	err := encoder.writeU29(uint32((len(value) << 1) | 0x01))
	if err != nil {
		return err
	}

	if value != "" {
		encoder.stringCache[value] = len(encoder.stringCache)
	}
	return encoder.writeBytes([]byte(value))
}

func (encoder *Encoder) writeMarker(value byte) error {

	return encoder.writeBytes([]byte{value})
}

func (encoder *Encoder) writeBytes(bytes []byte) error {

	length, err := encoder.writer.Write(bytes)
	if length != len(bytes) || err != nil {
		return errors.New("write data failed")
	}
	return err
}

func (encoder *Encoder) writeU29(value uint32) error {

	buffer := make([]byte, 0, 4)

	switch {
	case value < 0x80:
		buffer = append(buffer, byte(value))
	case value < 0x4000:
		buffer = append(buffer, byte((value>>7)|0x80))
		buffer = append(buffer, byte(value&0x7f))
	case value < 0x200000:
		buffer = append(buffer, byte((value>>14)|0x80))
		buffer = append(buffer, byte((value>>7)|0x80))
		buffer = append(buffer, byte(value&0x7f))
	case value < 0x20000000:
		buffer = append(buffer, byte((value>>22)|0x80))
		buffer = append(buffer, byte((value>>15)|0x80))
		buffer = append(buffer, byte((value>>7)|0x80))
		buffer = append(buffer, byte(value&0xff))
	default:
		return errors.New("u29 over flow")
	}

	return encoder.writeBytes(buffer)
}

func NewEncoder(writer io.Writer, reservStruct bool) *Encoder {

	encoder := new(Encoder)
	encoder.writer = writer
	encoder.objectCache = make(map[uintptr]int)
	encoder.stringCache = make(map[string]int)
	encoder.reservStruct = reservStruct
	return encoder
}
