package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// controlItem 造一项控制项结果；受限项带自己的原因引用，成立项不带（成立的依据在占用记录本身）。
func controlItem(
	t *testing.T,
	kind domain.ControlItemKind,
	order uint32,
	conclusion domain.ControlItemConclusion,
) domain.ControlItemResult {
	t.Helper()
	basis := domain.ControlBasisReference{}
	if conclusion == domain.ControlItemRestricted {
		basis = mustValue(t, domain.NewControlBasisReference, kind.String()+"_INSUFFICIENT")
	}
	item, err := domain.NewControlItemResult(kind, order, conclusion, basis)
	if err != nil {
		t.Fatalf("new control item result: %v", err)
	}
	return item
}

func executedControl(t *testing.T, items ...domain.ControlItemResult) domain.FinancialControlResult {
	t.Helper()
	result, err := domain.NewExecutedFinancialControlResult(domain.ExecutedFinancialControlSpec{
		ResultID:  mustValue(t, domain.NewFinancialControlResultID, "request-1/version-1"),
		Items:     items,
		JointPass: domain.AllControlsPass,
		AsOf:      financialControlAsOf(t),
	})
	if err != nil {
		t.Fatalf("new executed financial control result: %v", err)
	}
	return result
}

// Covers: SA CONTEXT「由 parcel-shipment 按策略的共同通过条件形成接受判断」在领域层的落法
// （ADR-0125 决定一、二）——接受侧结论由构造期从逐项结果按条件推出，不由调用方交入。「全部通过」
// 之下：任一项受限即 `RESTRICTED`、依据取判断顺序最靠前的受限项自己的原因；全部成立时成立的项里
// 有预付冻结即 `HELD`，否则 `CREDIT_EXPOSED`。逐项按判断顺序排定，与交入顺序无关。
func TestAnExecutedControlDerivesItsConclusionFromTheItemsByTheJointPassCondition(t *testing.T) {
	freezeOK := controlItem(t, domain.PrepaidFreezeControlItem, 1, domain.ControlItemSatisfied)
	creditOK := controlItem(t, domain.CreditCheckControlItem, 2, domain.ControlItemSatisfied)
	creditNo := controlItem(t, domain.CreditCheckControlItem, 2, domain.ControlItemRestricted)
	creditFirstNo := controlItem(t, domain.CreditCheckControlItem, 1, domain.ControlItemRestricted)
	freezeSecondOK := controlItem(t, domain.PrepaidFreezeControlItem, 2, domain.ControlItemSatisfied)

	cases := []struct {
		name      string
		items     []domain.ControlItemResult
		outcome   domain.FinancialControlOutcome
		basis     string
		firstKind domain.ControlItemKind
	}{
		{"只有冻结成立", []domain.ControlItemResult{freezeOK}, domain.FinancialControlHeld, "", domain.PrepaidFreezeControlItem},
		{"只有信用成立", []domain.ControlItemResult{controlItem(t, domain.CreditCheckControlItem, 1, domain.ControlItemSatisfied)},
			domain.FinancialControlCreditExposed, "", domain.CreditCheckControlItem},
		{"两项都成立", []domain.ControlItemResult{creditOK, freezeOK}, domain.FinancialControlHeld, "", domain.PrepaidFreezeControlItem},
		{"第二项受限", []domain.ControlItemResult{freezeOK, creditNo}, domain.FinancialControlRestricted, "CREDIT_CHECK_INSUFFICIENT", domain.PrepaidFreezeControlItem},
		{"第一项受限", []domain.ControlItemResult{freezeSecondOK, creditFirstNo}, domain.FinancialControlRestricted, "CREDIT_CHECK_INSUFFICIENT", domain.CreditCheckControlItem},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := executedControl(t, tc.items...)
			if result.Outcome() != tc.outcome {
				t.Fatalf("outcome = %q, want %q", result.Outcome(), tc.outcome)
			}
			if result.Basis().String() != tc.basis {
				t.Fatalf("basis = %q, want %q——受限的依据要是那一项自己的原因", result.Basis(), tc.basis)
			}
			if result.JointPassCondition() != domain.AllControlsPass {
				t.Fatalf("joint pass condition = %q, want ALL_CONTROLS_PASS", result.JointPassCondition())
			}
			items := result.Items()
			if len(items) != len(tc.items) {
				t.Fatalf("items = %d, want %d——逐项结果一项都不能丢", len(items), len(tc.items))
			}
			if items[0].Kind() != tc.firstKind {
				t.Fatalf("first item = %q, want %q——逐项按判断顺序排定", items[0].Kind(), tc.firstKind)
			}
			if result.ResultID().String() != "request-1/version-1" {
				t.Fatalf("result ID = %q, want the control request identity", result.ResultID())
			}
		})
	}
}

