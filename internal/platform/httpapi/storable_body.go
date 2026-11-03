package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"
)

// ErrUnstorableBody 是 StorableJSONBody 读到坏处时交回的读错：请求体里有库当文本存不下、或解码器会悄悄改写的东西。各口照
// 「请求体读不出」那一格答坏请求，重发同样的内容不会好（ADR-0029 按恢复动作分格）。
var ErrUnstorableBody = errors.New("request body is not storable text")

var (
	errBodyNotUTF8       = fmt.Errorf("%w: its bytes are not valid UTF-8", ErrUnstorableBody)
	errBodyNUL           = fmt.Errorf("%w: it carries a NUL character", ErrUnstorableBody)
	errBodyLoneSurrogate = fmt.Errorf("%w: a \\u escape leaves an unpaired surrogate", ErrUnstorableBody)
)

// StorableJSONBody 包住一份 JSON 请求体，边读边判：原始字节处处 StorableText，字符串里每个 \u 转义解出来也是。三种坏处：
//
//   - NUL，原始字节或 \u0000 转义：PostgreSQL 的 text 与 jsonb 都存不下，参数带着它进写，库拒收，答成可重试的 500。
//   - 原始字节不是合法 UTF-8：encoding/json 把串里的坏字节换成 U+FFFD 照常交出，落库的已不是调用方送来的值。
//   - \u 转义出孤代理（高位后面不跟低位，或低位单独出现）：encoding/json 同样换成 U+FFFD。
//
// 后两种不替换、只拒：悄悄改写调用方的值，比拒收更糟。
//
// 读到坏处，这一次 Read 交回 0 字节与 ErrUnstorableBody，此后每次都是它。坏处所在的那一段一个字节也不交出去：串里的坏处
// 判到时那个串还没收尾（孤高位要看到下一个字符才判得了，那个字符也压在这一段里），解码器凑不成含它的那个值，只能报错，不会
// 带着已换成 U+FFFD 的前半截往下走；串外的坏字节本就是语法错。交出 n 字节再报错不行：解码器若在这 n 字节里已凑齐一个值，
// 它会先交出那个值、不看这个错。
//
// 判的是整份请求体，不只是各口解的那几格：口不解的格带着 NUL 一样拒，这份请求体本就不是文本，与 StorableRequestTarget 判
// 整条目标同理。只有一种读法看不见坏处：不查尾随内容的口解完头一个值就不再读，坏处若在其后、又不与那个值同在一次 Read 里，
// 读不到也就不判；那一段不进命令，也不进库。
//
// 不判 JSON 语法：转义写坏、串没收尾、尾随内容，都归各口的解码器自己报。
func StorableJSONBody(body io.ReadCloser) io.ReadCloser {
	return &storableJSONBody{body: body}
}

// RequireStorableJSONBody 包住一个写口：请求体换成 StorableJSONBody，再交处理器。它自己不答：请求体由处理器在认证之后读，
// 读到坏处时拿到的是一个读错，答复落在该口「请求体读不出」那一格——形状归各族，认证先于请求体的次序也不动。
func RequireStorableJSONBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Body != nil && request.Body != http.NoBody {
			request.Body = StorableJSONBody(request.Body)
		}
		next.ServeHTTP(response, request)
	})
}

type storableJSONBody struct {
	body io.ReadCloser
	scan jsonTextScan
	err  error
}

func (judged *storableJSONBody) Read(p []byte) (int, error) {
	if judged.err != nil {
		return 0, judged.err
	}
	n, err := judged.body.Read(p)
	problem := judged.scan.feed(p[:n])
	if problem == nil && err == io.EOF {
		problem = judged.scan.end()
	}
	if problem != nil {
		judged.err = problem
		return 0, problem
	}
	return n, err
}

func (judged *storableJSONBody) Close() error {
	return judged.body.Close()
}

// jsonScanState 只跟字符串与 \u 转义：判 NUL 与孤代理要知道一个字节在不在串里、是不是转义的一部分，别的语法不归本判。
// 跟住它，`"\\u0000"`（转义的反斜杠后面跟着字面的 u0000）才不会被误当成 NUL。
type jsonScanState uint8

const (
	outsideString jsonScanState = iota
	inString
	afterBackslash
	inUnicodeEscape
	// afterHighSurrogate：刚读完一个高位代理的 \u 转义，下一个字符必须是开始低位的那个反斜杠。
	afterHighSurrogate
	afterHighSurrogateBackslash
	inLowSurrogateEscape
)

