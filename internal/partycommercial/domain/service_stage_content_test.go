package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func authorizationRule(t *testing.T) domain.CommercialVersion {
	t.Helper()
	published, err := commercialDraft(t, domain.AuthorizationRuleObject, "auth-1", "v1", "sha256:auth-1").
		Publish(approval(t, "approval-auth-1"), domain.ApprovalRoleConfirmed, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatalf("publish authorization rule: %v", err)
	}
	live, err := published.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	return live
}

// Covers: PAR-COM-16 的机制半边——收寄资格声明允许来源至少一格且不重（重复声明是
// 冲突不是宽容），资格项可为显式空清单（真没有硬资格也要声明出来），非法引用是缺件；
// 声明缺件的恢复动作是补规则正文（实例半边），不是本上下文代拟默认。拥有对象钉在已
// 生效接单规则包上（ADR-0058）。
func TestIntakeQualificationContentDemandsExplicitSources(t *testing.T) {
	owner := rulePackage(t)
	content, err := domain.NewIntakeQualificationContent(
		owner,
		[]domain.DeclaredIntakeSource{domain.DeclaredNodeIntake},
		[]domain.RuleReference{commercialValue(t, domain.NewRuleReference, "INTAKE-QUAL/customs-precheck")},
	)
	if err != nil {
		t.Fatalf("new intake qualification content: %v", err)
	}
	if !content.Owner().SameVersionAs(owner) {
		t.Fatal("声明没有钉住它所属的规则包")
	}
	if !content.Allows(domain.DeclaredNodeIntake) {
		t.Fatal("声明允许的来源答不允许")
	}
	if content.Allows(domain.DeclaredOffsitePickup) {
		t.Fatal("没声明的来源答允许——缺行不是放行")
	}
	if len(content.Qualifications()) != 1 {
		t.Fatalf("qualifications = %d", len(content.Qualifications()))
	}

	if _, err := domain.NewIntakeQualificationContent(owner, nil, nil); !errors.Is(err, domain.ErrIntakeContentNotConfigured) {
		t.Fatalf("err = %v; 一个来源都不允许的声明被收下了", err)
	}
	if _, err := domain.NewIntakeQualificationContent(
		owner,
		[]domain.DeclaredIntakeSource{domain.DeclaredNodeIntake, domain.DeclaredNodeIntake},
		nil,
	); !errors.Is(err, domain.ErrConflictingIntakeSource) {
		t.Fatalf("err = %v; 重复声明的来源被收下了", err)
	}

	explicit, err := domain.NewIntakeQualificationContent(
		owner,
		[]domain.DeclaredIntakeSource{domain.DeclaredNodeIntake, domain.DeclaredOffsitePickup},
		[]domain.RuleReference{},
	)
	if err != nil {
		t.Fatalf("new content with empty qualifications: %v", err)
	}
	if len(explicit.Qualifications()) != 0 {
		t.Fatal("显式空清单被改写了")
	}

	t.Run("a draft rule package cannot carry the declaration", func(t *testing.T) {
		draft := commercialDraft(t, domain.AcceptanceRulePackageObject, "rules-draft", "v1", "sha256:rules-draft")
		if _, err := domain.NewIntakeQualificationContent(
			draft,
			[]domain.DeclaredIntakeSource{domain.DeclaredNodeIntake},
			nil,
		); !errors.Is(err, domain.ErrUnusableRulePackage) {
			t.Fatalf("error = %v, want ErrUnusableRulePackage", err)
		}
	})

	t.Run("a contract is not a rule package", func(t *testing.T) {
		contract := effectiveVersionInScope(t, domain.CustomerContractObject, "contract-iq", "v1", "sha256:contract-iq", "scope-iq")
		if _, err := domain.NewIntakeQualificationContent(
			contract,
			[]domain.DeclaredIntakeSource{domain.DeclaredNodeIntake},
			nil,
		); !errors.Is(err, domain.ErrUnusableRulePackage) {
			t.Fatalf("error = %v, want ErrUnusableRulePackage", err)
		}
	})
}

