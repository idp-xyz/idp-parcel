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

const customerCancellationDraft = `{
  "accessRequestId": "SYN-ACCESS-REQUEST-0005",
  "originalAccessRequestId": " SYN-ACCESS-REQUEST-0001 ",
  "shipmentRequestId": "SHR-1",
  "parcelId": "SYN-PARCEL-1",
  "reason": "SYN-REASON-1",
  "requestedAt": "2026-09-02T19:00:00+08:00"
}`

type cancellationAuthenticatorDouble struct{ err error }

func (double cancellationAuthenticatorDouble) AuthenticateCancellation(
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
	key, _ := domain.NewSourceRequestKey("parcel-cancellation:" + strings.TrimSpace(raw))
	tenant, _ := domain.NewTenantID(customerTenant)
	customer, _ := domain.NewCustomerAccountID(customerAccount)
	source, _ := domain.NewSource(customerSource)
	return domain.NewSourceIdentity(tenant, customer, source, key)
}

func intakeCancellationOf(t *testing.T, authenticator shipmenthttp.CustomerCancellationAuthenticator, body string) (shipmenthttp.CancellationIntake, *http.Request) {
	t.Helper()
	intake, err := shipmenthttp.NewCustomerCancellationIntake(authenticator, frozenClock{at: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	return intake, httptest.NewRequest(http.MethodPost, "/v1/shipment-requests/parcel-cancellations", strings.NewReader(body))
}

// Covers: AT-PS-144——取消 Intake 把载荷译成命令：目标委托身份是同租户、同账户、同来源加去空白的原接入请求标识；命令带信封账户；
// 取消者是客户账户本人；请求时刻取载荷（UTC）；自报账户、租户、来源、取消者是未知字段整份 400；缺原提交标识、委托号、包裹、原因、请求时刻
// 或请求时刻是明显的未来即 400；未授予的错原样过。
func TestCustomerCancellationIntakeTakesIdentitiesFromAuthentication(t *testing.T) {
	intake, request := intakeCancellationOf(t, cancellationAuthenticatorDouble{}, customerCancellationDraft)
	command, err := intake.IntakeCancellation(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	target := command.Identity
	if target.RequestKey().String() != "SYN-ACCESS-REQUEST-0001" || target.TenantID().String() != customerTenant ||
		target.CustomerAccountID().String() != customerAccount || target.Source().String() != customerSource {
		t.Fatalf("目标委托身份 = %+v", target)
	}
	if command.CustomerAccount.String() != customerAccount || command.ShipmentRequestID.String() != "SHR-1" ||
		command.Parcel.String() != "SYN-PARCEL-1" || command.Reason.String() != "SYN-REASON-1" {
		t.Fatalf("命令 = %+v", command)
	}
	if !command.Requester.IsCustomerAccount(command.CustomerAccount) {
		t.Fatalf("取消者 %q 不是客户账户本人", command.Requester)
	}
	if want := time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC); !command.RequestedAt.Equal(want) || command.RequestedAt.Location() != time.UTC {
		t.Fatalf("请求时刻 = %v, want %v", command.RequestedAt, want)
	}

	for name, mutate := range map[string]func(string) string{
		"自报账户": func(b string) string {
			return strings.Replace(b, `"reason"`, `"customerAccountId": "SYN-ACCOUNT-02", "reason"`, 1)
		},
		"自报租户": func(b string) string {
			return strings.Replace(b, `"reason"`, `"tenantId": "SYN-TENANT-02", "reason"`, 1)
		},
		"自报来源":  func(b string) string { return strings.Replace(b, `"reason"`, `"source": "SYN-X", "reason"`, 1) },
		"自报取消者": func(b string) string { return strings.Replace(b, `"reason"`, `"requester": "SYN-X", "reason"`, 1) },
		"缺原提交标识": func(b string) string {
			return strings.Replace(b, `"originalAccessRequestId": " SYN-ACCESS-REQUEST-0001 ",`, "", 1)
		},
		"缺委托号": func(b string) string { return strings.Replace(b, `"shipmentRequestId": "SHR-1",`, "", 1) },
		"缺包裹":  func(b string) string { return strings.Replace(b, `"parcelId": "SYN-PARCEL-1",`, "", 1) },
		"缺原因":  func(b string) string { return strings.Replace(b, `"reason": "SYN-REASON-1"`, `"reason": " "`, 1) },
		"缺请求时刻": func(b string) string {
			return strings.Replace(b, `,
  "requestedAt": "2026-09-02T19:00:00+08:00"`, "", 1)
		},
		"请求时刻不是 RFC3339": func(b string) string { return strings.Replace(b, "2026-09-02T19:00:00+08:00", "yesterday", 1) },
		"请求时刻在未来": func(b string) string {
			return strings.Replace(b, "2026-09-02T19:00:00+08:00", "2026-09-02T21:00:00+08:00", 1)
		},
		"缺取消请求标识": func(b string) string {
			return strings.Replace(b, `"accessRequestId": "SYN-ACCESS-REQUEST-0005",`, "", 1)
		},
	} {
		intake, request := intakeCancellationOf(t, cancellationAuthenticatorDouble{}, mutate(customerCancellationDraft))
		if _, err := intake.IntakeCancellation(t.Context(), request); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
			t.Errorf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}

	intake, request = intakeCancellationOf(t, cancellationAuthenticatorDouble{err: shipmenthttp.ErrCustomerClientNotGranted}, "not json")
	if _, err := intake.IntakeCancellation(t.Context(), request); !errors.Is(err, shipmenthttp.ErrCustomerClientNotGranted) {
		t.Fatalf("未授予：err = %v", err)
	}
}
