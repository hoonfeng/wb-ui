// 版权所有 (C) 1999-2000 Harri Porten (porten@kde.org)
// 版权所有 (C) 2002-2023 Apple Inc. 保留所有权利。
// 版权所有 (C) 2010 Zoltan Herczeg (zherczeg@inf.u-szeged.hu)
//
// 使用约定：BSD 许可证
//
// 从 WebKit Source/JavaScriptCore/parser/Lexer.h 翻译为 Go

package parser

import (
	"math"
	"strings"
	"unicode"
	"wb-ui/jsc/runtime"
)

// CharacterType 字符分类枚举
type CharacterType uint8

const (
	CharLatin1IdStart CharacterType = iota
	CharZero
	CharNumber
	CharOtherIdPart
	CharBackSlash
	CharInvalid
	CharLineTerminator
	CharExclMark
	CharOpenParen
	CharCloseParen
	CharOpenBracket
	CharCloseBracket
	CharComma
	CharColon
	CharQuestion
	CharTilde
	CharQuote
	CharBackQuote
	CharDot
	CharSlash
	CharSemicolon
	CharOpenBrace
	CharCloseBrace
	CharPlus
	CharMinus
	CharTimes
	CharModulo
	CharAmpersand
	CharXor
	CharPipe
	CharLess
	CharGreater
	CharEqual
	CharWhiteSpace
	CharHash
	CharPrivateIdStart
)

var latin1CharTbl [256]CharacterType
var escValTbl [128]byte

func init() {
	for i := range latin1CharTbl {
		latin1CharTbl[i] = CharInvalid
	}
	latin1CharTbl[9] = CharWhiteSpace
	latin1CharTbl[10] = CharLineTerminator
	latin1CharTbl[11] = CharWhiteSpace
	latin1CharTbl[12] = CharWhiteSpace
	latin1CharTbl[13] = CharLineTerminator
	latin1CharTbl[32] = CharWhiteSpace
	latin1CharTbl[160] = CharWhiteSpace
	for _, c := range []byte{'!', '"', '#', '%', '&', '\'', '(', ')', '*', '+', ',', '-', '.', '/',
		':', ';', '<', '=', '>', '?', '@', '[', '\\', ']', '^', '`', '{', '|', '}', '~', '$'} {
		switch c {
		case '!':
			latin1CharTbl[c] = CharExclMark
		case '"', '\'':
			latin1CharTbl[c] = CharQuote
		case '#':
			latin1CharTbl[c] = CharHash
		case '%':
			latin1CharTbl[c] = CharModulo
		case '&':
			latin1CharTbl[c] = CharAmpersand
		case '(':
			latin1CharTbl[c] = CharOpenParen
		case ')':
			latin1CharTbl[c] = CharCloseParen
		case '*':
			latin1CharTbl[c] = CharTimes
		case '+':
			latin1CharTbl[c] = CharPlus
		case ',':
			latin1CharTbl[c] = CharComma
		case '-':
			latin1CharTbl[c] = CharMinus
		case '.':
			latin1CharTbl[c] = CharDot
		case '/':
			latin1CharTbl[c] = CharSlash
		case ':':
			latin1CharTbl[c] = CharColon
		case ';':
			latin1CharTbl[c] = CharSemicolon
		case '<':
			latin1CharTbl[c] = CharLess
		case '=':
			latin1CharTbl[c] = CharEqual
		case '>':
			latin1CharTbl[c] = CharGreater
		case '?':
			latin1CharTbl[c] = CharQuestion
		case '@':
			latin1CharTbl[c] = CharPrivateIdStart
		case '[':
			latin1CharTbl[c] = CharOpenBracket
		case '\\':
			latin1CharTbl[c] = CharBackSlash
		case ']':
			latin1CharTbl[c] = CharCloseBracket
		case '^':
			latin1CharTbl[c] = CharXor
		case '`':
			latin1CharTbl[c] = CharBackQuote
		case '{':
			latin1CharTbl[c] = CharOpenBrace
		case '|':
			latin1CharTbl[c] = CharPipe
		case '}':
			latin1CharTbl[c] = CharCloseBrace
		case '~':
			latin1CharTbl[c] = CharTilde
		case '$':
			latin1CharTbl[c] = CharLatin1IdStart
		case '_':
			latin1CharTbl[c] = CharLatin1IdStart
		}
	}
	latin1CharTbl['0'] = CharZero
	for i := '1'; i <= '9'; i++ {
		latin1CharTbl[i] = CharNumber
	}
	for i := 'A'; i <= 'Z'; i++ {
		latin1CharTbl[i] = CharLatin1IdStart
	}
	for i := 'a'; i <= 'z'; i++ {
		latin1CharTbl[i] = CharLatin1IdStart
	}
	for _, c := range []byte{170, 181, 186, 192, 193, 194, 195, 196, 197, 198, 199,
		200, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210,
		211, 212, 213, 214, 216, 217, 218, 219, 220, 221, 222,
		223, 224, 225, 226, 227, 228, 229, 230, 231, 232, 233,
		234, 235, 236, 237, 238, 239, 240, 241, 242, 243, 244,
		245, 246, 248, 249, 250, 251, 252, 253, 254, 255} {
		latin1CharTbl[c] = CharLatin1IdStart
	}
	latin1CharTbl[183] = CharOtherIdPart

	for i := range escValTbl {
		escValTbl[i] = 0
	}
	for i := 32; i <= 126; i++ {
		escValTbl[i] = byte(i)
	}
	escValTbl['n'] = '\n'
	escValTbl['r'] = '\r'
	escValTbl['t'] = '\t'
	escValTbl['b'] = '\b'
	escValTbl['f'] = '\f'
	escValTbl['v'] = '\v'
	escValTbl['0'] = 0
}

