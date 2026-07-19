// Copyright (C) 2017 Apple Inc. All rights reserved.
// Translated to Go.
package heap

import "fmt"

type GCRequest struct {
	Scope           *CollectionScope
	DidFinishEndPhase func()
}

func NewGCRequest() GCRequest {
	return GCRequest{}
}

func NewGCRequestWithScope(scope CollectionScope) GCRequest {
	return GCRequest{Scope: &scope}
}

func (r *GCRequest) SubsumedBy(other *GCRequest) bool {
	if r.Scope == nil {
		return true
	}
	if other.Scope == nil {
		return false
	}
	return *r.Scope == *other.Scope
}

func (r *GCRequest) String() string {
	if r.Scope == nil {
		return "GCRequest(Any)"
	}
	return fmt.Sprintf("GCRequest(%s)", CollectionScopeName(*r.Scope))
}