// Covers: PAR-COM-17 的机制半边与 UC-PS-004「有效交付不在所有产品中自动等于终局」——
// 缺行是真话（此规则包下这种结果不形成终局），不是配置缺件；同一责任结果声明两行是
// 冲突；零行声明是缺件（没声明不等于永不终局）。拥有对象钉在已生效接单规则包上。
func TestFinalRuleContentSpeaksByDeclaredRowsOnly(t *testing.T) {
	owner := rulePackage(t)
	content, err := domain.NewFinalRuleContent(owner, []domain.FinalizationDeclaration{
		{
			Outcome:   domain.DeclaredEffectiveDelivery,
			FinalKind: commercialValue(t, domain.NewRuleReference, "NETWORK_SERVICE_DELIVERED"),
		},
		{
			Outcome:   domain.DeclaredReturnCompleted,
			FinalKind: commercialValue(t, domain.NewRuleReference, "NETWORK_SERVICE_RETURNED"),
		},
	})
	if err != nil {
		t.Fatalf("new final rule content: %v", err)
	}
	if !content.Owner().SameVersionAs(owner) {
		t.Fatal("声明没有钉住它所属的规则包")
	}

	kind, declared := content.FinalKindFor(domain.DeclaredEffectiveDelivery)
	if !declared || kind.String() != "NETWORK_SERVICE_DELIVERED" {
		t.Fatalf("kind = %v declared = %v", kind, declared)
	}
	if _, declared := content.FinalKindFor(domain.DeclaredRegulatoryDisposition); declared {
		t.Fatal("没声明的责任结果答成了形成终局——缺行是真话")
	}

	if _, err := domain.NewFinalRuleContent(owner, nil); !errors.Is(err, domain.ErrFinalContentNotConfigured) {
		t.Fatalf("err = %v; 零行声明被收下了", err)
	}
	if _, err := domain.NewFinalRuleContent(owner, []domain.FinalizationDeclaration{
		{Outcome: domain.DeclaredEffectiveDelivery, FinalKind: commercialValue(t, domain.NewRuleReference, "A")},
		{Outcome: domain.DeclaredEffectiveDelivery, FinalKind: commercialValue(t, domain.NewRuleReference, "B")},
	}); !errors.Is(err, domain.ErrConflictingFinalization) {
		t.Fatalf("err = %v; 同一结果两行声明分不出真假", err)
	}

	t.Run("a product is not a rule package", func(t *testing.T) {
		product := effectiveServiceProduct(t)
		if _, err := domain.NewFinalRuleContent(product, []domain.FinalizationDeclaration{
			{Outcome: domain.DeclaredEffectiveDelivery, FinalKind: commercialValue(t, domain.NewRuleReference, "A")},
		}); !errors.Is(err, domain.ErrUnusableRulePackage) {
			t.Fatalf("error = %v, want ErrUnusableRulePackage", err)
		}
	})
}

// Covers: PC CONTEXT「面单服务终局规则」词条——规则可为其声明终局的责任结果含面单渠道服务的
// 非取消终局结果与终局失败结果两格（票 party-commercial-context-gaps/12）。两格各可声明一行、
// 原词照裁决取 LABEL_SERVICE_COMPLETED / LABEL_SERVICE_FAILED；同一格两行仍是冲突；只声明网络
// 服务各格时面单两格缺行——缺行是真话，PS 适配器据以答「此产品下不形成终局」而不是「未配置」。
func TestFinalRuleContentDeclaresLabelServiceOutcomes(t *testing.T) {
	owner := rulePackage(t)
	if got := domain.DeclaredLabelServiceCompleted.String(); got != "LABEL_SERVICE_COMPLETED" {
		t.Fatalf("DeclaredLabelServiceCompleted.String() = %q", got)
	}
	if got := domain.DeclaredLabelServiceFailed.String(); got != "LABEL_SERVICE_FAILED" {
		t.Fatalf("DeclaredLabelServiceFailed.String() = %q", got)
	}

	content, err := domain.NewFinalRuleContent(owner, []domain.FinalizationDeclaration{
		{Outcome: domain.DeclaredLabelServiceFailed, FinalKind: commercialValue(t, domain.NewRuleReference, "LABEL_SERVICE_FAILED_FINAL")},
		{Outcome: domain.DeclaredLabelServiceCompleted, FinalKind: commercialValue(t, domain.NewRuleReference, "LABEL_SERVICE_DONE")},
		{Outcome: domain.DeclaredEffectiveDelivery, FinalKind: commercialValue(t, domain.NewRuleReference, "NETWORK_SERVICE_DELIVERED")},
	})
	if err != nil {
		t.Fatalf("new final rule content with label service rows: %v", err)
	}
	if kind, declared := content.FinalKindFor(domain.DeclaredLabelServiceCompleted); !declared || kind.String() != "LABEL_SERVICE_DONE" {
		t.Fatalf("completed: kind = %v declared = %v", kind, declared)
	}
	if kind, declared := content.FinalKindFor(domain.DeclaredLabelServiceFailed); !declared || kind.String() != "LABEL_SERVICE_FAILED_FINAL" {
		t.Fatalf("failed: kind = %v declared = %v", kind, declared)
	}
	declarations := content.Declarations()
	if len(declarations) != 3 ||
		declarations[0].Outcome != domain.DeclaredEffectiveDelivery ||
		declarations[1].Outcome != domain.DeclaredLabelServiceCompleted ||
		declarations[2].Outcome != domain.DeclaredLabelServiceFailed {
		t.Fatalf("declarations = %#v; 网络服务各格在前、面单两格随后，顺序稳定", declarations)
	}

	if _, err := domain.NewFinalRuleContent(owner, []domain.FinalizationDeclaration{
		{Outcome: domain.DeclaredLabelServiceCompleted, FinalKind: commercialValue(t, domain.NewRuleReference, "A")},
		{Outcome: domain.DeclaredLabelServiceCompleted, FinalKind: commercialValue(t, domain.NewRuleReference, "B")},
	}); !errors.Is(err, domain.ErrConflictingFinalization) {
		t.Fatalf("err = %v; 面单同一格两行声明分不出真假", err)
	}

	networkOnly, err := domain.NewFinalRuleContent(owner, []domain.FinalizationDeclaration{
		{Outcome: domain.DeclaredEffectiveDelivery, FinalKind: commercialValue(t, domain.NewRuleReference, "NETWORK_SERVICE_DELIVERED")},
	})
	if err != nil {
		t.Fatalf("new network-only content: %v", err)
	}
	for _, outcome := range []domain.DeclaredResponsibilityOutcome{domain.DeclaredLabelServiceCompleted, domain.DeclaredLabelServiceFailed} {
		if _, declared := networkOnly.FinalKindFor(outcome); declared {
			t.Fatalf("%s: 只声明网络服务格的规则包替面单格答成了形成终局", outcome)
		}
	}
}

