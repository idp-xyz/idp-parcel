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

const customerAmendmentDraft = `{
  "accessRequestId": "SYN-ACCESS-REQUEST-0003",
  "originalAccessRequestId": " SYN-ACCESS-REQUEST-0001 ",
  "shipmentRequestId": "SHR-SYN-1",
  "basisVersionId": "SYN-VERSION-1",
  "dataGroup": "DELIVERY_PLACE",
  "intent": "CORRECTION",
  "reason": "SYN-REASON-ADDRESS",
  "postalCode": "SYN-POSTAL-9"
}`

type amendmentAuthenticatorDouble struct{ err error }

func (double amendmentAuthenticatorDouble) AuthenticateAmendment(
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
	key, _ := domain.NewSourceRequestKey("source-data-amendment:" + strings.TrimSpace(raw))
	tenant, _ := domain.NewTenantID(customerTenant)
	customer, _ := domain.NewCustomerAccountID(customerAccount)
	source, _ := domain.NewSource(customerSource)
	return domain.NewSourceIdentity(tenant, customer, source, key)
}

func intakeAmendmentOf(t *testing.T, authenticator shipmenthttp.CustomerAmendmentAuthenticator, body string) (shipmenthttp.SourceDataAmendmentIntake, *http.Request) {
	t.Helper()
	intake, err := shipmenthttp.NewCustomerAmendmentIntake(authenticator, frozenClock{at: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	return intake, httptest.NewRequest(http.MethodPost, "/v1/shipment-requests/source-data-amendments", strings.NewReader(body))
}

// Covers: AT-PS-141——资料修订 Intake 译命令：修订身份取自认证结果（`source-data-amendment:` 前缀），原提交身份是同租户、同账户、同来源加去空白的原标识；
// 请求方是信封的客户账户；内容由提交口同一个要素构造挑出；自报账户 / 租户、缺指名、意图集外、内容字段落不进范围、显式清空带内容都是 400；认证没过不读载荷。
func TestCustomerAmendmentIntakeTakesIdentitiesFromAuthenticationAndContentFromTheScope(t *testing.T) {
	intake, request := intakeAmendmentOf(t, amendmentAuthenticatorDouble{}, customerAmendmentDraft)
	command, err := intake.IntakeSourceDataAmendment(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got := command.AmendmentIdentity.RequestKey().String(); got != "source-data-amendment:SYN-ACCESS-REQUEST-0003" {
		t.Fatalf("修订来源请求键 = %q", got)
	}
	original := command.Identity
	if original.RequestKey().String() != "SYN-ACCESS-REQUEST-0001" || original.TenantID().String() != customerTenant ||
		original.CustomerAccountID().String() != customerAccount {
		t.Fatalf("原提交身份 = %+v", original)
	}
	if command.Requester.String() != "CUSTOMER_ACCOUNT:"+customerAccount || command.Intent != domain.CorrectionIntent ||
		command.Basis.OnAcceptanceBaseline() || command.Scope.ShipmentRequestID().String() != "SHR-SYN-1" {
		t.Fatalf("命令 = %+v", command)
	}
	elements, ok := command.Content.AddressElements()
	if !ok {
		t.Fatal("内容缺席")
	}
	if postal, declared := elements.PostalCode(); !declared || postal != "SYN-POSTAL-9" {
		t.Fatalf("邮编 = %q %v", postal, declared)
	}
	if !strings.HasPrefix(command.PayloadDigest.String(), domain.CustomerSourceDataAmendmentShapeVersion+":") {
		t.Fatalf("摘要 %q 不是 PAM-1", command.PayloadDigest)
	}
	if !command.EffectiveAt.IsZero() {
		t.Fatal("适用时间缺失应保持零值")
	}

	for name, mutate := range map[string]func(string) string{
		"自报账户": func(b string) string {
			return strings.Replace(b, `"reason"`, `"customerAccountId": "SYN-ACCOUNT-02", "reason"`, 1)
		},
		"自报租户": func(b string) string {
			return strings.Replace(b, `"reason"`, `"tenantId": "SYN-TENANT-02", "reason"`, 1)
		},
		"缺原提交标识": func(b string) string {
			return strings.Replace(b, `"originalAccessRequestId": " SYN-ACCESS-REQUEST-0001 ",`, "", 1)
		},
		"缺委托号": func(b string) string { return strings.Replace(b, `"shipmentRequestId": "SHR-SYN-1",`, "", 1) },
		"缺意图":  func(b string) string { return strings.Replace(b, `"intent": "CORRECTION",`, "", 1) },
		"意图集外": func(b string) string { return strings.Replace(b, "CORRECTION", "REWRITE", 1) },
		"缺原因":  func(b string) string { return strings.Replace(b, `"reason": "SYN-REASON-ADDRESS",`, "", 1) },
		"缺修订请求标识": func(b string) string {
			return strings.Replace(b, `"accessRequestId": "SYN-ACCESS-REQUEST-0003",`, "", 1)
		},
		"测量字段落在地址范围": func(b string) string {
			return strings.Replace(b, `"postalCode"`, `"declaredWeightValue": "1", "declaredWeightUnit": "KG", "postalCode"`, 1)
		},
		"显式清空带内容": func(b string) string { return strings.Replace(b, "CORRECTION", "EXPLICIT_CLEAR", 1) },
	} {
		intake, request := intakeAmendmentOf(t, amendmentAuthenticatorDouble{}, mutate(customerAmendmentDraft))
		if _, err := intake.IntakeSourceDataAmendment(t.Context(), request); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
			t.Errorf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}

	intake, request = intakeAmendmentOf(t, amendmentAuthenticatorDouble{err: shipmenthttp.ErrCustomerClientNotGranted}, "not json")
	if _, err := intake.IntakeSourceDataAmendment(t.Context(), request); !errors.Is(err, shipmenthttp.ErrCustomerClientNotGranted) {
		t.Fatalf("未授予：err = %v", err)
	}
}
