package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func effectiveCustomerContract(t *testing.T) domain.CommercialVersion {
	t.Helper()
	published, err := commercialDraft(t, domain.CustomerContractObject, "contract-pac", "v1", "sha256:contract-pac").
		Publish(approval(t, "approval-contract-pac"), domain.ApprovalRoleConfirmed, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatalf("publish contract: %v", err)
	}
	live, err := published.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	return live
}

func notApplicableBasis(t *testing.T) domain.ControlNotApplicableBasis {
	t.Helper()
	return commercialValue(t, domain.NewControlNotApplicableBasis, "CONTRACT-CLAUSE/NO-PRE-ACCEPTANCE-CONTROL")
}

// Covers: CONTEXT「确实不适用的控制必须按明确范围记录不适用依据。规则或策略缺失不得被解释
// 为允许接受」与 CONTEXT-MAP「合同明确无接受前财务控制时必须保存商业不适用依据，不能用
// 缺失结果或默认通过代替」——本口只答「要不要」（ADR-0042 归属、ADR-0054 未配置格）；
// 方式与采用政策不在这里（ADR-0044）。
func TestCustomerContractDeclaresWhetherPreAcceptanceControlApplies(t *testing.T) {
	contract := effectiveCustomerContract(t)

	required, err := domain.DeclarePreAcceptanceControl(
		contract, domain.PreAcceptanceControlRequired, domain.ControlNotApplicableBasis{})
	if err != nil {
		t.Fatalf("declare required control: %v", err)
	}
	if required.Requirement() != domain.PreAcceptanceControlRequired {
		t.Fatalf("requirement = %q, want REQUIRED", required.Requirement())
	}
	if _, notApplicable := required.NotApplicableBasis(); notApplicable {
		t.Fatal("`要求控制`被读成了`不适用`——两格分不开就会从结算方式倒推要不要控制")
	}
	if required.Contract().Kind() != domain.CustomerContractObject {
		t.Fatal("声明没有钉住它所属的客户合同")
	}

	notApplicable, err := domain.DeclarePreAcceptanceControl(
		contract, domain.PreAcceptanceControlNotApplicable, notApplicableBasis(t))
	if err != nil {
		t.Fatalf("declare not-applicable control: %v", err)
	}
	if notApplicable.Requirement() != domain.PreAcceptanceControlNotApplicable {
		t.Fatalf("requirement = %q, want NOT_APPLICABLE", notApplicable.Requirement())
	}
	basis, ok := notApplicable.NotApplicableBasis()
	if !ok || basis.String() != "CONTRACT-CLAUSE/NO-PRE-ACCEPTANCE-CONTROL" {
		t.Fatal("`不适用`丢了依据——没有依据的不适用与一次默认放行分不开")
	}

	t.Run("the zero value is undeclared, not a pass", func(t *testing.T) {
		var zero domain.PreAcceptanceControlDeclaration
		if zero.Requirement().Declared() {
			t.Fatal("零值声明被读成已声明——未配置成了默认通过")
		}
		if _, notApplicable := zero.NotApplicableBasis(); notApplicable {
			t.Fatal("零值声明被读成`不适用`")
		}
	})

	t.Run("required control cannot carry a not-applicable basis", func(t *testing.T) {
		if _, err := domain.DeclarePreAcceptanceControl(
			contract, domain.PreAcceptanceControlRequired, notApplicableBasis(t),
		); !errors.Is(err, domain.ErrPreAcceptanceControlNotDeclared) {
			t.Fatalf("error = %v, want ErrPreAcceptanceControlNotDeclared", err)
		}
	})

	t.Run("not applicable without a basis is not declared", func(t *testing.T) {
		if _, err := domain.DeclarePreAcceptanceControl(
			contract, domain.PreAcceptanceControlNotApplicable, domain.ControlNotApplicableBasis{},
		); !errors.Is(err, domain.ErrPreAcceptanceControlNotDeclared) {
			t.Fatalf("error = %v, want ErrPreAcceptanceControlNotDeclared", err)
		}
	})

	t.Run("an undeclared requirement is not a declaration", func(t *testing.T) {
		if _, err := domain.DeclarePreAcceptanceControl(
			contract, domain.PreAcceptanceControlUndeclared, domain.ControlNotApplicableBasis{},
		); !errors.Is(err, domain.ErrPreAcceptanceControlNotDeclared) {
			t.Fatalf("error = %v, want ErrPreAcceptanceControlNotDeclared", err)
		}
	})

	t.Run("a draft contract cannot carry the declaration", func(t *testing.T) {
		draft := commercialDraft(t, domain.CustomerContractObject, "contract-draft", "v1", "sha256:contract-draft")
		if _, err := domain.DeclarePreAcceptanceControl(
			draft, domain.PreAcceptanceControlRequired, domain.ControlNotApplicableBasis{},
		); !errors.Is(err, domain.ErrUnusableContract) {
			t.Fatalf("error = %v, want ErrUnusableContract", err)
		}
	})

	t.Run("a published but not yet effective contract cannot carry it", func(t *testing.T) {
		published, err := commercialDraft(t, domain.CustomerContractObject, "contract-pub", "v1", "sha256:contract-pub").
			Publish(approval(t, "approval-contract-pub"), domain.ApprovalRoleConfirmed, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), nil)
		if err != nil {
			t.Fatalf("publish contract: %v", err)
		}
		if _, err := domain.DeclarePreAcceptanceControl(
			published, domain.PreAcceptanceControlRequired, domain.ControlNotApplicableBasis{},
		); !errors.Is(err, domain.ErrUnusableContract) {
			t.Fatalf("error = %v, want ErrUnusableContract", err)
		}
	})

	t.Run("a rule package is not a customer contract", func(t *testing.T) {
		if _, err := domain.DeclarePreAcceptanceControl(
			rulePackage(t), domain.PreAcceptanceControlRequired, domain.ControlNotApplicableBasis{},
		); !errors.Is(err, domain.ErrUnusableContract) {
			t.Fatalf("error = %v, want ErrUnusableContract", err)
		}
	})
}

func TestControlNotApplicableBasisRejectsBlank(t *testing.T) {
	if _, err := domain.NewControlNotApplicableBasis("  "); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("error = %v, want ErrBlankValue", err)
	}
}
