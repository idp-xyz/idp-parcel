package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

var decisionReceivedAt = time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)

func mustValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

func decision(t *testing.T, quantity domain.RequiredQuantity) domain.RegulatoryDecision {
	t.Helper()
	built, err := domain.NewRegulatoryDecision(domain.RegulatoryDecisionSpec{
		ID:         mustValue(t, domain.NewRegulatoryDecisionID, "decision-1"),
		Authority:  mustValue(t, domain.NewRegulatoryAuthorityReference, "CUSTOMS/US-CBP"),
		Action:     mustValue(t, domain.NewLegalActionReference, "DESTRUCTION"),
		Scope:      mustValue(t, domain.NewDecisionScopeReference, "parcel-1"),
		Quantity:   quantity,
		ReceivedAt: decisionReceivedAt,
	})
	if err != nil {
		t.Fatalf("new regulatory decision: %v", err)
	}
	return built
}

func execution(t *testing.T, reference, scope string, units int) domain.ExecutionFact {
	t.Helper()
	built, err := domain.NewExecutionFact(domain.ExecutionFactSpec{
		Executor:   mustValue(t, domain.NewExecutorReference, "node-origin"),
		Fact:       mustValue(t, domain.NewExecutionFactReference, reference),
		Scope:      mustValue(t, domain.NewDecisionScopeReference, scope),
		Units:      units,
		OccurredAt: decisionReceivedAt.Add(4 * time.Hour),
	})
	if err != nil {
		t.Fatalf("new execution fact: %v", err)
	}
	return built
}

// Covers: CC CONTEXT「实际隔离、移交、开封、重封、销毁……由执行事实证明，不能由决定
// 本身推导」与 `AT-PS-092` 引用的两半——无执行事实即证据不足（决定单独形不成核对
// 覆盖）；数量由来源提供时按合计比较，等于才是已覆盖；核对结果类型上没有任何义务
// 终结字段（核对完成不自动终结监管义务）。
func TestVerificationDemandsFactsAndComparesProvidedQuantities(t *testing.T) {
	twoUnits := decision(t, domain.RequiredQuantity{Provided: true, Units: 2})

	insufficient, err := domain.VerifyDispositionExecution(twoUnits, nil, decisionReceivedAt.Add(5*time.Hour))
	if err != nil {
		t.Fatalf("verify without facts: %v", err)
	}
	if insufficient.Conclusion() != domain.ExecutionEvidenceInsufficient {
		t.Fatalf("conclusion = %q; 决定本身推导不出执行", insufficient.Conclusion())
	}

	covered, err := domain.VerifyDispositionExecution(twoUnits, []domain.ExecutionFact{
		execution(t, "DESTRUCTION-EXEC/1", "parcel-1", 1),
		execution(t, "DESTRUCTION-EXEC/2", "parcel-1", 1),
	}, decisionReceivedAt.Add(6*time.Hour))
	if err != nil {
		t.Fatalf("verify covered: %v", err)
	}
	if covered.Conclusion() != domain.ExecutionCovered || covered.ExecutedUnits() != 2 {
		t.Fatalf("conclusion = %q executed = %d", covered.Conclusion(), covered.ExecutedUnits())
	}

	partial, err := domain.VerifyDispositionExecution(twoUnits, []domain.ExecutionFact{
		execution(t, "DESTRUCTION-EXEC/1", "parcel-1", 1),
	}, decisionReceivedAt.Add(6*time.Hour))
	if err != nil {
		t.Fatalf("verify partial: %v", err)
	}
	if partial.Conclusion() != domain.ExecutionPartiallyCovered {
		t.Fatalf("conclusion = %q, want PARTIALLY_COVERED", partial.Conclusion())
	}

	deviation, err := domain.VerifyDispositionExecution(twoUnits, []domain.ExecutionFact{
		execution(t, "DESTRUCTION-EXEC/1", "parcel-1", 3),
	}, decisionReceivedAt.Add(6*time.Hour))
	if err != nil {
		t.Fatalf("verify deviation: %v", err)
	}
	if deviation.Conclusion() != domain.ExecutionDeviation {
		t.Fatalf("conclusion = %q; 执行超出决定明确覆盖的范围是差异不是多多益善", deviation.Conclusion())
	}
}

