package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// Covers: 票 document-numbering/15——判据只拦库存不下的值：NUL、非法 UTF-8（孤立高位字节、超长编码、代理区码点）；中文、全角、
// emoji 与空串照收，不是字符白名单。
func TestStorableTextRefusesOnlyWhatTheDatabaseCannotStore(t *testing.T) {
	for value, storable := range map[string]bool{
		"":                 true,
		"SYN-PARCEL-01":    true,
		"包裹一号":             true,
		"ＳＹＮ－０１":           true,
		"📦 SYN 运单":         true,
		"\t换行\n与回车\r":      true,
		"\x00":             false,
		"SYN\x00X":         false,
		"SYN\xffX":         false,
		"SYN\xc0\xafX":     false,
		"SYN\xed\xa0\x80X": false,
		"包裹\xe4\xb8":       false,
	} {
		if got := httpapi.StorableText(value); got != storable {
			t.Errorf("StorableText(%q) = %v, want %v", value, got, storable)
		}
	}
}

// Covers: 票 document-numbering/15——整条请求目标解码后判：查询值、查询键、路径参数段都算；合法的多字节百分号转义照收；转义坏掉
// 的那一段 ParseQuery 丢下不交，这里也不判，留给各口自己的解码。
func TestStorableRequestTargetJudgesTheDecodedPathKeysAndValues(t *testing.T) {
	for target, storable := range map[string]bool{
		"/things":                                    true,
		"/things?parcel=SYN-01&version=v1":           true,
		"/things?q=%E4%B8%AD%E6%96%87":               true,
		"/things?q=%EF%BC%B3%EF%BC%B9%EF%BC%AE":      true,
		"/things?q=%F0%9F%93%A6":                     true,
		"/things/%E5%8F%82%E4%B8%8E%E6%96%B9/revs":   true,
		"/things?q=a+b":                              true,
		"/things?bad=%ZZ&q=ok":                       true,
		"/things?parcel=SYN%00X":                     false,
		"/things?parcel=SYN%FFX":                     false,
		"/things?parcel=SYN%C0%AFX":                  false,
		"/things?parcel=SYN%ED%A0%80X":               false,
		"/things?ok=1&parcel=SYN-01&parcel=SYN%00":   false,
		"/things?SYN%00=1":                           false,
		"/things?%FF=1":                              false,
		"/things/SYN%00X/revs":                       false,
		"/things/SYN%FFX/revs":                       false,
		"/things/%E5%8F%82%E4%B8%8E%E6%96%B9%C0/rev": false,
	} {
		if got := httpapi.StorableRequestTarget(httptest.NewRequest(http.MethodGet, target, nil)); got != storable {
			t.Errorf("StorableRequestTarget(%s) = %v, want %v", target, got, storable)
		}
	}
}

// Covers: 票 document-numbering/15——GET 的坏目标交给装配处给的那一种坏请求，处理器一步不走；好目标照常进处理器；别的方法哪怕目标
// 坏也原样交给处理器，405 仍由它自己答。
func TestRefuseUnstorableReadTargetStopsBadReadsBeforeTheHandler(t *testing.T) {
	var reached, refused int
	guarded := httpapi.RefuseUnstorableReadTarget(
		http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			reached++
			response.WriteHeader(http.StatusOK)
		}),
		func(response http.ResponseWriter, _ *http.Request) {
			refused++
			response.WriteHeader(http.StatusBadRequest)
		},
	)
	serve := func(method, target string) int {
		response := httptest.NewRecorder()
		guarded.ServeHTTP(response, httptest.NewRequest(method, target, nil))
		return response.Code
	}

	for _, target := range []string{"/things?parcel=SYN%00X", "/things?parcel=SYN%FFX", "/things/SYN%00X"} {
		if code := serve(http.MethodGet, target); code != http.StatusBadRequest {
			t.Fatalf("GET %s = %d, want 400", target, code)
		}
	}
	if reached != 0 || refused != 3 {
		t.Fatalf("坏目标：处理器走了 %d 次、拒了 %d 次，want 0 与 3", reached, refused)
	}

	if code := serve(http.MethodGet, "/things?parcel=%E5%8C%85%E8%A3%B9%F0%9F%93%A6"); code != http.StatusOK || reached != 1 {
		t.Fatalf("合法多字节：答 %d、处理器走了 %d 次", code, reached)
	}
	if code := serve(http.MethodPost, "/things?parcel=SYN%00X"); code != http.StatusOK || reached != 2 || refused != 3 {
		t.Fatalf("POST 坏目标：答 %d、处理器走了 %d 次、拒了 %d 次；方法门归处理器", code, reached, refused)
	}
}
