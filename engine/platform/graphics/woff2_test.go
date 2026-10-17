package graphics

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/hoonfeng/goskia/skia"
)

// readFontFixture 读取 testdata 下的字体容器文件。
func readFontFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// sfntTableMap 解析 sfnt 表目录，返回 tag → 表长度。
//
// 这里刻意自己解析而不依赖 Skia：解码器产出的容器结构必须**独立**可校验
// （Skia 只回答「能不能用」，不回答「表目录写对没有」）。
func sfntTableMap(t *testing.T, data []byte) map[uint32]uint32 {
	t.Helper()
	if len(data) < 12 {
		t.Fatalf("sfnt too short: %d bytes", len(data))
	}
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	if numTables == 0 || 12+16*numTables > len(data) {
		t.Fatalf("sfnt table directory invalid: numTables=%d len=%d", numTables, len(data))
	}
	out := make(map[uint32]uint32, numTables)
	for i := 0; i < numTables; i++ {
		e := data[12+16*i : 12+16*(i+1)]
		tag := binary.BigEndian.Uint32(e[0:4])
		off := binary.BigEndian.Uint32(e[8:12])
		length := binary.BigEndian.Uint32(e[12:16])
		if int(off)+int(length) > len(data) {
			t.Fatalf("table %q range %d+%d exceeds sfnt size %d", tagString(tag), off, length, len(data))
		}
		out[tag] = length
	}
	return out
}

func tagString(tag uint32) string {
	return string([]byte{byte(tag >> 24), byte(tag >> 16), byte(tag >> 8), byte(tag)})
}

// requireSFNT 校验通用 sfnt 结构（flavor + 必需表 + 表目录边界）。
func requireSFNT(t *testing.T, name string, data []byte) map[uint32]uint32 {
	t.Helper()
	if len(data) < 12 {
		t.Fatalf("%s: decoded sfnt too short (%d bytes)", name, len(data))
	}
	flavor := binary.BigEndian.Uint32(data[0:4])
	if flavor != sfntTrueType && flavor != sfntOTTO {
		t.Fatalf("%s: unexpected sfnt flavor 0x%08X", name, flavor)
	}
	tables := sfntTableMap(t, data)
	if len(tables) == 0 {
		t.Fatalf("%s: no tables", name)
	}
	// 所有 sfnt 都必须有 head/maxp；TrueType 轮廓还必须有 glyf/loca。
	for _, required := range []uint32{tagHead} {
		if _, ok := tables[required]; !ok {
			t.Fatalf("%s: missing required table %q", name, tagString(required))
		}
	}
	return tables
}

// TestDecodeWOFF2ProducesUsableSFNT 用真实 woff2 字体验证：
//   - 解码结果表目录结构完整（glyf/loca/head/hhea/hmtx/maxp/cmap）；
//   - Skia 能直接用解码结果创建 typeface（最终裁判）。
func TestDecodeWOFF2ProducesUsableSFNT(t *testing.T) {
	for _, name := range []string{"ahem.woff2", "NotoSansMongolian-18xx.woff2"} {
		name := name
		t.Run(name, func(t *testing.T) {
			raw := readFontFixture(t, name)
			if !bytes.HasPrefix(raw, []byte("wOF2")) {
				t.Fatalf("fixture is not WOFF2")
			}
			sfnt, err := DecodeEmbeddedFont(raw)
			if err != nil {
				t.Fatalf("decode %s: %v", name, err)
			}
			tables := requireSFNT(t, name, sfnt)
			// 这两个 fixture 都是 TrueType 轮廓（Ahem / Noto Sans Mongolian），
			// 必须带 glyf + loca + hmtx + hhea + maxp。
			for _, tag := range []uint32{tagGlyf, tagLoca, tagHmtx, tagHhea, 0x6D617870 /* maxp */, 0x636D6170 /* cmap */} {
				if _, ok := tables[tag]; !ok {
					t.Fatalf("%s: decoded sfnt missing table %q", name, tagString(tag))
				}
			}
			tf := skia.NewTypefaceFromData(sfnt, 0)
			if tf == nil {
				t.Fatalf("%s: Skia rejected decoded sfnt (%d bytes)", name, len(sfnt))
			}
			t.Logf("%s: woff2 %d → sfnt %d bytes, %d tables", name, len(raw), len(sfnt), len(tables))
		})
	}
}

// TestDecodeWOFF1ProducesUsableSFNT 覆盖 WOFF1（zlib 容器）。
func TestDecodeWOFF1ProducesUsableSFNT(t *testing.T) {
	raw := readFontFixture(t, "ignorables.woff")
	if !bytes.HasPrefix(raw, []byte("wOFF")) {
		t.Fatalf("fixture is not WOFF1")
	}
	sfnt, err := DecodeEmbeddedFont(raw)
	if err != nil {
		t.Fatalf("decode woff: %v", err)
	}
	requireSFNT(t, "ignorables.woff", sfnt)
	if tf := skia.NewTypefaceFromData(sfnt, 0); tf == nil {
		t.Fatalf("Skia rejected decoded woff sfnt (%d bytes)", len(sfnt))
	}
}

// TestDecodeEmbeddedFontPassThrough 校验已经是 sfnt 的数据原样返回。
func TestDecodeEmbeddedFontPassThrough(t *testing.T) {
	raw := readFontFixture(t, "ahem.woff2")
	sfnt, err := DecodeEmbeddedFont(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	again, err := DecodeEmbeddedFont(sfnt)
	if err != nil {
		t.Fatalf("pass-through decode: %v", err)
	}
	if !bytes.Equal(again, sfnt) {
		t.Fatalf("non-woff data was modified by DecodeEmbeddedFont")
	}
}

// TestDecodeEmbeddedFontRejectsCorrupt 校验损坏容器返回错误而不是 panic/垃圾。
func TestDecodeEmbeddedFontRejectsCorrupt(t *testing.T) {
	cases := map[string][]byte{
		"truncated-woff2": []byte("wOF2\x00\x01"),
		"bad-brotli":      append([]byte("wOF2"), bytes.Repeat([]byte{0xFF}, 60)...),
	}
	for name, data := range cases {
		if _, err := DecodeEmbeddedFont(data); err == nil {
			t.Fatalf("%s: expected error, got nil", name)
		}
	}
}
