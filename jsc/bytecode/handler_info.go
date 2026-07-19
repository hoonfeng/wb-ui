// Copyright (C) 2012-2018 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/HandlerInfo.h

package bytecode

import "fmt"

type HandlerType uint8

const (
	HandlerTypeCatch             HandlerType = 0
	HandlerTypeFinally           HandlerType = 1
	HandlerTypeSynthesizedCatch  HandlerType = 2
	HandlerTypeSynthesizedFinally HandlerType = 3
)

type RequiredHandler uint8

const (
	RequiredHandlerCatchHandler RequiredHandler = 0
	RequiredHandlerAnyHandler   RequiredHandler = 1
)

// HandlerInfoBase is the base structure for exception handler information.
type HandlerInfoBase struct {
	Start   uint32
	End     uint32
	Target  uint32
	TypeBits uint32 // only low 2 bits used for HandlerType
}

func (h *HandlerInfoBase) HandlerType() HandlerType {
	return HandlerType(h.TypeBits & 3)
}

func (h *HandlerInfoBase) SetHandlerType(t HandlerType) {
	h.TypeBits = (h.TypeBits & ^uint32(3)) | uint32(t)
}

func (h *HandlerInfoBase) TypeName() string {
	switch h.HandlerType() {
	case HandlerTypeCatch:
		return "catch"
	case HandlerTypeFinally:
		return "finally"
	case HandlerTypeSynthesizedCatch:
		return "synthesized catch"
	case HandlerTypeSynthesizedFinally:
		return "synthesized finally"
	default:
		panic("unknown HandlerType")
	}
}

func (h *HandlerInfoBase) IsCatchHandler() bool {
	return h.HandlerType() == HandlerTypeCatch
}

// HandlerForIndex finds the handler that contains the given index from a slice of HandlerInfoBase.
// Handlers are ordered innermost first.
func HandlerForIndex(exceptionHandlers []HandlerInfoBase, index uint32, requiredHandler RequiredHandler) *HandlerInfoBase {
	for i := range exceptionHandlers {
		handler := &exceptionHandlers[i]
		if requiredHandler == RequiredHandlerCatchHandler && !handler.IsCatchHandler() {
			continue
		}
		if handler.Start <= index && handler.End > index {
			return handler
		}
	}
	return nil
}

func (h *HandlerInfoBase) GetStart() uint32 { return h.Start }
func (h *HandlerInfoBase) GetEnd() uint32   { return h.End }

// UnlinkedHandlerInfo is the unlinked (before linking) exception handler info.
type UnlinkedHandlerInfo struct {
	HandlerInfoBase
}

func NewUnlinkedHandlerInfo(start, end, target uint32, handlerType HandlerType) UnlinkedHandlerInfo {
	h := UnlinkedHandlerInfo{}
	h.Start = start
	h.End = end
	h.Target = target
	h.SetHandlerType(handlerType)
	return h
}

// HandlerInfo is the linked exception handler info.
type HandlerInfo struct {
	HandlerInfoBase
}

func (h *HandlerInfo) Initialize(unlinked UnlinkedHandlerInfo) {
	h.Start = unlinked.Start
	h.End = unlinked.End
	h.Target = unlinked.Target
	h.TypeBits = unlinked.TypeBits
}

func (h *HandlerInfoBase) String() string {
	return fmt.Sprintf("HandlerInfo{type=%s start=%d end=%d target=%d}", h.TypeName(), h.Start, h.End, h.Target)
}