// Covers: ADR-0125 决定四——「有没有占用」是逐项的事实，不是结论的事实。第一项冻结成立、第二项
// 受限的结果结论是 `RESTRICTED`，但那笔冻结确已占下，接受确定未成立时必须释放；只有一项受限时
// 没有任何占用；`明确无控制`同样没有。
func TestOccupationFollowsTheSatisfiedItemsNotTheConclusion(t *testing.T) {
	heldThenRestricted := executedControl(t,
		controlItem(t, domain.PrepaidFreezeControlItem, 1, domain.ControlItemSatisfied),
		controlItem(t, domain.CreditCheckControlItem, 2, domain.ControlItemRestricted),
	)
	if heldThenRestricted.Outcome() != domain.FinancialControlRestricted || !heldThenRestricted.OccupationFormed() {
		t.Fatalf("outcome = %q occupation = %t; 受限结论下第一项占下的资金仍在账本上",
			heldThenRestricted.Outcome(), heldThenRestricted.OccupationFormed())
	}

	onlyRestricted := executedControl(t,
		controlItem(t, domain.PrepaidFreezeControlItem, 1, domain.ControlItemRestricted),
	)
	if onlyRestricted.OccupationFormed() {
		t.Fatal("唯一一项受限却报形成了占用——限制不入账本，无可释放")
	}

	exposed := executedControl(t,
		controlItem(t, domain.CreditCheckControlItem, 1, domain.ControlItemSatisfied),
	)
	if !exposed.OccupationFormed() {
		t.Fatal("信用暴露已记录却报没有占用——账期额度同样要释放（ADR-0047）")
	}

	inapplicable, err := domain.NewInapplicableFinancialControlResult(
		mustValue(t, domain.NewControlBasisReference, "CONTRACT_DECLARES_NO_PRE_ACCEPTANCE_CONTROL"),
		financialControlAsOf(t),
	)
	if err != nil {
		t.Fatalf("new inapplicable financial control result: %v", err)
	}
	if inapplicable.OccupationFormed() {
		t.Fatal("明确无控制却报形成了占用")
	}
	if (domain.FinancialControlResult{}).OccupationFormed() {
		t.Fatal("从未形成的控制却报形成了占用")
	}
}

// Covers: ADR-0025 全函数——本上下文还不会算的共同通过条件不得静默按「全部通过」折；ADR-0115 决定三
// 点名的那条红线。集外取值在构造期就拒绝，而不是等到形成接受判断时。
func TestAnExecutedControlRefusesAJointPassConditionItCannotEvaluate(t *testing.T) {
	_, err := domain.NewExecutedFinancialControlResult(domain.ExecutedFinancialControlSpec{
		ResultID:  mustValue(t, domain.NewFinancialControlResultID, "request-1/version-1"),
		Items:     []domain.ControlItemResult{controlItem(t, domain.PrepaidFreezeControlItem, 1, domain.ControlItemSatisfied)},
		JointPass: domain.JointPassCondition(99),
		AsOf:      financialControlAsOf(t),
	})
	if !errors.Is(err, domain.ErrInvalidFinancialControlResult) {
		t.Fatalf("error = %v, want ErrInvalidFinancialControlResult", err)
	}
}