// RawStringsBuildMode 模板字符串模式
type RawStringsBuildMode int

const (
	BuildRawStrings RawStringsBuildMode = iota
	DontBuildRawStrings
)

// Lexer 词法分析器
type Lexer struct {
	vm                           *runtime.VM
	arena                        *IdentifierArena
	code, codeStart, codeEnd, lineStart []byte
	lexErrorMessage              string
	lineNumber                   int
	current                      byte
	hasLineTerminatorBeforeToken bool
	atLineStart                  bool
	parsingBuiltinFunction       bool
	buffer8                      []byte
	buffer16                     []uint16
	bufferForRawTemplateString16 []uint16
	positionBeforeLastNewline    JSTextPosition
	isReparsingFunction          bool
	err                          bool
	sourceURLDirective           string
	sourceMappingURLDirective    string
	scriptMode                   JSParserScriptMode
	source                       *SourceCode
	sourceOffset                 uint32
	codeWithOffset               []byte
}

func NewLexer(vm *runtime.VM, builtinMode JSParserBuiltinMode, scriptMode JSParserScriptMode) *Lexer {
	return &Lexer{vm: vm, scriptMode: scriptMode, parsingBuiltinFunction: builtinMode == Builtin}
}

func (lx *Lexer) IsWhiteSpace(ch byte) bool { return ch == ' ' || ch == '\t' || ch == 0x0B || ch == 0x0C || ch == 0xA0 }
func (lx *Lexer) IsLineTerminator(ch byte) bool { return ch == '\r' || ch == '\n' }

func (lx *Lexer) curOff() int { return len(lx.codeStart) - len(lx.code) }
func (lx *Lexer) curLineOff() int { return len(lx.codeStart) - len(lx.lineStart) }
func (lx *Lexer) curPos() JSTextPosition {
	return JSTextPosition{Line: lx.lineNumber, Offset: lx.curOff(), LineStartOffset: lx.curLineOff()}
}
func (lx *Lexer) atEnd() bool { return len(lx.code) == 0 }
func (lx *Lexer) peek(n int) byte { if n < len(lx.code) { return lx.code[n] }; return 0 }
func (lx *Lexer) shift() {
	if len(lx.code) > 1 { lx.code = lx.code[1:]; lx.current = lx.code[0] } else { lx.code = lx.code[:0]; lx.current = 0 }
}
func (lx *Lexer) shiftN(n int) {
	lx.code = lx.code[n:]
	if len(lx.code) > 0 { lx.current = lx.code[0] } else { lx.current = 0 }
}

