package amf

//任意类型，代表AMF中所有类型，比如
//对象，其实是一个key/value对，用map表示
//数组，其实是一个任意类型的数组，用array表示
//整数，用于表示u29
//浮点数，用于表示double
//字符串，用于表示string
type AMFAny interface{}

const (
	UNDEFINED_MARKER = 0x00
	NULL_MARKER = 0x01
	FALSE_MARKER = 0x02
	TRUE_MARKER = 0x03
	INTEGER_MARKER = 0x04
	DOUBLE_MARKER = 0x05
	STRING_MARKER = 0x06
	XMLDOC_MARKER = 0x07
	DATE_MARKER = 0x08
	ARRAY_MARKER = 0x09
	OBJECT_MARKER = 0x0a
	XML_MARKER = 0x0b
	BYTEARRAY_MARKER = 0x0c
)
