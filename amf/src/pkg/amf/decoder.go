package amf

import (
	"errors"
	"io"
	"math"
	"reflect"
	"strconv"
	"unicode"
)

type Decoder struct {
	reader      io.Reader
	stringCache []string
	objectCache []reflect.Value
}

func NewDecoder(reader io.Reader) *Decoder {
	decoder := new(Decoder)
	decoder.reader = reader
	decoder.Reset()
	return decoder
}

func (decoder *Decoder) Reset() {
	decoder.objectCache = make([]reflect.Value, 0, 10)
	decoder.stringCache = make([]string, 0, 10)
}

func (decoder *Decoder) getField(key string, t reflect.Type) (reflect.StructField, bool) {
	chars := []rune(key)
	upperKey := key
	if unicode.IsLower(chars[0]) {
		chars[0] = unicode.ToUpper(chars[0])
		upperKey = string(chars)
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		if f.Name == upperKey {
			return f, true
		}

		if f.Tag.Get("amf.name") == key {
			return f, true
		}

	}

	return *new(reflect.StructField), false
}

func (decoder *Decoder) decode(value reflect.Value) error {
	marker, err := decoder.readMarker()
	if err != nil {
		return err
	}

	//处理空指针的情况
	if marker == NULL_MARKER {
		if value.IsNil() {
			return nil
		}

		switch value.Kind() {
		case reflect.Interface, reflect.Slice, reflect.Map, reflect.Ptr:
			value.Set(reflect.Zero(value.Type()))
			return nil
		default:
			return errors.New("invalid type:" + value.Type().String() + " for nil")
		}
	}
	
	if value.Kind() == reflect.Interface {
		v := reflect.ValueOf(value.Interface())
		if v.Kind() == reflect.Ptr || v.Kind() == reflect.Slice || v.Kind() == reflect.Map  {
			value = v
		}
	}

	//如果当前为空指针则初始化
	for value.Kind() == reflect.Ptr {
		if value.IsNil() {
			value.Set(reflect.New(value.Type().Elem()))
		}
		value = value.Elem()
	}
	
	switch marker {
	case FALSE_MARKER:
		return decoder.setBool(value, false)
	case TRUE_MARKER:
		return decoder.setBool(value, true)
	case STRING_MARKER:
		return decoder.readString(value)
	case DOUBLE_MARKER:
		return decoder.readFloat(value)
	case INTEGER_MARKER:
		return decoder.readInteger(value)
	case ARRAY_MARKER:
		return decoder.readSlice(value)
	case OBJECT_MARKER:
		return decoder.readObject(value)
	default:
		return errors.New("unsupported marker:" + strconv.Itoa(int(marker)))
	}

	return nil
}

func (decoder *Decoder) readFloat(value reflect.Value) error {
	bytes, err := decoder.readBytes(8)
	if err != nil {
		return err
	}

	n := uint64(0)
	for _, b := range bytes {
		n <<= 8
		n |= uint64(b)
	}

	v := math.Float64frombits(n)

	switch value.Kind() {
	case reflect.Float32, reflect.Float64:
		value.SetFloat(v)
	case reflect.Int32, reflect.Int, reflect.Int64:
		value.SetInt(int64(v))
	case reflect.Uint32, reflect.Uint, reflect.Uint64:
		value.SetUint(uint64(v))
	case reflect.Interface:
		value.Set(reflect.ValueOf(v))
	default:
		return errors.New("invalid type:" + value.Type().String() + " for double")
	}

	return nil
}

func (decoder *Decoder) readInteger(value reflect.Value) error {

	uv, err := decoder.readU29()
	if err != nil {
		return err
	}

	vv := int32(uv)
	if uv > 0xfffffff {
		vv = int32(uv - 0x20000000)
	}

	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value.SetInt(int64(vv))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value.SetUint(uint64(uv))
	case reflect.Interface:
		value.Set(reflect.ValueOf(uv))
	default:
		return errors.New("invalid type:" + value.Type().String() + " for integer")
	}

	return nil
}

func (decoder *Decoder) readString(value reflect.Value) error {

	index, err := decoder.readU29()
	if err != nil {
		return err
	}

	ret := ""
	if (index & 0x01) == 0 {
		ret = decoder.stringCache[int(index>>1)]
	} else {
		index >>= 1
		bytes, err := decoder.readBytes(int(index))
		if err != nil {
			return err
		}

		ret = string(bytes)
	}
	
	if ret != "" {
		decoder.stringCache = append(decoder.stringCache, ret)
	}

	switch value.Kind() {
	case reflect.Int, reflect.Int32, reflect.Int64:
		num, err := strconv.ParseInt(ret, 10, 64)
		if err != nil {
			return err
		}

		value.SetInt(num)
	case reflect.Uint, reflect.Uint32, reflect.Uint64:
		num, err := strconv.ParseUint(ret, 10, 64)
		if err != nil {
			return err
		}

		value.SetUint(num)
	case reflect.String:
		value.SetString(ret)
	case reflect.Interface:
		value.Set(reflect.ValueOf(ret))
	default:
		return errors.New("invalid type:" + value.Type().String() + " for string")
	}

	return nil
}

