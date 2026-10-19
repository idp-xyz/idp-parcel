package registrationjson_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件证 ADR-0190 两本租户取值册的译装：正——每一格原样到命令，时点按封闭词表译回领域常量；反——形状坏、未知字段、
// 词表外的时点、缺格，一律在入库前拒并指名差哪格。不补默认：没写时点就是没写，不默认成任何一种。

const sellBillingMomentAdoptionDocument = `{
	"tenantId": "SYN-T1",
	"accountId": "SYN-ACCT-1",
	"billingMoment": "EFFECTIVE_DELIVERY_FINALIZED",
	"pricingScopeId": "SYN-SCOPE-SELL-1"
}`

func TestSellBillingMomentAdoptionFromJSONCarriesEveryCell(t *testing.T) {
	command, err := registrationjson.SellBillingMomentAdoptionFromJSON([]byte(sellBillingMomentAdoptionDocument))
	if err != nil {
		t.Fatalf("译装被拒：%v", err)
	}
	adoption := command.Adoption
	if command.Tenant.String() != "SYN-T1" || adoption.Account().String() != "SYN-ACCT-1" ||
		adoption.Moment() != domain.BillingMomentEffectiveDeliveryFinalized || adoption.PricingScope().String() != "SYN-SCOPE-SELL-1" {
		t.Fatalf("command = %+v", command)
	}
}

func TestSellBillingMomentAdoptionFromJSONRejectsBadShapeVocabularyAndBlanks(t *testing.T) {
	cases := map[string]struct {
		document string
		field    string
	}{
		"未知字段":       {strings.Replace(sellBillingMomentAdoptionDocument, `"tenantId"`, `"typo": 1, "tenantId"`, 1), "形状"},
		"词表外的时点":     {strings.Replace(sellBillingMomentAdoptionDocument, "EFFECTIVE_DELIVERY_FINALIZED", "COMMISSION_ACCEPTED", 1), "billingMoment"},
		"没给时点":       {strings.Replace(sellBillingMomentAdoptionDocument, `"billingMoment": "EFFECTIVE_DELIVERY_FINALIZED",`, "", 1), "billingMoment"},
		"空白账户":       {strings.Replace(sellBillingMomentAdoptionDocument, "SYN-ACCT-1", " ", 1), "accountId"},
		"空白计价范围":     {strings.Replace(sellBillingMomentAdoptionDocument, "SYN-SCOPE-SELL-1", "", 1), "pricingScopeId"},
		"空白租户":       {strings.Replace(sellBillingMomentAdoptionDocument, "SYN-T1", "", 1), "tenantId"},
		"不是 JSON 对象": {`[]`, "形状"},
	}
	for name, spec := range cases {
		_, err := registrationjson.SellBillingMomentAdoptionFromJSON([]byte(spec.document))
		if err == nil {
			t.Fatalf("%s：译装没拒", name)
		}
		if !strings.Contains(err.Error(), spec.field) {
			t.Fatalf("%s：错误 %q 没指名 %s", name, err, spec.field)
		}
	}
}

const feeItemMappingDocument = `{
	"tenantId": "SYN-T1",
	"pricingChargeCode": "BASE_FREIGHT",
	"feeItem": "SYN-FEE-FREIGHT",
	"basisRef": "SYN-CLAUSE-1"
}`

func TestFeeItemMappingFromJSONCarriesEveryCell(t *testing.T) {
	tenant, mapping, err := registrationjson.FeeItemMappingFromJSON([]byte(feeItemMappingDocument))
	if err != nil {
		t.Fatalf("译装被拒：%v", err)
	}
	if tenant.String() != "SYN-T1" || mapping.ChargeCode().String() != "BASE_FREIGHT" ||
		mapping.FeeItem().String() != "SYN-FEE-FREIGHT" || mapping.Basis().String() != "SYN-CLAUSE-1" {
		t.Fatalf("tenant = %s mapping = %+v", tenant, mapping)
	}
}

func TestFeeItemMappingFromJSONRejectsBadShapeAndBlanks(t *testing.T) {
	cases := map[string]struct {
		document string
		field    string
	}{
		"未知字段":   {strings.Replace(feeItemMappingDocument, `"tenantId"`, `"chargeCode": "X", "tenantId"`, 1), "形状"},
		"空白代码":   {strings.Replace(feeItemMappingDocument, "BASE_FREIGHT", " ", 1), "pricingChargeCode"},
		"空白费用项目": {strings.Replace(feeItemMappingDocument, "SYN-FEE-FREIGHT", "", 1), "feeItem"},
		"没给依据": {strings.Replace(feeItemMappingDocument, `,
	"basisRef": "SYN-CLAUSE-1"`, "", 1), "basisRef"},
		"空白租户": {strings.Replace(feeItemMappingDocument, "SYN-T1", "", 1), "tenantId"},
	}
	for name, spec := range cases {
		_, _, err := registrationjson.FeeItemMappingFromJSON([]byte(spec.document))
		if err == nil {
			t.Fatalf("%s：译装没拒", name)
		}
		if !strings.Contains(err.Error(), spec.field) {
			t.Fatalf("%s：错误 %q 没指名 %s", name, err, spec.field)
		}
	}
	if _, _, err := registrationjson.FeeItemMappingFromJSON([]byte(strings.Replace(feeItemMappingDocument, "SYN-CLAUSE-1", "", 1))); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("空白依据 err = %v，想要领域的空值门", err)
	}
}
