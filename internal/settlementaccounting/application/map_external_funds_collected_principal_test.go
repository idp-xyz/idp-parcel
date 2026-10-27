package application_test

import (
	"context"
	"reflect"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// principalMapCommand 把 adoptCommand 那条事实（bank-receipt-1）整条认作代收本金：目标就是事实自己。
func principalMapCommand(t *testing.T, mapping string) application.MapFundsCommand {
	t.Helper()
	return claimMapCommand(t, mapping, domain.TargetCollectedPrincipal, "bank-receipt-1")
}

// Covers: AT-SA-216——一条银行回款只记一次——整条认作代收本金之后它不再接受任何别的映射：映到对账单、账户、审核应付都答
// 映射冲突，什么都不落，客户应收作用域一个不碰（入账余额因此永远不会多出这笔受托资金）。
func TestAFactSetAsideAsCollectedPrincipalAcceptsNoOtherMapping(t *testing.T) {
	fixture := newFundsFixture(t)
	ctx := context.Background()
	fixture.books.registerStatement(t, "STMT-A", "account-a")
	fixture.books.registerAccount(t, "account-b")
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}

	*fixture.locks = (*fixture.locks)[:0]
	result, err := fixture.handler.Map(ctx, principalMapCommand(t, "mapping-principal"))
	mustFunds(t, result, err, application.FundsMapped, "整条认作代收本金")
	if _, posted := result.FactScopePosting(); posted {
		t.Fatal("认作代收本金的答复带了客户应收作用域的记入方式：它不该碰任何入账余额")
	}
	if got := firstLocks(*fixture.locks); !reflect.DeepEqual(got, []string{"fact:bank-receipt-1"}) {
		t.Fatalf("认作代收本金取锁 = %v，要只取事实锁（无作用域可推）", got)
	}

	for _, other := range []struct {
		mapping string
		kind    domain.SettlementTargetKind
		target  string
	}{
		{"mapping-statement", domain.TargetStatement, "STMT-A"},
		{"mapping-account", domain.TargetAccount, "account-b"},
		{"mapping-payable", domain.TargetPayable, "PAYABLE-1"},
		{"mapping-credit", domain.TargetCreditNote, "CREDIT-1"},
	} {
		result, err := fixture.handler.Map(ctx, claimMapCommand(t, other.mapping, other.kind, other.target))
		mustFunds(t, result, err, application.FundsMappingConflict, "认作代收本金之后又映到 "+other.kind.String())
		if _, found, _ := fixture.mappings.FindByKey(ctx, mappingKeyOf(t, other.mapping)); found {
			t.Fatalf("映射冲突仍把 %s 落了行", other.mapping)
		}
	}
	if len(fixture.mappings.records) != 1 {
		t.Fatalf("事实上映射行 = %d，要只有认作代收本金那一条", len(fixture.mappings.records))
	}
}

// Covers: AT-SA-216——反向同理——事实上已有任何一条别的映射，就不能再整条认作代收本金：已映到对账单的回款可能已经记进某个
// 作用域的入账余额，再认作受托资金，同一笔钱就在两处各当一次。
func TestAFactWithAnyOtherMappingCannotBeSetAsideAsCollectedPrincipal(t *testing.T) {
	for _, existing := range []struct {
		name   string
		kind   domain.SettlementTargetKind
		target string
	}{
		{"statement", domain.TargetStatement, "STMT-A"},
		{"account", domain.TargetAccount, "account-b"},
		{"payable", domain.TargetPayable, "PAYABLE-1"},
	} {
		t.Run(existing.name, func(t *testing.T) {
			fixture := newFundsFixture(t)
			ctx := context.Background()
			fixture.books.registerStatement(t, "STMT-A", "account-a")
			fixture.books.registerAccount(t, "account-b")
			if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
				t.Fatalf("adopt: %v", err)
			}
			result, err := fixture.handler.Map(ctx, claimMapCommand(t, "mapping-first", existing.kind, existing.target))
			mustFunds(t, result, err, application.FundsMapped, "先映到 "+existing.name)

			result, err = fixture.handler.Map(ctx, principalMapCommand(t, "mapping-principal"))
			mustFunds(t, result, err, application.FundsMappingConflict, "已有映射的回款认作代收本金")
			if len(fixture.mappings.records) != 1 {
				t.Fatalf("事实上映射行 = %d，冲突不该落行", len(fixture.mappings.records))
			}
		})
	}
}