func (lx *Lexer) SetCode(source *SourceCode, arena *ParserArena) {
	lx.arena = arena.identifierArena
	lx.lineNumber = source.FirstLineVal()
	src := source.Provider.SourceVal()
	if src != "" {
		lx.codeStart = []byte(src)
	} else {
		lx.codeStart = nil
	}
	lx.source = source
	lx.sourceOffset = uint32(source.StartOffset)
	lx.codeWithOffset = lx.codeStart[lx.sourceOffset:]
	lx.code = lx.codeWithOffset
	lx.codeEnd = lx.codeStart[source.EndOffset:]
	lx.err = false
	lx.atLineStart = true
	lx.lineStart = lx.code
	lx.lexErrorMessage = ""
	lx.sourceURLDirective = ""
	lx.sourceMappingURLDirective = ""
	lx.buffer8 = make([]byte, 0, 32)
	lx.buffer16 = make([]uint16, 0, 32)
	lx.bufferForRawTemplateString16 = make([]uint16, 0, 32)
	if len(lx.code) > 0 { lx.current = lx.code[0] } else { lx.current = 0 }
}

func (lx *Lexer) shiftLineTerm() {
	lx.positionBeforeLastNewline = lx.curPos()
	prev := lx.current
	lx.shift()
	if prev == '\r' && lx.current == '\n' { lx.shift() }
	lx.lineNumber++
	lx.lineStart = lx.code
}

func (lx *Lexer) LineNumber() int { return lx.lineNumber }
func (lx *Lexer) HasLineTerminatorBeforeToken() bool { return lx.hasLineTerminatorBeforeToken }
func (lx *Lexer) SetHasLineTerminatorBeforeToken(v bool) { lx.hasLineTerminatorBeforeToken = v }
func (lx *Lexer) SawError() bool { return lx.err }
func (lx *Lexer) SetSawError(v bool) { lx.err = v }
func (lx *Lexer) GetErrorMessage() string { return lx.lexErrorMessage }
func (lx *Lexer) SetErrorMessage(s string) { lx.lexErrorMessage = s }
func (lx *Lexer) SourceURLDirective() string { return lx.sourceURLDirective }
func (lx *Lexer) SourceMappingURLDirective() string { return lx.sourceMappingURLDirective }
func (lx *Lexer) SetOffset(offset, lineStartOffset int) {
	lx.err = false; lx.lexErrorMessage = ""
	lx.code = lx.codeStart[offset:]
	lx.lineStart = lx.codeStart[lineStartOffset:]
	lx.buffer8 = lx.buffer8[:0]; lx.buffer16 = lx.buffer16[:0]
	if len(lx.code) > 0 { lx.current = lx.code[0] } else { lx.current = 0 }
}
func (lx *Lexer) SetLineNumber(line int) { if line >= 0 { lx.lineNumber = line } }
func (lx *Lexer) IsReparsingFunction() bool { return lx.isReparsingFunction }
func (lx *Lexer) SetIsReparsingFunction(v bool) { lx.isReparsingFunction = v }

// ===== Lex =====

func (lx *Lexer) Lex(tok *JSToken, flags int, strict bool) JSTokenType {
	lx.hasLineTerminatorBeforeToken = false
	return lx.lexMain(tok, flags, strict)
}