func (decoder *Decoder) readObject(value reflect.Value) error {

	index, err := decoder.readU29()
	if err != nil {
		return err
	}

	if (index & 0x01) == 0 {
		value.Set(decoder.objectCache[int(index>>1)])
		return nil
	}

	if index != 0x0b {
		return errors.New("invalid object type")
	}

	sep, err := decoder.readMarker()
	if err != nil {
		return err
	}

	if sep != 0x01 {
		return errors.New("type object not allowed")
	}

	if value.Kind() == reflect.Interface {
		var dummy map[string]AMFAny
		v := reflect.MakeMap(reflect.TypeOf(dummy))
		value.Set(v)
		value = v
	}

	if value.Kind() == reflect.Map {
		if value.IsNil() {
			v := reflect.MakeMap(value.Type())
			value.Set(v)
			value = v
		}
		
		decoder.objectCache = append(decoder.objectCache, value)

		for {
			key := ""
			err = decoder.readString(reflect.ValueOf(&key).Elem())
			if err != nil {
				return err
			}
			

			if key == "" {
				break
			}

			v := reflect.New(value.Type().Elem())
			err = decoder.decode(v)
			if err != nil {
				return err
			}

			value.SetMapIndex(reflect.ValueOf(key), v.Elem())
		}
		
		return nil
	}

	if value.Kind() != reflect.Struct {
		return errors.New("struct type expected, found:" + value.Type().String())
	}
	

	decoder.objectCache = append(decoder.objectCache, value)

	for {
	
		key := ""
		err = decoder.readString(reflect.ValueOf(&key).Elem())
		if err != nil {
			return err
		}
		
		if key == "" {
			break
		}

		f, ok := decoder.getField(key, value.Type())
		if !ok {
			return errors.New("key:" + key + " not found in struct:" + value.Type().String())
		}
		
		err = decoder.decode(value.FieldByName(f.Name))
		if err != nil {
			return err
		}
	}
	
	return nil
}

func (decoder *Decoder) readSlice(value reflect.Value) error {

	index, err := decoder.readU29()
	if err != nil {
		return err
	}

	if (index & 0x01) == 0 {
		slice := decoder.objectCache[int(index>>1)]
		value.Set(slice)
		return nil
	}

	index >>= 1
	sep, err := decoder.readMarker()
	if err != nil {
		return err
	}

	if sep != 0x01 {
		return errors.New("ecma array not allowed")
	}
	
	if value.IsNil() {
		var v reflect.Value
		if value.Type().Kind() == reflect.Slice {
			v = reflect.MakeSlice(value.Type(), int(index), int(index))
		} else if value.Type().Kind() == reflect.Interface {
			v = reflect.ValueOf(make([]AMFAny,int(index), int(index)))
		} else {
			return errors.New("invalid type:" + value.Type().String() + " for array")
		}
		value.Set(v)
		value = v
	}
	
	decoder.objectCache = append(decoder.objectCache, value)
	
	for i := 0; i < int(index); i++ {
		c := value.Index(i)
		err = decoder.decode(c)
		if err != nil {
			return err
		}
	}
	
	return nil
}

func (decoder *Decoder) setBool(value reflect.Value, v bool) error {

	switch value.Kind() {
		case reflect.Bool:
		value.SetBool(v)
	case reflect.Interface:
		value.Set(reflect.ValueOf(v))
	default:
		return errors.New("invalid type:" + value.Type().String() + " for bool")
	}
	return nil
}

func (decoder *Decoder) Decode(value AMFAny) error {
	return decoder.decode(reflect.ValueOf(value))
}

func (decoder *Decoder) DecodeValue(value reflect.Value) error {
	return decoder.decode(value)
}

func (decoder *Decoder) readU29() (uint32, error) {

	var ret uint32 = 0
	for i := 0; i < 4; i++ {
		b, err := decoder.readMarker()
		if err != nil {
			return 0, err
		}

		if i != 3 {
			ret = (ret << 7) | uint32(b&0x7f)
			if (b & 0x80) == 0 {
				break
			}
		} else {
			ret = (ret << 8) | uint32(b)
		}
	}

	return ret, nil
}

func (decoder *Decoder) readBytes(length int) ([]byte, error) {
	buffer := make([]byte, length)
	for length != 0 {
		l, err := decoder.reader.Read(buffer)
		if err != nil {
			return nil, err
		}
		length -= l
	}

	return buffer, nil
}

func (decoder *Decoder) readMarker() (byte, error) {
	bytes, err := decoder.readBytes(1)
	if err != nil {
		return 0, err
	}

	return bytes[0], nil
}