// Covers: 与 PC 正文的行级约束同形（ADR-0115 决定二）——至少一项、判断顺序唯一、同一种控制至多
// 一项；已执行的控制必须带请求身份作结果标识（ADR-0027），时点必须是回显过的。
func TestAnExecutedControlKeepsTheShapeOfThePolicyContent(t *testing.T) {
	freezeOK := controlItem(t, domain.PrepaidFreezeControlItem, 1, domain.ControlItemSatisfied)
	specs := map[string]domain.ExecutedFinancialControlSpec{
		"零项": {
			ResultID:  mustValue(t, domain.NewFinancialControlResultID, "request-1/version-1"),
			JointPass: domain.AllControlsPass, AsOf: financialControlAsOf(t),
		},
		"顺序重复": {
			ResultID: mustValue(t, domain.NewFinancialControlResultID, "request-1/version-1"),
			Items: []domain.ControlItemResult{
				freezeOK,
				controlItem(t, domain.CreditCheckControlItem, 1, domain.ControlItemSatisfied),
			},
			JointPass: domain.AllControlsPass, AsOf: financialControlAsOf(t),
		},
		"种类重复": {
			ResultID: mustValue(t, domain.NewFinancialControlResultID, "request-1/version-1"),
			Items: []domain.ControlItemResult{
				freezeOK,
				controlItem(t, domain.PrepaidFreezeControlItem, 2, domain.ControlItemSatisfied),
			},
			JointPass: domain.AllControlsPass, AsOf: financialControlAsOf(t),
		},
		"无标识": {
			Items:     []domain.ControlItemResult{freezeOK},
			JointPass: domain.AllControlsPass, AsOf: financialControlAsOf(t),
		},
		"无时点": {
			ResultID:  mustValue(t, domain.NewFinancialControlResultID, "request-1/version-1"),
			Items:     []domain.ControlItemResult{freezeOK},
			JointPass: domain.AllControlsPass,
		},
		"零值项": {
			ResultID:  mustValue(t, domain.NewFinancialControlResultID, "request-1/version-1"),
			Items:     []domain.ControlItemResult{{}},
			JointPass: domain.AllControlsPass, AsOf: financialControlAsOf(t),
		},
	}
	for name, spec := range specs {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewExecutedFinancialControlResult(spec); !errors.Is(err, domain.ErrInvalidFinancialControlResult) {
				t.Fatalf("error = %v, want ErrInvalidFinancialControlResult", err)
			}
		})
	}
}

// Covers: 控制项结果的形状——受限必带该项自己的原因（没有原因的`业务限制`说不出限制什么），
// 成立不带（依据在占用记录本身，带了就是第二处定义）；顺序从 1 起；种类与结论都在封闭集内。
func TestAControlItemResultCarriesABasisExactlyWhenRestricted(t *testing.T) {
	basis := mustValue(t, domain.NewControlBasisReference, "AVAILABLE_BALANCE_INSUFFICIENT")
	if _, err := domain.NewControlItemResult(
		domain.PrepaidFreezeControlItem, 1, domain.ControlItemRestricted, domain.ControlBasisReference{},
	); !errors.Is(err, domain.ErrInvalidControlItemResult) {
		t.Fatalf("受限无依据 error = %v, want ErrInvalidControlItemResult", err)
	}
	if _, err := domain.NewControlItemResult(
		domain.PrepaidFreezeControlItem, 1, domain.ControlItemSatisfied, basis,
	); !errors.Is(err, domain.ErrInvalidControlItemResult) {
		t.Fatalf("成立带依据 error = %v, want ErrInvalidControlItemResult", err)
	}
	if _, err := domain.NewControlItemResult(
		domain.PrepaidFreezeControlItem, 0, domain.ControlItemSatisfied, domain.ControlBasisReference{},
	); !errors.Is(err, domain.ErrInvalidControlItemResult) {
		t.Fatalf("顺序为零 error = %v, want ErrInvalidControlItemResult", err)
	}
	if _, err := domain.NewControlItemResult(
		domain.ControlItemKind(7), 1, domain.ControlItemSatisfied, domain.ControlBasisReference{},
	); !errors.Is(err, domain.ErrInvalidControlItemResult) {
		t.Fatalf("种类集外 error = %v, want ErrInvalidControlItemResult", err)
	}

	restricted, err := domain.NewControlItemResult(domain.CreditCheckControlItem, 2, domain.ControlItemRestricted, basis)
	if err != nil {
		t.Fatalf("new control item result: %v", err)
	}
	if restricted.Kind() != domain.CreditCheckControlItem || restricted.Order() != 2 ||
		restricted.Conclusion() != domain.ControlItemRestricted || restricted.Basis() != basis || restricted.Satisfied() {
		t.Fatalf("item = %+v; 字段没有原样保全", restricted)
	}
}

