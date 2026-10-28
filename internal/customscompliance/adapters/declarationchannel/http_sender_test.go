package declarationchannel_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/adapters/declarationchannel"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 钉随产品带的 HTTP 连接器的线格式（ADR-0342 决定三）：申报包以 JSON POST，带幂等键与登了的凭据；ACCEPTED 记已收、REJECTED 记拒收
// 并带回消息号；5xx 与读不懂的答复记没拿到答复（待确认）；连不上通道确知没发出去；凭据变量没有值不发。

func pkg() application.DeclarationPackage {
	return application.DeclarationPackage{Tenant: "SYN-TENANT-01", Version: "DECLV-1", Sequence: 1, Unit: "SYN-UNIT-1",
		Procedure: "SYN-PROC-1", Members: []string{"DPCL-1"}, Dossier: "SYN-DOSSIER-1", Roles: "SYN-ROLES-1",
		Target: "SYN-CHANNEL-01", IdempotencyKey: "SYN-TENANT-01/DECLV-1/1"}
}

func channelTo(t *testing.T, endpoint, credential string) domain.DeclarationChannel {
	t.Helper()
	channel, err := domain.NewDeclarationChannel("SYN-CHANNEL-01", endpoint, credential)
	if err != nil {
		t.Fatal(err)
	}
	return channel
}

// Covers: AT-CC-454（连接器半边）
func TestTheHTTPSenderSpeaksTheProductWireFormat(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL + "/declarations"
	dead.Close()
	var seen *http.Request
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Clone(context.Background())
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch body["unit"] {
		case "REJECT":
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"outcome":"REJECTED","messageId":"MSG-R"}`))
		case "BOOM":
			w.WriteHeader(http.StatusBadGateway)
		case "REDIRECT":
			w.Header().Set("Location", deadURL)
			w.WriteHeader(http.StatusTemporaryRedirect)
		case "GARBLE":
			_, _ = w.Write([]byte(`not json`))
		default:
			_, _ = w.Write([]byte(`{"outcome":"ACCEPTED","messageId":"MSG-A"}`))
		}
	}))
	defer server.Close()
	sender := declarationchannel.NewHTTPSender(server.Client(), func(name string) string {
		if name == "SYN_BROKER_TOKEN" {
			return "syn-token"
		}
		return ""
	})
	channel := channelTo(t, server.URL+"/declarations", "SYN_BROKER_TOKEN")

	answer, err := sender.SendDeclaration(t.Context(), channel, pkg())
	if err != nil || !answer.Answered || !answer.Accepted || answer.Message != "MSG-A" {
		t.Fatalf("已收 = %+v err = %v", answer, err)
	}
	if seen.Header.Get("Idempotency-Key") != "SYN-TENANT-01/DECLV-1/1" || seen.Header.Get("Authorization") != "Bearer syn-token" ||
		seen.URL.Path != "/declarations" || body["version"] != "DECLV-1" {
		t.Fatalf("请求 = %v %v %v", seen.Header, seen.URL, body)
	}
	for unit, want := range map[string]application.ChannelAnswer{
		"REJECT": {Answered: true, Accepted: false, Message: "MSG-R"},
		"BOOM":   {Answered: false},
		// 第一跳已经收下了正文：不跟去拨不通的下一跳，不认作没发出去。
		"REDIRECT": {Answered: false},
		"GARBLE":   {Answered: false},
	} {
		request := pkg()
		request.Unit = unit
		answer, err := sender.SendDeclaration(t.Context(), channel, request)
		if err != nil || answer.Answered != want.Answered || answer.Accepted != want.Accepted || answer.Message != want.Message {
			t.Fatalf("%s = %+v err = %v", unit, answer, err)
		}
	}

	if _, err := sender.SendDeclaration(t.Context(), channelTo(t, server.URL, "SYN_MISSING"), pkg()); !errors.Is(err, application.ErrDeclarationCredentialMissing) {
		t.Fatalf("凭据缺值 err = %v", err)
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	url := closed.URL
	closed.Close()
	if _, err := sender.SendDeclaration(t.Context(), channelTo(t, url, ""), pkg()); !errors.Is(err, application.ErrDeclarationNotSent) {
		t.Fatalf("连不上 err = %v", err)
	}
}
