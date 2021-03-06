// Code generated by the FlatBuffers compiler. DO NOT EDIT.

package dscachefb

import (
	flatbuffers "github.com/google/flatbuffers/go"
)

type Dscache struct {
	_tab flatbuffers.Table
}

func GetRootAsDscache(buf []byte, offset flatbuffers.UOffsetT) *Dscache {
	n := flatbuffers.GetUOffsetT(buf[offset:])
	x := &Dscache{}
	x.Init(buf, n+offset)
	return x
}

func GetSizePrefixedRootAsDscache(buf []byte, offset flatbuffers.UOffsetT) *Dscache {
	n := flatbuffers.GetUOffsetT(buf[offset+flatbuffers.SizeUint32:])
	x := &Dscache{}
	x.Init(buf, n+offset+flatbuffers.SizeUint32)
	return x
}

func (rcv *Dscache) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *Dscache) Table() flatbuffers.Table {
	return rcv._tab
}

func (rcv *Dscache) Users(obj *UserAssoc, j int) bool {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		x := rcv._tab.Vector(o)
		x += flatbuffers.UOffsetT(j) * 4
		x = rcv._tab.Indirect(x)
		obj.Init(rcv._tab.Bytes, x)
		return true
	}
	return false
}

func (rcv *Dscache) UsersLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func (rcv *Dscache) Refs(obj *RefEntryInfo, j int) bool {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		x := rcv._tab.Vector(o)
		x += flatbuffers.UOffsetT(j) * 4
		x = rcv._tab.Indirect(x)
		obj.Init(rcv._tab.Bytes, x)
		return true
	}
	return false
}

func (rcv *Dscache) RefsLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func DscacheStart(builder *flatbuffers.Builder) {
	builder.StartObject(2)
}
func DscacheAddUsers(builder *flatbuffers.Builder, users flatbuffers.UOffsetT) {
	builder.PrependUOffsetTSlot(0, flatbuffers.UOffsetT(users), 0)
}
func DscacheStartUsersVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT {
	return builder.StartVector(4, numElems, 4)
}
func DscacheAddRefs(builder *flatbuffers.Builder, refs flatbuffers.UOffsetT) {
	builder.PrependUOffsetTSlot(1, flatbuffers.UOffsetT(refs), 0)
}
func DscacheStartRefsVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT {
	return builder.StartVector(4, numElems, 4)
}
func DscacheEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT {
	return builder.EndObject()
}
