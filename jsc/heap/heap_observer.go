// Copyright (C) 2015 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type HeapObserver interface {
	WillGarbageCollect()
	DidGarbageCollect(scope CollectionScope)
}