func (lx *Lexer) lexMain(tok *JSToken, flags int, strict bool) JSTokenType {
	for !lx.atEnd() {
		if lx.IsWhiteSpace(lx.current) { lx.shift(); continue }
		if lx.IsLineTerminator(lx.current) { lx.hasLineTerminatorBeforeToken = true; lx.shiftLineTerm(); continue }
		break
	}
	if lx.atEnd() { tok.Type = EOFTOK; return EOFTOK }

	tok.StartPosition = lx.curPos()
	ch := lx.current
	ct := CharInvalid
	if int(ch) < 256 { ct = latin1CharTbl[ch] }

	switch ct {
	case CharLatin1IdStart:
		return lx.parseIdent(&tok.Data)
	case CharBackSlash:
		lx.shift()
		if lx.current == 'u' { lx.shiftN(4); return lx.parseIdent(&tok.Data) }
		lx.err = true; lx.lexErrorMessage = "非法转义"; return ERRORTOK
	case CharZero, CharNumber:
		return lx.parseNum(&tok.Data)
	case CharDot:
		if len(lx.code) >= 3 && lx.code[1] == '.' && lx.code[2] == '.' {
			lx.shiftN(3); tok.Type = DOTDOTDOT; tok.EndPosition = lx.curPos(); return DOTDOTDOT
		}
		if len(lx.code) >= 2 && lx.code[1] >= '0' && lx.code[1] <= '9' {
			return lx.parseNum(&tok.Data)
		}
		lx.shift(); tok.Type = DOT; tok.EndPosition = lx.curPos(); return DOT
	case CharSemicolon:
		lx.shift(); tok.Type = SEMICOLON; tok.EndPosition = lx.curPos(); return SEMICOLON
	case CharOpenBrace:
		lx.shift(); tok.Type = OPENBRACE; tok.EndPosition = lx.curPos(); return OPENBRACE
	case CharCloseBrace:
		lx.shift(); tok.Type = CLOSEBRACE; tok.EndPosition = lx.curPos(); return CLOSEBRACE
	case CharOpenParen:
		lx.shift(); tok.Type = OPENPAREN; tok.EndPosition = lx.curPos(); return OPENPAREN
	case CharCloseParen:
		lx.shift(); tok.Type = CLOSEPAREN; tok.EndPosition = lx.curPos(); return CLOSEPAREN
	case CharOpenBracket:
		lx.shift(); tok.Type = OPENBRACKET; tok.EndPosition = lx.curPos(); return OPENBRACKET
	case CharCloseBracket:
		lx.shift(); tok.Type = CLOSEBRACKET; tok.EndPosition = lx.curPos(); return CLOSEBRACKET
	case CharComma:
		lx.shift(); tok.Type = COMMA; tok.EndPosition = lx.curPos(); return COMMA
	case CharColon:
		lx.shift(); tok.Type = COLON; tok.EndPosition = lx.curPos(); return COLON
	case CharQuestion:
		lx.shift()
		if lx.current == '?' {
			lx.shift()
			if lx.current == '=' { lx.shift(); tok.Type = COALESCEEQUAL; tok.EndPosition = lx.curPos(); return COALESCEEQUAL }
			tok.Type = COALESCE; tok.EndPosition = lx.curPos(); return COALESCE
		}
		if lx.current == '.' { lx.shift(); tok.Type = QUESTIONDOT; tok.EndPosition = lx.curPos(); return QUESTIONDOT }
		tok.Type = QUESTION; tok.EndPosition = lx.curPos(); return QUESTION
	case CharTilde:
		lx.shift(); tok.Type = TILDE; tok.EndPosition = lx.curPos(); return TILDE
	case CharQuote:
		return lx.parseStr(tok)
	case CharBackQuote:
		return lx.parseTmpl(tok)
	case CharSlash:
		return lx.parseSlash(tok)
	case CharPlus:
		lx.shift()
		if lx.current == '+' { lx.shift(); tok.Type = PLUSPLUS; tok.EndPosition = lx.curPos(); return PLUSPLUS }
		if lx.current == '=' { lx.shift(); tok.Type = PLUSEQUAL; tok.EndPosition = lx.curPos(); return PLUSEQUAL }
		tok.Type = PLUS; tok.EndPosition = lx.curPos(); return PLUS
	case CharMinus:
		lx.shift()
		if lx.current == '-' { lx.shift(); tok.Type = MINUSMINUS; tok.EndPosition = lx.curPos(); return MINUSMINUS }
		if lx.current == '=' { lx.shift(); tok.Type = MINUSEQUAL; tok.EndPosition = lx.curPos(); return MINUSEQUAL }
		tok.Type = MINUS; tok.EndPosition = lx.curPos(); return MINUS
	case CharTimes:
		lx.shift()
		if lx.current == '*' {
			lx.shift()
			if lx.current == '=' { lx.shift(); tok.Type = POWEQUAL; tok.EndPosition = lx.curPos(); return POWEQUAL }
			tok.Type = POW; tok.EndPosition = lx.curPos(); return POW
		}
		if lx.current == '=' { lx.shift(); tok.Type = MULTEQUAL; tok.EndPosition = lx.curPos(); return MULTEQUAL }
		tok.Type = TIMES; tok.EndPosition = lx.curPos(); return TIMES
	case CharModulo:
		lx.shift()
		if lx.current == '=' { lx.shift(); tok.Type = MODEQUAL; tok.EndPosition = lx.curPos(); return MODEQUAL }
		tok.Type = MOD; tok.EndPosition = lx.curPos(); return MOD
	case CharAmpersand:
		lx.shift()
		if lx.current == '&' { lx.shift(); tok.Type = AND; tok.EndPosition = lx.curPos(); return AND }
		if lx.current == '=' { lx.shift(); tok.Type = ANDEQUAL; tok.EndPosition = lx.curPos(); return ANDEQUAL }
		tok.Type = BITAND; tok.EndPosition = lx.curPos(); return BITAND
	case CharXor:
		lx.shift()
		if lx.current == '=' { lx.shift(); tok.Type = BITXOREQUAL; tok.EndPosition = lx.curPos(); return BITXOREQUAL }
		tok.Type = BITXOR; tok.EndPosition = lx.curPos(); return BITXOR
	case CharPipe:
		lx.shift()
		if lx.current == '|' { lx.shift(); tok.Type = OR; tok.EndPosition = lx.curPos(); return OR }
		if lx.current == '=' { lx.shift(); tok.Type = BITOREQUAL; tok.EndPosition = lx.curPos(); return BITOREQUAL }
		tok.Type = BITOR; tok.EndPosition = lx.curPos(); return BITOR
	case CharLess:
		lx.shift()
		if lx.current == '<' {
			lx.shift()
			if lx.current == '=' { lx.shift(); tok.Type = LSHIFTEQUAL; tok.EndPosition = lx.curPos(); return LSHIFTEQUAL }
			tok.Type = LSHIFT; tok.EndPosition = lx.curPos(); return LSHIFT
		}
		if lx.current == '=' { lx.shift(); tok.Type = LE; tok.EndPosition = lx.curPos(); return LE }
		tok.Type = LT; tok.EndPosition = lx.curPos(); return LT
	case CharGreater:
		lx.shift()
		if lx.current == '>' {
			lx.shift()
			if lx.current == '>' {
				lx.shift()
				if lx.current == '=' { lx.shift(); tok.Type = URSHIFTEQUAL; tok.EndPosition = lx.curPos(); return URSHIFTEQUAL }
				tok.Type = URSHIFT; tok.EndPosition = lx.curPos(); return URSHIFT
			}
			if lx.current == '=' { lx.shift(); tok.Type = RSHIFTEQUAL; tok.EndPosition = lx.curPos(); return RSHIFTEQUAL }
			tok.Type = RSHIFT; tok.EndPosition = lx.curPos(); return RSHIFT
		}
		if lx.current == '=' { lx.shift(); tok.Type = GE; tok.EndPosition = lx.curPos(); return GE }
		tok.Type = GT; tok.EndPosition = lx.curPos(); return GT
	case CharEqual:
		lx.shift()
		if lx.current == '=' {
			lx.shift()
			if lx.current == '=' { lx.shift(); tok.Type = STREQ; tok.EndPosition = lx.curPos(); return STREQ }
			tok.Type = EQEQ; tok.EndPosition = lx.curPos(); return EQEQ
		}
		if lx.current == '>' { lx.shift(); tok.Type = ARROWFUNCTION; tok.EndPosition = lx.curPos(); return ARROWFUNCTION }
		tok.Type = EQUAL; tok.EndPosition = lx.curPos(); return EQUAL
	case CharExclMark:
		lx.shift()
		if lx.current == '=' {
			lx.shift()
			if lx.current == '=' { lx.shift(); tok.Type = STRNEQ; tok.EndPosition = lx.curPos(); return STRNEQ }
			tok.Type = NE; tok.EndPosition = lx.curPos(); return NE
		}
		tok.Type = EXCLAMATION; tok.EndPosition = lx.curPos(); return EXCLAMATION
	case CharHash:
		lx.shift()
		if isIdStartByte(lx.current) { return lx.parsePrivId(&tok.Data) }
		lx.err = true; lx.lexErrorMessage = "非法私有标识符"; return ERRORTOK
	case CharPrivateIdStart:
		lx.shift()
		if lx.parsingBuiltinFunction && isIdStartByte(lx.current) { return lx.parseIdent(&tok.Data) }
		lx.err = true; lx.lexErrorMessage = "非法字符 '@'"; return ERRORTOK
	default:
		lx.err = true; lx.lexErrorMessage = "非法字符"; return ERRORTOK
	}
}

