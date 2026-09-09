package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func creditPolicyReference(t *testing.T, raw string) domain.CreditPolicyReference {
	t.Helper()
	reference, err := domain.NewCreditPolicyReference(raw)
	if err != nil {
		t.Fatalf("new credit policy reference: %v", err)
	}
	return reference
}

// Covers: ADR-0127 决定四——授信依据两格封闭（金额 / 比例），恰一在场；零额度是合法的商业声明，
// 负值与无出处立不住。
func TestCreditBasisIsEitherAnAmountOrARatioWithAPolicyBehindIt(t *testing.T) {
	policy := creditPolicyReference(t, "credit-1/v1")

	amount, err := domain.NewCreditAmountBasis(policy, 0)
	if err != nil {
		t.Fatalf("零额度是合法声明，却被拒：%v", err)
	}
	if minor, ok := amount.AmountMinor(); !ok || minor != 0 {
		t.Fatalf("amount = (%d, %v), want (0, true)", minor, ok)
	}
	if _, ok := amount.RatioBasisPoints(); ok {
		t.Fatal("金额额度答出了比例在场")
	}
	if amount.Policy() != policy {
		t.Fatal("依据丢了政策出处")
	}

	ratio, err := domain.NewCreditRatioBasis(policy, 2500, domain.PostedBalanceBase)
	if err != nil {
		t.Fatalf("new ratio basis: %v", err)
	}
	if bps, ok := ratio.RatioBasisPoints(); !ok || bps != 2500 {
		t.Fatalf("ratio = (%d, %v), want (2500, true)", bps, ok)
	}
	if base, ok := ratio.RatioBase(); !ok || base != domain.PostedBalanceBase {
		t.Fatalf("ratio base = (%s, %v), want (POSTED_BALANCE, true)", base, ok)
	}
	if _, ok := ratio.AmountMinor(); ok {
		t.Fatal("比例额度答出了金额在场")
	}
	if _, ok := amount.RatioBase(); ok {
		t.Fatal("金额额度答出了基数在场")
	}

	if _, err := domain.NewCreditAmountBasis(policy, -1); !errors.Is(err, domain.ErrInvalidCreditBasis) {
		t.Fatalf("负金额：err = %v, want ErrInvalidCreditBasis", err)
	}
	if _, err := domain.NewCreditRatioBasis(policy, -1, domain.PostedBalanceBase); !errors.Is(err, domain.ErrInvalidCreditBasis) {
		t.Fatalf("负比例：err = %v, want ErrInvalidCreditBasis", err)
	}
	if _, err := domain.NewCreditRatioBasis(policy, 2500, domain.CreditRatioBaseUndeclared); !errors.Is(err, domain.ErrInvalidCreditBasis) {
		t.Fatalf("新依据缺基数：err = %v, want ErrInvalidCreditBasis——没有基数的比例只有存量正文一条来路", err)
	}
	if _, err := domain.NewCreditRatioBasis(policy, 2500, domain.CreditRatioBase(250)); !errors.Is(err, domain.ErrInvalidCreditBasis) {
		t.Fatalf("集外基数：err = %v, want ErrInvalidCreditBasis", err)
	}
	if _, err := domain.NewCreditAmountBasis(domain.CreditPolicyReference{}, 100); !errors.Is(err, domain.ErrInvalidCreditBasis) {
		t.Fatalf("无出处：err = %v, want ErrInvalidCreditBasis——没有政策出处的额度就是本上下文自己发明的额度", err)
	}
	if !(domain.CreditPolicyReference{}).IsZero() || policy.IsZero() {
		t.Fatal("IsZero 答反了：零值引用没有出处，构造出来的有")
	}
}