// Covers: ADR-0027「消费方凭据不持有提供方不曾签发的东西」——`明确无控制`那一支下 settlement-accounting
// 不形成冻结，因而没有结果标识可交回；它必带合同声明的商业不适用依据（没有依据与默认信用通过
// 无从分辨），不带逐项也不带共同通过条件（没有执行过任何一项）。
func TestAnInapplicableControlCarriesItsCommercialBasisAndNothingElse(t *testing.T) {
	basis := mustValue(t, domain.NewControlBasisReference, "CONTRACT_DECLARES_NO_PRE_ACCEPTANCE_CONTROL")
	result, err := domain.NewInapplicableFinancialControlResult(basis, financialControlAsOf(t))
	if err != nil {
		t.Fatalf("new inapplicable financial control result: %v", err)
	}
	if result.Outcome() != domain.FinancialControlNotApplicable || result.Basis() != basis {
		t.Fatalf("outcome = %q basis = %q", result.Outcome(), result.Basis())
	}
	if result.ResultID().String() != "" || len(result.Items()) != 0 ||
		result.JointPassCondition() != domain.JointPassConditionInvalid {
		t.Fatalf("无控制凭空得到了标识、逐项或条件：%+v", result)
	}

	if _, err := domain.NewInapplicableFinancialControlResult(
		domain.ControlBasisReference{}, financialControlAsOf(t),
	); !errors.Is(err, domain.ErrInvalidFinancialControlResult) {
		t.Fatalf("无依据 error = %v, want ErrInvalidFinancialControlResult", err)
	}
	if _, err := domain.NewInapplicableFinancialControlResult(basis, domain.JudgmentAsOf{}); !errors.Is(
		err, domain.ErrInvalidFinancialControlResult,
	) {
		t.Fatalf("无时点 error = %v, want ErrInvalidFinancialControlResult", err)
	}
}

// Covers: ADR-0028「重建只校验不重算」在本值上的落法（ADR-0125 决定二）——重建门把库里记的结论当
// 数据收下，只核它与逐项在所记条件下一致：一致照收，不一致按坏数据拒绝而不是用今天的推导改写；
// `明确无控制`记了逐项或条件同样是坏数据。
func TestRehydrationValidatesTheRecordedConclusionAgainstTheItems(t *testing.T) {
	creditOK := controlItem(t, domain.CreditCheckControlItem, 1, domain.ControlItemSatisfied)
	consistent, err := domain.RehydrateFinancialControlResult(domain.RehydrateFinancialControlResultSpec{
		ResultID:  mustValue(t, domain.NewFinancialControlResultID, "request-1/version-1"),
		Outcome:   domain.FinancialControlCreditExposed,
		Items:     []domain.ControlItemResult{creditOK},
		JointPass: domain.AllControlsPass,
		AsOf:      financialControlAsOf(t),
	})
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if consistent.Outcome() != domain.FinancialControlCreditExposed || len(consistent.Items()) != 1 {
		t.Fatalf("rehydrated = %+v", consistent)
	}

	if _, err := domain.RehydrateFinancialControlResult(domain.RehydrateFinancialControlResultSpec{
		ResultID:  mustValue(t, domain.NewFinancialControlResultID, "request-1/version-1"),
		Outcome:   domain.FinancialControlHeld,
		Items:     []domain.ControlItemResult{creditOK},
		JointPass: domain.AllControlsPass,
		AsOf:      financialControlAsOf(t),
	}); !errors.Is(err, domain.ErrInvalidRehydratedFinancialControlResult) {
		t.Fatalf("记 HELD 而逐项只有信用成立 error = %v, want ErrInvalidRehydratedFinancialControlResult", err)
	}

	restrictedBasis := mustValue(t, domain.NewControlBasisReference, "OTHER_REASON")
	if _, err := domain.RehydrateFinancialControlResult(domain.RehydrateFinancialControlResultSpec{
		ResultID:  mustValue(t, domain.NewFinancialControlResultID, "request-1/version-1"),
		Outcome:   domain.FinancialControlRestricted,
		Basis:     restrictedBasis,
		Items:     []domain.ControlItemResult{controlItem(t, domain.CreditCheckControlItem, 1, domain.ControlItemRestricted)},
		JointPass: domain.AllControlsPass,
		AsOf:      financialControlAsOf(t),
	}); !errors.Is(err, domain.ErrInvalidRehydratedFinancialControlResult) {
		t.Fatalf("记的依据不是受限项自己的原因 error = %v, want ErrInvalidRehydratedFinancialControlResult", err)
	}

	if _, err := domain.RehydrateFinancialControlResult(domain.RehydrateFinancialControlResultSpec{
		Outcome:   domain.FinancialControlNotApplicable,
		Basis:     mustValue(t, domain.NewControlBasisReference, "CONTRACT_DECLARES_NO_PRE_ACCEPTANCE_CONTROL"),
		Items:     []domain.ControlItemResult{creditOK},
		JointPass: domain.AllControlsPass,
		AsOf:      financialControlAsOf(t),
	}); !errors.Is(err, domain.ErrInvalidRehydratedFinancialControlResult) {
		t.Fatalf("明确无控制带了逐项 error = %v, want ErrInvalidRehydratedFinancialControlResult", err)
	}
}
