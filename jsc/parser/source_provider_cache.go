// SourceProviderCache.h Go 翻译
package parser

// SourceProviderCache 源提供程序缓存
type SourceProviderCache struct {
	items map[int]*SourceProviderCacheItem
}

func NewSourceProviderCache() *SourceProviderCache {
	return &SourceProviderCache{items: make(map[int]*SourceProviderCacheItem)}
}

func (c *SourceProviderCache) Clear() {
	c.items = make(map[int]*SourceProviderCacheItem)
}

func (c *SourceProviderCache) Add(sourcePosition int, item *SourceProviderCacheItem) {
	c.items[sourcePosition] = item
}

func (c *SourceProviderCache) Get(sourcePosition int) *SourceProviderCacheItem {
	return c.items[sourcePosition]
}
