package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func controlItem(
	t *testing.T,
	kind domain.PreAcceptanceControlKind,
	scope string,
	order int,
	disposition domain.ControlFailureDisposition,
) domain.PreAcceptanceControlItem {
	t.Helper()
	item, err := domain.NewPreAcceptanceControlItem(
		kind,
		commercialValue(t, domain.NewChargeScopeReference, scope),
		order,
		disposition,
		commercialValue(t, domain.NewControlResponsibilityReference, "responsibility-"+scope),
	)
	if err != nil {
		t.Fatalf("control item: %v", err)
	}
	return item
}

// Covers: ADR-0115 Decision 一——正文只表达要执行的控制项，种类封闭两值且是行的键；以及类别那一格
// 在构造门上的落点（判据同 NewCustomerServiceRuleVersion：挂错类别的版本入册与被选中都不报错，
// 错要到下游取不到控制项时才显形，而那时它长得像「这个合同没配控制」）。
func TestPreAcceptanceFinancialControlPolicyRefusesAVersionOfAnotherKind(t *testing.T) {
	items := []domain.PreAcceptanceControlItem{
		controlItem(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure),
	}

	t.Run("a control policy version is accepted", func(t *testing.T) {
		policy, err := domain.NewPreAcceptanceFinancialControlPolicy(
			effectiveVersionOfKind(t, domain.PreAcceptanceFinancialControlPolicyObject, "fcp-1"),
			domain.AllControlsPass, items,
		)
		if err != nil {
			t.Fatalf("new control policy: %v", err)
		}
		if policy.JointPassCondition() != domain.AllControlsPass {
			t.Fatal("共同通过条件没有原样留在正文上")
		}
		if got := policy.Items(); len(got) != 1 || got[0].Kind() != domain.PrepaidFreezeControl {
			t.Fatalf("控制项变形：%#v", got)
		}
	})

	t.Run("a version of another kind is refused", func(t *testing.T) {
		// 信用政策也是集内成员、也能生效，两者在版本壳上唯一的差别就是类别。
		_, err := domain.NewPreAcceptanceFinancialControlPolicy(
			effectiveVersionOfKind(t, domain.CreditPolicyObject, "credit-1"),
			domain.AllControlsPass, items,
		)
		if !errors.Is(err, domain.ErrInvalidPreAcceptanceFinancialControlPolicy) {
			t.Fatalf("error = %v，别的类别的版本挂成了控制策略", err)
		}
	})

	t.Run("a draft version cannot carry the body", func(t *testing.T) {
		draft := registerable(t, domain.PreAcceptanceFinancialControlPolicyObject, "fcp-draft", "v1", "sha256:fcp-draft")
		_, err := domain.NewPreAcceptanceFinancialControlPolicy(draft, domain.AllControlsPass, items)
		if !errors.Is(err, domain.ErrInvalidPreAcceptanceFinancialControlPolicy) {
			t.Fatalf("error = %v，未生效的版本承载了正文", err)
		}
	})

	t.Run("an unstated joint pass condition is refused", func(t *testing.T) {
		// 零值不是「默认全部通过」：CONTEXT 要求策略明确共同通过条件，没说就是没说。
		_, err := domain.NewPreAcceptanceFinancialControlPolicy(
			effectiveVersionOfKind(t, domain.PreAcceptanceFinancialControlPolicyObject, "fcp-2"),
			domain.JointPassConditionInvalid, items,
		)
		if !errors.Is(err, domain.ErrInvalidPreAcceptanceFinancialControlPolicy) {
			t.Fatalf("error = %v，没说共同通过条件的策略立住了", err)
		}
	})
}

