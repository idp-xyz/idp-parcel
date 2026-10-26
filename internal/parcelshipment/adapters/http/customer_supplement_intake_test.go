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

const customerSupplementDraft = `{
  "accessRequestId": "SYN-ACCESS-REQUEST-0002",
  "originalAccessRequestId": " SYN-ACCESS-REQUEST-0001 ",
  "shipmentRequestId": "SHR-SYN-1",
  "basisSubmissionVersionId": "SYN-VERSION-1",
  "customerShipmentReference": "SYN-CUSTREF-0001",
  "requestedServiceProduct": "国际小包标准",
  "recipientAddress": "合成收件地址",
  "parcels": [{"parcelId": "DPCL-SYN-1", "customerParcelReference": "SYN-PCLREF-0001", "declaredWeightValue": "1.50", "declaredWeightUnit": "KG"}]
}`

// supplementAuthenticatorDouble 替身认证那一侧：身份三格取自绑定，来源请求键 = `supplement:` 加调回去拿到的标识（真铸造器的推导）。
type supplementAuthenticatorDouble struct{ err error }

func (double supplementAuthenticatorDouble) AuthenticateSupplement(
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
	key, _ := domain.NewSourceRequestKey("supplement:" + strings.TrimSpace(raw))
	tenant, _ := domain.NewTenantID(customerTenant)
	customer, _ := domain.NewCustomerAccountID(customerAccount)
	source, _ := domain.NewSource(customerSource)
	return domain.NewSourceIdentity(tenant, customer, source, key)
}

func intakeSupplementOf(t *testing.T, authenticator shipmenthttp.CustomerSupplementAuthenticator, body string) (shipmenthttp.SupplementIntake, *http.Request) {
	t.Helper()
	intake, err := shipmenthttp.NewCustomerSupplementIntake(authenticator, frozenClock{at: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	return intake, httptest.NewRequest(http.MethodPost, "/v1/shipment-requests/supplements", strings.NewReader(body))
}

// Covers: AT-PS-130——补充 Intake 把载荷译成命令：补充身份取自认证结果（来源请求键带 supplement: 前缀），原提交身份是同租户、同客户
// 账户、同来源加载荷里去空白的原接入请求标识，基准与成员取自回执；载荷里自报账户、租户或操作者是未知字段，整份 400；缺指名即 400。
func TestCustomerSupplementIntakeTakesIdentitiesFromAuthenticationAndNamesFromTheReceipt(t *testing.T) {
	intake, request := intakeSupplementOf(t, supplementAuthenticatorDouble{}, customerSupplementDraft)
	command, err := intake.IntakeSupplement(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got := command.SupplementIdentity.RequestKey().String(); got != "supplement:SYN-ACCESS-REQUEST-0002" {
		t.Fatalf("补充来源请求键 = %q", got)
	}
	original := command.Identity
	if original.RequestKey().String() != "SYN-ACCESS-REQUEST-0001" || original.TenantID().String() != customerTenant ||
		original.CustomerAccountID().String() != customerAccount || original.Source() != command.SupplementIdentity.Source() {
		t.Fatalf("原提交身份 = %+v", original)
	}
	if command.BasisVersion.String() != "SYN-VERSION-1" || command.ShipmentRequestID.String() != "SHR-SYN-1" ||
		len(command.DeclaredParcelIDs) != 1 || command.DeclaredParcelIDs[0].String() != "DPCL-SYN-1" {
		t.Fatalf("指名取自回执：基准 %s、委托 %s、成员 %v", command.BasisVersion, command.ShipmentRequestID, command.DeclaredParcelIDs)
	}
	if !strings.HasPrefix(command.PayloadDigest.String(), domain.CustomerSupplementShapeVersion+":") {
		t.Fatalf("摘要 %q 不是 PSU-1", command.PayloadDigest)
	}

	for name, mutate := range map[string]func(string) string{
		"自报账户": func(body string) string {
			return strings.Replace(body, `"parcels"`, `"customerAccountId": "SYN-ACCOUNT-02", "parcels"`, 1)
		},
		"自报租户": func(body string) string {
			return strings.Replace(body, `"parcels"`, `"tenantId": "SYN-TENANT-02", "parcels"`, 1)
		},
		"缺原提交标识": func(body string) string {
			return strings.Replace(body, `"originalAccessRequestId": " SYN-ACCESS-REQUEST-0001 ",`, "", 1)
		},
		"缺基准": func(body string) string {
			return strings.Replace(body, `"basisSubmissionVersionId": "SYN-VERSION-1",`, "", 1)
		},
		"缺成员包裹号": func(body string) string { return strings.Replace(body, `"parcelId": "DPCL-SYN-1", `, "", 1) },
		"缺补充请求标识": func(body string) string {
			return strings.Replace(body, `"accessRequestId": "SYN-ACCESS-REQUEST-0002",`, "", 1)
		},
	} {
		intake, request := intakeSupplementOf(t, supplementAuthenticatorDouble{}, mutate(customerSupplementDraft))
		if _, err := intake.IntakeSupplement(t.Context(), request); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
			t.Errorf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}

	// 认证没过：载荷一个字节都不读。
	intake, request = intakeSupplementOf(t, supplementAuthenticatorDouble{err: shipmenthttp.ErrCustomerClientNotGranted}, "not json")
	if _, err := intake.IntakeSupplement(t.Context(), request); !errors.Is(err, shipmenthttp.ErrCustomerClientNotGranted) {
		t.Fatalf("未授予：err = %v", err)
	}
}