// ===== parseSlash =====
func (lx *Lexer) parseSlash(tok *JSToken) JSTokenType {
	lx.shift()
	if lx.current == '/' {
		for !lx.atEnd() && !lx.IsLineTerminator(lx.current) { lx.shift() }
		return lx.lexMain(tok, 0, false)
	}
	if lx.current == '*' {
		lx.shift()
		for !lx.atEnd() {
			if lx.current == '*' && len(lx.code) >= 2 && lx.code[1] == '/' { lx.shiftN(2); return lx.lexMain(tok, 0, false) }
			if lx.IsLineTerminator(lx.current) { lx.shiftLineTerm() } else { lx.shift() }
		}
		lx.err = true; lx.lexErrorMessage = "未结束的多行注释"; return ERRORTOK
	}
	if lx.current == '=' { lx.shift() }
	tok.Type = DIVIDE; tok.EndPosition = lx.curPos(); return DIVIDE
}

// ===== parseIdent =====
func (lx *Lexer) parseIdent(data *JSTokenData) JSTokenType {
	start := lx.curOff()
	lx.shift()
	for !lx.atEnd() && isIdPartByte(lx.current) { lx.shift() }
	s := string(lx.codeStart[start:lx.curOff()])
	if kw := keywordType(s); kw != 0 { return kw }
	data.Ident = lx.arena.MakeIdentifier(lx.vm, []byte(s))
	return IDENT
}