// Covers: ADR-0115 Decision 一「至少一项」与 Decision 二「顺序唯一、（种类 × 范围）唯一」——零项不是
// 显式无控制（那由合同声明），同键两行或两行抢一个顺序都答不出该按哪条；以及 Items 按判断顺序交回。
func TestPreAcceptanceFinancialControlPolicyCarriesAtLeastOneItemWithUniqueKeysAndOrder(t *testing.T) {
	version := effectiveVersionOfKind(t, domain.PreAcceptanceFinancialControlPolicyObject, "fcp-3")

	t.Run("no item at all is refused", func(t *testing.T) {
		_, err := domain.NewPreAcceptanceFinancialControlPolicy(version, domain.AllControlsPass, nil)
		if !errors.Is(err, domain.ErrInvalidPreAcceptanceFinancialControlPolicy) {
			t.Fatalf("error = %v，一项控制都没有的策略立住了", err)
		}
	})

	t.Run("a combination is filed in evaluation order", func(t *testing.T) {
		policy, err := domain.NewPreAcceptanceFinancialControlPolicy(version, domain.AllControlsPass,
			[]domain.PreAcceptanceControlItem{
				controlItem(t, domain.CreditCheckControl, "charge-scope-a", 2, domain.AuthorizedDispositionOnControlFailure),
				controlItem(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure),
				controlItem(t, domain.CreditCheckControl, "charge-scope-b", 3, domain.RejectOnControlFailure),
			})
		if err != nil {
			t.Fatalf("组合控制：%v", err)
		}
		items := policy.Items()
		if len(items) != 3 || items[0].EvaluationOrder() != 1 || items[1].EvaluationOrder() != 2 || items[2].EvaluationOrder() != 3 {
			t.Fatalf("控制项没有按判断顺序交回：%#v", items)
		}
		if items[0].Kind() != domain.PrepaidFreezeControl || items[1].FailureDisposition() != domain.AuthorizedDispositionOnControlFailure {
			t.Fatalf("控制项内容变形：%#v", items)
		}
		scopeA := commercialValue(t, domain.NewChargeScopeReference, "charge-scope-a")
		if got := policy.ItemsFor(scopeA); len(got) != 2 || got[0].Kind() != domain.PrepaidFreezeControl || got[1].Kind() != domain.CreditCheckControl {
			t.Fatalf("按范围取控制项变形：%#v", got)
		}
		if got := policy.ItemsFor(commercialValue(t, domain.NewChargeScopeReference, "charge-scope-none")); len(got) != 0 {
			t.Fatalf("没规定控制的范围读出了控制项：%#v", got)
		}
	})

	t.Run("the same control on the same scope twice is refused", func(t *testing.T) {
		_, err := domain.NewPreAcceptanceFinancialControlPolicy(version, domain.AllControlsPass,
			[]domain.PreAcceptanceControlItem{
				controlItem(t, domain.CreditCheckControl, "charge-scope-a", 1, domain.RejectOnControlFailure),
				controlItem(t, domain.CreditCheckControl, "charge-scope-a", 2, domain.AuthorizedDispositionOnControlFailure),
			})
		if !errors.Is(err, domain.ErrDuplicatePreAcceptanceControlItem) {
			t.Fatalf("error = %v，同一范围上同一种控制两行立住了", err)
		}
	})

	t.Run("two items claiming one evaluation order are refused", func(t *testing.T) {
		_, err := domain.NewPreAcceptanceFinancialControlPolicy(version, domain.AllControlsPass,
			[]domain.PreAcceptanceControlItem{
				controlItem(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure),
				controlItem(t, domain.CreditCheckControl, "charge-scope-a", 1, domain.RejectOnControlFailure),
			})
		if !errors.Is(err, domain.ErrDuplicatePreAcceptanceControlItem) {
			t.Fatalf("error = %v，两行抢同一个判断顺序立住了", err)
		}
	})

	t.Run("a zero-value item smuggled past the constructor is refused", func(t *testing.T) {
		_, err := domain.NewPreAcceptanceFinancialControlPolicy(version, domain.AllControlsPass,
			[]domain.PreAcceptanceControlItem{{}})
		if !errors.Is(err, domain.ErrInvalidPreAcceptanceControlItem) {
			t.Fatalf("error = %v，零值控制项立住了", err)
		}
	})
}

