// WOFF / WOFF2 嵌入字体解码器（纯 Go）。
//
// 为什么需要：Skia 的 *skia.Typeface 只接受 sfnt（TrueType/OpenType）数据，
// 不内置 woff/woff2 解码；而真实站点的 @font-face 几乎只提供 woff2（体积
// 比 woff 再小 30%）。没有本解码器时，`@font-face { src: url(x.woff2) }`
// 一律注册失败，页面只能落到回退字型——图标字体变成方框、品牌字体丢失。
//
// 实现范围：
//   - WOFF2（W3C 规范）：目录解析 → brotli 解压 → glyf/loca/hmtx 变换重构
//     → 重组 sfnt（checksum + head.checkSumAdjustment）；
//   - WOFF1：目录解析 → 逐表 zlib 解压 → 重组 sfnt；
//   - TTC（字体集合）里的 WOFF2 包不在此支持（返回错误，调用方按 src 回退）。
//
// 算法移植自 Google 的 woff2 参考解码器（MIT 许可）：
// https://github.com/google/woff2 — 本文件保持与参考实现逐条同构，便于对照。
package graphics

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/andybalholm/brotli"
)

// 字体容器签名。
var (
	woffSignature  = "wOFF"
	woff2Signature = "wOF2"
)

// sfnt flavor（表目录的首 4 字节）。
const (
	sfntTrueType = 0x00010000
	sfntOTTO     = 0x4F54544F // 'OTTO'（CFF 轮廓）
	sfntTTC      = 0x74746366 // 'ttcf'（字体集合）
)

// 本文件用到的表 tag。
const (
	tagHead = 0x68656164 // 'head'
	tagHhea = 0x68686561 // 'hhea'
	tagHmtx = 0x686D7478 // 'hmtx'
	tagGlyf = 0x676C7966 // 'glyf'
	tagLoca = 0x6C6F6361 // 'loca'
)

// knownTags 是 WOFF2 目录里前 63 个常用表的隐式 tag 表（与规范顺序一致）。
var knownTags = [63]uint32{
	0x636D6170, // cmap
	0x68656164, // head
	0x68686561, // hhea
	0x686D7478, // hmtx
	0x6D617870, // maxp
	0x6E616D65, // name
	0x4F532F32, // OS/2
	0x706F7374, // post
	0x63767420, // cvt
	0x6670676D, // fpgm
	0x676C7966, // glyf
	0x6C6F6361, // loca
	0x70726570, // prep
	0x43464620, // CFF
	0x564F5247, // VORG
	0x45424454, // EBDT
	0x45424C43, // EBLC
	0x67617370, // gasp
	0x68646D78, // hdmx
	0x6B65726E, // kern
	0x4C545348, // LTSH
	0x50434C54, // PCLT
	0x56444D58, // VDMX
	0x76686561, // vhea
	0x766D7478, // vmtx
	0x42415345, // BASE
	0x47444546, // GDEF
	0x47504F53, // GPOS
	0x47535542, // GSUB
	0x45425343, // EBSC
	0x4A535446, // JSTF
	0x4D415448, // MATH
	0x43424454, // CBDT
	0x43424C43, // CBLC
	0x434F4C52, // COLR
	0x4350414C, // CPAL
	0x53564720, // SVG
	0x73626978, // sbix
	0x61636E74, // acnt
	0x61766172, // avar
	0x62646174, // bdat
	0x626C6F63, // bloc
	0x62736C6E, // bsln
	0x63766172, // cvar
	0x66647363, // fdsc
	0x66656174, // feat
	0x666D7478, // fmtx
	0x66766172, // fvar
	0x67766172, // gvar
	0x68737479, // hsty
	0x6A757374, // just
	0x6C636172, // lcar
	0x6D6F7274, // mort
	0x6D6F7278, // morx
	0x6F706264, // opbd
	0x70726F70, // prop
	0x7472616B, // trak
	0x5A617066, // Zapf
	0x53696C66, // Silf
	0x476C6174, // Glat
	0x476C6F63, // Gloc
	0x46656174, // Feat
	0x53696C6C, // Sill
}

// errFontDecode 是解码失败的统一错误前缀（调用方按错误继续尝试下一个 src）。
var errFontDecode = errors.New("graphics: embedded font decode failed")

func fontDecodeErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errFontDecode, fmt.Sprintf(format, args...))
}

// DecodeEmbeddedFont 把 WOFF/WOFF2 容器解码为 sfnt 字节。非 WOFF 数据
// （已经是 ttf/otf/ttc）原样返回，便于调用方无脑先过一遍本函数。
func DecodeEmbeddedFont(data []byte) ([]byte, error) {
	switch {
	case bytes.HasPrefix(data, []byte(woff2Signature)):
		return decodeWOFF2(data)
	case bytes.HasPrefix(data, []byte(woffSignature)):
		return decodeWOFF(data)
	}
	return data, nil
}

