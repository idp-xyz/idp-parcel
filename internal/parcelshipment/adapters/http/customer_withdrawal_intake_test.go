package shipmenthttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

const customerWithdrawalDraft = `{
  "accessRequestId": "SYN-ACCESS-REQUEST-0003",
  "originalAccessRequestId": " SYN-ACCESS-REQUEST-0001 ",
  "reason": "SYN-REASON-1"
}`

type withdrawalAuthenticatorDouble struct{ err error }

func (double withdrawalAuthenticatorDouble) AuthenticateWithdrawal(
	_ context.Context,
	_ shipmenthttp.PresentedClientCredential,
	accessRequestID func() (string, error),
) (domain.SourceIdentity, error) {
	if double.err != nil {
		return domain.SourceIdentity{}, double.err
	}
	raw, err := accessRequestID()
	if err != nil {
		return domain.SourceIdentity{}, err
	}
	if strings.TrimSpace(raw) == "" {
		return domain.SourceIdentity{}, shipmenthttp.ErrMalformedRequest
	}
	key, _ := domain.NewSourceRequestKey("withdrawal:" + strings.TrimSpace(raw))
	tenant, _ := domain.NewTenantID(customerTenant)
	customer, _ := domain.NewCustomerAccountID(customerAccount)
	source, _ := domain.NewSource(customerSource)
	return domain.NewSourceIdentity(tenant, customer, source, key)
}

func intakeWithdrawalOf(t *testing.T, authenticator shipmenthttp.CustomerWithdrawalAuthenticator, body string) (shipmenthttp.WithdrawalIntake, *http.Request) {
	t.Helper()
	intake, err := shipmenthttp.NewCustomerWithdrawalIntake(authenticator, frozenClock{at: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	return intake, httptest.NewRequest(http.MethodPost, "/v1/shipment-requests/withdrawals", strings.NewReader(body))
}

// Covers: AT-PS-134——撤回 Intake 把载荷译成命令：撤回身份取自认证结果（键带 withdrawal: 前缀），被撤委托身份是同租户、同账户、同来源加去空白的
// 原接入请求标识，撤回者取渠道行客户端来源，摘要是 PWD-1；自报账户、租户、撤回者是未知字段整份 400，缺原提交标识或原因即 400。
func TestCustomerWithdrawalIntakeTakesIdentitiesFromAuthentication(t *testing.T) {
	intake, request := intakeWithdrawalOf(t, withdrawalAuthenticatorDouble{}, customerWithdrawalDraft)
	command, err := intake.IntakeWithdrawal(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got := command.WithdrawalIdentity.RequestKey().String(); got != "withdrawal:SYN-ACCESS-REQUEST-0003" {
		t.Fatalf("撤回来源请求键 = %q", got)
	}
	original := command.Identity
	if original.RequestKey().String() != "SYN-ACCESS-REQUEST-0001" || original.TenantID().String() != customerTenant ||
		original.CustomerAccountID().String() != customerAccount || original.Source() != command.WithdrawalIdentity.Source() {
		t.Fatalf("原提交身份 = %+v", original)
	}
	if command.Requester.String() != customerSource || command.Reason.String() != "SYN-REASON-1" {
		t.Fatalf("撤回者 %q、原因 %q", command.Requester, command.Reason)
	}
	if !strings.HasPrefix(command.PayloadDigest.String(), domain.CustomerWithdrawalShapeVersion+":") {
		t.Fatalf("摘要 %q 不是 PWD-1", command.PayloadDigest)
	}

	for name, mutate := range map[string]func(string) string{
		"自报账户": func(b string) string {
			return strings.Replace(b, `"reason"`, `"customerAccountId": "SYN-ACCOUNT-02", "reason"`, 1)
		},
		"自报租户": func(b string) string {
			return strings.Replace(b, `"reason"`, `"tenantId": "SYN-TENANT-02", "reason"`, 1)
		},
		"自报撤回者": func(b string) string { return strings.Replace(b, `"reason"`, `"requester": "SYN-X", "reason"`, 1) },
		"缺原提交标识": func(b string) string {
			return strings.Replace(b, `"originalAccessRequestId": " SYN-ACCESS-REQUEST-0001 ",`, "", 1)
		},
		"缺原因": func(b string) string { return strings.Replace(b, `"reason": "SYN-REASON-1"`, `"reason": " "`, 1) },
		"缺撤回请求标识": func(b string) string {
			return strings.Replace(b, `"accessRequestId": "SYN-ACCESS-REQUEST-0003",`, "", 1)
		},
	} {
		intake, request := intakeWithdrawalOf(t, withdrawalAuthenticatorDouble{}, mutate(customerWithdrawalDraft))
		if _, err := intake.IntakeWithdrawal(t.Context(), request); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
			t.Errorf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}

	intake, request = intakeWithdrawalOf(t, withdrawalAuthenticatorDouble{err: shipmenthttp.ErrCustomerClientNotGranted}, "not json")
	if _, err := intake.IntakeWithdrawal(t.Context(), request); !errors.Is(err, shipmenthttp.ErrCustomerClientNotGranted) {
		t.Fatalf("未授予：err = %v", err)
	}
}
