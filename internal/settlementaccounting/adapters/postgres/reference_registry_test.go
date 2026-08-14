package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 16 证两个引用视图的三格分开：`未配置`（目录空）、`未满足 /
// 无规则`（配了但依据或登记不在）与`已满足`。两者的实例内容都留空——空表正是首发唯一
// 走得到的真实分支，而空表答的必须是`未配置`，不是任何一种放行。

var registryAt = time.Date(2026, 9, 12, 11, 0, 0, 0, time.UTC)

func TestAnUnconfiguredConfirmationConditionIsNotFound(t *testing.T) {
	registry := newReferenceRegistries(t)
	ctx := t.Context()

	condition, found, err := registry.conditions.LoadConfirmationCondition(
		ctx,
		saTenant(t, "tenant-1"),
		saValue(t, domain.NewCustomerChargeID, "charge-1"),
		saValue(t, domain.NewFeeItemReference, "BASE_FREIGHT"),
	)
	if err != nil {
		t.Fatalf("核对确认条件：%v", err)
	}
	if found {
		t.Fatalf("目录是空的，却答了 found=true：%+v", condition)
	}
	if condition.Met {
		t.Fatal("未配置被读成了`已满足`——这正是默认转正")
	}
}

// TestAConfiguredConditionWithoutItsBasisIsUnmet 证第二格：条件配着、依据没到，答
// `未满足`带缺口。缺口点名要求的依据种类，续办方才知道该去取什么。
func TestAConfiguredConditionWithoutItsBasisIsUnmet(t *testing.T) {
	registry := newReferenceRegistries(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	feeItem := saValue(t, domain.NewFeeItemReference, "BASE_FREIGHT")

	mustRegisterCondition(t, registry, tenant, feeItem, "DELIVERY_CONFIRMED")

	condition, found, err := registry.conditions.LoadConfirmationCondition(
		ctx, tenant, saValue(t, domain.NewCustomerChargeID, "charge-1"), feeItem)
	if err != nil || !found {
		t.Fatalf("核对确认条件：found=%v err=%v", found, err)
	}
	if condition.Met {
		t.Fatal("依据没到，条件却算满足了")
	}
	if !strings.HasSuffix(condition.Gap, "DELIVERY_CONFIRMED") {
		t.Fatalf("缺口 = %q，没有点名要求的依据种类", condition.Gap)
	}
}

func TestAnArrivedBasisMeetsTheCondition(t *testing.T) {
	registry := newReferenceRegistries(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	feeItem := saValue(t, domain.NewFeeItemReference, "BASE_FREIGHT")
	charge := saValue(t, domain.NewCustomerChargeID, "charge-1")

	mustRegisterCondition(t, registry, tenant, feeItem, "DELIVERY_CONFIRMED")
	mustRecordBasis(t, registry, tenant, charge, feeItem, "DELIVERY_CONFIRMED", "delivery-1")

	condition, found, err := registry.conditions.LoadConfirmationCondition(ctx, tenant, charge, feeItem)
	if err != nil || !found {
		t.Fatalf("核对确认条件：found=%v err=%v", found, err)
	}
	if !condition.Met {
		t.Fatalf("依据到了却算未满足：%+v", condition)
	}
	if condition.Basis.String() != "delivery-1" {
		t.Fatalf("确认依据 = %q, want delivery-1", condition.Basis)
	}
	if condition.Gap != "" {
		t.Fatalf("已满足还带着缺口：%q", condition.Gap)
	}
}

// TestABasisOfAnotherKindDoesNotMeetTheCondition 证核对认的是目录点名的那一种依据：
// 到了另一种依据不顶替，否则`已满足`就成了拿一份不相干的证据放行。
func TestABasisOfAnotherKindDoesNotMeetTheCondition(t *testing.T) {
	registry := newReferenceRegistries(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	feeItem := saValue(t, domain.NewFeeItemReference, "BASE_FREIGHT")
	charge := saValue(t, domain.NewCustomerChargeID, "charge-1")

	mustRegisterCondition(t, registry, tenant, feeItem, "DELIVERY_CONFIRMED")
	mustRecordBasis(t, registry, tenant, charge, feeItem, "MILESTONE_REACHED", "milestone-1")

	condition, found, err := registry.conditions.LoadConfirmationCondition(ctx, tenant, charge, feeItem)
	if err != nil || !found {
		t.Fatalf("核对确认条件：found=%v err=%v", found, err)
	}
	if condition.Met {
		t.Fatalf("另一种依据顶替了要求的那种：%+v", condition)
	}
}

// TestConfirmationBasesAreChargeAndTenantScoped 证依据不串：另一笔费用、另一个租户
// 登记的依据不能让这一笔的条件满足。
func TestConfirmationBasesAreChargeAndTenantScoped(t *testing.T) {
	registry := newReferenceRegistries(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	feeItem := saValue(t, domain.NewFeeItemReference, "BASE_FREIGHT")

	mustRegisterCondition(t, registry, tenant, feeItem, "DELIVERY_CONFIRMED")
	mustRecordBasis(t, registry, tenant,
		saValue(t, domain.NewCustomerChargeID, "charge-1"), feeItem, "DELIVERY_CONFIRMED", "delivery-1")

	other, found, err := registry.conditions.LoadConfirmationCondition(
		ctx, tenant, saValue(t, domain.NewCustomerChargeID, "charge-2"), feeItem)
	if err != nil || !found {
		t.Fatalf("核对另一笔费用：found=%v err=%v", found, err)
	}
	if other.Met {
		t.Fatal("另一笔费用的依据让这一笔满足了")
	}

	crossTenant, found, err := registry.conditions.LoadConfirmationCondition(
		ctx, saTenant(t, "tenant-2"),
		saValue(t, domain.NewCustomerChargeID, "charge-1"), feeItem)
	if err != nil {
		t.Fatalf("跨租户核对：%v", err)
	}
	if found || crossTenant.Met {
		t.Fatalf("另一个租户读到了本租户的目录：found=%v %+v", found, crossTenant)
	}
}

func TestReRegisteringAConditionDoesNotOverwriteIt(t *testing.T) {
	registry := newReferenceRegistries(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	feeItem := saValue(t, domain.NewFeeItemReference, "BASE_FREIGHT")

	mustRegisterCondition(t, registry, tenant, feeItem, "DELIVERY_CONFIRMED")

	var outcome adapter.RegisterOutcome
	saWithin(t, registry.transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = registry.conditions.RegisterCondition(
			txCtx, tenant, feeItem, "ANYTHING_GOES", registryAt)
		return err
	})
	if outcome != adapter.AlreadyRegistered {
		t.Fatalf("outcome = %s, want ALREADY_REGISTERED", outcome)
	}

	mustRecordBasis(t, registry, tenant,
		saValue(t, domain.NewCustomerChargeID, "charge-1"), feeItem, "ANYTHING_GOES", "anything-1")

	condition, _, err := registry.conditions.LoadConfirmationCondition(
		ctx, tenant, saValue(t, domain.NewCustomerChargeID, "charge-1"), feeItem)
	if err != nil {
		t.Fatalf("核对确认条件：%v", err)
	}
	if condition.Met {
		t.Fatal("第二次登记把要求的依据种类改掉了")
	}
}

func TestAConditionWithoutARequiredKindIsRefused(t *testing.T) {
	registry := newReferenceRegistries(t)

	saWithin(t, registry.transactor, t.Context(), func(txCtx context.Context) error {
		if _, err := registry.conditions.RegisterCondition(
			txCtx, saTenant(t, "tenant-1"),
			saValue(t, domain.NewFeeItemReference, "BASE_FREIGHT"), "   ", registryAt,
		); err == nil {
			t.Error("没有点名依据种类的确认条件登记成功了")
		}
		return nil
	})
}

func TestAnUnconfiguredAllocationRuleIsNotFound(t *testing.T) {
	registry := newReferenceRegistries(t)

	rule, found, err := registry.allocationRules.LoadAllocationRule(
		t.Context(),
		saTenant(t, "tenant-1"),
		saValue(t, domain.NewAllocationSourceReference, "shared-cost-1"),
	)
	if err != nil {
		t.Fatalf("取分摊规则：%v", err)
	}
	if found {
		t.Fatalf("目录是空的，却答了 found=true：%s", rule)
	}
}

func TestARegisteredAllocationRuleComesBackAndDoesNotChangeSilently(t *testing.T) {
	registry := newReferenceRegistries(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	source := saValue(t, domain.NewAllocationSourceReference, "shared-cost-1")

	mustRegisterAllocationRule(t, registry, tenant, source, "allocation-rule-v1")

	rule, found, err := registry.allocationRules.LoadAllocationRule(ctx, tenant, source)
	if err != nil || !found {
		t.Fatalf("取分摊规则：found=%v err=%v", found, err)
	}
	if rule.String() != "allocation-rule-v1" {
		t.Fatalf("规则版本 = %q, want allocation-rule-v1", rule)
	}

	var outcome adapter.RegisterOutcome
	saWithin(t, registry.transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = registry.allocationRules.Register(txCtx, tenant, source,
			saValue(t, domain.NewAllocationRuleVersionReference, "allocation-rule-v2"), registryAt)
		return err
	})
	if outcome != adapter.AlreadyRegistered {
		t.Fatalf("outcome = %s, want ALREADY_REGISTERED", outcome)
	}

	unchanged, _, err := registry.allocationRules.LoadAllocationRule(ctx, tenant, source)
	if err != nil {
		t.Fatalf("取分摊规则：%v", err)
	}
	if unchanged.String() != "allocation-rule-v1" {
		t.Fatalf("规则版本被静默换成了 %q", unchanged)
	}

	if _, found, err := registry.allocationRules.LoadAllocationRule(
		ctx, saTenant(t, "tenant-2"), source,
	); err != nil || found {
		t.Fatalf("另一个租户读到了本租户的规则：found=%v err=%v", found, err)
	}
}

func TestRegisteringOutsideATransactionIsRefused(t *testing.T) {
	registry := newReferenceRegistries(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")

	if _, err := registry.conditions.RegisterCondition(
		ctx, tenant, saValue(t, domain.NewFeeItemReference, "BASE_FREIGHT"),
		"DELIVERY_CONFIRMED", registryAt,
	); err == nil {
		t.Fatal("事务外登记确认条件成功了")
	}
	if _, err := registry.allocationRules.Register(
		ctx, tenant, saValue(t, domain.NewAllocationSourceReference, "shared-cost-1"),
		saValue(t, domain.NewAllocationRuleVersionReference, "allocation-rule-v1"), registryAt,
	); err == nil {
		t.Fatal("事务外登记分摊规则成功了")
	}
}

// ---- 夹具 ----

type referenceRegistries struct {
	conditions      *adapter.ChargeConfirmationConditions
	allocationRules *adapter.AllocationRuleApplicability
	transactor      bentoapp.Transactor
}

func newReferenceRegistries(t *testing.T) referenceRegistries {
	t.Helper()

	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	conditions, err := adapter.NewChargeConfirmationConditions(db)
	if err != nil {
		t.Fatalf("构造确认条件登记册：%v", err)
	}
	rules, err := adapter.NewAllocationRuleApplicability(db)
	if err != nil {
		t.Fatalf("构造分摊规则登记册：%v", err)
	}
	return referenceRegistries{conditions: conditions, allocationRules: rules, transactor: db.Transactor()}
}

func mustRegisterCondition(
	t *testing.T,
	registry referenceRegistries,
	tenant domain.TenantID,
	feeItem domain.FeeItemReference,
	requiredKind string,
) {
	t.Helper()
	saWithin(t, registry.transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := registry.conditions.RegisterCondition(
			txCtx, tenant, feeItem, requiredKind, registryAt)
		if err != nil {
			return err
		}
		if outcome != adapter.Registered {
			t.Errorf("登记确认条件：outcome = %s, want REGISTERED", outcome)
		}
		return nil
	})
}

func mustRecordBasis(
	t *testing.T,
	registry referenceRegistries,
	tenant domain.TenantID,
	charge domain.CustomerChargeID,
	feeItem domain.FeeItemReference,
	basisKind, basisRef string,
) {
	t.Helper()
	saWithin(t, registry.transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := registry.conditions.RecordBasis(
			txCtx, tenant, charge, feeItem, basisKind,
			saValue(t, domain.NewConfirmationBasisReference, basisRef), registryAt)
		if err != nil {
			return err
		}
		if outcome != adapter.Registered {
			t.Errorf("登记确认依据：outcome = %s, want REGISTERED", outcome)
		}
		return nil
	})
}

func mustRegisterAllocationRule(
	t *testing.T,
	registry referenceRegistries,
	tenant domain.TenantID,
	source domain.AllocationSourceReference,
	ruleVersion string,
) {
	t.Helper()
	saWithin(t, registry.transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := registry.allocationRules.Register(
			txCtx, tenant, source,
			saValue(t, domain.NewAllocationRuleVersionReference, ruleVersion), registryAt)
		if err != nil {
			return err
		}
		if outcome != adapter.Registered {
			t.Errorf("登记分摊规则：outcome = %s, want REGISTERED", outcome)
		}
		return nil
	})
}
