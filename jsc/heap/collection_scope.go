// Copyright (C) 2016 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type CollectionScope uint8

const (
	CollectionScopeEden CollectionScope = iota
	CollectionScopeFull
)

func CollectionScopeName(scope CollectionScope) string {
	switch scope {
	case CollectionScopeEden:
		return "Eden"
	case CollectionScopeFull:
		return "Full"
	default:
		return "Unknown"
	}
}
