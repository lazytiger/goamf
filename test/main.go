package main

import (
	"amf"
	"bytes"
	"encoding/hex"
	"fmt"
	"reflect"
)

type Abc struct {
	Uname string
	Uid int32
}

type Test struct {
	String string "amf.name:\"str\""
	Uint	uint
	Uint8 	uint8
	Uint16 uint16
	Uint32 uint32
	Uint64 uint64
	Int int
	Int8 int8
	Int16 int16
	Int32 int32
	Int64 int64
	Float32 float32
	Float64 float64
	Map map[string]*Abc
	Slice []*Abc
	Struct Abc
	Null *Abc
}

func main() {
	writer := bytes.NewBuffer(make([]byte, 0, 1024000))
	encoder := amf.NewEncoder(writer, false)
	t := new(Test)
	t.Float32 = 1.23
	t.Float64=0.000001
	t.Int = 1
	t.Int16 = 2
	t.Int32 = 3
	t.Int64 = 4
	t.Int8 = 32
	t.Map = make(map[string]*Abc)
	t.Map["hello"] = new(Abc)
	t.Slice = make([]*Abc, 1)
	t.Slice[0] = new(Abc)
	t.String = "测试"
	t.Struct.Uname = "hello"
	err := encoder.Encode(t)
	if err != nil {
		println(err.Error())
		return
	}
	fmt.Println(hex.EncodeToString(writer.Bytes()))
	
	reader := bytes.NewBuffer(writer.Bytes())
	decoder := amf.NewDecoder(reader)
	
	var a Test
	v, err := decoder.Decode(reflect.TypeOf(a))
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	a = v.(Test)
	fmt.Println(a)
}