type jsonTextScan struct {
	state jsonScanState
	// digits 与 unit：当前 \u 转义已收的十六进制位数与已拼出的码元。
	digits int
	unit   rune
	// carried 与 held：上一段末尾没收全的多字节字符，等下一段补齐再判。
	carried [utf8.UTFMax]byte
	held    int
}

func (scan *jsonTextScan) feed(chunk []byte) error {
	for len(chunk) > 0 {
		if scan.held > 0 {
			for scan.held < utf8.UTFMax && len(chunk) > 0 && !utf8.FullRune(scan.carried[:scan.held]) {
				scan.carried[scan.held] = chunk[0]
				scan.held++
				chunk = chunk[1:]
			}
			if !utf8.FullRune(scan.carried[:scan.held]) {
				return nil
			}
			char, size := utf8.DecodeRune(scan.carried[:scan.held])
			if char == utf8.RuneError && size == 1 {
				return errBodyNotUTF8
			}
			scan.held = 0
			if err := scan.step(char); err != nil {
				return err
			}
			continue
		}
		if chunk[0] < utf8.RuneSelf {
			if err := scan.step(rune(chunk[0])); err != nil {
				return err
			}
			chunk = chunk[1:]
			continue
		}
		if !utf8.FullRune(chunk) {
			scan.held = copy(scan.carried[:], chunk)
			return nil
		}
		char, size := utf8.DecodeRune(chunk)
		if char == utf8.RuneError && size == 1 {
			return errBodyNotUTF8
		}
		if err := scan.step(char); err != nil {
			return err
		}
		chunk = chunk[size:]
	}
	return nil
}

// end 在请求体读完时判：末尾还压着没收全的多字节字符，就是截断的 UTF-8。
func (scan *jsonTextScan) end() error {
	if scan.held > 0 {
		return errBodyNotUTF8
	}
	return nil
}

func (scan *jsonTextScan) step(char rune) error {
	if char == 0 {
		return errBodyNUL
	}
	switch scan.state {
	case outsideString:
		if char == '"' {
			scan.state = inString
		}
	case inString:
		switch char {
		case '\\':
			scan.state = afterBackslash
		case '"':
			scan.state = outsideString
		}
	case afterBackslash:
		if char == 'u' {
			scan.beginEscape(inUnicodeEscape)
		} else {
			scan.state = inString
		}
	case inUnicodeEscape, inLowSurrogateEscape:
		digit, ok := hexDigit(char)
		if !ok {
			// 转义写坏了，是解码器的语法错。回到串里接着跟，这个字符照串里的字符走。
			scan.state = inString
			return scan.step(char)
		}
		scan.unit = scan.unit<<4 | digit
		scan.digits++
		if scan.digits == 4 {
			return scan.escaped()
		}
	case afterHighSurrogate:
		if char != '\\' {
			return errBodyLoneSurrogate
		}
		scan.state = afterHighSurrogateBackslash
	case afterHighSurrogateBackslash:
		if char != 'u' {
			return errBodyLoneSurrogate
		}
		scan.beginEscape(inLowSurrogateEscape)
	}
	return nil
}

func (scan *jsonTextScan) beginEscape(state jsonScanState) {
	scan.state, scan.digits, scan.unit = state, 0, 0
}

// escaped 判一个收全了的 \u 转义。
func (scan *jsonTextScan) escaped() error {
	isHigh := scan.unit >= 0xD800 && scan.unit <= 0xDBFF
	isLow := scan.unit >= 0xDC00 && scan.unit <= 0xDFFF
	switch {
	case scan.state == inLowSurrogateEscape && !isLow:
		return errBodyLoneSurrogate
	case scan.state == inLowSurrogateEscape:
	case scan.unit == 0:
		return errBodyNUL
	case isHigh:
		scan.state = afterHighSurrogate
		return nil
	case isLow:
		return errBodyLoneSurrogate
	}
	scan.state = inString
	return nil
}

func hexDigit(char rune) (rune, bool) {
	switch {
	case char >= '0' && char <= '9':
		return char - '0', true
	case char >= 'a' && char <= 'f':
		return char - 'a' + 10, true
	case char >= 'A' && char <= 'F':
		return char - 'A' + 10, true
	}
	return 0, false
}
