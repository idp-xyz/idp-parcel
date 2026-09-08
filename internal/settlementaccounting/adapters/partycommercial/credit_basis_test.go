package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/partycommercial"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 16 证 SA→PC 授信依据适配器的三格（ADR-0127 决定四）。走真库而不是替身，
// 理由与控制策略那只相同：这一段翻译的要害在闭包快照落库又读回之后额度还在不在、是不是那一格。

type creditFixture struct {
	adapter     *adapter.CreditBasis
	persistence pcPersistence
	closure     pcdomain.CommercialClosure
}

func (fixture *creditFixture) load(t *testing.T) (sadomain.CreditBasis, bool, error) {
	t.Helper()
	return fixture.adapter.LoadCreditBasis(t.Context(), saTenant(t, "tenant-1"), settlementScope(t),
		saResolution(t, fixture.closure.ResolutionID().String()))
}

func newCreditFixture(t *testing.T, closure pcdomain.CommercialClosure) *creditFixture {
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
	built, err := adapter.NewCreditBasis(persistence.resolutions)
	if err != nil {
		t.Fatalf("构造 SA→PC 授信依据适配器：%v", err)
	}
	return &creditFixture{adapter: built, persistence: persistence, closure: closure}
}

// creditClosure 造一份含客户合同、接单规则包与（可选）信用政策的唯一已解析闭包。withCredit=false
// 模拟租户登记的解析键没要求信用政策那一项；limit 为零值时只登版本壳不登正文（闭包解不出，调用方
// 不该走到这里）。
func creditClosure(t *testing.T, withCredit bool, limit pcdomain.CreditLimit) pcdomain.CommercialClosure {
	t.Helper()

	registry := pcdomain.NewCommercialRegistry()
	register(t, registry, effectiveVersion(t, pcdomain.CustomerContractObject, "contract-1", "v1", "digest-c1"))
	register(t, registry, effectiveVersion(t, pcdomain.AcceptanceRulePackageObject, "rules-1", "v1", "digest-r1"))
	required := []pcdomain.CommercialObjectKind{pcdomain.CustomerContractObject, pcdomain.AcceptanceRulePackageObject}

	key := pcdomain.ClosureResolutionKey{
		TenantID:             pcValue(t, pcdomain.NewTenantID, "tenant-1"),
		CustomerAccountID:    pcValue(t, pcdomain.NewCustomerAccountID, "customer-1"),
		LegalEntityCandidate: pcValue(t, pcdomain.NewLegalEntityReference, "legal-1"),
		Scope:                pcValue(t, pcdomain.NewCommercialScopeReference, "scope-1"),
		Purpose:              pcdomain.AcceptanceControlPurpose,
	}
	if withCredit {
		creditVersion := effectiveVersion(t, pcdomain.CreditPolicyObject, "credit-1", "v1", "digest-k1")
		register(t, registry, creditVersion)
		interval, err := pcdomain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
		if err != nil {
			t.Fatalf("有效区间：%v", err)
		}
		policy, err := pcdomain.NewCreditPolicy(
			creditVersion,
			pcValue(t, pcdomain.NewLegalEntityReference, "legal-1"),
			pcValue(t, pcdomain.NewAuthorityLevel, "level-commercial"),
			pcValue(t, pcdomain.NewChargeTypeReference, "charge-freight"),
			limit,
			interval,
		)
		if err != nil {
			t.Fatalf("构造信用政策：%v", err)
		}
		registry.RegisterCreditPolicy(policy)
		required = append(required, pcdomain.CreditPolicyObject)
		key.Credit = pcdomain.CreditSelector{
			Level:      pcValue(t, pcdomain.NewAuthorityLevel, "level-commercial"),
			ChargeType: pcValue(t, pcdomain.NewChargeTypeReference, "charge-freight"),
		}
	}
	anchor, err := pcdomain.NewSelectionAnchor(effectiveAtRow.Add(24*time.Hour),
		pcValue(t, pcdomain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("选择锚点：%v", err)
	}
	key.Anchor = anchor
	key.RequiredBases = required

	closure := pcdomain.ResolveCommercialClosure(registry, key, nil)
	if closure.Outcome() != pcdomain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED（reason=%q unresolved=%v）",
			closure.Outcome(), closure.Reason(), closure.UnresolvedBases())
	}
	return closure
}

func pcAmountLimit(t *testing.T, minor int64) pcdomain.CreditLimit {
	t.Helper()
	limit, err := pcdomain.NewCreditAmountLimit(minor)
	if err != nil {
		t.Fatalf("金额额度：%v", err)
	}
	return limit
}

func pcRatioLimit(t *testing.T, bps int64) pcdomain.CreditLimit {
	t.Helper()
	limit, err := pcdomain.NewCreditRatioLimit(bps)
	if err != nil {
		t.Fatalf("比例额度：%v", err)
	}
	return limit
}

