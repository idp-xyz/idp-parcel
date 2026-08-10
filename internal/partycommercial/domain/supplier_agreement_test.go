package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func supplierAgreement(t *testing.T, objectID, supplier, scope string) domain.SupplierAgreement {
	t.Helper()
	live, err := registerable(t, domain.SupplierAgreementObject, objectID, "v1", "sha256:"+objectID).
		TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	agreement, err := domain.NewSupplierAgreement(
		live,
		commercialValue(t, domain.NewPartyID, supplier),
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewCommercialScopeReference, scope),
		commercialValue(t, domain.NewPricingPlanReference, "buy-plan-"+objectID),
		mustInterval(t),
	)
	if err != nil {
		t.Fatalf("new supplier agreement: %v", err)
	}
	return agreement
}

// Covers: CONTEXT「供应商商业协议在批准生效后，才能用于新的采购决定和供应商预期成本
// 计算」。
func TestOnlyAnEffectiveAgreementSupportsNewProcurement(t *testing.T) {
	agreement := supplierAgreement(t, "agreement-1", "supplier-1", "scope-a")
	within := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	if !agreement.SupportsProcurementAt(within) {
		t.Fatal("an effective agreement did not support a new procurement decision")
	}
	if agreement.SupportsProcurementAt(time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("an agreement outside its interval supported a new procurement decision")
	}

	draft := commercialDraft(t, domain.SupplierAgreementObject, "agreement-x", "v1", "sha256:x")
	if _, err := domain.NewSupplierAgreement(
		draft,
		commercialValue(t, domain.NewPartyID, "supplier-1"),
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
		commercialValue(t, domain.NewPricingPlanReference, "buy-plan-x"),
		mustInterval(t),
	); !errors.Is(err, domain.ErrInvalidSupplierAgreement) {
		t.Fatalf("error = %v; an unapproved agreement was usable", err)
	}
}

// Covers: CONTEXT「协议版本到期、终止或被替代后，不改变已经形成的运输委托、履约事实、
// 供应商账单主张或审核应付依据」— 终止只停止新采购，不动既有引用。
func TestTerminationStopsNewProcurementWithoutRewritingHistory(t *testing.T) {
	agreement := supplierAgreement(t, "agreement-1", "supplier-1", "scope-a")
	terminatedAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	terminated, err := agreement.Terminate(commercialValue(t, domain.NewRelationshipBasisReference, "terminate-1"), terminatedAt)
	if err != nil {
		t.Fatalf("terminate: %v", err)
	}

	if terminated.SupportsProcurementAt(terminatedAt.Add(time.Hour)) {
		t.Fatal("a terminated agreement still supported a new procurement decision")
	}
	if agreement.SupportsProcurementAt(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) != true {
		t.Fatal("termination mutated the original value")
	}
	if terminated.Supplier() != agreement.Supplier() ||
		terminated.PurchasePricingPlan() != agreement.PurchasePricingPlan() {
		t.Fatal("termination rewrote the supplier or the purchase pricing plan it once bound")
	}
	if _, present := terminated.TerminatedAt(); !present {
		t.Fatal("a terminated agreement records no termination time")
	}
}

// Covers: CONTEXT 所有权 —— `transport-fulfillment` 保存实际运输委托采用的协议与履约条件
// 快照，`settlement-accounting` 形成供应商预期成本、账单匹配与审核应付；本上下文都不拥有。
func TestAgreementHoldsNoFulfilmentOrPayableFacts(t *testing.T) {
	agreementType := reflect.TypeOf(domain.SupplierAgreement{})
	forbidden := []string{
		"consignment", "shipmentorder", "actual", "fulfil", "fulfill",
		"invoice", "bill", "payable", "cost", "expected",
	}
	for index := 0; index < agreementType.NumField(); index++ {
		name := strings.ToLower(agreementType.Field(index).Name)
		for _, word := range forbidden {
			if strings.Contains(name, word) {
				t.Fatalf("SupplierAgreement carries %s, which pulls a fulfilment or payable fact into the commercial context",
					agreementType.Field(index).Name)
			}
		}
	}
}

// Covers: CONTEXT「销售价格规则、采购价格规则和法人间结算价格规则分别表达」— 协议绑定的
// 是采购方向的定价方案，不能当作对客可执行价格。
func TestAgreementBindsAPurchasePlanDistinctFromSellingPrice(t *testing.T) {
	agreement := supplierAgreement(t, "agreement-1", "supplier-1", "scope-a")
	sellPolicy := pricePolicy(t, "policy-sell", domain.SellDirection, "scope-a", "plan-sell")

	if agreement.PurchasePricingPlan() == sellPolicy.PricingPlan() {
		t.Fatal("the purchase plan and the selling plan are the same reference")
	}
	if agreement.Direction() != domain.BuyDirection {
		t.Fatalf("direction = %q, want BUY", agreement.Direction())
	}
}

// Covers: CONTEXT — 协议必须指名供应商参与方、责任法人、适用范围与采购定价方案，缺一
// 建不成。
func TestAgreementNeedsSupplierEntityScopeAndPlan(t *testing.T) {
	live, err := registerable(t, domain.SupplierAgreementObject, "agreement-y", "v1", "sha256:y").
		TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}

	missing := map[string]func() error{
		"no supplier": func() error {
			_, err := domain.NewSupplierAgreement(live, domain.PartyID{},
				commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
				commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
				commercialValue(t, domain.NewPricingPlanReference, "buy-plan"), mustInterval(t))
			return err
		},
		"no legal entity": func() error {
			_, err := domain.NewSupplierAgreement(live, commercialValue(t, domain.NewPartyID, "supplier-1"),
				domain.LegalEntityReference{},
				commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
				commercialValue(t, domain.NewPricingPlanReference, "buy-plan"), mustInterval(t))
			return err
		},
		"no purchase plan": func() error {
			_, err := domain.NewSupplierAgreement(live, commercialValue(t, domain.NewPartyID, "supplier-1"),
				commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
				commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
				domain.PricingPlanReference{}, mustInterval(t))
			return err
		},
	}
	for name, build := range missing {
		t.Run(name, func(t *testing.T) {
			if err := build(); !errors.Is(err, domain.ErrInvalidSupplierAgreement) {
				t.Fatalf("error = %v, want ErrInvalidSupplierAgreement", err)
			}
		})
	}
}
