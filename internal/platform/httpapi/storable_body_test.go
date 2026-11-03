package httpapi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// storableBodies 是照收的请求体：合法多字节（中文、全角、emoji）原样或写成 \u 转义都收，代理对收；`\\u0000` 是转义的反斜杠
// 后面跟着字面的 u0000，不是 NUL；`\u0001` 与显式写出的 U+FFFD 库存得下，不是本判拒的东西。语法坏的也照交：那归解码器报。
var storableBodies = map[string]string{
	"空":         ``,
	"ASCII":     `{"partyId":"SYN-PARTY-01","revision":1,"active":true,"basis":null}`,
	"中文与全角":     `{"name":"SYN 合成参与方","code":"ＳＹＮ－０１"}`,
	"emoji 原样":  `{"name":"📦 SYN 运单"}`,
	"代理对转义":     `{"name":"\ud83d\udce6","upper":"\uD83D\uDCE6"}`,
	"BMP 转义":    `{"name":"\u5305\u88f9","e":"\u00e9"}`,
	"转义的反斜杠":    `{"path":"C:\\u0000","twice":"\\\\u0000"}`,
	"控制字符转义":    `{"tab":"\t","one":"\u0001"}`,
	"显式 U+FFFD": `{"name":"\ufffd"}`,
	"键里的转义":     `{"\u5305":"x"}`,
	"语法坏的":      `{"name":"SYN`,
	"转义写坏的":     `{"name":"\u00zz"}`,
}

// unstorableBodies 是拒收的请求体：串里的、串外的、跨值的，原始字节的与转义出来的。
var unstorableBodies = map[string]string{
	"NUL 转义":          `{"name":"SYN\u0000X"}`,
	"整串只有 NUL 转义":     `{"name":"\u0000"}`,
	"键里的 NUL 转义":      `{"\u0000":"x"}`,
	"反斜杠后的 NUL 转义":    `{"path":"C:\\\u0000"}`,
	"原始 NUL 字节":       "{\"name\":\"SYN\x00X\"}",
	"孤立 FF":           "{\"name\":\"SYN\xffX\"}",
	"超长编码":            "{\"name\":\"SYN\xc0\xafX\"}",
	"代理区码点原样":         "{\"name\":\"SYN\xed\xa0\x80X\"}",
	"串外的坏字节":          "{\"name\":\"SYN\"}\xff",
	"末尾截断的多字节":        "{\"name\":\"包\xe4\xb8",
	"孤高位后收串":          `{"name":"\ud800"}`,
	"孤高位后跟字符":         `{"name":"\uD800X"}`,
	"孤高位后跟别的转义":       `{"name":"\ud800\n"}`,
	"孤高位后跟非低位":        `{"name":"\ud800\u0041"}`,
	"两个高位":            `{"name":"\ud83d\ud83d"}`,
	"孤低位":             `{"name":"\udce6"}`,
	"孤高位后跟 emoji 原样":  `{"name":"\ud83d📦"}`,
	"代理对之后的 NUL 转义":   `{"name":"\ud83d\udce6\u0000"}`,
	"串内合法之后的孤低位":      `{"name":"包裹\uDFFF"}`,
	"第二个值里的 NUL 转义":   `{"a":"x"} {"b":"\u0000"}`,
	"数组里的孤代理":         `["SYN","\udbff"]`,
	"FF 在 emoji 之后":   "{\"name\":\"📦\xff\"}",
	"emoji 截了尾":       "{\"name\":\"\xf0\x9f\x93\"}",
	"四字节编码超出 Unicode": "{\"name\":\"\xf4\x90\x80\x80\"}",
}

func readStorable(t *testing.T, reader io.Reader) ([]byte, error) {
	t.Helper()
	return io.ReadAll(httpapi.StorableJSONBody(io.NopCloser(reader)))
}