func (lx *Lexer) parsePrivId(data *JSTokenData) JSTokenType {
	start := lx.curOff()
	lx.shift()
	for !lx.atEnd() && isIdPartByte(lx.current) { lx.shift() }
	s := "#" + string(lx.codeStart[start:lx.curOff()])
	data.Ident = lx.arena.MakeIdentifier(lx.vm, []byte(s))
	return PRIVATENAME
}

// ===== parseNum =====
func (lx *Lexer) parseNum(data *JSTokenData) JSTokenType {
	if lx.current == '0' && len(lx.code) > 1 {
		switch lx.peek(1) {
		case 'x', 'X':
			lx.shiftN(2); return parseHexLit(data, lx)
		case 'o', 'O':
			lx.shiftN(2); return parseOctalLit(data, lx)
		case 'b', 'B':
			lx.shiftN(2); return parseBinaryLit(data, lx)
		case 'n', 'N':
			lx.shiftN(2); return BIGINT
		}
	}
	return parseDecimalLit(data, lx)
}

func parseHexLit(data *JSTokenData, lx *Lexer) JSTokenType {
	var v uint64
	for !lx.atEnd() {
		ch := lx.current
		switch {
		case ch >= '0' && ch <= '9':
			v = v*16 + uint64(ch-'0')
		case ch >= 'a' && ch <= 'f':
			v = v*16 + uint64(ch-'a'+10)
		case ch >= 'A' && ch <= 'F':
			v = v*16 + uint64(ch-'A'+10)
		default:
			data.DoubleValue = float64(v); return INTEGER
		}
		lx.shift()
	}
	data.DoubleValue = float64(v)
	return INTEGER
}

func parseOctalLit(data *JSTokenData, lx *Lexer) JSTokenType {
	var v uint64
	for !lx.atEnd() && lx.current >= '0' && lx.current <= '7' {
		v = v*8 + uint64(lx.current-'0')
		lx.shift()
	}
	data.DoubleValue = float64(v)
	return INTEGER
}

func parseBinaryLit(data *JSTokenData, lx *Lexer) JSTokenType {
	var v uint64
	for !lx.atEnd() && (lx.current == '0' || lx.current == '1') {
		v = v*2 + uint64(lx.current-'0')
		lx.shift()
	}
	data.DoubleValue = float64(v)
	return INTEGER
}

