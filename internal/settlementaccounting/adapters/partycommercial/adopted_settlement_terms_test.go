package partycommercial_test

import (
	"context"
	"errors"
	"testing"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/partycommercial"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 16 证「包裹 → 结算账户」链中段的三格（ADR-0190 决定三）：闭包落库又读回之后，采用的结算政策
// 四维还在不在、是不是账户册登的那个形——政策引用取对象不带版本。

func newSettlementTermsAdapter(t *testing.T, closure pcdomain.CommercialClosure) *adapter.AdoptedSettlementTerms {
	t.Helper()
	persistence := newPersistence(t)
	var outcome pcports.ResolutionSaveOutcome
	if err := persistence.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = persistence.resolutions.Save(txCtx, closure)
		return err
	}); err != nil {
		t.Fatalf("固定解析：%v", err)
	}
	if outcome != pcports.ResolutionSaved {
		t.Fatalf("save outcome = %s, want SAVED", outcome)
	}
	built, err := adapter.NewAdoptedSettlementTerms(persistence.resolutions)
	if err != nil {
		t.Fatalf("构造 SA→PC 结算政策四维适配器：%v", err)
	}
	return built
}

// Covers: 主径——闭包采用的结算政策译成四维：法人、相对方、币种取适用范围，政策取对象标识不带版本。
func TestTheAdoptedSettlementPolicyBecomesTheFourAccountKeyDimensions(t *testing.T) {
	closure := fixedClosure(t, pcdomain.TermsMethod, false)
	terms, found, err := newSettlementTermsAdapter(t, closure).LoadAdoptedSettlementTerms(
		t.Context(), saTenant(t, "tenant-1"), saResolution(t, closure.ResolutionID().String()))
	if err != nil || !found {
		t.Fatalf("读回结算政策四维：found=%v err=%v", found, err)
	}
	if terms.LegalEntity().String() != "legal-1" || terms.Counterparty().String() != "customer-1" ||
		terms.Currency().String() != "CNY" || terms.Policy().String() != "policy-1" {
		t.Fatalf("四维 = %+v, want legal-1 / customer-1 / CNY / policy-1（对象不带版本）", terms)
	}
}

// Covers: 闭包没采用结算政策是 found=false 不是错误——那是解析键没要这一项，等租户补。
func TestAClosureWithoutASettlementPolicyAnswersNotAdopted(t *testing.T) {
	closure := creditClosure(t, false, pcdomain.CreditLimit{})
	_, found, err := newSettlementTermsAdapter(t, closure).LoadAdoptedSettlementTerms(
		t.Context(), saTenant(t, "tenant-1"), saResolution(t, closure.ResolutionID().String()))
	if err != nil || found {
		t.Fatalf("found=%v err=%v, want 没采用且不报错", found, err)
	}
}

// Covers: 坏回指不是「没采用」：查无闭包、他租户借用回指、空回指都是 error。
func TestABadResolutionReferenceIsAnErrorNotSettlementTermsNotAdopted(t *testing.T) {
	closure := fixedClosure(t, pcdomain.TermsMethod, false)
	terms := newSettlementTermsAdapter(t, closure)

	if _, found, err := terms.LoadAdoptedSettlementTerms(t.Context(), saTenant(t, "tenant-1"),
		saResolution(t, "RES-NOBODY")); !errors.Is(err, adapter.ErrUntranslatableAnswer) || found {
		t.Fatalf("查无闭包：found=%v err=%v, want ErrUntranslatableAnswer", found, err)
	}
	if _, found, err := terms.LoadAdoptedSettlementTerms(t.Context(), saTenant(t, "tenant-b"),
		saResolution(t, closure.ResolutionID().String())); err == nil || found {
		t.Fatalf("他租户借用回指：found=%v err=%v", found, err)
	}
	if _, _, err := terms.LoadAdoptedSettlementTerms(t.Context(), saTenant(t, "tenant-1"),
		sadomain.CommercialResolutionReference{}); !errors.Is(err, adapter.ErrUntranslatableAnswer) {
		t.Fatalf("空回指 err = %v, want ErrUntranslatableAnswer", err)
	}
}

func TestTheSettlementTermsAdapterRefusesToAssembleWithoutTheClosureView(t *testing.T) {
	if _, err := adapter.NewAdoptedSettlementTerms(nil); err == nil {
		t.Fatal("缺闭包读口仍装配成功")
	}
}