// Covers: 票 document-numbering/16——请求体边读边判：NUL（原始字节或 \u0000 转义）、非法 UTF-8、\u 转义出的孤代理拒，报的错是
// ErrUnstorableBody；合法多字节、代理对、转义的反斜杠照收，交出的字节与原文逐字相同。一次读完、逐字节读、每次读一半，结论都
// 一样：坏处跨了两次 Read 也判得到。
func TestStorableJSONBodyRefusesOnlyWhatTheStoreCannotHoldOrTheDecoderWouldRewrite(t *testing.T) {
	readers := map[string]func(string) io.Reader{
		"一次读完": func(body string) io.Reader { return strings.NewReader(body) },
		"逐字节读": func(body string) io.Reader { return iotest.OneByteReader(strings.NewReader(body)) },
		"每次一半": func(body string) io.Reader { return iotest.HalfReader(strings.NewReader(body)) },
	}
	for readerName, reader := range readers {
		for name, body := range storableBodies {
			got, err := readStorable(t, reader(body))
			if err != nil || string(got) != body {
				t.Errorf("%s %s：交出 %q、err %v，want 原文照交", readerName, name, got, err)
			}
		}
		for name, body := range unstorableBodies {
			if _, err := readStorable(t, reader(body)); !errors.Is(err, httpapi.ErrUnstorableBody) {
				t.Errorf("%s %s：err %v，want ErrUnstorableBody", readerName, name, err)
			}
		}
	}
}

// nulOnly 是 unstorableBodies 里坏处只有 NUL 的那几份。
var nulOnly = map[string]bool{
	"NUL 转义": true, "整串只有 NUL 转义": true, "键里的 NUL 转义": true, "反斜杠后的 NUL 转义": true,
	"原始 NUL 字节": true, "代理对之后的 NUL 转义": true, "第二个值里的 NUL 转义": true,
}

// Covers: 票 document-numbering/16——UnrewrittenJSONBody 只放过 NUL：坏处只有 NUL 的请求体原文照交，留给处理器自己的构造门；
// 非法 UTF-8 与孤代理照拒。照收那一张表照收。逐字节读与一次读完结论一样。
func TestUnrewrittenJSONBodyLetsOnlyNULThrough(t *testing.T) {
	for name := range nulOnly {
		if _, listed := unstorableBodies[name]; !listed {
			t.Fatalf("nulOnly 点名的 %q 不在 unstorableBodies 里", name)
		}
	}
	for readerName, reader := range map[string]func(string) io.Reader{
		"一次读完": func(body string) io.Reader { return strings.NewReader(body) },
		"逐字节读": func(body string) io.Reader { return iotest.OneByteReader(strings.NewReader(body)) },
	} {
		read := func(body string) ([]byte, error) {
			return io.ReadAll(httpapi.UnrewrittenJSONBody(io.NopCloser(reader(body))))
		}
		for name, body := range storableBodies {
			if got, err := read(body); err != nil || string(got) != body {
				t.Errorf("%s %s：交出 %q、err %v，want 原文照交", readerName, name, got, err)
			}
		}
		for name, body := range unstorableBodies {
			got, err := read(body)
			if nulOnly[name] {
				if err != nil || string(got) != body {
					t.Errorf("%s %s：交出 %q、err %v，want NUL 照交", readerName, name, got, err)
				}
				continue
			}
			if !errors.Is(err, httpapi.ErrUnstorableBody) {
				t.Errorf("%s %s：err %v，want ErrUnstorableBody", readerName, name, err)
			}
		}
	}
}

