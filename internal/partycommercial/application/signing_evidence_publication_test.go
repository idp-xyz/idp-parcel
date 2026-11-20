package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// signingEvidenceViewDouble 演要求没登记、要求已登记但缺证据、以及补登之后这三格。
type signingEvidenceViewDouble struct {
	rule       domain.SigningEvidenceRequirement
	found      bool
	registered bool
}

func (double signingEvidenceViewDouble) LoadSigningEvidenceRequirement(context.Context, domain.TenantID) (domain.SigningEvidenceRequirement, bool, error) {
	return double.rule, double.found, nil
}

func (double signingEvidenceViewDouble) HasSigningEvidence(
	context.Context, domain.TenantID, domain.CommercialObjectKind, domain.CommercialObjectID, domain.CommercialVersionLabel,
) (bool, error) {
	return double.registered, nil
}

func (double signingEvidenceViewDouble) ListSigningEvidence(
	context.Context, domain.TenantID, domain.CommercialObjectKind, domain.CommercialObjectID, domain.CommercialVersionLabel,
) ([]domain.SigningEvidence, error) {
	return nil, nil
}

func publishApprovedContract(t *testing.T, evidence applicationPublisherEvidence) (*publicationRegistryDouble, application.PublishPublicationDraftResult, domain.PublicationDraft) {
	t.Helper()
	declarations := contractDeclarations(t, "rules-1")
	drafts := newDraftRegistryDouble()
	shell := draftShell(t, domain.CustomerContractObject, "contract-1", "v1")
	submitted, err := application.NewSubmitPublicationDraftHandler(drafts, fixedClock{at: draftSubmittedAt}).Handle(context.Background(),
		application.SubmitPublicationDraftCommand{
			Shell: shell, Content: customerContractContent(t, declarations),
			Submitter: pcValue(t, domain.NewOperatorSubjectReference, "op-submitter"),
		})
	if err != nil || submitted.Outcome() != application.PublicationDraftSubmitted {
		t.Fatalf("录入 = %q, %v", submitted.Outcome(), err)
	}
	approved, err := application.NewApprovePublicationDraftHandler(drafts, approvalDutyRuleDouble{rule: dutyRule(t, true, ""), found: true}, fixedClock{at: draftApprovedAt}).
		Handle(context.Background(), application.ApprovePublicationDraftCommand{
			Tenant: shell.TenantID, Kind: shell.Kind, ObjectID: shell.ObjectID, Version: shell.Version,
			Approver: operatorSubject(t, "op-approver"),
		})
	if err != nil || approved.Outcome() != application.PublicationDraftApproved {
		t.Fatalf("批准 = %q, %v", approved.Outcome(), err)
	}
	registry := &publicationRegistryDouble{}
	publisher := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: draftPublishedAt}, &operatorRegistrationHandoffDouble{})
	if evidence.attach {
		publisher.WithSigningEvidence(evidence.view)
	}
	result, err := application.NewPublishPublicationDraftHandler(drafts, publisher, fixedClock{at: draftPublishedAt}).
		Handle(context.Background(), application.PublishPublicationDraftCommand{
			Tenant: shell.TenantID, Kind: shell.Kind, ObjectID: shell.ObjectID, Version: shell.Version,
		})
	if err != nil {
		t.Fatalf("发布：%v", err)
	}
	stored, _, _ := drafts.LoadDraft(context.Background(), shell.TenantID, shell.Kind, shell.ObjectID, shell.Version)
	return registry, result, stored
}

type applicationPublisherEvidence struct {
	attach bool
	view   signingEvidenceViewDouble
}

func TestPublishingAContractDoesNotAskForSigningEvidenceUntilTheTenantRequiresIt(t *testing.T) {
	open, result, stored := publishApprovedContract(t, applicationPublisherEvidence{
		attach: true,
		view:   signingEvidenceViewDouble{},
	})
	if result.Outcome() != application.PublicationDraftPublished || len(open.savedVersions) != 1 {
		t.Fatalf("没登记要求却被拦住：%s versions=%d", result.Outcome(), len(open.savedVersions))
	}
	if stored.Status() != domain.PublicationDraftPublished {
		t.Fatalf("status = %s", stored.Status())
	}

	rule, err := domain.NewSigningEvidenceRequirement(pcValue(t, domain.NewTenantID, "tenant-1"), true, false)
	if err != nil {
		t.Fatalf("要求：%v", err)
	}
	blockedRegistry, blocked, stored := publishApprovedContract(t, applicationPublisherEvidence{
		attach: true,
		view:   signingEvidenceViewDouble{rule: rule, found: true},
	})
	if blocked.Outcome() != application.PublicationDraftSigningEvidenceRequired {
		t.Fatalf("缺证据 outcome = %s", blocked.Outcome())
	}
	publication, ok := blocked.Publication()
	if !ok || publication.Outcome() != application.CommercialPublicationSigningEvidenceRequired {
		t.Fatalf("发布用例 = %s %v", publication.Outcome(), ok)
	}
	if len(blockedRegistry.savedVersions) != 0 {
		t.Fatalf("缺证据仍入册 %d 行", len(blockedRegistry.savedVersions))
	}
	if stored.Status() != domain.PublicationDraftApproved {
		t.Fatalf("载体 status = %s，want APPROVED", stored.Status())
	}

	landed, again, stored := publishApprovedContract(t, applicationPublisherEvidence{
		attach: true,
		view:   signingEvidenceViewDouble{rule: rule, found: true, registered: true},
	})
	if again.Outcome() != application.PublicationDraftPublished || len(landed.savedVersions) != 1 {
		t.Fatalf("补登后 = %s versions=%d", again.Outcome(), len(landed.savedVersions))
	}
	if stored.Status() != domain.PublicationDraftPublished {
		t.Fatalf("补登后 status = %s", stored.Status())
	}

	// 只要求协议时，合同发布不设这道门。
	agreementOnly, err := domain.NewSigningEvidenceRequirement(rule.Tenant(), false, true)
	if err != nil {
		t.Fatalf("要求：%v", err)
	}
	untouched, result, _ := publishApprovedContract(t, applicationPublisherEvidence{
		attach: true,
		view:   signingEvidenceViewDouble{rule: agreementOnly, found: true},
	})
	if result.Outcome() != application.PublicationDraftPublished || len(untouched.savedVersions) != 1 {
		t.Fatalf("不要求合同却被拦住：%s", result.Outcome())
	}
}
