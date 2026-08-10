package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var controlAsOfAt = time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)

// Covers: settlement-accounting CONTEXT「合同对明确范围规定接受前无财务控制时，该不适用
// 依据由 `party-commercial` 提供，本上下文不得用一次虚假零金额冻结或默认信用通过冒充无
// 控制」——没有依据的`不要求控制`就是那个「默认信用通过」。
func TestDeclaringNoPreAcceptanceControlDemandsAnExplicitBasis(t *testing.T) {
	_, err := domain.NewPreAcceptanceControlPolicy(domain.ControlNotRequired, domain.ControlBasisReference{})
	if !errors.Is(err, domain.ErrInvalidControlPolicy) {
		t.Fatalf("err = %v, want ErrInvalidControlPolicy——不带依据的无控制被接受了", err)
	}

	basis, err := domain.NewControlBasisReference("CONTRACT_DECLARES_NO_PRE_ACCEPTANCE_CONTROL")
	if err != nil {
		t.Fatalf("new control basis: %v", err)
	}
	policy, err := domain.NewPreAcceptanceControlPolicy(domain.ControlNotRequired, basis)
	if err != nil {
		t.Fatalf("new control policy: %v", err)
	}
	if policy.ControlRequired() {
		t.Fatal("不要求控制却报告需要控制")
	}
	if policy.Basis() != basis {
		t.Fatalf("basis = %q, want %q", policy.Basis(), basis)
	}
}

// Covers: 零值不得被读成一个回答。端口没答话时应用层拿到的是零值，若它报告「不要求控制」，
// 一次商业侧沉默就变成了接受前无财务控制。
func TestTheZeroControlPolicyDoesNotAnswerNotRequired(t *testing.T) {
	var unanswered domain.PreAcceptanceControlPolicy
	if unanswered.ControlRequired() {
		t.Fatal("零值报告需要控制，那会让端口的沉默看起来像一个肯定回答")
	}
	if unanswered.Basis().String() != "" {
		t.Fatal("零值携带了不适用依据")
	}
}

// Covers: settlement-accounting CONTEXT——`asOf` 语义、取值与策略版本三项缺一即构造不出来，
// 因为控制结果必须能说明它按哪一版策略在哪一刻判断。
func TestControlAsOfNeedsSemanticValueAndStrategyVersionTogether(t *testing.T) {
	semantic, err := domain.NewAsOfSemantic("CONTROL_EVALUATION_AT")
	if err != nil {
		t.Fatalf("new asOf semantic: %v", err)
	}
	strategy, err := domain.NewAsOfStrategyVersion("control-strategy-v1")
	if err != nil {
		t.Fatalf("new asOf strategy version: %v", err)
	}

	if _, err := domain.NewControlAsOf(semantic, time.Time{}, strategy); !errors.Is(err, domain.ErrInvalidControlAsOf) {
		t.Fatalf("err = %v, want ErrInvalidControlAsOf for a zero instant", err)
	}
	if _, err := domain.NewControlAsOf(domain.AsOfSemantic{}, controlAsOfAt, strategy); !errors.Is(err, domain.ErrInvalidControlAsOf) {
		t.Fatalf("err = %v, want ErrInvalidControlAsOf for a missing semantic", err)
	}
	if _, err := domain.NewControlAsOf(semantic, controlAsOfAt, domain.AsOfStrategyVersion{}); !errors.Is(err, domain.ErrInvalidControlAsOf) {
		t.Fatalf("err = %v, want ErrInvalidControlAsOf for a missing strategy version", err)
	}

	asOf, err := domain.NewControlAsOf(semantic, controlAsOfAt, strategy)
	if err != nil {
		t.Fatalf("new control asOf: %v", err)
	}
	if !asOf.Valid() {
		t.Fatal("三项齐备的 asOf 报告为无效")
	}
}
