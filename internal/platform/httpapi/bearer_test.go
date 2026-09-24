package httpapi_test

import (
	"net/http/httptest"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

func TestBearerTokenReadsOnlyTheBearerScheme(t *testing.T) {
	cases := map[string]string{
		"Bearer abc.def.ghi": "abc.def.ghi",
		"bearer abc":         "abc",
		"Bearer   abc  ":     "abc",
		"Basic dXNlcjpwYXNz": "",
		"Bearer":             "",
		"":                   "",
	}
	for header, want := range cases {
		request := httptest.NewRequest("POST", "/", nil)
		if header != "" {
			request.Header.Set("Authorization", header)
		}
		if got := httpapi.BearerToken(request); got != want {
			t.Fatalf("Authorization %q: token = %q, want %q", header, got, want)
		}
	}
}