// Covers: PAR-COM-17 取消授权目录的机制半边——有行即允许带规则引用；缺行是真话
// （此授权规则下这种请求方不许取消），不是配置缺件；同一请求方格两行是冲突；零行是
// 缺件（没声明不等于永不允许，更不等于默认客户可取消）。拥有对象钉在已生效授权规则上。
func TestCancellationAuthorityContentSpeaksByDeclaredRowsOnly(t *testing.T) {
	owner := authorizationRule(t)
	content, err := domain.NewCancellationAuthorityContent(owner, []domain.CancellationAuthorityDeclaration{
		{
			Party: domain.DeclaredCustomerCancellation,
			Rule:  commercialValue(t, domain.NewRuleReference, "CANCEL-RULE/CUSTOMER"),
		},
	})
	if err != nil {
		t.Fatalf("new cancellation authority content: %v", err)
	}
	if !content.Owner().SameVersionAs(owner) {
		t.Fatal("目录没有钉住它所属的授权规则")
	}

	rule, declared := content.RuleFor(domain.DeclaredCustomerCancellation)
	if !declared || rule.String() != "CANCEL-RULE/CUSTOMER" {
		t.Fatalf("rule = %v declared = %v", rule, declared)
	}
	if _, declared := content.RuleFor(domain.DeclaredOperationsCancellation); declared {
		t.Fatal("没声明的请求方格答成了允许——缺行是真话")
	}

	if _, err := domain.NewCancellationAuthorityContent(owner, nil); !errors.Is(err, domain.ErrCancellationAuthorityNotConfigured) {
		t.Fatalf("err = %v; 零行声明被收下了", err)
	}
	if _, err := domain.NewCancellationAuthorityContent(owner, []domain.CancellationAuthorityDeclaration{
		{Party: domain.DeclaredCustomerCancellation, Rule: commercialValue(t, domain.NewRuleReference, "A")},
		{Party: domain.DeclaredCustomerCancellation, Rule: commercialValue(t, domain.NewRuleReference, "B")},
	}); !errors.Is(err, domain.ErrConflictingCancellationAuthority) {
		t.Fatalf("err = %v; 同一请求方格两行声明分不出真假", err)
	}
	if _, err := domain.NewCancellationAuthorityContent(owner, []domain.CancellationAuthorityDeclaration{
		{Party: domain.DeclaredCancellationPartyInvalid, Rule: commercialValue(t, domain.NewRuleReference, "A")},
	}); !errors.Is(err, domain.ErrCancellationAuthorityNotConfigured) {
		t.Fatalf("err = %v; 零值请求方格被收下了", err)
	}

	t.Run("a contract is not an authorization rule", func(t *testing.T) {
		contract := effectiveVersionInScope(t, domain.CustomerContractObject, "contract-ca", "v1", "sha256:contract-ca", "scope-ca")
		if _, err := domain.NewCancellationAuthorityContent(contract, []domain.CancellationAuthorityDeclaration{
			{Party: domain.DeclaredCustomerCancellation, Rule: commercialValue(t, domain.NewRuleReference, "A")},
		}); !errors.Is(err, domain.ErrUnusableAuthorizationRule) {
			t.Fatalf("error = %v, want ErrUnusableAuthorizationRule", err)
		}
	})
}
