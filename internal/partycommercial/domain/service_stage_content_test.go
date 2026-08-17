package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: PAR-COM-16 的机制半边——收寄资格声明允许来源至少一格且不重（重复声明是
// 冲突不是宽容），资格项可为显式空清单（真没有硬资格也要声明出来），非法引用是缺件；
// 声明缺件的恢复动作是补规则正文（实例半边），不是本上下文代拟默认。
func TestIntakeQualificationContentDemandsExplicitSources(t *testing.T) {
	content, err := domain.NewIntakeQualificationContent(
		[]domain.DeclaredIntakeSource{domain.DeclaredNodeIntake},
		[]domain.RuleReference{commercialValue(t, domain.NewRuleReference, "INTAKE-QUAL/customs-precheck")},
	)
	if err != nil {
		t.Fatalf("new intake qualification content: %v", err)
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

	if _, err := domain.NewIntakeQualificationContent(nil, nil); !errors.Is(err, domain.ErrIntakeContentNotConfigured) {
		t.Fatalf("err = %v; 一个来源都不允许的声明被收下了", err)
	}
	if _, err := domain.NewIntakeQualificationContent(
		[]domain.DeclaredIntakeSource{domain.DeclaredNodeIntake, domain.DeclaredNodeIntake},
		nil,
	); !errors.Is(err, domain.ErrConflictingIntakeSource) {
		t.Fatalf("err = %v; 重复声明的来源被收下了", err)
	}

	explicit, err := domain.NewIntakeQualificationContent(
		[]domain.DeclaredIntakeSource{domain.DeclaredNodeIntake, domain.DeclaredOffsitePickup},
		[]domain.RuleReference{},
	)
	if err != nil {
		t.Fatalf("new content with empty qualifications: %v", err)
	}
	if len(explicit.Qualifications()) != 0 {
		t.Fatal("显式空清单被改写了")
	}
}

// Covers: PAR-COM-17 的机制半边与 UC-PS-004「有效交付不在所有产品中自动等于终局」——
// 缺行是真话（此产品下这种结果不形成终局），不是配置缺件；同一责任结果声明两行是
// 冲突；零行声明是缺件（没声明不等于永不终局）。
func TestFinalRuleContentSpeaksByDeclaredRowsOnly(t *testing.T) {
	content, err := domain.NewFinalRuleContent([]domain.FinalizationDeclaration{
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

	kind, declared := content.FinalKindFor(domain.DeclaredEffectiveDelivery)
	if !declared || kind.String() != "NETWORK_SERVICE_DELIVERED" {
		t.Fatalf("kind = %v declared = %v", kind, declared)
	}
	if _, declared := content.FinalKindFor(domain.DeclaredRegulatoryDisposition); declared {
		t.Fatal("没声明的责任结果答成了形成终局——缺行是真话")
	}

	if _, err := domain.NewFinalRuleContent(nil); !errors.Is(err, domain.ErrFinalContentNotConfigured) {
		t.Fatalf("err = %v; 零行声明被收下了", err)
	}
	if _, err := domain.NewFinalRuleContent([]domain.FinalizationDeclaration{
		{Outcome: domain.DeclaredEffectiveDelivery, FinalKind: commercialValue(t, domain.NewRuleReference, "A")},
		{Outcome: domain.DeclaredEffectiveDelivery, FinalKind: commercialValue(t, domain.NewRuleReference, "B")},
	}); !errors.Is(err, domain.ErrConflictingFinalization) {
		t.Fatalf("err = %v; 同一结果两行声明分不出真假", err)
	}
}

// Covers: PAR-COM-17 取消授权目录的机制半边——有行即允许带规则引用；缺行是真话
// （此产品下这种请求方不许取消），不是配置缺件；同一请求方格两行是冲突；零行是缺件
// （没声明不等于永不允许，更不等于默认客户可取消）。
func TestCancellationAuthorityContentSpeaksByDeclaredRowsOnly(t *testing.T) {
	content, err := domain.NewCancellationAuthorityContent([]domain.CancellationAuthorityDeclaration{
		{
			Party: domain.DeclaredCustomerCancellation,
			Rule:  commercialValue(t, domain.NewRuleReference, "CANCEL-RULE/CUSTOMER"),
		},
	})
	if err != nil {
		t.Fatalf("new cancellation authority content: %v", err)
	}

	rule, declared := content.RuleFor(domain.DeclaredCustomerCancellation)
	if !declared || rule.String() != "CANCEL-RULE/CUSTOMER" {
		t.Fatalf("rule = %v declared = %v", rule, declared)
	}
	if _, declared := content.RuleFor(domain.DeclaredOperationsCancellation); declared {
		t.Fatal("没声明的请求方格答成了允许——缺行是真话")
	}

	if _, err := domain.NewCancellationAuthorityContent(nil); !errors.Is(err, domain.ErrCancellationAuthorityNotConfigured) {
		t.Fatalf("err = %v; 零行声明被收下了", err)
	}
	if _, err := domain.NewCancellationAuthorityContent([]domain.CancellationAuthorityDeclaration{
		{Party: domain.DeclaredCustomerCancellation, Rule: commercialValue(t, domain.NewRuleReference, "A")},
		{Party: domain.DeclaredCustomerCancellation, Rule: commercialValue(t, domain.NewRuleReference, "B")},
	}); !errors.Is(err, domain.ErrConflictingCancellationAuthority) {
		t.Fatalf("err = %v; 同一请求方格两行声明分不出真假", err)
	}
	if _, err := domain.NewCancellationAuthorityContent([]domain.CancellationAuthorityDeclaration{
		{Party: domain.DeclaredCancellationPartyInvalid, Rule: commercialValue(t, domain.NewRuleReference, "A")},
	}); !errors.Is(err, domain.ErrCancellationAuthorityNotConfigured) {
		t.Fatalf("err = %v; 零值请求方格被收下了", err)
	}
}
