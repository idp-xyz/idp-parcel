package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

type intakeContentDouble struct {
	content    pcdomain.IntakeQualificationContent
	configured bool
}

func (double intakeContentDouble) IntakeContentFor(
	_ context.Context,
	_ psdomain.SourceIdentity,
	_ psdomain.ShipmentRequestID,
) (pcdomain.IntakeQualificationContent, bool, error) {
	return double.content, double.configured, nil
}

type finalContentDouble struct {
	content    pcdomain.FinalRuleContent
	configured bool
}

func (double finalContentDouble) FinalContentFor(
	_ context.Context,
	_ psdomain.SourceIdentity,
) (pcdomain.FinalRuleContent, bool, error) {
	return double.content, double.configured, nil
}

func stageIdentity(t *testing.T) psdomain.SourceIdentity {
	t.Helper()
	identity, err := psdomain.NewSourceIdentity(
		value(t, psdomain.NewTenantID, "tenant-1"),
		value(t, psdomain.NewCustomerAccountID, "customer-1"),
		value(t, psdomain.NewSource, "source-a"),
		value(t, psdomain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	return identity
}

func nodeIntakeSource(t *testing.T) psdomain.IntakeSource {
	t.Helper()
	source, err := psdomain.NewIntakeSource(psdomain.IntakeSourceSpec{
		Kind:       psdomain.NodeIntakeSource,
		Object:     value(t, psdomain.NewSourceObjectReference, "unit-1"),
		Parcel:     value(t, psdomain.NewDeclaredParcelID, "parcel-1"),
		Place:      value(t, psdomain.NewIntakePlaceReference, "node-origin"),
		Control:    value(t, psdomain.NewIntakeControlReference, "NODE-INTAKE/SIGN-7"),
		Version:    value(t, psdomain.NewSourceResultVersion, "intake-result/v1"),
		OccurredAt: time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("new intake source: %v", err)
	}
	return source
}

func deliveryOutcome(t *testing.T) psdomain.ResponsibilityOutcome {
	t.Helper()
	outcome, err := psdomain.NewResponsibilityOutcome(psdomain.ResponsibilityOutcomeSpec{
		Kind:       psdomain.EffectiveDeliveryOutcome,
		Parcel:     value(t, psdomain.NewDeclaredParcelID, "parcel-1"),
		Decision:   value(t, psdomain.NewResponsibilityDecisionReference, "DELIVERY-JUDGMENT/TF-11"),
		Execution:  value(t, psdomain.NewExecutionEvidenceReference, "POD/POD-3"),
		Version:    value(t, psdomain.NewResponsibilityOutcomeVersion, "delivery-result/v1"),
		OccurredAt: time.Date(2026, 8, 10, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("new responsibility outcome: %v", err)
	}
	return outcome
}

// Covers: PAR-COM-16 声明经适配器译成 PS 资格三值——允许集合外的来源是`不适用`带
// 来源依据（服务形态不承担，不是资格没过）；声明硬资格而证据缝属实例时如实答未成立
// 点名头一项缺口（AT-PS-047 的续办路——首发无租户这是唯一走得到的真实分支）；空清单
// 声明即成立；未配置即 found=false。
func TestIntakeEligibilityTranslatesTheDeclaration(t *testing.T) {
	nodeOnly, err := pcdomain.NewIntakeQualificationContent(
		stageRulePackage(t),
		[]pcdomain.DeclaredIntakeSource{pcdomain.DeclaredNodeIntake},
		[]pcdomain.RuleReference{},
	)
	if err != nil {
		t.Fatalf("new content: %v", err)
	}
	subject := adapter.NewServiceStageRulesAdapter(
		intakeContentDouble{content: nodeOnly, configured: true},
		finalContentDouble{},
		nil,
		nil,
	)

	established, configured, err := subject.JudgeIntakeEligibility(
		context.Background(), stageIdentity(t),
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		nodeIntakeSource(t),
	)
	if err != nil || !configured {
		t.Fatalf("judge: configured = %v err = %v", configured, err)
	}
	if established.Outcome != psports.IntakeEligibilityEstablished {
		t.Fatalf("outcome = %q; 空清单声明即成立", established.Outcome)
	}

	withQualifications, err := pcdomain.NewIntakeQualificationContent(
		stageRulePackage(t),
		[]pcdomain.DeclaredIntakeSource{pcdomain.DeclaredNodeIntake},
		[]pcdomain.RuleReference{commercialRule(t, "INTAKE-QUAL/customs-precheck")},
	)
	if err != nil {
		t.Fatalf("new content with qualifications: %v", err)
	}
	unproven := adapter.NewServiceStageRulesAdapter(
		intakeContentDouble{content: withQualifications, configured: true},
		finalContentDouble{},
		nil,
		nil,
	)
	notEstablished, _, err := unproven.JudgeIntakeEligibility(
		context.Background(), stageIdentity(t),
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		nodeIntakeSource(t),
	)
	if err != nil {
		t.Fatalf("judge unproven: %v", err)
	}
	if notEstablished.Outcome != psports.IntakeEligibilityNotEstablished ||
		notEstablished.Basis.String() != "INTAKE_QUALIFICATION_UNPROVEN/INTAKE-QUAL/customs-precheck" {
		t.Fatalf("outcome = %q basis = %q", notEstablished.Outcome, notEstablished.Basis)
	}

	offsiteOnly, err := pcdomain.NewIntakeQualificationContent(
		stageRulePackage(t),
		[]pcdomain.DeclaredIntakeSource{pcdomain.DeclaredOffsitePickup},
		[]pcdomain.RuleReference{},
	)
	if err != nil {
		t.Fatalf("new offsite-only content: %v", err)
	}
	notApplicable := adapter.NewServiceStageRulesAdapter(
		intakeContentDouble{content: offsiteOnly, configured: true},
		finalContentDouble{},
		nil,
		nil,
	)
	answer, _, err := notApplicable.JudgeIntakeEligibility(
		context.Background(), stageIdentity(t),
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		nodeIntakeSource(t),
	)
	if err != nil {
		t.Fatalf("judge not applicable: %v", err)
	}
	if answer.Outcome != psports.IntakeServiceNotApplicable {
		t.Fatalf("outcome = %q; 允许集合外的来源是不适用", answer.Outcome)
	}

	if _, configured, err := adapter.NewServiceStageRulesAdapter(
		intakeContentDouble{configured: false},
		finalContentDouble{},
		nil,
		nil,
	).JudgeIntakeEligibility(
		context.Background(), stageIdentity(t),
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		nodeIntakeSource(t),
	); err != nil || configured {
		t.Fatalf("configured = %v err = %v; 未声明即未配置", configured, err)
	}
}

// Covers: PAR-COM-17 声明经适配器译成 PS 终局判断——声明行即满足带终局类型与规则
// 版本；缺行即不满足带依据（此产品下这种结果不形成终局是声明的真话——「有效交付
// 不在所有产品中自动等于终局」的提供方半边）；未配置即 found=false。
func TestFinalJudgmentTranslatesDeclaredRows(t *testing.T) {
	content, err := pcdomain.NewFinalRuleContent(stageRulePackage(t), []pcdomain.FinalizationDeclaration{{
		Outcome:   pcdomain.DeclaredReturnCompleted,
		FinalKind: commercialRule(t, "NETWORK_SERVICE_RETURNED"),
	}})
	if err != nil {
		t.Fatalf("new final content: %v", err)
	}
	subject := adapter.NewServiceStageRulesAdapter(
		intakeContentDouble{},
		finalContentDouble{content: content, configured: true},
		nil,
		nil,
	)

	judgment, configured, err := subject.JudgeFinalOutcome(
		context.Background(), stageIdentity(t), deliveryOutcome(t))
	if err != nil || !configured {
		t.Fatalf("judge: configured = %v err = %v", configured, err)
	}
	if judgment.Satisfied {
		t.Fatal("缺行的责任结果被判成了满足——有效交付不自动等于终局")
	}
	if judgment.Basis.String() != "OUTCOME_NOT_FINAL_FOR_PRODUCT/EFFECTIVE_DELIVERY" {
		t.Fatalf("basis = %q", judgment.Basis)
	}

	withDelivery, err := pcdomain.NewFinalRuleContent(stageRulePackage(t), []pcdomain.FinalizationDeclaration{{
		Outcome:   pcdomain.DeclaredEffectiveDelivery,
		FinalKind: commercialRule(t, "NETWORK_SERVICE_DELIVERED"),
	}})
	if err != nil {
		t.Fatalf("new delivery content: %v", err)
	}
	satisfied, _, err := adapter.NewServiceStageRulesAdapter(
		intakeContentDouble{},
		finalContentDouble{content: withDelivery, configured: true},
		nil,
		nil,
	).JudgeFinalOutcome(context.Background(), stageIdentity(t), deliveryOutcome(t))
	if err != nil {
		t.Fatalf("judge satisfied: %v", err)
	}
	if !satisfied.Satisfied ||
		satisfied.Kind.String() != "NETWORK_SERVICE_DELIVERED" ||
		satisfied.RuleVersion.String() != "PAR-COM-17/NETWORK_SERVICE_DELIVERED" {
		t.Fatalf("judgment = %#v", satisfied)
	}
}

type cancellationContentDouble struct {
	content    pcdomain.CancellationAuthorityContent
	configured bool
	err        error
}

func (double cancellationContentDouble) CancellationContentFor(
	_ context.Context,
	_ psdomain.SourceIdentity,
) (pcdomain.CancellationAuthorityContent, bool, error) {
	if double.err != nil {
		return pcdomain.CancellationAuthorityContent{}, false, double.err
	}
	return double.content, double.configured, nil
}

type requesterClassDouble struct {
	party  pcdomain.DeclaredCancellationParty
	formed bool
}

func (double requesterClassDouble) FormCancellationParty(
	_ context.Context,
	_ psdomain.SourceIdentity,
	_ psdomain.CancellationRequesterReference,
) (pcdomain.DeclaredCancellationParty, bool, error) {
	return double.party, double.formed, nil
}

type requesterClassMustNotBeCalled struct{ t *testing.T }

func (double requesterClassMustNotBeCalled) FormCancellationParty(
	context.Context,
	psdomain.SourceIdentity,
	psdomain.CancellationRequesterReference,
) (pcdomain.DeclaredCancellationParty, bool, error) {
	double.t.Fatal("未配置时仍调用了请求方格映射")
	return pcdomain.DeclaredCancellationPartyInvalid, false, nil
}

// Covers: PAR-COM-17 取消授权目录经适配器译成 PS 授权判断——未配置如实空白且不调
// 映射；目录含本格即允许带规则引用；缺行即不允许带依据；源失败上抛；未知格不吸收。
func TestCancellationAuthorityTranslatesTheCatalog(t *testing.T) {
	t.Run("unconfigured does not ask the requester mapping", func(t *testing.T) {
		subject := adapter.NewServiceStageRulesAdapter(
			intakeContentDouble{},
			finalContentDouble{},
			cancellationContentDouble{configured: false},
			requesterClassMustNotBeCalled{t: t},
		)
		_, configured, err := subject.JudgeCancellationAuthority(
			context.Background(), stageIdentity(t), cancellationRequester(t), cancellationParcel(t))
		if err != nil || configured {
			t.Fatalf("configured = %v err = %v; 未声明即未配置", configured, err)
		}
	})

	operationsOnly, err := pcdomain.NewCancellationAuthorityContent(stageAuthorizationRule(t), []pcdomain.CancellationAuthorityDeclaration{{
		Party: pcdomain.DeclaredOperationsCancellation,
		Rule:  commercialRule(t, "CANCEL-RULE/OPERATIONS"),
	}})
	if err != nil {
		t.Fatalf("new operations catalog: %v", err)
	}

	t.Run("matching operations row is granted", func(t *testing.T) {
		subject := adapter.NewServiceStageRulesAdapter(
			intakeContentDouble{},
			finalContentDouble{},
			cancellationContentDouble{content: operationsOnly, configured: true},
			requesterClassDouble{party: pcdomain.DeclaredOperationsCancellation, formed: true},
		)
		judgment, configured, err := subject.JudgeCancellationAuthority(
			context.Background(), stageIdentity(t), cancellationRequester(t), cancellationParcel(t))
		if err != nil || !configured {
			t.Fatalf("judge: configured = %v err = %v", configured, err)
		}
		if !judgment.Granted || judgment.Basis.String() != "CANCEL-RULE/OPERATIONS" {
			t.Fatalf("judgment = %#v; 命中应允许并带规则引用", judgment)
		}
	})

	t.Run("customer row missing refuses operations", func(t *testing.T) {
		customerOnly, err := pcdomain.NewCancellationAuthorityContent(stageAuthorizationRule(t), []pcdomain.CancellationAuthorityDeclaration{{
			Party: pcdomain.DeclaredCustomerCancellation,
			Rule:  commercialRule(t, "CANCEL-RULE/CUSTOMER"),
		}})
		if err != nil {
			t.Fatalf("new customer catalog: %v", err)
		}
		subject := adapter.NewServiceStageRulesAdapter(
			intakeContentDouble{},
			finalContentDouble{},
			cancellationContentDouble{content: customerOnly, configured: true},
			requesterClassDouble{party: pcdomain.DeclaredOperationsCancellation, formed: true},
		)
		judgment, configured, err := subject.JudgeCancellationAuthority(
			context.Background(), stageIdentity(t), cancellationRequester(t), cancellationParcel(t))
		if err != nil || !configured {
			t.Fatalf("judge: configured = %v err = %v", configured, err)
		}
		if judgment.Granted || judgment.Basis.String() != "PARTY_NOT_AUTHORIZED_TO_CANCEL/OPERATIONS" {
			t.Fatalf("judgment = %#v; 缺行应拒绝带依据", judgment)
		}
	})

	t.Run("source failure surfaces", func(t *testing.T) {
		unavailable := errors.New("目录不可读")
		subject := adapter.NewServiceStageRulesAdapter(
			intakeContentDouble{},
			finalContentDouble{},
			cancellationContentDouble{err: unavailable},
			requesterClassMustNotBeCalled{t: t},
		)
		_, _, err := subject.JudgeCancellationAuthority(
			context.Background(), stageIdentity(t), cancellationRequester(t), cancellationParcel(t))
		if !errors.Is(err, unavailable) {
			t.Fatalf("error = %v, want wrapped source failure", err)
		}
	})

	t.Run("unknown party is untranslatable", func(t *testing.T) {
		subject := adapter.NewServiceStageRulesAdapter(
			intakeContentDouble{},
			finalContentDouble{},
			cancellationContentDouble{content: operationsOnly, configured: true},
			requesterClassDouble{party: pcdomain.DeclaredCancellationPartyInvalid, formed: true},
		)
		_, _, err := subject.JudgeCancellationAuthority(
			context.Background(), stageIdentity(t), cancellationRequester(t), cancellationParcel(t))
		if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
			t.Fatalf("error = %v, want ErrUntranslatableAnswer", err)
		}
	})
}

func cancellationRequester(t *testing.T) psdomain.CancellationRequesterReference {
	t.Helper()
	return value(t, psdomain.NewCancellationRequesterReference, "OPERATOR-1")
}

func cancellationParcel(t *testing.T) psdomain.DeclaredParcelID {
	t.Helper()
	return value(t, psdomain.NewDeclaredParcelID, "parcel-1")
}

func commercialRule(t *testing.T, raw string) pcdomain.RuleReference {
	t.Helper()
	rule, err := pcdomain.NewRuleReference(raw)
	if err != nil {
		t.Fatalf("new rule reference %q: %v", raw, err)
	}
	return rule
}

func stageRulePackage(t *testing.T) pcdomain.CommercialVersion {
	t.Helper()
	return liveStageVersion(t, pcdomain.AcceptanceRulePackageObject, "rules-stage", "v1", "sha256:rules-stage")
}

func stageAuthorizationRule(t *testing.T) pcdomain.CommercialVersion {
	t.Helper()
	return liveStageVersion(t, pcdomain.AuthorizationRuleObject, "auth-stage", "v1", "sha256:auth-stage")
}

func liveStageVersion(
	t *testing.T,
	kind pcdomain.CommercialObjectKind,
	objectID, version, digest string,
) pcdomain.CommercialVersion {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	draft, err := pcdomain.NewCommercialDraft(pcdomain.CommercialVersionSpec{
		TenantID:      value(t, pcdomain.NewTenantID, "tenant-1"),
		Kind:          kind,
		ObjectID:      value(t, pcdomain.NewCommercialObjectID, objectID),
		Version:       value(t, pcdomain.NewCommercialVersionLabel, version),
		Scope:         value(t, pcdomain.NewCommercialScopeReference, "scope-"+objectID),
		ContentDigest: value(t, pcdomain.NewCommercialContentDigest, digest),
		Effective:     interval,
	})
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	basis, err := pcdomain.NewApprovalBasis(
		value(t, pcdomain.NewApprovalReference, "approval-"+objectID),
		value(t, pcdomain.NewCommercialSourceReference, "source-"+objectID),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("new approval basis: %v", err)
	}
	published, err := draft.Publish(basis, pcdomain.ApprovalRoleConfirmed, approvedAt, nil)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	live, err := published.TakeEffect(approvedAt)
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	return live
}