// Covers: ADR-0127 决定四主径 / UC-SA-002 步 7 账期分支 / AT-SA-171——凭回指取回已固定闭包，从闭包里
// 采用的信用政策版本读出额度与出处：金额译金额、比例译比例，出处是「对象/版本」写法、与结算政策
// 引用同一形。额度来自快照，不回 0020 重读（本夹具根本没往 0020 写正文）。
func TestTheAdoptedCreditPolicyBecomesACreditBasisWithItsLimitAndVersion(t *testing.T) {
	t.Run("amount", func(t *testing.T) {
		fixture := newCreditFixture(t, creditClosure(t, true, pcAmountLimit(t, 500000)))

		basis, found, err := fixture.load(t)
		if err != nil || !found {
			t.Fatalf("读回授信依据：found=%v err=%v", found, err)
		}
		if minor, ok := basis.AmountMinor(); !ok || minor != 500000 {
			t.Fatalf("amount = (%d, %v), want (500000, true)", minor, ok)
		}
		if _, ok := basis.RatioBasisPoints(); ok {
			t.Fatal("金额额度译出了比例在场")
		}
		if basis.Policy().String() != "credit-1/v1" {
			t.Fatalf("policy = %q, want credit-1/v1——额度出自的政策版本要随答复带回", basis.Policy())
		}
	})

	t.Run("ratio", func(t *testing.T) {
		fixture := newCreditFixture(t, creditClosure(t, true, pcRatioLimit(t, 2500)))

		basis, found, err := fixture.load(t)
		if err != nil || !found {
			t.Fatalf("读回授信依据：found=%v err=%v", found, err)
		}
		if bps, ok := basis.RatioBasisPoints(); !ok || bps != 2500 {
			t.Fatalf("ratio = (%d, %v), want (2500, true)——比例原样携带，不在这里折成金额", bps, ok)
		}
		if _, ok := basis.AmountMinor(); ok {
			t.Fatal("比例额度译出了金额在场")
		}
	})
}

// Covers: 本口的「未配置」格——闭包没采用信用政策（解析键没要求它）：found=false 且不是错误，恢复
// 动作是租户补解析键与正文；不得读成「无限信用」或「零额度」。
func TestAClosureWithoutACreditPolicyIsNotConfiguredNotAnError(t *testing.T) {
	fixture := newCreditFixture(t, creditClosure(t, false, pcdomain.CreditLimit{}))

	basis, found, err := fixture.load(t)
	if err != nil {
		t.Fatalf("闭包没采用信用政策被当成了错误：%v", err)
	}
	if found {
		t.Fatalf("闭包里没有信用政策却答了 found=true / %#v", basis)
	}
	if _, ok := basis.AmountMinor(); ok {
		t.Fatal("未配置带出了一份金额——那正是替商业侧说的「零额度」")
	}
}

// Covers: 坏回指与「未配置」分格，纪律同控制策略那只：查无闭包是调用方给的标识坏了、他租户不得借
// 用本租户的回指、空回指译不动是编程错误——三者都不是「商业侧没登记过」。
func TestABadResolutionReferenceIsAnErrorNotAnUnconfiguredCreditBasis(t *testing.T) {
	fixture := newCreditFixture(t, creditClosure(t, true, pcAmountLimit(t, 500000)))

	t.Run("unknown resolution", func(t *testing.T) {
		_, found, err := fixture.adapter.LoadCreditBasis(t.Context(), saTenant(t, "tenant-1"), settlementScope(t),
			saResolution(t, "RES-NOBODY"))
		if !errors.Is(err, adapter.ErrUntranslatableAnswer) || found {
			t.Fatalf("查无闭包：found=%v err=%v, want ErrUntranslatableAnswer", found, err)
		}
	})

	t.Run("another tenant cannot borrow the reference", func(t *testing.T) {
		_, found, err := fixture.adapter.LoadCreditBasis(t.Context(), saTenant(t, "tenant-b"), settlementScope(t),
			saResolution(t, fixture.closure.ResolutionID().String()))
		if err == nil || found {
			t.Fatalf("他租户凭本租户回指读到了额度：found=%v err=%v", found, err)
		}
	})

	t.Run("empty reference", func(t *testing.T) {
		_, _, err := fixture.adapter.LoadCreditBasis(t.Context(), saTenant(t, "tenant-1"), settlementScope(t),
			sadomain.CommercialResolutionReference{})
		if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
			t.Fatalf("err = %v；空回指译不动是编程错误，不是未配置", err)
		}
	})
}

// Covers: 装配疏漏不得伪装成未配置——缺闭包读口构造期就拒。
func TestTheCreditBasisAdapterRefusesToAssembleWithoutTheClosureView(t *testing.T) {
	if _, err := adapter.NewCreditBasis(nil); err == nil {
		t.Fatal("缺闭包读口仍装配成功——它会让一次接线遗漏与租户没登记长得一模一样")
	}
}
