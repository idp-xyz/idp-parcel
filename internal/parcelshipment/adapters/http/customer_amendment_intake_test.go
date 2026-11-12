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

const customerGoodsAmendmentDraft = `{
  "accessRequestId": "SYN-ACCESS-REQUEST-0004",
  "originalAccessRequestId": "SYN-ACCESS-REQUEST-0001",
  "shipmentRequestId": "SHR-SYN-1",
  "basisVersionId": "SYN-VERSION-1",
  "dataGroup": "GOODS_DETAILS",
  "intent": "CORRECTION",
  "reason": "SYN-REASON-CUSTOMS-RETURN",
  "goods": {"contentCategory": "SALE_OF_GOODS", "lines": [
    {"descriptions": [{"lang": "en", "text": "Cotton T-shirt"}], "quantity": "48"},
    {"descriptions": [{"lang": "en", "text": "Cap"}], "quantity": "2"}]},
  "packing": [
    {"parcelId": "DPCL-SYN-1", "line": 1, "quantity": "24"}, {"parcelId": "DPCL-SYN-1", "line": 2, "quantity": "2"},
    {"parcelId": "DPCL-SYN-2", "line": 1, "quantity": "24"}]
}`

// Covers: 票 declared-goods-lines/06——资料修订口在资料组「货物明细」上收整组 goods 与按包裹号指的 packing，作用于整份委托；
// 货物明细字段落在别的资料组、只有 packing 没有 goods、显式清空带货物明细、分配数不成形都答 400，错指到出错的那一格。
func TestCustomerAmendmentIntakeTakesAWholeGoodsGroupWithItsPacking(t *testing.T) {
	intake, request := intakeAmendmentOf(t, amendmentAuthenticatorDouble{}, customerGoodsAmendmentDraft)
	command, err := intake.IntakeSourceDataAmendment(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, named := command.Scope.DeclaredParcelID(); named || command.Scope.DataGroup() != domain.GoodsDetailsDataGroup() {
		t.Fatalf("范围 = %+v, want 整份委托上的 GOODS_DETAILS", command.Scope)
	}
	goods, declared := command.Content.Goods()
	if !declared || len(goods.Lines()) != 2 || len(goods.Packing()) != 3 {
		t.Fatalf("内容：declared = %v", declared)
	}
	if !strings.HasPrefix(command.PayloadDigest.String(), domain.CustomerSourceDataAmendmentShapeVersion+":") {
		t.Fatalf("摘要 %q 不是 PAM-1", command.PayloadDigest)
	}

	for name, item := range map[string]struct {
		mutate func(string) string
		field  string
	}{
		"货物明细落在收件范围": {func(b string) string { return strings.Replace(b, "GOODS_DETAILS", "DELIVERY_PLACE", 1) }, ""},
		"只有分配没有货物": {func(b string) string {
			return b[:strings.Index(b, `  "goods"`)] + b[strings.Index(b, `  "packing"`):]
		}, "packing"},
		"显式清空带货物明细": {func(b string) string { return strings.Replace(b, "CORRECTION", "EXPLICIT_CLEAR", 1) }, ""},
		"分配数不成形": {func(b string) string {
			return strings.Replace(b, `"line": 2, "quantity": "2"`, `"line": 2, "quantity": "2 件"`, 1)
		}, "packing"},
		"分配的包裹号空着": {func(b string) string { return strings.Replace(b, `"parcelId": "DPCL-SYN-2"`, `"parcelId": " "`, 1) }, "packing[2].parcelId"},
		// 下面两格在来源保全之前就答：编排之后才答，客户改好载荷用同一个修订请求标识重发会撞来源冲突。
		"更正不带货物明细": {func(b string) string {
			return b[:strings.Index(b, ",\n  \"goods\"")] + "\n}"
		}, "goods"},
		"按包裹指货物明细": {func(b string) string {
			return strings.Replace(b, `"dataGroup": "GOODS_DETAILS",`, `"dataGroup": "GOODS_DETAILS", "parcelId": "DPCL-SYN-1",`, 1)
		}, "parcelId"},
	} {
		intake, request := intakeAmendmentOf(t, amendmentAuthenticatorDouble{}, item.mutate(customerGoodsAmendmentDraft))
		_, err := intake.IntakeSourceDataAmendment(t.Context(), request)
		if !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
			t.Errorf("%s：err = %v, want ErrMalformedRequest", name, err)
			continue
		}
		if item.field != "" && !strings.Contains(err.Error(), item.field) {
			t.Errorf("%s：err = %v, 要指到 %s", name, err, item.field)
		}
	}
}