// Covers: CC CONTEXT「数量、期限、条件……未提供或不适用的内容必须明确记录，不能猜测
// 补齐」——来源未提供数量时数量不是核对维度：范围相符即已覆盖，不虚构一个「默认
// 数量」去比；范围不符的执行事实是事实冲突（拿别的范围的执行凑数分不出真假）。
func TestUnprovidedDimensionsAreHonestlyOutOfScope(t *testing.T) {
	noQuantity := decision(t, domain.RequiredQuantity{})

	covered, err := domain.VerifyDispositionExecution(noQuantity, []domain.ExecutionFact{
		execution(t, "DESTRUCTION-EXEC/1", "parcel-1", 5),
	}, decisionReceivedAt.Add(6*time.Hour))
	if err != nil {
		t.Fatalf("verify without quantity: %v", err)
	}
	if covered.Conclusion() != domain.ExecutionCovered {
		t.Fatalf("conclusion = %q; 未提供的数量不是核对维度", covered.Conclusion())
	}

	conflict, err := domain.VerifyDispositionExecution(noQuantity, []domain.ExecutionFact{
		execution(t, "DESTRUCTION-EXEC/1", "parcel-9", 1),
	}, decisionReceivedAt.Add(6*time.Hour))
	if err != nil {
		t.Fatalf("verify foreign scope: %v", err)
	}
	if conflict.Conclusion() != domain.ExecutionFactConflict {
		t.Fatalf("conclusion = %q, want FACT_CONFLICT", conflict.Conclusion())
	}
}

// Covers: CC CONTEXT「监管处置决定必须具有可唯一关联的决定身份、来源、法律动作语义
// 和明确适用对象或范围」——四件缺一立不起；声称提供却给非正数数量的决定是矛盾输入。
func TestADecisionDemandsItsFullIdentity(t *testing.T) {
	cases := map[string]domain.RegulatoryDecisionSpec{
		"no authority": {
			ID:         mustValue(t, domain.NewRegulatoryDecisionID, "decision-1"),
			Action:     mustValue(t, domain.NewLegalActionReference, "DESTRUCTION"),
			Scope:      mustValue(t, domain.NewDecisionScopeReference, "parcel-1"),
			ReceivedAt: decisionReceivedAt,
		},
		"no legal action": {
			ID:         mustValue(t, domain.NewRegulatoryDecisionID, "decision-1"),
			Authority:  mustValue(t, domain.NewRegulatoryAuthorityReference, "CUSTOMS/US-CBP"),
			Scope:      mustValue(t, domain.NewDecisionScopeReference, "parcel-1"),
			ReceivedAt: decisionReceivedAt,
		},
		"no scope": {
			ID:         mustValue(t, domain.NewRegulatoryDecisionID, "decision-1"),
			Authority:  mustValue(t, domain.NewRegulatoryAuthorityReference, "CUSTOMS/US-CBP"),
			Action:     mustValue(t, domain.NewLegalActionReference, "DESTRUCTION"),
			ReceivedAt: decisionReceivedAt,
		},
		"claimed quantity without units": {
			ID:         mustValue(t, domain.NewRegulatoryDecisionID, "decision-1"),
			Authority:  mustValue(t, domain.NewRegulatoryAuthorityReference, "CUSTOMS/US-CBP"),
			Action:     mustValue(t, domain.NewLegalActionReference, "DESTRUCTION"),
			Scope:      mustValue(t, domain.NewDecisionScopeReference, "parcel-1"),
			Quantity:   domain.RequiredQuantity{Provided: true},
			ReceivedAt: decisionReceivedAt,
		},
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewRegulatoryDecision(spec); !errors.Is(err, domain.ErrInvalidRegulatoryDecision) {
				t.Fatalf("err = %v, want ErrInvalidRegulatoryDecision", err)
			}
		})
	}
}