// Covers: AT-SA-216——认作代收本金的重放与重复——同映射标识同内容答已存在，不翻倍；同一条事实换映射标识再认作一次答映射冲突
// （认作是事实一级的一个状态，不是可叠加的金额）；映射标识相同、内容不同同样冲突。
func TestSettingAFactAsideAsCollectedPrincipalIsIdempotentPerFact(t *testing.T) {
	fixture := newFundsFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	result, err := fixture.handler.Map(ctx, principalMapCommand(t, "mapping-principal"))
	mustFunds(t, result, err, application.FundsMapped, "首次认作")

	result, err = fixture.handler.Map(ctx, principalMapCommand(t, "mapping-principal"))
	mustFunds(t, result, err, application.FundsMappingExisting, "同标识同内容重放")

	result, err = fixture.handler.Map(ctx, principalMapCommand(t, "mapping-principal-2"))
	mustFunds(t, result, err, application.FundsMappingConflict, "换映射标识再认作一次")

	changed := principalMapCommand(t, "mapping-principal")
	changed.Basis = "another-basis"
	result, err = fixture.handler.Map(ctx, changed)
	mustFunds(t, result, err, application.FundsMappingConflict, "同标识改依据")
	if len(fixture.mappings.records) != 1 {
		t.Fatalf("事实上映射行 = %d，要 1", len(fixture.mappings.records))
	}
}

// Covers: AT-SA-216——认作代收本金的输入门——事实未采用答未受理；目标不是事实自己、资金退回事实、付款失败事实都不认作。
func TestOnlyAnAdoptedConfirmedReceiptNamingItselfCanBeSetAside(t *testing.T) {
	fixture := newFundsFixture(t)
	ctx := context.Background()
	result, err := fixture.handler.Map(ctx, principalMapCommand(t, "mapping-before-adoption"))
	mustFunds(t, result, err, application.FundsNotAccepted, "事实未采用")

	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	wrongTarget := claimMapCommand(t, "mapping-wrong-target", domain.TargetCollectedPrincipal, "another-fact")
	result, err = fixture.handler.Map(ctx, wrongTarget)
	mustFunds(t, result, err, application.FundsNotAccepted, "目标不是事实自己")
	if len(fixture.mappings.records) != 0 {
		t.Fatalf("被拒的认作落了行：%d", len(fixture.mappings.records))
	}

	returned := newFundsFixture(t)
	command := adoptCommand(t, domain.FundsReturned)
	if _, err := returned.handler.AdoptFact(ctx, command); err != nil {
		t.Fatalf("adopt returned: %v", err)
	}
	result, err = returned.handler.Map(ctx, principalMapCommand(t, "mapping-returned"))
	mustFunds(t, result, err, application.FundsNotAccepted, "资金退回认作代收本金")

	failed := newFundsFixture(t)
	if _, err := failed.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsPaymentFailed)); err != nil {
		t.Fatalf("adopt failed: %v", err)
	}
	result, err = failed.handler.Map(ctx, principalMapCommand(t, "mapping-failed"))
	mustFunds(t, result, err, application.UnfundableFactOutcome, "付款失败认作代收本金")
}

// Covers: AT-SA-216——认作代收本金的事实不核销：没有结算目标可分配，核销指向它一律未受理，事实上未分配金额不动。
func TestACollectedPrincipalFactIsNeverApplied(t *testing.T) {
	fixture := newFundsFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	result, err := fixture.handler.Map(ctx, principalMapCommand(t, "mapping-principal"))
	mustFunds(t, result, err, application.FundsMapped, "认作")

	command := claimApplyCommand(t, "application-principal", "mapping-principal",
		domain.TargetCollectedPrincipal, "bank-receipt-1", 20000)
	result, err = fixture.handler.Apply(ctx, command)
	mustFunds(t, result, err, application.FundsNotAccepted, "核销到代收本金")
	if len(fixture.applications.records) != 0 {
		t.Fatalf("被拒的核销落了行：%d", len(fixture.applications.records))
	}
}

// Covers: AT-SA-216——外部更正照采（ADR-0285）对认作代收本金的事实同样成立：金额被银行更正后新版本照落，没有作用域可重算也不
// 报超核——结算一侧对这笔钱没有核销可言；被更正后的金额是代收与清分按引用读回的那一版。
func TestACorrectionOfACollectedPrincipalFactIsAdoptedWithNothingToRepost(t *testing.T) {
	fixture := newFundsFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	result, err := fixture.handler.Map(ctx, principalMapCommand(t, "mapping-principal"))
	mustFunds(t, result, err, application.FundsMapped, "认作")

	correction := application.CorrectFundsFactCommand{
		TenantID:    adoptCommand(t, domain.FundsReceiptConfirmed).TenantID,
		Fact:        "bank-receipt-1",
		Corrects:    "bank-receipt-1/v1",
		Version:     "bank-receipt-1/v2",
		AmountMinor: 18000,
		CorrectedAt: fundsOccurredAt.Add(1),
	}
	result, err = fixture.handler.CorrectFact(ctx, correction)
	mustFunds(t, result, err, application.FundsFactAdopted, "更正照采")
	if result.OverAppliedMinor() != 0 || len(result.UnreconciledScopes()) != 0 {
		t.Fatalf("认作代收本金的事实更正答出超核 %d / 未推进作用域 %v", result.OverAppliedMinor(), result.UnreconciledScopes())
	}
}

func mappingKeyOf(t *testing.T, mapping string) ports.FundsMappingKey {
	t.Helper()
	command := mapCommand(t)
	ref, err := domain.NewMappingReference(mapping)
	if err != nil {
		t.Fatalf("mapping ref: %v", err)
	}
	return ports.FundsMappingKey{TenantID: command.TenantID, Mapping: ref}
}
