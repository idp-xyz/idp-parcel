package partycommercial_test

import (
	"context"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

type intakeViewDouble struct {
	content pcdomain.IntakeQualificationContent
	found   bool
}

func (double intakeViewDouble) LoadIntakeQualification(
	_ context.Context,
	_ pcdomain.TenantID,
	_ pcdomain.CommercialVersion,
) (pcdomain.IntakeQualificationContent, bool, error) {
	return double.content, double.found, nil
}

type finalViewDouble struct {
	content pcdomain.FinalRuleContent
	found   bool
}

func (double finalViewDouble) LoadFinalRule(
	_ context.Context,
	_ pcdomain.TenantID,
	_ pcdomain.CommercialVersion,
) (pcdomain.FinalRuleContent, bool, error) {
	return double.content, double.found, nil
}

type cancellationViewDouble struct {
	content pcdomain.CancellationAuthorityContent
	found   bool
}

func (double cancellationViewDouble) LoadCancellationAuthority(
	_ context.Context,
	_ pcdomain.TenantID,
	_ pcdomain.CommercialVersion,
) (pcdomain.CancellationAuthorityContent, bool, error) {
	return double.content, double.found, nil
}

type boundStageOwner struct {
	rules pcdomain.CommercialVersion
	auth  pcdomain.CommercialVersion
}

func (owner boundStageOwner) AcceptanceRulePackageFor(
	context.Context,
	psdomain.SourceIdentity,
) (pcdomain.CommercialVersion, bool, error) {
	return owner.rules, true, nil
}

func (owner boundStageOwner) AuthorizationRuleFor(
	context.Context,
	psdomain.SourceIdentity,
) (pcdomain.CommercialVersion, bool, error) {
	return owner.auth, true, nil
}

// Covers: 装配缝诚实未配置——SourceIdentity 不含采用的规则版本，生产适配器不得
// 代拟默认内容。三族同切：收寄、终局、取消都停在 found=false。
func TestUnconfiguredAdoptedOwnerLeavesAllStageFamiliesUnconfigured(t *testing.T) {
	source := adapter.NewDeclaredStageContent(nil, nil, nil, adapter.UnconfiguredAdoptedStageOwner{})
	subject := adapter.NewServiceStageRulesAdapter(
		source, source, source, requesterClassMustNotBeCalled{t: t},
	)

	if _, configured, err := subject.JudgeIntakeEligibility(
		context.Background(), stageIdentity(t),
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		nodeIntakeSource(t),
	); err != nil || configured {
		t.Fatalf("收寄资格：configured = %v err = %v", configured, err)
	}
	if _, configured, err := subject.JudgeFinalOutcome(
		context.Background(), stageIdentity(t), deliveryOutcome(t),
	); err != nil || configured {
		t.Fatalf("终局规则：configured = %v err = %v", configured, err)
	}
	if _, configured, err := subject.JudgeCancellationAuthority(
		context.Background(), stageIdentity(t), cancellationRequester(t), cancellationParcel(t),
	); err != nil || configured {
		t.Fatalf("取消授权：configured = %v err = %v", configured, err)
	}
}

// Covers: 生产适配器接通 PC 三口后，翻译覆盖三族——有采用版本且声明在场时走既有
// 判断口径，不在装配层发明内容。
func TestDeclaredStageContentTranslatesAllThreeFamilies(t *testing.T) {
	rules := stageRulePackage(t)
	auth := stageAuthorizationRule(t)
	owners := boundStageOwner{rules: rules, auth: auth}

	intake, err := pcdomain.NewIntakeQualificationContent(
		rules,
		[]pcdomain.DeclaredIntakeSource{pcdomain.DeclaredNodeIntake},
		[]pcdomain.RuleReference{},
	)
	if err != nil {
		t.Fatalf("new intake: %v", err)
	}
	final, err := pcdomain.NewFinalRuleContent(rules, []pcdomain.FinalizationDeclaration{{
		Outcome:   pcdomain.DeclaredEffectiveDelivery,
		FinalKind: commercialRule(t, "NETWORK_SERVICE_DELIVERED"),
	}})
	if err != nil {
		t.Fatalf("new final: %v", err)
	}
	catalog, err := pcdomain.NewCancellationAuthorityContent(auth, []pcdomain.CancellationAuthorityDeclaration{{
		Party: pcdomain.DeclaredOperationsCancellation,
		Rule:  commercialRule(t, "CANCEL-RULE/OPERATIONS"),
	}})
	if err != nil {
		t.Fatalf("new cancellation: %v", err)
	}

	source := adapter.NewDeclaredStageContent(
		intakeViewDouble{content: intake, found: true},
		finalViewDouble{content: final, found: true},
		cancellationViewDouble{content: catalog, found: true},
		owners,
	)
	subject := adapter.NewServiceStageRulesAdapter(
		source, source, source,
		requesterClassDouble{party: pcdomain.DeclaredOperationsCancellation, formed: true},
	)

	established, configured, err := subject.JudgeIntakeEligibility(
		context.Background(), stageIdentity(t),
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		nodeIntakeSource(t),
	)
	if err != nil || !configured {
		t.Fatalf("收寄资格：configured = %v err = %v", configured, err)
	}
	if established.Outcome != psports.IntakeEligibilityEstablished {
		t.Fatalf("outcome = %q", established.Outcome)
	}

	judgment, configured, err := subject.JudgeFinalOutcome(
		context.Background(), stageIdentity(t), deliveryOutcome(t),
	)
	if err != nil || !configured {
		t.Fatalf("终局规则：configured = %v err = %v", configured, err)
	}
	if !judgment.Satisfied || judgment.Kind.String() != "NETWORK_SERVICE_DELIVERED" {
		t.Fatalf("judgment = %#v", judgment)
	}

	authority, configured, err := subject.JudgeCancellationAuthority(
		context.Background(), stageIdentity(t), cancellationRequester(t), cancellationParcel(t),
	)
	if err != nil || !configured {
		t.Fatalf("取消授权：configured = %v err = %v", configured, err)
	}
	if !authority.Granted || authority.Basis.String() != "CANCEL-RULE/OPERATIONS" {
		t.Fatalf("authority = %#v", authority)
	}
}