// Covers: ADR-0129 决定三——比例额度按基数取值折成金额：⌊基数 × 万分比 ÷ 10000⌋ 向下取整；基数为负或为零折 0
// （账上没有可放大的资金，不是替缺席补零）；金额额度与未声明基数的存量比例都折不得，各答自己的哨兵；
// 乘不进 int64 的额度报溢出而不是绕回。
func TestARatioBasisFoldsOntoItsBaseByFlooring(t *testing.T) {
	policy := creditPolicyReference(t, "credit-1/v1")
	ratio, err := domain.NewCreditRatioBasis(policy, 2500, domain.PriorPeriodConfirmedChargesBase)
	if err != nil {
		t.Fatalf("new ratio basis: %v", err)
	}
	for name, tc := range map[string]struct {
		base int64
		want int64
	}{
		"quarter of 100000":         {base: 100_000, want: 25_000},
		"floors, never rounds up":   {base: 3, want: 0},
		"floors below one unit":     {base: 39, want: 9},
		"zero base is zero limit":   {base: 0, want: 0},
		"negative base is zero too": {base: -500_000, want: 0},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := ratio.LimitOnBase(tc.base)
			if err != nil || got != tc.want {
				t.Fatalf("LimitOnBase(%d) = (%d, %v), want (%d, nil)", tc.base, got, err, tc.want)
			}
		})
	}

	amount, err := domain.NewCreditAmountBasis(policy, 100)
	if err != nil {
		t.Fatalf("new amount basis: %v", err)
	}
	if _, err := amount.LimitOnBase(1_000); !errors.Is(err, domain.ErrCreditBasisNotRatio) {
		t.Fatalf("金额额度折算：err = %v, want ErrCreditBasisNotRatio", err)
	}

	undeclared, err := domain.NewUndeclaredCreditRatioBasis(policy, 2500)
	if err != nil {
		t.Fatalf("new undeclared ratio basis: %v", err)
	}
	if _, ok := undeclared.RatioBase(); ok {
		t.Fatal("未声明基数的存量比例答出了基数在场")
	}
	if _, err := undeclared.LimitOnBase(1_000); !errors.Is(err, domain.ErrCreditRatioBaseUndeclared) {
		t.Fatalf("未声明基数折算：err = %v, want ErrCreditRatioBaseUndeclared", err)
	}

	huge, err := domain.NewCreditRatioBasis(policy, 1<<62, domain.PostedBalanceBase)
	if err != nil {
		t.Fatalf("new huge ratio basis: %v", err)
	}
	if _, err := huge.LimitOnBase(1 << 62); !errors.Is(err, domain.ErrCreditLimitOverflow) {
		t.Fatalf("溢出：err = %v, want ErrCreditLimitOverflow", err)
	}
}

// Covers: ADR-0127 决定四——信用状况换上商业侧授权额度：只换额度，已占用与逾期原样留着；原值不改。
// 额度与它的出处一起换上：登记状况自己没有政策出处，授权额度不带出处进不来——没有出处的额度就是本
// 上下文自己发明的额度。
func TestWithAuthorizedLimitReplacesOnlyTheLimit(t *testing.T) {
	scope := settlementScope(t, "legal-1", "account-1", "CNY")
	policy := creditPolicyReference(t, "credit-1/v1")
	registered, err := domain.NewCreditStanding(scope, 1_000, 300, true)
	if err != nil {
		t.Fatalf("new credit standing: %v", err)
	}
	if registered.Policy().String() != "" {
		t.Fatalf("registered policy = %q；登记状况没有政策出处可言", registered.Policy())
	}

	authorized, err := registered.WithAuthorizedLimit(10_000, policy)
	if err != nil {
		t.Fatalf("with authorized limit: %v", err)
	}
	if authorized.LimitMinor() != 10_000 || authorized.Headroom() != 9_700 || !authorized.Overdue() {
		t.Fatalf("authorized = limit %d headroom %d overdue %v, want 10000 / 9700 / true",
			authorized.LimitMinor(), authorized.Headroom(), authorized.Overdue())
	}
	if authorized.Policy() != policy {
		t.Fatalf("authorized policy = %q, want %q——额度换上了，出处没跟着", authorized.Policy(), policy)
	}
	if authorized.Scope() != scope {
		t.Fatal("换额度换掉了作用域")
	}
	if registered.LimitMinor() != 1_000 || registered.Headroom() != 700 || registered.Policy().String() != "" {
		t.Fatal("原状况快照被就地改写")
	}
	if _, err := registered.WithAuthorizedLimit(-1, policy); !errors.Is(err, domain.ErrInvalidCreditStanding) {
		t.Fatalf("负额度：err = %v, want ErrInvalidCreditStanding", err)
	}
	if _, err := registered.WithAuthorizedLimit(10_000, domain.CreditPolicyReference{}); !errors.Is(err, domain.ErrInvalidCreditStanding) {
		t.Fatalf("无出处：err = %v, want ErrInvalidCreditStanding", err)
	}
	if _, err := (domain.CreditStanding{}).WithAuthorizedLimit(1, policy); !errors.Is(err, domain.ErrInvalidCreditStanding) {
		t.Fatalf("零值状况：err = %v, want ErrInvalidCreditStanding", err)
	}
}

