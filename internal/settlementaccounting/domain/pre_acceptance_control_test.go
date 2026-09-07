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
	_, err := domain.NewNoControlPolicy(domain.ControlBasisReference{})
	if !errors.Is(err, domain.ErrInvalidControlPolicy) {
		t.Fatalf("err = %v, want ErrInvalidControlPolicy——不带依据的无控制被接受了", err)
	}

	basis, err := domain.NewControlBasisReference("CONTRACT_DECLARES_NO_PRE_ACCEPTANCE_CONTROL")
	if err != nil {
		t.Fatalf("new control basis: %v", err)
	}
	policy, err := domain.NewNoControlPolicy(basis)
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

// Covers: ADR-0122 决定一——`要求`带的是策略正文里**要执行的控制项**（种类 × 判断顺序）、共同
// 通过条件与采用的控制策略版本；结算方式与结算政策仍随答复带回，因为 CONTEXT 要求每项冻结与
// 信用暴露保存「实际采用的结算政策、预付/账期方式」，只是它们不再决定走哪条控制路。缺任何
// 一样构造即死。
func TestARequiredControlPolicyCarriesItsItemsJointConditionAndAdoptedBases(t *testing.T) {
	adopted := controlValue(t, domain.NewAdoptedPolicyReference, "PC-SETTLEMENT-POLICY-V3")
	controlPolicy := controlValue(t, domain.NewControlPolicyReference, "PC-CONTROL-POLICY/v1")
	freeze := controlItem(t, domain.PrepaidFreezeControl, 1)
	credit := controlItem(t, domain.CreditCheckControl, 2)

	cases := map[string]func() (domain.PreAcceptanceControlPolicy, error){
		"没有控制项的要求什么也执行不了": func() (domain.PreAcceptanceControlPolicy, error) {
			return domain.NewRequiredControlPolicy(nil, domain.AllControlsPass, controlPolicy, domain.TermsSettlement, adopted)
		},
		"共同通过条件缺席就说不出多项怎么合起来看": func() (domain.PreAcceptanceControlPolicy, error) {
			return domain.NewRequiredControlPolicy([]domain.ControlItem{freeze}, domain.JointPassConditionInvalid, controlPolicy, domain.TermsSettlement, adopted)
		},
		"没有控制策略引用的结果保存不下按哪一版策略执行": func() (domain.PreAcceptanceControlPolicy, error) {
			return domain.NewRequiredControlPolicy([]domain.ControlItem{freeze}, domain.AllControlsPass, domain.ControlPolicyReference{}, domain.TermsSettlement, adopted)
		},
		"没有方式的结果保存不下预付/账期方式": func() (domain.PreAcceptanceControlPolicy, error) {
			return domain.NewRequiredControlPolicy([]domain.ControlItem{freeze}, domain.AllControlsPass, controlPolicy, domain.SettlementMethodInvalid, adopted)
		},
		"没有结算政策引用的结果保存不下采用依据": func() (domain.PreAcceptanceControlPolicy, error) {
			return domain.NewRequiredControlPolicy([]domain.ControlItem{freeze}, domain.AllControlsPass, controlPolicy, domain.TermsSettlement, domain.AdoptedPolicyReference{})
		},
		"同一种控制两行答不出该按哪条": func() (domain.PreAcceptanceControlPolicy, error) {
			return domain.NewRequiredControlPolicy([]domain.ControlItem{freeze, controlItem(t, domain.PrepaidFreezeControl, 2)}, domain.AllControlsPass, controlPolicy, domain.TermsSettlement, adopted)
		},
		"两行抢同一个判断顺序就没有顺序": func() (domain.PreAcceptanceControlPolicy, error) {
			return domain.NewRequiredControlPolicy([]domain.ControlItem{freeze, controlItem(t, domain.CreditCheckControl, 1)}, domain.AllControlsPass, controlPolicy, domain.TermsSettlement, adopted)
		},
	}
	for name, construct := range cases {
		if _, err := construct(); !errors.Is(err, domain.ErrInvalidControlPolicy) {
			t.Fatalf("%s：err = %v, want ErrInvalidControlPolicy", name, err)
		}
	}

	// 交进去的顺序是乱的，读回来必须按判断顺序：执行方就是照这个次序走的。
	policy, err := domain.NewRequiredControlPolicy(
		[]domain.ControlItem{credit, freeze}, domain.AllControlsPass, controlPolicy, domain.TermsSettlement, adopted)
	if err != nil {
		t.Fatalf("new required control policy: %v", err)
	}
	if !policy.ControlRequired() || policy.Method() != domain.TermsSettlement || policy.AdoptedPolicy() != adopted {
		t.Fatalf("policy = %v/%v; 方式与采用政策没有随答复带回", policy.Method(), policy.AdoptedPolicy())
	}
	if policy.ControlPolicy() != controlPolicy || policy.JointPassCondition() != domain.AllControlsPass {
		t.Fatalf("control policy/joint = %q/%q; 采用的控制策略与共同通过条件没有随答复带回",
			policy.ControlPolicy(), policy.JointPassCondition())
	}
	items := policy.Items()
	if len(items) != 2 || items[0].Kind() != domain.PrepaidFreezeControl || items[1].Kind() != domain.CreditCheckControl {
		t.Fatalf("items = %v; 控制项没有按判断顺序交回", items)
	}
	if items[0].Order() != 1 || items[1].Order() != 2 {
		t.Fatalf("orders = %d/%d, want 1/2", items[0].Order(), items[1].Order())
	}
}

// Covers: 一项控制立不住的两种情形——种类集外、判断顺序不是正整数。零值种类不是任何一条
// 控制路，接受它就是让执行方在 switch 里落进 default。
func TestAControlItemNeedsAKnownKindAndAPositiveOrder(t *testing.T) {
	if _, err := domain.NewControlItem(domain.ControlKindInvalid, 1); !errors.Is(err, domain.ErrInvalidControlPolicy) {
		t.Fatalf("err = %v, want ErrInvalidControlPolicy for an unknown kind", err)
	}
	if _, err := domain.NewControlItem(domain.PrepaidFreezeControl, 0); !errors.Is(err, domain.ErrInvalidControlPolicy) {
		t.Fatalf("err = %v, want ErrInvalidControlPolicy for order 0", err)
	}
	if domain.PrepaidFreezeControl.String() != "PREPAID_FREEZE" || domain.CreditCheckControl.String() != "CREDIT_CHECK" {
		t.Fatalf("kind names = %q/%q; 与 party-commercial 的正文词汇不同名，翻译就要多一层对照表",
			domain.PrepaidFreezeControl, domain.CreditCheckControl)
	}
	if domain.AllControlsPass.String() != "ALL_CONTROLS_PASS" {
		t.Fatalf("joint pass = %q, want ALL_CONTROLS_PASS", domain.AllControlsPass)
	}
}

func controlItem(t *testing.T, kind domain.ControlKind, order uint32) domain.ControlItem {
	t.Helper()
	item, err := domain.NewControlItem(kind, order)
	if err != nil {
		t.Fatalf("new control item %s/%d: %v", kind, order, err)
	}
	return item
}

func controlValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
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