// Covers: 票 document-numbering/16——坏处所在的那一段一个字节也不交：整份请求体在一次 Read 里到齐、含坏处的值在这一段里已经
// 收尾，解码器也拿不到它，Decode 报的是 ErrUnstorableBody，不会带着 U+FFFD 解出一个值。此后每次 Read 都是同一个错。
func TestADecoderNeverSeesTheChunkCarryingTheProblem(t *testing.T) {
	for name, body := range map[string]string{
		"NUL 转义": `{"name":"SYN\u0000X"}`,
		"孤立 FF":  "{\"name\":\"SYN\xffX\"}",
		"孤高位":    `{"name":"\ud800"}`,
	} {
		judged := httpapi.StorableJSONBody(io.NopCloser(strings.NewReader(body)))
		var decoded struct{ Name string }
		if err := json.NewDecoder(judged).Decode(&decoded); !errors.Is(err, httpapi.ErrUnstorableBody) {
			t.Fatalf("%s：Decode err %v、解出 %q，want ErrUnstorableBody", name, err, decoded.Name)
		}
		if n, err := judged.Read(make([]byte, 64)); n != 0 || !errors.Is(err, httpapi.ErrUnstorableBody) {
			t.Fatalf("%s：再读答 %d、%v，want 0 与同一个错", name, n, err)
		}
	}
}

// countingBody 记下被读了几次、关没关。
type countingBody struct {
	reader io.Reader
	reads  int
	closed bool
}

func (body *countingBody) Read(p []byte) (int, error) {
	body.reads++
	return body.reader.Read(p)
}

func (body *countingBody) Close() error {
	body.closed = true
	return nil
}

// Covers: 票 document-numbering/16——RequireStorableJSONBody 自己不答、不先读：处理器不读请求体（例如令牌不过先答了），请求体
// 一个字节都不读；处理器读，读到坏处拿到的是 ErrUnstorableBody，答复由处理器写；合法请求体原样到处理器手上。Close 交回原请求体。
func TestRequireStorableJSONBodyOnlySwapsTheBody(t *testing.T) {
	var seen []byte
	var seenErr error
	reading := httpapi.RequireStorableJSONBody(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		seen, seenErr = io.ReadAll(request.Body)
		_ = request.Body.Close()
		if seenErr != nil {
			http.Error(response, "unreadable", http.StatusBadRequest)
			return
		}
		response.WriteHeader(http.StatusCreated)
	}))
	refusing := httpapi.RequireStorableJSONBody(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusUnauthorized)
	}))

	unread := &countingBody{reader: strings.NewReader(`{"name":"\u0000"}`)}
	response := httptest.NewRecorder()
	refusing.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/things", unread))
	if response.Code != http.StatusUnauthorized || unread.reads != 0 {
		t.Fatalf("处理器不读：答 %d、请求体读了 %d 次，want 401、一次没读", response.Code, unread.reads)
	}

	bad := &countingBody{reader: strings.NewReader(`{"name":"\u0000"}`)}
	response = httptest.NewRecorder()
	reading.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/things", bad))
	if response.Code != http.StatusBadRequest || !errors.Is(seenErr, httpapi.ErrUnstorableBody) || !bad.closed {
		t.Fatalf("坏请求体：答 %d、处理器读到 %v、关了 %v，want 400、ErrUnstorableBody、关了", response.Code, seenErr, bad.closed)
	}

	good := `{"name":"包裹📦\ud83d\udce6"}`
	response = httptest.NewRecorder()
	reading.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/things", strings.NewReader(good)))
	if response.Code != http.StatusCreated || seenErr != nil || !bytes.Equal(seen, []byte(good)) {
		t.Fatalf("合法请求体：答 %d、处理器读到 %q（%v），want 201、原文", response.Code, seen, seenErr)
	}

	for name, body := range map[string]io.Reader{"无请求体": nil, "NoBody": http.NoBody} {
		request := httptest.NewRequest(http.MethodPost, "/things", body)
		if body == http.NoBody {
			request.Body = http.NoBody
		}
		response = httptest.NewRecorder()
		reading.ServeHTTP(response, request)
		if response.Code != http.StatusCreated || len(seen) != 0 {
			t.Fatalf("%s：答 %d、读到 %q，want 201、空", name, response.Code, seen)
		}
	}
}
