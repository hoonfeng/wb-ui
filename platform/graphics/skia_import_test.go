package graphics_test

import (
	"testing"

	"github.com/hoonfeng/goskia/skia"
)

func TestSkiaTypesExist(t *testing.T) {
	s, err := skia.NewRasterSurfaceN32Premul(1, 1)
	if err != nil {
		t.Fatalf("NewRasterSurfaceN32Premul: %v", err)
	}
	s.Release()

	t.Logf("Surface type works: %T", s)
}
