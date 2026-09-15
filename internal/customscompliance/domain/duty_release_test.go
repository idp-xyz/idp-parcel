package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

var verifiedAt = time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)

// Covers: CC CONTEXT「覆盖状态（无覆盖、部分覆盖、已覆盖）、差额状态（无差额、不足、超额或待确认）和有效性状态（有效、失效、冲突或待确认）分别表达，不能实现为一组互斥总状态」——三个独立枚举各自取值（部分覆盖+不足+有效并存），任一轴缺失
// 立不起核对；类型上无付款/回收/放行字段。
func TestDutyVerificationKeepsItsThreeAxesApart(t *testing.T) {
	verification, err := domain.VerifyDutyPayment(
		mustValue(t, domain.NewAssessedDutyReference, "assessed-duty/v1"),
		mustValue(t, domain.NewExternalFundsFactReference, "bank-fact/77"),
		mustValue(t, domain.NewFundsFactVersion, "bank-fact/77/v1"),
		mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		domain.CoveragePartial,
		domain.DeltaShort,
		domain.FundsFactValid,
		verifiedAt,
	)
	if err != nil {
		t.Fatalf("verify duty payment: %v", err)
	}
	if verification.Coverage() != domain.CoveragePartial ||
		verification.Delta() != domain.DeltaShort ||
		verification.Validity() != domain.FundsFactValid {
		t.Fatalf("axes = %s/%s/%s; 三轴必须各自取值",
			verification.Coverage(), verification.Delta(), verification.Validity())
	}

	if _, err := domain.VerifyDutyPayment(
		mustValue(t, domain.NewAssessedDutyReference, "assessed-duty/v1"),
		mustValue(t, domain.NewExternalFundsFactReference, "bank-fact/77"),
		mustValue(t, domain.NewFundsFactVersion, "bank-fact/77/v1"),
		mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		domain.DutyCoverageInvalid,
		domain.DeltaNone,
		domain.FundsFactValid,
		verifiedAt,
	); !errors.Is(err, domain.ErrInvalidDutyVerification) {
		t.Fatalf("err = %v; 缺覆盖轴的核对被收下了", err)
	}
}

// Covers: 票 sa-cc/22 裁决 1——核对记录带「付款人维是按哪个真实程序的规则判的」（CC CONTEXT「税费付款核对」
// 词条：付款人是「来源提供或真实程序要求的」维度；ADR-0137 决定一：记录带依据引用）。程序是构造期必填的一维，
// 与其余入参同待遇：读口原样交回，空白立不起核对——一份不说自己按哪个程序判的核对，事后无从复核那一维。
func TestDutyVerificationRecordsTheProcedureItWasJudgedUnder(t *testing.T) {
	procedure := mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT")
	verification, err := domain.VerifyDutyPayment(
		mustValue(t, domain.NewAssessedDutyReference, "assessed-duty/v1"),
		mustValue(t, domain.NewExternalFundsFactReference, "bank-fact/77"),
		mustValue(t, domain.NewFundsFactVersion, "bank-fact/77/v1"),
		mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		procedure,
		domain.CoverageFull,
		domain.DeltaNone,
		domain.FundsFactValid,
		verifiedAt,
	)
	if err != nil {
		t.Fatalf("verify duty payment: %v", err)
	}
	if verification.Procedure() != procedure {
		t.Fatalf("procedure = %q; 核对没带上它按哪个程序判", verification.Procedure())
	}

	if _, err := domain.VerifyDutyPayment(
		mustValue(t, domain.NewAssessedDutyReference, "assessed-duty/v1"),
		mustValue(t, domain.NewExternalFundsFactReference, "bank-fact/77"),
		mustValue(t, domain.NewFundsFactVersion, "bank-fact/77/v1"),
		mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		domain.CustomsProcedureReference{},
		domain.CoverageFull,
		domain.DeltaNone,
		domain.FundsFactValid,
		verifiedAt,
	); !errors.Is(err, domain.ErrInvalidDutyVerification) {
		t.Fatalf("err = %v; 不说按哪个程序判的核对被收下了", err)
	}
}

// Covers: 票 sa-cc/19 裁决 3 (3)——核对引用的是资金事实的**哪一版**，不只是事实身份（CC CONTEXT「税费付款核对」：
// 「版本化比较判断」；UC-CC-009「外部资金事实迟到、更正……形成新核对版本」——新版本换的是被比较的那一版事实）。
// 资金版本是构造期必填的一维：读口原样交回，空白立不起核对——不说按哪一版事实判的核对，新版本到了无从知道它
// 判的是不是已被取代的那一版。
func TestDutyVerificationRecordsWhichFundsFactVersionItJudged(t *testing.T) {
	version := mustValue(t, domain.NewFundsFactVersion, "bank-fact/77/v2")
	verification, err := domain.VerifyDutyPayment(
		mustValue(t, domain.NewAssessedDutyReference, "assessed-duty/v1"),
		mustValue(t, domain.NewExternalFundsFactReference, "bank-fact/77"),
		version,
		mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		domain.CoverageFull,
		domain.DeltaPending,
		domain.FundsFactPending,
		verifiedAt,
	)
	if err != nil {
		t.Fatalf("verify duty payment: %v", err)
	}
	if verification.FundsVersion() != version {
		t.Fatalf("funds version = %q; 核对没带上它判的是事实的哪一版", verification.FundsVersion())
	}

	if _, err := domain.VerifyDutyPayment(
		mustValue(t, domain.NewAssessedDutyReference, "assessed-duty/v1"),
		mustValue(t, domain.NewExternalFundsFactReference, "bank-fact/77"),
		domain.FundsFactVersion{},
		mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		domain.CoverageFull,
		domain.DeltaNone,
		domain.FundsFactValid,
		verifiedAt,
	); !errors.Is(err, domain.ErrInvalidDutyVerification) {
		t.Fatalf("err = %v; 不说判的是哪一版事实的核对被收下了", err)
	}
}

