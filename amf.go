// Copyright 2011 baihaoping@gmail.com. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package amf

//Anything in amf
type AMFAny interface{}

const (
	UNDEFINED_MARKER = 0x00
	NULL_MARKER      = 0x01
	FALSE_MARKER     = 0x02
	TRUE_MARKER      = 0x03
	INTEGER_MARKER   = 0x04
	DOUBLE_MARKER    = 0x05
	STRING_MARKER    = 0x06
	XMLDOC_MARKER    = 0x07
	DATE_MARKER      = 0x08
	ARRAY_MARKER     = 0x09
	OBJECT_MARKER    = 0x0a
	XML_MARKER       = 0x0b
	BYTEARRAY_MARKER = 0x0c
)
