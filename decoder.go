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
	objectCache []AMFAny
}

func NewDecoder(reader io.Reader) *Decoder {
	decoder := new(Decoder)
	decoder.reader = reader
	decoder.objectCache = make([]AMFAny, 0, 10)
	decoder.stringCache = make([]string, 0, 10)
	return decoder
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

func (decoder *Decoder) Decode(tpl reflect.Type) (AMFAny, error) {

	marker, err := decoder.readMarker()
	if err != nil {
		return nil, err
	}

	pointerLevel := 0
	if tpl != nil {
		for tpl.Kind() == reflect.Ptr {
			tpl = tpl.Elem()
			pointerLevel++
		}
	}

	var ret AMFAny
	i := 0

	switch marker {
	case FALSE_MARKER:
		ret = false
	case TRUE_MARKER:
		ret = true
	case STRING_MARKER:
		ret, err = decoder.readString(tpl)
	case DOUBLE_MARKER:
		ret, err = decoder.readFloat(tpl)
	case INTEGER_MARKER:
		ret, err = decoder.readInteger(tpl)
	case NULL_MARKER:
		ret = nil
	case ARRAY_MARKER:
		ret, err = decoder.readSlice(tpl)
	case OBJECT_MARKER:
		ret, err = decoder.readObject(tpl)
		if tpl != nil && tpl.Kind() == reflect.Struct {
			i = 1
		}
	default:
		return nil, errors.New("unsupported marker:" + strconv.Itoa(int(marker)))
	}

	if err != nil {
		return nil, err
	}

	if tpl == nil {
		return ret, nil
	}

	if ret == nil && tpl != nil {
		for ; i < pointerLevel; i++ {
			tpl = reflect.PtrTo(tpl)
		}
		return reflect.Zero(tpl).Interface(), nil
	}
	
	v := reflect.ValueOf(ret)
	if i > pointerLevel {
		return reflect.Indirect(v).Interface(), nil
	}

	for ; i < pointerLevel; i++ {
		v = v.Addr()
	}

	return v.Interface(), nil
}

func (decoder *Decoder) readObject(tpl reflect.Type) (AMFAny, error) {

	index, err := decoder.readU29()
	if err != nil {
		return nil, err
	}

	if (index & 0x01) == 0 {
		return decoder.objectCache[int(index>>1)], nil
	}

	if index != 0x0b {
		return nil, errors.New("invalid object type")
	}

	sep, err := decoder.readMarker()
	if err != nil {
		return nil, err
	}

	if sep != 0x01 {
		return nil, errors.New("ecma array not allowed")
	}

	if tpl == nil {
		ret := make(map[string]AMFAny)
		decoder.objectCache = append(decoder.objectCache, ret)

		for {
			key, err := decoder.readString(nil)
			if err != nil {
				return nil, err
			}

			if key == "" {
				break
			}

			value, err := decoder.Decode(nil)
			if err != nil {
				return nil, err
			}

			ret[key.(string)] = value
		}
		return ret, nil
	}

	if tpl.Kind() == reflect.Map {
		et := tpl.Elem()
		ret := reflect.MakeMap(tpl)
		decoder.objectCache = append(decoder.objectCache, ret)

		for {
			key, err := decoder.readString(nil)
			if err != nil {
				return nil, err
			}

			if key == "" {
				break
			}

			value, err := decoder.Decode(et)
			if err != nil {
				return nil, err
			}

			ret.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(value))
		}
		return ret.Interface(), nil
	}

	if tpl.Kind() != reflect.Struct {
		return nil, errors.New("struct type expected, found:" + tpl.String())
	}

	ret := reflect.New(tpl)
	decoder.objectCache = append(decoder.objectCache, ret.Interface())
	ret = reflect.Indirect(ret)

	for {
		key, err := decoder.readString(nil)
		if err != nil {
			return nil, err
		}

		if key == "" {
			break
		}

		f, ok := decoder.getField(key.(string), tpl)
		if !ok {
			return nil, errors.New("key:" + key.(string) + " not found in struct:" + tpl.String())
		}

		value, err := decoder.Decode(f.Type)
		if err != nil {
			return nil, err
		}

		ret.FieldByName(f.Name).Set(reflect.ValueOf(value))
	}

	//println(ret.Addr().Type().String())
	return ret.Addr().Interface(), nil
}