func parseDecimalLit(data *JSTokenData, lx *Lexer) JSTokenType {
	var v float64
	isF := false

	for !lx.atEnd() && lx.current >= '0' && lx.current <= '9' {
		v = v*10 + float64(lx.current-'0')
		lx.shift()
	}

	if !lx.atEnd() && lx.current == '.' && !(len(lx.code) >= 2 && lx.code[1] == '.' && len(lx.code) >= 3 && lx.code[2] == '.') {
		isF = true
		lx.shift()
		div := 10.0
		for !lx.atEnd() && lx.current >= '0' && lx.current <= '9' {
			v += float64(lx.current-'0') / div
			div *= 10
			lx.shift()
		}
	}

	if !lx.atEnd() && (lx.current == 'e' || lx.current == 'E') {
		isF = true
		lx.shift()
		neg := false
		if lx.current == '+' { lx.shift() } else if lx.current == '-' { neg = true; lx.shift() }
		var exp int
		for !lx.atEnd() && lx.current >= '0' && lx.current <= '9' {
			exp = exp*10 + int(lx.current-'0')
			lx.shift()
		}
		if neg { exp = -exp }
		v *= math.Pow10(exp)
	}

	if !lx.atEnd() && lx.current == 'n' { lx.shift(); return BIGINT }
	data.DoubleValue = v
	if isF { return DOUBLE }
	return INTEGER
}

// ===== parseStr =====
func (lx *Lexer) parseStr(tok *JSToken) JSTokenType {
	quote := lx.current
	lx.shift()
	var sb strings.Builder
	for !lx.atEnd() {
		if lx.current == quote { lx.shift(); tok.Data.Ident = lx.arena.MakeIdentifier(lx.vm, []byte(sb.String())); return STRING }
		if lx.current == '\\' {
			lx.shift()
			if lx.atEnd() { break }
			switch lx.current {
			case 'b': sb.WriteByte('\b'); case 'f': sb.WriteByte('\f')
			case 'n': sb.WriteByte('\n'); case 'r': sb.WriteByte('\r')
			case 't': sb.WriteByte('\t'); case 'v': sb.WriteByte('\v')
			case '0': sb.WriteByte(0)
			case 'x':
				if len(lx.code) >= 3 { sb.WriteByte(byte(hexVal(lx.code[1])*16 + hexVal(lx.code[2]))); lx.shiftN(2) }
			case 'u':
				if !lx.atEnd() && lx.current == '{' {
					lx.shift(); var cp uint32
					for !lx.atEnd() && lx.current != '}' { cp = cp*16 + hexVal(lx.current); lx.shift() }
					if lx.current == '}' { lx.shift() }
					sb.WriteRune(rune(cp))
				} else if len(lx.code) >= 4 {
					var val uint32
					for i := 0; i < 4; i++ { val = val*16 + hexVal(lx.code[i]) }
					sb.WriteRune(rune(val)); lx.shiftN(4)
				}
			default:
				if lx.IsLineTerminator(lx.current) {
					if lx.current == '\r' && len(lx.code) >= 2 && lx.code[1] == '\n' { lx.shift() }
				} else { sb.WriteByte(lx.current) }
			}
			lx.shift()
			continue
		}
		if lx.IsLineTerminator(lx.current) { lx.err = true; lx.lexErrorMessage = "未终止字符串"; return ERRORTOK }
		sb.WriteByte(lx.current); lx.shift()
	}
	lx.err = true; lx.lexErrorMessage = "未终止字符串"; return ERRORTOK
}

// ===== parseTmpl =====
func (lx *Lexer) parseTmpl(tok *JSToken) JSTokenType {
	lx.shift()
	var sb strings.Builder
	for !lx.atEnd() {
		if lx.current == '`' { lx.shift(); tok.Data.Ident = lx.arena.MakeIdentifier(lx.vm, []byte(sb.String())); return TEMPLATE }
		if lx.current == '$' && len(lx.code) >= 2 && lx.code[1] == '{' {
			lx.shiftN(2); tok.Data.Ident = lx.arena.MakeIdentifier(lx.vm, []byte(sb.String())); return TEMPLATE
		}
		if lx.current == '\\' {
			lx.shift()
			if lx.atEnd() { break }
			switch lx.current {
			case '`': sb.WriteByte('`'); case '$': sb.WriteByte('$')
			case 'n': sb.WriteByte('\n'); case 'r': sb.WriteByte('\r'); case 't': sb.WriteByte('\t')
			default: sb.WriteByte('\\'); sb.WriteByte(lx.current)
			}
			lx.shift()
			continue
		}
		if lx.IsLineTerminator(lx.current) { lx.shiftLineTerm(); sb.WriteByte('\n'); continue }
		sb.WriteByte(lx.current); lx.shift()
	}
	lx.err = true; lx.lexErrorMessage = "未终止模板字符串"; return ERRORTOK
}