// Covers: CC CONTEXT「放行结果不能由技术成功、业务受理、税费支付或内部合规解除推导」与「放行可以针对全部、部分或附条件范围形成」——监管来源引用必备（别的东西
// 换不成它）；附条件必带条件、全部放行不带条件（两向拦）。
func TestAReleaseOutcomeComesOnlyFromTheAuthority(t *testing.T) {
	conditional, err := domain.ReceiveReleaseOutcome(
		domain.ConditionalRelease,
		mustValue(t, domain.NewRegulatoryAuthorityReference, "CUSTOMS/US-CBP"),
		mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		"present permit within 10 days",
		verifiedAt,
	)
	if err != nil {
		t.Fatalf("receive conditional release: %v", err)
	}
	condition, has := conditional.Condition()
	if !has || condition == "" {
		t.Fatal("附条件放行没带条件")
	}

	if _, err := domain.ReceiveReleaseOutcome(
		domain.ConditionalRelease,
		mustValue(t, domain.NewRegulatoryAuthorityReference, "CUSTOMS/US-CBP"),
		mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		"",
		verifiedAt,
	); !errors.Is(err, domain.ErrInvalidReleaseOutcome) {
		t.Fatalf("err = %v; 说不出条件的附条件放行被收下了", err)
	}
	if _, err := domain.ReceiveReleaseOutcome(
		domain.FullRelease,
		mustValue(t, domain.NewRegulatoryAuthorityReference, "CUSTOMS/US-CBP"),
		mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		"leftover condition",
		verifiedAt,
	); !errors.Is(err, domain.ErrInvalidReleaseOutcome) {
		t.Fatalf("err = %v; 全部放行带了条件", err)
	}
	if _, err := domain.ReceiveReleaseOutcome(
		domain.FullRelease,
		domain.RegulatoryAuthorityReference{},
		mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		"",
		verifiedAt,
	); !errors.Is(err, domain.ErrInvalidReleaseOutcome) {
		t.Fatalf("err = %v; 没有监管来源的放行被收下了——技术成功换不成监管来源", err)
	}
}

// Covers: CC CONTEXT「放行门禁核对……可以形成待满足、部分满足、满足、冲突或不适用等判断，但不代替监管机构形成放行结果，也不能复用于其他动作或监管边界」——五值封闭、
// 动作与边界构造期绑定（AppliesTo 供消费方核对）、非不适用必带前置条件清单；类型上
// 无放行字段。
func TestAGateVerificationBindsItsActionAndBoundary(t *testing.T) {
	gate, err := domain.VerifyReleaseGate(
		mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		domain.LoadingDeparture,
		mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		[]domain.PreconditionReference{
			mustValue(t, domain.NewPreconditionReference, "duty-verification/v1"),
			mustValue(t, domain.NewPreconditionReference, "restriction-check/v1"),
		},
		domain.GateMet,
		verifiedAt,
	)
	if err != nil {
		t.Fatalf("verify release gate: %v", err)
	}
	if !gate.AppliesTo(domain.LoadingDeparture, mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86")) {
		t.Fatal("门禁判断不适用于它自己核对的动作")
	}
	if gate.AppliesTo(domain.FinalDelivery, mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86")) {
		t.Fatal("门禁判断被复用到了别的动作")
	}
	if gate.AppliesTo(domain.LoadingDeparture, mustValue(t, domain.NewCustomsProcedureReference, "US-EXPORT/EEI")) {
		t.Fatal("门禁判断被复用到了别的监管边界")
	}

	if _, err := domain.VerifyReleaseGate(
		mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		domain.LoadingDeparture,
		mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		nil,
		domain.GateMet,
		verifiedAt,
	); !errors.Is(err, domain.ErrInvalidGateVerification) {
		t.Fatalf("err = %v; 说不出核对了什么的满足被收下了", err)
	}

	notApplicable, err := domain.VerifyReleaseGate(
		mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		domain.FinalDelivery,
		mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		nil,
		domain.GateNotApplicable,
		verifiedAt,
	)
	if err != nil {
		t.Fatalf("verify not-applicable gate: %v", err)
	}
	if notApplicable.Conclusion() != domain.GateNotApplicable {
		t.Fatalf("conclusion = %q", notApplicable.Conclusion())
	}
}