// Covers: ADR-0115 Decision 三的槽位形态——种类与处置在集内、范围与责任引用非空、顺序为正；失败处置
// 两值逐字对应 UC-PS-001「按策略拒绝或进入授权处置」，零值不落进任何一格。
func TestAPreAcceptanceControlItemOnlyChecksShape(t *testing.T) {
	scope := commercialValue(t, domain.NewChargeScopeReference, "charge-scope-a")
	responsibility := commercialValue(t, domain.NewControlResponsibilityReference, "customer")

	for name, attempt := range map[string]func() (domain.PreAcceptanceControlItem, error){
		"kind outside the closed set": func() (domain.PreAcceptanceControlItem, error) {
			return domain.NewPreAcceptanceControlItem(domain.PreAcceptanceControlKindInvalid, scope, 1, domain.RejectOnControlFailure, responsibility)
		},
		"blank scope": func() (domain.PreAcceptanceControlItem, error) {
			return domain.NewPreAcceptanceControlItem(domain.CreditCheckControl, domain.ChargeScopeReference{}, 1, domain.RejectOnControlFailure, responsibility)
		},
		"zero order": func() (domain.PreAcceptanceControlItem, error) {
			return domain.NewPreAcceptanceControlItem(domain.CreditCheckControl, scope, 0, domain.RejectOnControlFailure, responsibility)
		},
		"negative order": func() (domain.PreAcceptanceControlItem, error) {
			return domain.NewPreAcceptanceControlItem(domain.CreditCheckControl, scope, -1, domain.RejectOnControlFailure, responsibility)
		},
		"unstated disposition": func() (domain.PreAcceptanceControlItem, error) {
			return domain.NewPreAcceptanceControlItem(domain.CreditCheckControl, scope, 1, domain.ControlFailureDispositionInvalid, responsibility)
		},
		"blank responsibility": func() (domain.PreAcceptanceControlItem, error) {
			return domain.NewPreAcceptanceControlItem(domain.CreditCheckControl, scope, 1, domain.RejectOnControlFailure, domain.ControlResponsibilityReference{})
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := attempt(); !errors.Is(err, domain.ErrInvalidPreAcceptanceControlItem) {
				t.Fatalf("error = %v，立不住的控制项立住了", err)
			}
		})
	}

	item, err := domain.NewPreAcceptanceControlItem(domain.CreditCheckControl, scope, 7, domain.AuthorizedDispositionOnControlFailure, responsibility)
	if err != nil {
		t.Fatalf("合法控制项：%v", err)
	}
	if item.Kind() != domain.CreditCheckControl || item.Scope() != scope || item.EvaluationOrder() != 7 ||
		item.FailureDisposition() != domain.AuthorizedDispositionOnControlFailure || item.Responsibility() != responsibility {
		t.Fatalf("控制项内容变形：%#v", item)
	}

	// 封闭集的名字是批文与库列共用的镜像；零值的名字为空，谁把零值写出去都会在下一道门被拒。
	if domain.PrepaidFreezeControl.String() != "PREPAID_FREEZE" || domain.CreditCheckControl.String() != "CREDIT_CHECK" ||
		domain.PreAcceptanceControlKindInvalid.String() != "" {
		t.Fatal("控制种类的名字与封闭集不一致")
	}
	if domain.RejectOnControlFailure.String() != "REJECT" || domain.AuthorizedDispositionOnControlFailure.String() != "AUTHORIZED_DISPOSITION" ||
		domain.ControlFailureDispositionInvalid.String() != "" {
		t.Fatal("失败处置的名字与封闭集不一致")
	}
	if domain.AllControlsPass.String() != "ALL_CONTROLS_PASS" || domain.JointPassConditionInvalid.String() != "" {
		t.Fatal("共同通过条件的名字与封闭集不一致")
	}
}