// ── 通用读取工具 ──────────────────────────────────────────────────────

type binReader struct {
	data []byte
	off  int
}

func (r *binReader) remaining() int { return len(r.data) - r.off }

func (r *binReader) u8() (uint8, error) {
	if r.off+1 > len(r.data) {
		return 0, io.ErrUnexpectedEOF
	}
	v := r.data[r.off]
	r.off++
	return v, nil
}

func (r *binReader) u16() (uint16, error) {
	if r.off+2 > len(r.data) {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.BigEndian.Uint16(r.data[r.off:])
	r.off += 2
	return v, nil
}

func (r *binReader) s16() (int16, error) {
	v, err := r.u16()
	return int16(v), err
}

func (r *binReader) u32() (uint32, error) {
	if r.off+4 > len(r.data) {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.BigEndian.Uint32(r.data[r.off:])
	r.off += 4
	return v, nil
}

func (r *binReader) skip(n int) error {
	if n < 0 || r.off+n > len(r.data) {
		return io.ErrUnexpectedEOF
	}
	r.off += n
	return nil
}

func (r *binReader) take(n int) ([]byte, error) {
	if n < 0 || r.off+n > len(r.data) {
		return nil, io.ErrUnexpectedEOF
	}
	b := r.data[r.off : r.off+n]
	r.off += n
	return b, nil
}

// read255UShort 读 WOFF2 的 255UInt16 编码（MicroType Express 6.1.1）。
func read255UShort(r *binReader) (int, error) {
	code, err := r.u8()
	if err != nil {
		return 0, err
	}
	switch code {
	case 253:
		v, err := r.u16()
		return int(v), err
	case 255:
		b, err := r.u8()
		return int(b) + 253, err
	case 254:
		b, err := r.u8()
		return int(b) + 506, err
	default:
		return int(code), nil
	}
}

// readBase128 读 WOFF2 的 UIntBase128（禁止前导 0x80，最多 5 字节）。
func readBase128(r *binReader) (uint32, error) {
	var result uint32
	for i := 0; i < 5; i++ {
		code, err := r.u8()
		if err != nil {
			return 0, err
		}
		if i == 0 && code == 0x80 {
			return 0, fontDecodeErr("UIntBase128 has leading zero")
		}
		if result&0xfe000000 != 0 {
			return 0, fontDecodeErr("UIntBase128 overflows uint32")
		}
		result = (result << 7) | uint32(code&0x7f)
		if code&0x80 == 0 {
			return result, nil
		}
	}
	return 0, fontDecodeErr("UIntBase128 is longer than 5 bytes")
}

// ── 表结构 ────────────────────────────────────────────────────────────

type woff2Table struct {
	tag         uint32
	origLen     uint32 // 规范里的 origLength（重构后的表长度）
	xformLen    uint32 // 压缩流里该表所占字节数（transformLength）
	transformed bool
	data        []byte // 解压后的表数据（变换表：仍是变换格式）
	dstLen      uint32 // 重构后的 sfnt 表长度
}

func findTable(tables []*woff2Table, tag uint32) *woff2Table {
	for _, t := range tables {
		if t.tag == tag {
			return t
		}
	}
	return nil
}

// woff2FontInfo 收集跨表重构信息（glyf → hmtx 依赖）。
type woff2FontInfo struct {
	numGlyphs   uint16
	indexFormat uint16
	xMins       []int16
}

// ── WOFF2 ────────────────────────────────────────────────────────────

// decodeWOFF2 把 WOFF2 容器还原成 sfnt。
func decodeWOFF2(data []byte) ([]byte, error) {
	if len(data) < 48 {
		return nil, fontDecodeErr("woff2 header truncated (%d bytes)", len(data))
	}
	r := &binReader{data: data}
	if _, err := r.take(4); err != nil { // signature（调用方已判）
		return nil, err
	}
	flavor, err := r.u32()
	if err != nil {
		return nil, err
	}
	if flavor == sfntTTC {
		return nil, fontDecodeErr("woff2 font collections (ttcf) are not supported")
	}
	reportedLen, err := r.u32()
	if err != nil {
		return nil, err
	}
	if int(reportedLen) != len(data) {
		return nil, fontDecodeErr("woff2 length field %d != actual %d", reportedLen, len(data))
	}
	numTables, err := r.u16()
	if err != nil {
		return nil, err
	}
	if numTables == 0 {
		return nil, fontDecodeErr("woff2 has no tables")
	}
	if err := r.skip(2 + 4); err != nil { // reserved + totalSfntSize
		return nil, err
	}
	compressedLen, err := r.u32()
	if err != nil {
		return nil, err
	}
	if err := r.skip(4); err != nil { // major/minor version
		return nil, err
	}
	metaOffset, err := r.u32()
	if err != nil {
		return nil, err
	}
	metaLength, err := r.u32()
	if err != nil {
		return nil, err
	}
	if _, err := r.u32(); err != nil { // metaOrigLength
		return nil, err
	}
	privOffset, err := r.u32()
	if err != nil {
		return nil, err
	}
	privLength, err := r.u32()
	if err != nil {
		return nil, err
	}
	if metaOffset != 0 {
		if int(metaOffset) >= len(data) || uint64(metaOffset)+uint64(metaLength) > uint64(len(data)) {
			return nil, fontDecodeErr("woff2 metadata range out of bounds")
		}
	}
	if privOffset != 0 {
		if int(privOffset) >= len(data) || uint64(privOffset)+uint64(privLength) > uint64(len(data)) {
			return nil, fontDecodeErr("woff2 private range out of bounds")
		}
	}

	// 表目录。
	tables := make([]*woff2Table, 0, numTables)
	var uncompressedSize uint64
	for i := 0; i < int(numTables); i++ {
		flagByte, err := r.u8()
		if err != nil {
			return nil, err
		}
		var tag uint32
		if flagByte&0x3f == 0x3f {
			tag, err = r.u32()
			if err != nil {
				return nil, err
			}
		} else {
			tag = knownTags[flagByte&0x3f]
		}
		xformVersion := (flagByte >> 6) & 0x03
		transformed := false
		if tag == tagGlyf || tag == tagLoca {
			// glyf/loca：0 = 变换，3 = 不变换。
			transformed = xformVersion == 0
		} else if xformVersion != 0 {
			// 其他表：0 = 不变换，非 0 = 变换（hmtx 的 1 也是变换）。
			transformed = true
		}
		origLen, err := readBase128(r)
		if err != nil {
			return nil, err
		}
		xformLen := origLen
		if transformed {
			xformLen, err = readBase128(r)
			if err != nil {
				return nil, err
			}
			if tag == tagLoca && xformLen != 0 {
				return nil, fontDecodeErr("transformed loca must be empty")
			}
		}
		uncompressedSize += uint64(xformLen)
		if uncompressedSize > 1<<31 {
			return nil, fontDecodeErr("woff2 uncompressed size is implausibly large")
		}
		tables = append(tables, &woff2Table{
			tag:         tag,
			origLen:     origLen,
			xformLen:    xformLen,
			transformed: transformed,
		})
	}

	// 压缩流。
	compressedOffset := r.off
	if compressedLen > uint32(len(data)-compressedOffset) {
		return nil, fontDecodeErr("woff2 compressed stream out of bounds")
	}
	compressed := data[compressedOffset : compressedOffset+int(compressedLen)]
	decompressed, err := readBrotli(compressed, int(uncompressedSize))
	if err != nil {
		return nil, err
	}

	// 按目录顺序切分。
	cursor := 0
	for _, t := range tables {
		n := int(t.xformLen)
		if cursor+n > len(decompressed) {
			return nil, fontDecodeErr("woff2 table data truncated")
		}
		t.data = decompressed[cursor : cursor+n]
		cursor += n
		if !t.transformed {
			t.dstLen = t.origLen
		}
	}

	// glyf/loca 成对存在且变换状态一致。
	glyf := findTable(tables, tagGlyf)
	loca := findTable(tables, tagLoca)
	if (glyf == nil) != (loca == nil) {
		return nil, fontDecodeErr("woff2 has only one of glyf/loca")
	}
	info := &woff2FontInfo{}
	if glyf != nil {
		if glyf.transformed != loca.transformed {
			return nil, fontDecodeErr("woff2 transforms only one of glyf/loca")
		}
		if glyf.transformed {
			glyfOut, locaOut, xInfo, err := reconstructGlyf(glyf.data, loca.origLen)
			if err != nil {
				return nil, err
			}
			glyf.data = glyfOut
			glyf.dstLen = uint32(len(glyfOut))
			loca.data = locaOut
			loca.dstLen = uint32(len(locaOut))
			info = xInfo
		}
	}

	// hmtx 变换（需要 glyf 重构出的 xMin 与 hhea.numberOfHMetrics）。
	if hmtx := findTable(tables, tagHmtx); hmtx != nil && hmtx.transformed {
		hhea := findTable(tables, tagHhea)
		if hhea == nil || len(hhea.data) < 36 {
			return nil, fontDecodeErr("transformed hmtx without a usable hhea")
		}
		numHMetrics := binary.BigEndian.Uint16(hhea.data[34:36])
		out, err := reconstructHmtx(hmtx.data, info.numGlyphs, numHMetrics, info.xMins)
		if err != nil {
			return nil, err
		}
		hmtx.data = out
		hmtx.dstLen = uint32(len(out))
	}

	return assembleSFNT(flavor, tables)
}

// readBrotli 解压 WOFF2 的 brotli 流（严格校验解压长度）。
func readBrotli(src []byte, want int) ([]byte, error) {
	br := brotli.NewReader(bytes.NewReader(src))
	out, err := io.ReadAll(io.LimitReader(br, int64(want)+1))
	if err != nil {
		return nil, fontDecodeErr("brotli: %v", err)
	}
	if len(out) != want {
		return nil, fontDecodeErr("brotli produced %d bytes, want %d", len(out), want)
	}
	return out, nil
}

// ── glyf 重组 ────────────────────────────────────────────────────────

// point 是重构过程中的一个字形点（设计坐标，绝对值而非增量）。
type point struct {
	x, y    int
	onCurve bool
}

// withSign 按参考实现：baseval 的正负由 flag 最低位决定。
func withSign(flag, baseval int) int {
	if flag&1 != 0 {
		return baseval
	}
	return -baseval
}

// reconstructGlyf 把变换后的 glyf 还原成原始 glyf + loca 表。
func reconstructGlyf(data []byte, locaOrigLen uint32) (glyfOut, locaOut []byte, info *woff2FontInfo, err error) {
	const numSubStreams = 7

	r := &binReader{data: data}
	version, err := r.u16()
	if err != nil {
		return nil, nil, nil, err
	}
	if version != 0 {
		return nil, nil, nil, fontDecodeErr("transformed glyf version %d", version)
	}
	flags, err := r.u16()
	if err != nil {
		return nil, nil, nil, err
	}
	hasOverlapBitmap := flags&0x01 != 0
	numGlyphs, err := r.u16()
	if err != nil {
		return nil, nil, nil, err
	}
	indexFormat, err := r.u16()
	if err != nil {
		return nil, nil, nil, err
	}
	// loca 的原长必须与 indexFormat/numGlyphs 一致（规范强制）。
	offsetSize := uint32(2)
	if indexFormat != 0 {
		offsetSize = 4
	}
	if locaOrigLen != offsetSize*(uint32(numGlyphs)+1) {
		return nil, nil, nil, fontDecodeErr("loca length %d mismatches numGlyphs %d", locaOrigLen, numGlyphs)
	}

	var sizes [numSubStreams]uint32
	for i := 0; i < numSubStreams; i++ {
		sizes[i], err = r.u32()
		if err != nil {
			return nil, nil, nil, err
		}
	}
	// 子流紧跟在 36 字节头部之后，按目录声明顺序排列。
	subOff := 36
	subs := make([][]byte, numSubStreams)
	for i := 0; i < numSubStreams; i++ {
		if subOff+int(sizes[i]) > len(data) {
			return nil, nil, nil, fontDecodeErr("glyf substream %d out of bounds", i)
		}
		subs[i] = data[subOff : subOff+int(sizes[i])]
		subOff += int(sizes[i])
	}
	var overlapBitmap []byte
	if hasOverlapBitmap {
		n := (int(numGlyphs) + 7) >> 3
		if subOff+n > len(data) {
			return nil, nil, nil, fontDecodeErr("glyf overlap bitmap out of bounds")
		}
		overlapBitmap = data[subOff : subOff+n]
	}

	nContourR := &binReader{data: subs[0]}
	nPointsR := &binReader{data: subs[1]}
	flagR := &binReader{data: subs[2]}
	glyphR := &binReader{data: subs[3]}
	compositeR := &binReader{data: subs[4]}
	bboxR := &binReader{data: subs[5]}
	instrR := &binReader{data: subs[6]}

	// bbox 位图在 bbox 子流开头。
	bitmapLen := ((int(numGlyphs) + 31) >> 5) << 2
	if bitmapLen > len(subs[5]) {
		return nil, nil, nil, fontDecodeErr("glyf bbox bitmap truncated")
	}
	bboxBitmap := subs[5][:bitmapLen]
	if err := bboxR.skip(bitmapLen); err != nil {
		return nil, nil, nil, err
	}

	locaValues := make([]uint32, int(numGlyphs)+1)
	xMins := make([]int16, int(numGlyphs))
	glyf := make([]byte, 0, int(numGlyphs)*64)

	for i := 0; i < int(numGlyphs); i++ {
		haveBBox := bboxBitmap[i>>3]&(0x80>>(uint(i)&7)) != 0
		nContoursRaw, err := nContourR.u16()
		if err != nil {
			return nil, nil, nil, err
		}
		nContours := int16(nContoursRaw)

		var glyph []byte
		switch {
		case nContours == -1:
			// 复合字形：必须有显式 bbox。
			if !haveBBox {
				return nil, nil, nil, fontDecodeErr("composite glyph %d has no bbox", i)
			}
			compositeSize, haveInstr, err := sizeOfComposite(compositeR)
			if err != nil {
				return nil, nil, nil, err
			}
			instrLen := 0
			if haveInstr {
				instrLen, err = read255UShort(glyphR)
				if err != nil {
					return nil, nil, nil, err
				}
			}
			glyph = make([]byte, 0, 12+compositeSize+instrLen)
			glyph = appendU16(glyph, 0xFFFF)
			bbox, err := bboxR.take(8)
			if err != nil {
				return nil, nil, nil, err
			}
			glyph = append(glyph, bbox...)
			comp, err := compositeR.take(compositeSize)
			if err != nil {
				return nil, nil, nil, err
			}
			glyph = append(glyph, comp...)
			if haveInstr {
				glyph = appendU16(glyph, uint16(instrLen))
				ins, err := instrR.take(instrLen)
				if err != nil {
					return nil, nil, nil, err
				}
				glyph = append(glyph, ins...)
			}
		case nContours > 0:
			// 简单字形：点数 → 标志 → triplet 坐标 → 指令。
			nPointsPer := make([]int, nContours)
			total := 0
			for j := 0; j < int(nContours); j++ {
				n, err := read255UShort(nPointsR)
				if err != nil {
					return nil, nil, nil, err
				}
				nPointsPer[j] = n
				total += n
				if total > 1<<20 {
					return nil, nil, nil, fontDecodeErr("glyph %d has implausible point count", i)
				}
			}
			if total > flagR.remaining() {
				return nil, nil, nil, fontDecodeErr("glyph %d flag stream truncated", i)
			}
			flagsBuf := flagR.data[flagR.off : flagR.off+total]
			points, consumed, err := tripletDecode(flagsBuf, glyphR.data[glyphR.off:], total)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("glyph %d: %w", i, err)
			}
			if err := flagR.skip(total); err != nil {
				return nil, nil, nil, err
			}
			if err := glyphR.skip(consumed); err != nil {
				return nil, nil, nil, err
			}
			instrLen, err := read255UShort(glyphR)
			if err != nil {
				return nil, nil, nil, err
			}

			glyph = make([]byte, 0, 12+2*int(nContours)+5*total+instrLen)
			glyph = appendU16(glyph, uint16(nContours))
			if haveBBox {
				bbox, err := bboxR.take(8)
				if err != nil {
					return nil, nil, nil, err
				}
				glyph = append(glyph, bbox...)
			} else {
				glyph = append(glyph, computeBBox(points)...)
			}
			end := -1
			for _, n := range nPointsPer {
				end += n
				if end >= 1<<16 {
					return nil, nil, nil, fontDecodeErr("glyph %d endPoint overflow", i)
				}
				glyph = appendU16(glyph, uint16(end))
			}
			glyph = appendU16(glyph, uint16(instrLen))
			ins, err := instrR.take(instrLen)
			if err != nil {
				return nil, nil, nil, err
			}
			glyph = append(glyph, ins...)

			hasOverlap := hasOverlapBitmap && overlapBitmap[i>>3]&(0x80>>(uint(i)&7)) != 0
			glyph = append(glyph, storePoints(points, hasOverlap)...)
		default:
			// 空字形：不得带 bbox。
			if haveBBox {
				return nil, nil, nil, fontDecodeErr("empty glyph %d has a bbox", i)
			}
		}

		locaValues[i] = uint32(len(glyf))
		glyf = append(glyf, glyph...)
		for len(glyf)%4 != 0 {
			glyf = append(glyf, 0)
		}
		if nContours > 0 && len(glyph) >= 4 {
			xMins[i] = int16(binary.BigEndian.Uint16(glyph[2:4]))
		}
	}
	locaValues[numGlyphs] = uint32(len(glyf))

	loca := make([]byte, 0, len(locaValues)*int(offsetSize))
	for _, v := range locaValues {
		if indexFormat != 0 {
			loca = appendU32(loca, v)
		} else {
			loca = appendU16(loca, uint16(v>>1))
		}
	}
	return glyf, loca, &woff2FontInfo{numGlyphs: numGlyphs, indexFormat: indexFormat, xMins: xMins}, nil
}

// tripletDecode 解码 WOFF2 的 triplet 点编码（规范 §5.2）。
func tripletDecode(flags []byte, data []byte, nPoints int) ([]point, int, error) {
	points := make([]point, nPoints)
	x, y := 0, 0
	idx := 0
	for i := 0; i < nPoints; i++ {
		flag := int(flags[i])
		onCurve := flag>>7 == 0
		flag &= 0x7f
		var nBytes int
		switch {
		case flag < 84:
			nBytes = 1
		case flag < 120:
			nBytes = 2
		case flag < 124:
			nBytes = 3
		default:
			nBytes = 4
		}
		if idx+nBytes > len(data) {
			return nil, 0, fontDecodeErr("triplet stream truncated at point %d", i)
		}
		var dx, dy int
		switch {
		case flag < 10:
			dx = 0
			dy = withSign(flag, ((flag&14)<<7)+int(data[idx]))
		case flag < 20:
			dx = withSign(flag, (((flag-10)&14)<<7)+int(data[idx]))
			dy = 0
		case flag < 84:
			b0 := flag - 20
			b1 := int(data[idx])
			dx = withSign(flag, 1+(b0&0x30)+(b1>>4))
			dy = withSign(flag>>1, 1+((b0&0x0c)<<2)+(b1&0x0f))
		case flag < 120:
			b0 := flag - 84
			dx = withSign(flag, 1+((b0/12)<<8)+int(data[idx]))
			dy = withSign(flag>>1, 1+(((b0%12)>>2)<<8)+int(data[idx+1]))
		case flag < 124:
			b2 := int(data[idx+1])
			dx = withSign(flag, (int(data[idx])<<4)+(b2>>4))
			dy = withSign(flag>>1, ((b2&0x0f)<<8)+int(data[idx+2]))
		default:
			dx = withSign(flag, (int(data[idx])<<8)+int(data[idx+1]))
			dy = withSign(flag>>1, (int(data[idx+2])<<8)+int(data[idx+3]))
		}
		idx += nBytes
		x += dx
		y += dy
		points[i] = point{x: x, y: y, onCurve: onCurve}
	}
	return points, idx, nil
}

// storePoints 把点集编码成原始 glyf 的点数据（flag 流 + x 字节 + y 字节）。
func storePoints(points []point, overlapFirst bool) []byte {
	var flagBytes, xBytes, yBytes []byte
	lastX, lastY := 0, 0
	lastFlag := -1
	repeat := 0

	for i, p := range points {
		flag := 0
		if p.onCurve {
			flag |= 1 << 0
		}
		if overlapFirst && i == 0 {
			flag |= 1 << 6
		}
		dx := p.x - lastX
		dy := p.y - lastY
		if dx == 0 {
			flag |= 1 << 4 // x 与上一点相同
		} else if dx > -256 && dx < 256 {
			flag |= 1 << 1 // x 短坐标
			if dx > 0 {
				flag |= 1 << 4 // 正值标志（ThisXIsSame 位兼作符号）
			}
			xBytes = append(xBytes, byte(absInt(dx)))
		} else {
			xBytes = appendU16(xBytes, uint16(int16(dx)))
		}
		if dy == 0 {
			flag |= 1 << 5
		} else if dy > -256 && dy < 256 {
			flag |= 1 << 2
			if dy > 0 {
				flag |= 1 << 5
			}
			yBytes = append(yBytes, byte(absInt(dy)))
		} else {
			yBytes = appendU16(yBytes, uint16(int16(dy)))
		}

		if flag == lastFlag && repeat != 255 {
			flagBytes[len(flagBytes)-1] |= 1 << 3 // repeat
			repeat++
		} else {
			if repeat != 0 {
				flagBytes = append(flagBytes, byte(repeat))
			}
			flagBytes = append(flagBytes, byte(flag))
			repeat = 0
		}
		lastX, lastY = p.x, p.y
		lastFlag = flag
	}
	if repeat != 0 {
		flagBytes = append(flagBytes, byte(repeat))
	}

	out := make([]byte, 0, len(flagBytes)+len(xBytes)+len(yBytes))
	out = append(out, flagBytes...)
	out = append(out, xBytes...)
	out = append(out, yBytes...)
	return out
}

// computeBBox 计算点集包围盒（glyf 记录的第 2-9 字节）。
func computeBBox(points []point) []byte {
	xMin, yMin, xMax, yMax := 0, 0, 0, 0
	if len(points) > 0 {
		xMin, xMax = points[0].x, points[0].x
		yMin, yMax = points[0].y, points[0].y
	}
	for _, p := range points[1:] {
		if p.x < xMin {
			xMin = p.x
		}
		if p.x > xMax {
			xMax = p.x
		}
		if p.y < yMin {
			yMin = p.y
		}
		if p.y > yMax {
			yMax = p.y
		}
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint16(out[0:], uint16(int16(xMin)))
	binary.BigEndian.PutUint16(out[2:], uint16(int16(yMin)))
	binary.BigEndian.PutUint16(out[4:], uint16(int16(xMax)))
	binary.BigEndian.PutUint16(out[6:], uint16(int16(yMax)))
	return out
}

// sizeOfComposite 计算一个复合字形数据的字节数，并报告是否带指令。
//
// ★ 只在**副本**上消费：参考实现按值传递 Buffer，原 composite 流游标保持
// 不动，随后调用方还要用原游标把这段数据读走。此前直接在原 reader 上
// 读取，复合字形的数据被跳过后再也读不到，多复合字形的字体解码到一半
// 就报 unexpected EOF。
func sizeOfComposite(r *binReader) (int, bool, error) {
	sub := &binReader{data: r.data, off: r.off}
	start := sub.off
	haveInstr := false
	flags := uint16(0x0020) // FLAG_MORE_COMPONENTS
	for flags&0x0020 != 0 {
		f, err := sub.u16()
		if err != nil {
			return 0, false, err
		}
		flags = f
		if flags&0x0100 != 0 { // WE_HAVE_INSTRUCTIONS
			haveInstr = true
		}
		argSize := 2 // glyph index
		if flags&0x0001 != 0 {
			argSize += 4 // ARG_1_AND_2_ARE_WORDS
		} else {
			argSize += 2
		}
		switch {
		case flags&0x0008 != 0: // WE_HAVE_A_SCALE
			argSize += 2
		case flags&0x0040 != 0: // WE_HAVE_AN_X_AND_Y_SCALE
			argSize += 4
		case flags&0x0080 != 0: // WE_HAVE_A_TWO_BY_TWO
			argSize += 8
		}
		if err := sub.skip(argSize); err != nil {
			return 0, false, err
		}
	}
	return sub.off - start, haveInstr, nil
}

// ── hmtx 重组 ────────────────────────────────────────────────────────

// reconstructHmtx 把变换后的 hmtx 还原成原始 hmtx（advanceWidth + lsb 序列）。
func reconstructHmtx(data []byte, numGlyphs, numHMetrics uint16, xMins []int16) ([]byte, error) {
	r := &binReader{data: data}
	hmtxFlags, err := r.u8()
	if err != nil {
		return nil, err
	}
	hasProportionalLsbs := hmtxFlags&1 == 0
	hasMonospaceLsbs := hmtxFlags&2 == 0
	if hmtxFlags&0xFC != 0 {
		return nil, fontDecodeErr("hmtx has reserved flag bits set")
	}
	if hasProportionalLsbs && hasMonospaceLsbs {
		return nil, fontDecodeErr("hmtx declares no transformed lsb arrays")
	}
	if numHMetrics > numGlyphs {
		return nil, fontDecodeErr("hhea.numberOfHMetrics %d > numGlyphs %d", numHMetrics, numGlyphs)
	}
	if numHMetrics < 1 {
		return nil, fontDecodeErr("hhea.numberOfHMetrics is zero")
	}
	if len(xMins) < int(numGlyphs) {
		return nil, fontDecodeErr("hmtx needs %d xMin values, have %d", numGlyphs, len(xMins))
	}

	advanceWidths := make([]uint16, numHMetrics)
	for i := range advanceWidths {
		v, err := r.u16()
		if err != nil {
			return nil, err
		}
		advanceWidths[i] = v
	}
	lsbs := make([]int16, 0, numGlyphs)
	for i := 0; i < int(numHMetrics); i++ {
		if hasProportionalLsbs {
			v, err := r.s16()
			if err != nil {
				return nil, err
			}
			lsbs = append(lsbs, v)
		} else {
			lsbs = append(lsbs, xMins[i])
		}
	}
	for i := int(numHMetrics); i < int(numGlyphs); i++ {
		if hasMonospaceLsbs {
			v, err := r.s16()
			if err != nil {
				return nil, err
			}
			lsbs = append(lsbs, v)
		} else {
			lsbs = append(lsbs, xMins[i])
		}
	}

	out := make([]byte, 0, 2*int(numGlyphs)+2*int(numHMetrics))
	for i := 0; i < int(numGlyphs); i++ {
		if i < int(numHMetrics) {
			out = appendU16(out, advanceWidths[i])
		}
		out = appendU16(out, uint16(lsbs[i]))
	}
	return out, nil
}

// ── WOFF1 ────────────────────────────────────────────────────────────

// decodeWOFF 把 WOFF1 容器还原成 sfnt（逐表 zlib 解压，未压缩的表原样拷贝）。
func decodeWOFF(data []byte) ([]byte, error) {
	if len(data) < 44 {
		return nil, fontDecodeErr("woff header truncated")
	}
	r := &binReader{data: data}
	if _, err := r.take(4); err != nil { // signature
		return nil, err
	}
	flavor, err := r.u32()
	if err != nil {
		return nil, err
	}
	if flavor == sfntTTC {
		return nil, fontDecodeErr("woff font collections (ttcf) are not supported")
	}
	if _, err := r.u32(); err != nil { // length
		return nil, err
	}
	numTables, err := r.u16()
	if err != nil {
		return nil, err
	}
	if numTables == 0 {
		return nil, fontDecodeErr("woff has no tables")
	}
	if err := r.skip(2 + 4 + 4); err != nil { // reserved + totalSfntSize + version
		return nil, err
	}
	if err := r.skip(4 + 4 + 4 + 4 + 4); err != nil { // meta/priv fields
		return nil, err
	}

	tables := make([]*woff2Table, 0, numTables)
	for i := 0; i < int(numTables); i++ {
		entry, err := r.take(20)
		if err != nil {
			return nil, err
		}
		tag := binary.BigEndian.Uint32(entry[0:4])
		offset := binary.BigEndian.Uint32(entry[4:8])
		compLen := binary.BigEndian.Uint32(entry[8:12])
		origLen := binary.BigEndian.Uint32(entry[12:16])
		if uint64(offset)+uint64(compLen) > uint64(len(data)) {
			return nil, fontDecodeErr("woff table %d range out of bounds", i)
		}
		raw := data[offset : offset+compLen]
		var content []byte
		if compLen < origLen {
			zr, err := zlib.NewReader(bytes.NewReader(raw))
			if err != nil {
				return nil, fontDecodeErr("woff table %d zlib: %v", i, err)
			}
			content, err = io.ReadAll(io.LimitReader(zr, int64(origLen)+1))
			zr.Close()
			if err != nil {
				return nil, fontDecodeErr("woff table %d inflate: %v", i, err)
			}
			if len(content) != int(origLen) {
				return nil, fontDecodeErr("woff table %d inflated to %d bytes, want %d",
					i, len(content), origLen)
			}
		} else {
			content = raw
		}
		tables = append(tables, &woff2Table{
			tag:     tag,
			origLen: origLen,
			dstLen:  origLen,
			data:    content,
		})
	}
	return assembleSFNT(flavor, tables)
}

// ── sfnt 组装 ────────────────────────────────────────────────────────

// assembleSFNT 按 OpenType 表目录顺序（tag 升序）写出 sfnt，并计算
// 各表 checksum 与 head.checkSumAdjustment。
func assembleSFNT(flavor uint32, tables []*woff2Table) ([]byte, error) {
	if len(tables) == 0 {
		return nil, fontDecodeErr("no tables to write")
	}
	if flavor != sfntTrueType && flavor != sfntOTTO {
		// 'true'/'typ1' 老式 Apple 字体也按 sfnt 处理；未知 flavor 交给
		// Skia 判断（与浏览器一致：容器合法即可尝试解码）。
	}
	sorted := make([]*woff2Table, len(tables))
	copy(sorted, tables)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].tag < sorted[j].tag })

	// head 的 checkSumAdjustment 必须先清零再计算（sfnt 规范）。
	for _, t := range sorted {
		if t.tag == tagHead {
			if len(t.data) < 12 {
				return nil, fontDecodeErr("head table is shorter than 12 bytes")
			}
			head := make([]byte, len(t.data))
			copy(head, t.data)
			binary.BigEndian.PutUint32(head[8:], 0)
			t.data = head
		}
	}

	numTables := len(sorted)
	headerLen := 12 + 16*numTables
	offsets := make([]int, numTables)
	pos := headerLen
	for i, t := range sorted {
		pos = align4(pos)
		offsets[i] = pos
		pos += len(t.data)
	}
	total := align4(pos)
	out := make([]byte, total)

	binary.BigEndian.PutUint32(out[0:], flavor)
	binary.BigEndian.PutUint16(out[4:], uint16(numTables))
	maxPow2 := 0
	for (1 << (maxPow2 + 1)) <= numTables {
		maxPow2++
	}
	searchRange := (1 << maxPow2) * 16
	binary.BigEndian.PutUint16(out[6:], uint16(searchRange))
	binary.BigEndian.PutUint16(out[8:], uint16(maxPow2))
	binary.BigEndian.PutUint16(out[10:], uint16(numTables*16-searchRange))

	headOffset := -1
	for i, t := range sorted {
		entry := out[12+16*i : 12+16*(i+1)]
		binary.BigEndian.PutUint32(entry[0:], t.tag)
		binary.BigEndian.PutUint32(entry[4:], checksum32(t.data))
		binary.BigEndian.PutUint32(entry[8:], uint32(offsets[i]))
		binary.BigEndian.PutUint32(entry[12:], uint32(len(t.data)))
		copy(out[offsets[i]:], t.data)
		if t.tag == tagHead {
			headOffset = offsets[i]
		}
	}

	if headOffset >= 0 {
		adjustment := uint32(0xB1B0AFBA) - checksum32(out)
		binary.BigEndian.PutUint32(out[headOffset+8:], adjustment)
	}
	return out, nil
}

// checksum32 是 sfnt 的 checksum 算法：按 4 字节大端累加，尾部不足补 0。
func checksum32(data []byte) uint32 {
	var sum uint32
	for i := 0; i < len(data); i += 4 {
		var v uint32
		for j := 0; j < 4; j++ {
			v <<= 8
			if i+j < len(data) {
				v |= uint32(data[i+j])
			}
		}
		sum += v
	}
	return sum
}

func align4(n int) int { return (n + 3) &^ 3 }

func appendU16(dst []byte, v uint16) []byte {
	return append(dst, byte(v>>8), byte(v))
}

func appendU32(dst []byte, v uint32) []byte {
	return append(dst, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
