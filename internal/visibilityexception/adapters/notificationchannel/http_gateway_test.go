package notificationchannel_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/notificationchannel"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

func must[T any](t *testing.T, build func(string) (T, error), raw string) T {
	t.Helper()
	value, err := build(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return value
}

type directory struct {
	endpoint, credential string
	registered           bool
	asked                []string
}

func (double *directory) FindChannel(_ context.Context, tenant domain.TenantID, channel domain.NotificationChannelReference) (domain.NotificationChannelEndpoint, bool, error) {
	double.asked = append(double.asked, tenant.String()+"|"+channel.String())
	if !double.registered {
		return domain.NotificationChannelEndpoint{}, false, nil
	}
	registered, err := domain.NewNotificationChannelEndpoint(channel, double.endpoint, double.credential)
	return registered, err == nil, err
}

var decidedAt = time.Date(2026, 11, 13, 2, 0, 0, 123456000, time.UTC)

func notification(t *testing.T) *domain.CustomerNotification {
	t.Helper()
	disclosure, err := domain.DecideDisclosure(
		must(t, domain.NewEpisodeID, "SYN-EPISODE-1"), must(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-01"),
		must(t, domain.NewDisclosurePolicyReference, "SYN-exception-disclosure/v1"), domain.DiscloseToCustomer,
		must(t, domain.NewDisclosureContentReference, "SYN-content/eta-breach/v1"), decidedAt)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := domain.GenerateNotification(
		must(t, domain.NewNotificationID, "SYN-NOTIFICATION-1"), disclosure, decidedAt.Add(24*time.Hour),
		must(t, domain.NewNotificationChannelReference, "SYN-customer-webhook"),
		must(t, domain.NewDisclosurePolicyReference, "SYN-notify-policy/v1"), decidedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	return generated
}

func gateway(t *testing.T, channels *directory, getenv func(string) string) *notificationchannel.HTTPGateway {
	t.Helper()
	built, err := notificationchannel.NewHTTPGateway(channels, must(t, domain.NewTenantID, "SYN-TENANT-01"), notificationchannel.Subject{
		Parcel: must(t, domain.NewTrackedParcelReference, "SYN-PARCEL-1"), Kind: must(t, domain.NewExceptionSignalKindReference, "ETA_BREACH_RISK"),
	}, nil, getenv)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: 线格式——POST JSON 到登记的端点，正文带租户、通知、客户、包裹、信号类型、发作期、内容引用与时限；幂等键取
// 披露身份；登了凭据带 Bearer；2xx 即已收下。渠道按（钉住的租户，通知上的渠道引用）找。
func TestTheGatewayPostsTheNotificationToTheRegisteredEndpoint(t *testing.T) {
	var body map[string]string
	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		headers = request.Header.Clone()
		raw, _ := io.ReadAll(request.Body)
		_ = json.Unmarshal(raw, &body)
		response.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	channels := &directory{endpoint: server.URL + "/hooks/exceptions", credential: "SYN_HOOK_TOKEN", registered: true}

	if err := gateway(t, channels, func(name string) string {
		if name == "SYN_HOOK_TOKEN" {
			return "s3cret"
		}
		return ""
	}).SubmitToChannel(t.Context(), notification(t)); err != nil {
		t.Fatalf("提交：%v", err)
	}
	want := map[string]string{
		"tenant": "SYN-TENANT-01", "notificationId": "SYN-NOTIFICATION-1", "customer": "SYN-ACCOUNT-01", "parcel": "SYN-PARCEL-1",
		"signalKind": "ETA_BREACH_RISK", "episodeId": "SYN-EPISODE-1", "content": "SYN-content/eta-breach/v1", "deadline": "2026-11-14T02:00:00Z",
	}
	for key, value := range want {
		if body[key] != value {
			t.Errorf("正文 %s = %q，想要 %q", key, body[key], value)
		}
	}
	if len(body) != len(want) {
		t.Errorf("正文多了键：%v", body)
	}
	if got := headers.Get("Idempotency-Key"); got != "SYN-TENANT-01/SYN-EPISODE-1/SYN-ACCOUNT-01/2026-11-13T02:00:00.123456Z" {
		t.Errorf("幂等键 = %q", got)
	}
	if headers.Get("Authorization") != "Bearer s3cret" || headers.Get("Content-Type") != "application/json" {
		t.Errorf("请求头 = %v", headers)
	}
	if len(channels.asked) != 1 || channels.asked[0] != "SYN-TENANT-01|SYN-customer-webhook" {
		t.Errorf("渠道查的是 %v", channels.asked)
	}
}

// Covers: 渠道没登记、凭据变量没有值各交回自己的格，且一个字节不发；非 2xx、重定向、连不上都算这一次没提交成，重定向
// 不跟。
func TestTheGatewayNamesUnconfiguredChannelsAndTreatsEverythingButSuccessAsNotSubmitted(t *testing.T) {
	hits := 0
	status := http.StatusInternalServerError
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		hits++
		if status == http.StatusFound {
			response.Header().Set("Location", "/elsewhere")
		}
		response.WriteHeader(status)
	}))
	defer server.Close()
	noEnv := func(string) string { return "" }

	if err := gateway(t, &directory{}, noEnv).SubmitToChannel(t.Context(), notification(t)); !errors.Is(err, ports.ErrNotificationChannelNotConfigured) {
		t.Fatalf("渠道没登记：%v", err)
	}
	withCredential := &directory{endpoint: server.URL, credential: "SYN_HOOK_TOKEN", registered: true}
	if err := gateway(t, withCredential, noEnv).SubmitToChannel(t.Context(), notification(t)); !errors.Is(err, ports.ErrNotificationChannelCredentialMissing) {
		t.Fatalf("凭据变量没有值：%v", err)
	}
	if hits != 0 {
		t.Fatalf("没配好的渠道发出了 %d 次请求", hits)
	}

	open := &directory{endpoint: server.URL, registered: true}
	for _, status = range []int{http.StatusInternalServerError, http.StatusBadRequest, http.StatusFound} {
		err := gateway(t, open, noEnv).SubmitToChannel(t.Context(), notification(t))
		if err == nil || errors.Is(err, ports.ErrNotificationChannelNotConfigured) || errors.Is(err, ports.ErrNotificationChannelCredentialMissing) {
			t.Fatalf("HTTP %d：err=%v，想要一次普通的提交失败", status, err)
		}
	}
	if hits != 3 {
		t.Fatalf("请求 %d 次，想要 3：重定向不跟", hits)
	}
	server.Close()
	if err := gateway(t, open, noEnv).SubmitToChannel(t.Context(), notification(t)); err == nil {
		t.Fatal("连不上的端点被当成了已提交")
	}
}