// Covers: CONTEXT「每项费用、冻结、信用暴露和核销必须保存实际采用的结算政策、预付/账期方式及其适用
// 范围」的信用政策那一份（ADR-0127：三份采用依据哪一份都替不了另一份）——暴露记下它据以判额度的
// 那一版信用政策，`业务限制`同样带（受限结果也要答得出「按哪一版额度判的」）；重放交回原暴露，出处
// 随原暴露走、不随本次的状况换。没换上授权额度的登记状况不能来判：据它占额度就是拿登记值当政策额度
// （ADR-0127 决定四），形成的暴露也答不出出处——这条守在账本上，编排少调一步立刻炸出来。
func TestAnExposureKeepsTheCreditPolicyItWasJudgedAgainst(t *testing.T) {
	scope := settlementScope(t, "legal-1", "account-1", "CNY")
	first := creditPolicyReference(t, "credit-1/v1")
	second := creditPolicyReference(t, "credit-1/v2")
	registered, err := domain.NewCreditStanding(scope, 100_000, 0, false)
	if err != nil {
		t.Fatalf("new credit standing: %v", err)
	}
	ledger := domain.NewCreditExposureLedger()
	if _, err := ledger.Expose(exposureRequestFor(t, scope, "control-1", 4_000), registered); !errors.Is(err, domain.ErrStandingNotAuthorized) {
		t.Fatalf("登记状况直接判暴露：err = %v, want ErrStandingNotAuthorized——登记的 100000 不是政策额度", err)
	}

	authorized, err := registered.WithAuthorizedLimit(5_000, first)
	if err != nil {
		t.Fatalf("with authorized limit: %v", err)
	}
	recorded, err := ledger.Expose(exposureRequestFor(t, scope, "control-1", 4_000), authorized)
	if err != nil {
		t.Fatalf("expose: %v", err)
	}
	if recorded.Status() != domain.ExposureRecorded || recorded.Policy() != first {
		t.Fatalf("recorded = %s / policy %q, want RECORDED / %q", recorded.Status(), recorded.Policy(), first)
	}

	restricted, err := ledger.Expose(exposureRequestFor(t, scope, "control-2", 6_000), authorized)
	if err != nil {
		t.Fatalf("expose beyond headroom: %v", err)
	}
	if restricted.Status() != domain.ExposureRestricted || restricted.Policy() != first {
		t.Fatalf("restricted = %s / policy %q, want RESTRICTED / %q——受限结果也要答得出按哪一版额度判的",
			restricted.Status(), restricted.Policy(), first)
	}

	reauthorized, err := registered.WithAuthorizedLimit(50_000, second)
	if err != nil {
		t.Fatalf("with authorized limit: %v", err)
	}
	replayed, err := ledger.Expose(exposureRequestFor(t, scope, "control-1", 4_000), reauthorized)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed.ExposureID() != recorded.ExposureID() || replayed.Policy() != first {
		t.Fatalf("replayed policy = %q, want the original %q——重放交回的是原暴露，出处不随本次状况改口",
			replayed.Policy(), first)
	}
}

func exposureRequestFor(t *testing.T, scope domain.SettlementScope, requestID string, amountMinor int64) domain.ExposureRequest {
	t.Helper()
	controlRequest, err := domain.NewControlRequestID(requestID)
	if err != nil {
		t.Fatalf("new control request id: %v", err)
	}
	association, err := domain.NewBusinessAssociationReference("submission-" + requestID)
	if err != nil {
		t.Fatalf("new business association: %v", err)
	}
	request, err := domain.NewExposureRequest(controlRequest, scope, amountMinor, association, frozenAt)
	if err != nil {
		t.Fatalf("new exposure request: %v", err)
	}
	return request
}