func (decoder *Decoder) readSlice(tpl reflect.Type) (AMFAny, error) {

	index, err := decoder.readU29()
	if err != nil {
		return nil, err
	}

	if (index & 0x01) == 0 {
		return decoder.objectCache[int(index>>1)], nil
	}

	index >>= 1
	sep, err := decoder.readMarker()
	if err != nil {
		return nil, err
	}

	if sep != 0x01 {
		return nil, errors.New("ecma array not allowed")
	}

	if tpl == nil {
		ret := make([]AMFAny, index)
		decoder.objectCache = append(decoder.objectCache, ret)

		for i, _ := range ret {
			ret[i], err = decoder.Decode(nil)
			if err != nil {
				return nil, err
			}
		}

		return ret, nil
	}

	if tpl.Kind() != reflect.Array && tpl.Kind() != reflect.Slice {
		return nil, errors.New("type:" + tpl.String() + " is not allowed for array")
	}

	et := tpl.Elem()
	ret := reflect.MakeSlice(tpl, int(index), int(index))
	decoder.objectCache = append(decoder.objectCache, ret.Interface())

	for i := 0; i < int(index); i++ {
		v, err := decoder.Decode(et)
		if err != nil {
			return nil, err
		}

		ret.Index(i).Set(reflect.ValueOf(v))
	}

	return ret.Interface(), nil
}

func (decoder *Decoder) readInteger(tpl reflect.Type) (AMFAny, error) {
	uv, err := decoder.readU29()
	if err != nil {
		return nil, err
	}

	if tpl == nil {
		return uv, nil
	}

	vv := int32(uv)
	if uv > 0xfffffff {
		vv = int32(uv - 0x20000000)
	}

	switch tpl.Kind() {
	case reflect.Int8:
		return int8(vv), nil
	case reflect.Int16:
		return int16(vv), nil
	case reflect.Int32:
		return int32(vv), nil
	case reflect.Int64:
		return int64(vv), nil
	case reflect.Int:
		return int(vv), nil
	case reflect.Uint8:
		return uint8(uv), nil
	case reflect.Uint16:
		return uint16(uv), nil
	case reflect.Uint32:
		return uint32(uv), nil
	case reflect.Uint64:
		return uint64(uv), nil
	case reflect.Uint:
		return uint(uv), nil
	}

	return nil, errors.New("invalid type:" + tpl.String() + " for integer")
}

func (decoder *Decoder) readFloat(tpl reflect.Type) (AMFAny, error) {
	bytes, err := decoder.readBytes(8)
	if err != nil {
		return nil, err
	}

	n := uint64(0)
	for _, b := range bytes {
		n <<= 8
		n |= uint64(b)
	}

	v := math.Float64frombits(n)
	if tpl == nil {
		return v, nil
	}

	switch tpl.Kind() {
	case reflect.Float32:
		return float32(v), nil
	case reflect.Float64:
		return v, nil
	case reflect.Uint32:
		return uint32(v), nil
	case reflect.Int32:
		return int32(v), nil
	}

	return nil, errors.New("invalid type:" + tpl.String() + " for double")
}

func (decoder *Decoder) readString(tpl reflect.Type) (AMFAny, error) {
	index, err := decoder.readU29()
	if err != nil {
		return nil, err
	}

	if (index & 0x01) == 0 {
		return decoder.stringCache[int(index>>1)], nil
	}

	index >>= 1
	bytes, err := decoder.readBytes(int(index))
	if err != nil {
		return "", err
	}

	ret := string(bytes)
	if ret != "" {
		decoder.stringCache = append(decoder.stringCache, ret)
	}
	if tpl == nil {
		return ret, nil
	}

	switch tpl.Kind() {
	case reflect.Int32:
		num, err := strconv.Atoi(ret)
		if err != nil {
			return nil, err
		}

		return int32(num), nil
	case reflect.Int:
		num, err := strconv.Atoi(ret)
		if err != nil {
			return nil, err
		}

		return int(num), nil
	case reflect.Uint32:
		num, err := strconv.Atoui(ret)
		if err != nil {
			return nil, err
		}

		return uint32(num), nil
	case reflect.Uint:
		num, err := strconv.Atoui(ret)
		if err != nil {
			return nil, err
		}

		return uint(num), nil
	case reflect.Int64:
		num, err := strconv.Atoi64(ret)
		if err != nil {
			return nil, err
		}

		return num, nil
	case reflect.Uint64:
		num, err := strconv.Atoui64(ret)
		if err != nil {
			return nil, err
		}

		return num, nil
	case reflect.String:
		return ret, nil
	}

	return nil, errors.New("invalid type:" + tpl.String() + " for string")
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