// ===== LexExpectIdentifier =====
func (lx *Lexer) LexExpectIdentifier(tok *JSToken, flags int, strict bool) JSTokenType {
	if lx.atEnd() || !isAlpha(lx.current) { return lx.Lex(tok, flags, strict) }
	tok.StartPosition = lx.curPos(); lx.shift()
	for !lx.atEnd() && isAlnum(lx.current) { lx.shift() }
	tok.EndPosition = lx.curPos()
	return IDENT
}

// ===== 工具方法 =====
func (lx *Lexer) GetToken(tok JSToken) string {
	if lx.source != nil && lx.source.Provider != nil { return lx.source.Provider.GetRange(tok.StartPosition.Offset, tok.EndPosition.Offset) }
	return ""
}
func (lx *Lexer) CodeLength() int { return len(lx.codeEnd) - len(lx.codeStart) }
func (lx *Lexer) Clear() {
	lx.err = false; lx.lexErrorMessage = ""
	lx.buffer8 = lx.buffer8[:0]; lx.buffer16 = lx.buffer16[:0]
	lx.bufferForRawTemplateString16 = lx.bufferForRawTemplateString16[:0]
	lx.sourceURLDirective = ""; lx.sourceMappingURLDirective = ""; lx.isReparsingFunction = false
}
func (lx *Lexer) ClearErrorCodeAndBuffers() { lx.err = false; lx.lexErrorMessage = ""; lx.buffer8 = lx.buffer8[:0]; lx.buffer16 = lx.buffer16[:0] }

// ===== 包级辅助 =====
func isIdStartByte(ch byte) bool {
	if int(ch) < len(latin1CharTbl) { return latin1CharTbl[ch] == CharLatin1IdStart }
	return unicode.IsLetter(rune(ch))
}
func isIdPartByte(ch byte) bool {
	if int(ch) < len(latin1CharTbl) { return latin1CharTbl[ch] <= CharOtherIdPart }
	return unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch))
}
func hexVal(c byte) uint32 {
	switch {
	case c >= '0' && c <= '9': return uint32(c - '0')
	case c >= 'a' && c <= 'f': return uint32(c - 'a' + 10)
	case c >= 'A' && c <= 'F': return uint32(c - 'A' + 10)
	}
	return 0
}
func isAlpha(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isAlnum(c byte) bool { return isAlpha(c) || (c >= '0' && c <= '9') }

func keywordType(s string) JSTokenType {
	switch s {
	case "null": return NULLTOKEN
	case "true": return TRUETOKEN
	case "false": return FALSETOKEN
	case "break": return BREAK
	case "case": return CASE
	case "default": return DEFAULT
	case "for": return FOR
	case "new": return NEW
	case "var": return VAR
	case "const": return CONSTTOKEN
	case "continue": return CONTINUE
	case "function": return FUNCTION
	case "return": return RETURN
	case "if": return IF
	case "this": return THISTOKEN
	case "do": return DO
	case "while": return WHILE
	case "switch": return SWITCH
	case "with": return WITH
	case "throw": return THROW
	case "try": return TRY
	case "catch": return CATCH
	case "finally": return FINALLY
	case "debugger": return DEBUGGER
	case "else": return ELSE
	case "import": return IMPORT
	case "export": return EXPORT_
	case "class": return CLASSTOKEN
	case "extends": return EXTENDS
	case "super": return SUPER
	case "let": return LET
	case "yield": return YIELD
	case "await": return AWAIT
	case "typeof": return TYPEOF
	case "delete": return DELETETOKEN
	case "void": return VOIDTOKEN
	case "in": return INTOKEN
	case "instanceof": return INSTANCEOF
	case "enum": return RESERVED
	case "implements", "interface", "package", "private", "protected", "public", "static":
		return RESERVED_IF_STRICT
	}
	return 0
}
