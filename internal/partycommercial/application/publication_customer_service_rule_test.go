package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件证客户服务规则册在应用层的两个方向都接上了（票 admin-write-faces/18）：受控批文那一半的对账门对本册开门
// （publicationContentOf 一支），载体那一半把正文交回发布用例（declarationsOfContent 一支）——适用对象、责任方、范围与两张
// 子表原样往返，行序由各自的键归一，不代填任何一项（ADR-0104）。

func customerServiceRuleContent(t *testing.T, declaration *application.CustomerServiceRuleBodyDeclaration) domain.PublicationContent {
	t.Helper()
	return domain.PublicationContent{
		Kind: domain.CustomerServiceRuleObject,
		CustomerServiceRule: &domain.CustomerServiceRuleBody{
			Applicability: declaration.Applicability,
			Responsible:   declaration.Responsible,
			Scope:         declaration.Scope,
			Deadlines:     declaration.Deadlines,
			Materials:     declaration.Materials,
		},
	}
}

// customerServiceRuleBodyWith 造一份挂在客户合同上、两张子表都有行的正文：期限故意把结论复核写在首次索赔前面，材料把
// 索赔类型串序颠倒——两处都由服务端按键归一，不是载体或批文里的行序。
func customerServiceRuleBodyWith(t *testing.T, contract string) *application.CustomerServiceRuleBodyDeclaration {
	t.Helper()
	deadline := func(kind domain.ClaimDeadlineKind, startEvent string, days int) domain.ClaimDeadlineRule {
		rule, err := domain.NewClaimDeadlineRule(kind,
			pcValue(t, domain.NewDeadlineStartEventReference, startEvent), days,
			pcValue(t, domain.NewBusinessCalendarReference, "calendar-cn"))
		if err != nil {
			t.Fatalf("索赔期限规则：%v", err)
		}
		return rule
	}
	materials := func(claimKind string, references ...string) domain.MinimumMaterialsRule {
		items := make([]domain.MaterialRequirementReference, 0, len(references))
		for _, reference := range references {
			items = append(items, pcValue(t, domain.NewMaterialRequirementReference, reference))
		}
		rule, err := domain.NewMinimumMaterialsRule(pcValue(t, domain.NewClaimKindReference, claimKind), items)
		if err != nil {
			t.Fatalf("最低材料规则：%v", err)
		}
		return rule
	}
	return &application.CustomerServiceRuleBodyDeclaration{
		Applicability: domain.CustomerServiceRuleAppliesToCustomerContract(pcValue(t, domain.NewCommercialObjectID, contract)),
		Responsible:   pcValue(t, domain.NewPartyID, "operator-1"),
		Scope:         pcValue(t, domain.NewCommercialScopeReference, "scope-1"),
		Deadlines: []domain.ClaimDeadlineRule{
			deadline(domain.ConclusionReviewDeadline, "event-conclusion-notified", 15),
			deadline(domain.FirstClaimDeadline, "event-delivered", 30),
		},
		Materials: []domain.MinimumMaterialsRule{
			materials("claim-loss", "material-photo", "material-invoice"),
			materials("claim-damage", "material-photo"),
		},
	}
}

// Covers: ADR-0126 Decision 二 — 客户服务规则册接进规范化后对账门对它开门：声明的旧式串与算出的不等即`未受理`，一个字节不写、
// 整册不读，结果带出两个串；算出的串放行且两张表按键落册；壳单独发布（正文缺席）没有可比对象照旧登记；正文过不了门（两项都空）
// 同归`未受理`带成因，不再是 error。
func TestACustomerServiceRuleDeclaredDigestIsReconciled(t *testing.T) {
	body := customerServiceRuleBodyWith(t, "contract-1")

	t.Run("a mismatching declared digest is not accepted", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.CustomerServiceRuleObject, "csr-1", "v1"),
			Approval:     publishApproval(t, "csr-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{CustomerServiceRuleBody: body},
		})
		if err != nil {
			t.Fatalf("Handle：%v——未受理不是 error", err)
		}
		if result.Outcome() != application.CommercialPublicationNotAccepted || !errors.Is(result.RefusalCause(), domain.ErrDeclaredDigestMismatch) {
			t.Fatalf("outcome = %q, cause = %v; want NOT_ACCEPTED / ErrDeclaredDigestMismatch", result.Outcome(), result.RefusalCause())
		}
		declared, computed, ok := result.DigestReconciliation()
		if !ok || declared != "sha256:csr-1-v1" || !strings.HasPrefix(computed, "PCC-1:") {
			t.Fatalf("DigestReconciliation = (%q, %q, %v)", declared, computed, ok)
		}
		if len(registry.savedVersions) != 0 || len(registry.savedServiceRules) != 0 || registry.loads != 0 {
			t.Fatalf("未受理写了 %d 版本 / %d 正文、读了 %d 次整册", len(registry.savedVersions), len(registry.savedServiceRules), registry.loads)
		}
	})

	t.Run("the computed digest publishes and both tables land keyed", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         customerServiceRuleSpec(t, "csr-1", "v1", body),
			Approval:     publishApproval(t, "csr-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{CustomerServiceRuleBody: body},
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.CommercialVersionPublishedEffective || len(registry.savedServiceRules) != 1 {
			t.Fatalf("outcome = %q, saved rules = %d", result.Outcome(), len(registry.savedServiceRules))
		}
		saved := registry.savedServiceRules[0]
		deadlines := saved.ClaimDeadlines()
		if len(deadlines) != 2 || deadlines[0].Kind() != domain.FirstClaimDeadline || deadlines[1].Kind() != domain.ConclusionReviewDeadline {
			t.Fatalf("deadlines = %#v", deadlines)
		}
		materials := saved.MinimumMaterials()
		if len(materials) != 2 || materials[0].ClaimKind().String() != "claim-damage" || materials[1].ClaimKind().String() != "claim-loss" {
			t.Fatalf("materials = %#v", materials)
		}
	})

	t.Run("a shell without its body still publishes", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.CustomerServiceRuleObject, "csr-1", "v1"),
			Approval:     publishApproval(t, "csr-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
		})
		if err != nil || result.Outcome() != application.CommercialVersionPublishedEffective {
			t.Fatalf("outcome = %q, err = %v; want PUBLISHED_EFFECTIVE", result.Outcome(), err)
		}
	})

	t.Run("a body that fails its own gate is not accepted before any write", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		empty := customerServiceRuleBodyWith(t, "contract-1")
		empty.Deadlines, empty.Materials = nil, nil
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.CustomerServiceRuleObject, "csr-1", "v1"),
			Approval:     publishApproval(t, "csr-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{CustomerServiceRuleBody: empty},
		})
		if err != nil {
			t.Fatalf("Handle：%v——未受理不是 error", err)
		}
		if result.Outcome() != application.CommercialPublicationNotAccepted || !errors.Is(result.RefusalCause(), domain.ErrInvalidCustomerServiceRuleVersion) {
			t.Fatalf("outcome = %q, cause = %v; want NOT_ACCEPTED / ErrInvalidCustomerServiceRuleVersion", result.Outcome(), result.RefusalCause())
		}
		if _, _, ok := result.DigestReconciliation(); ok {
			t.Fatal("折不成文档就没有算出的串可比")
		}
		if len(registry.savedVersions) != 0 || len(registry.savedServiceRules) != 0 || registry.loads != 0 {
			t.Fatal("两项都空的规则正文写了库或读了整册")
		}
	})
}

// Covers: ADR-0126 Decision 三 — 客户服务规则载体走完录入 → 批准 → 发布：正文随版本同笔登记到规则册（适用对象、责任方、范围
// 与两张表就是录入时那一份，行序按各自的键）、报告一条 CUSTOMER_SERVICE_RULE_BODY=SAVED、入册摘要就是载体的摘要；壳与正文的
// 适用一致（ADR-0104 Decision 四）在发布用例写入前那一道照旧核——载体上壳指名的合同与正文挂的不是同一个时整项拒。
func TestACustomerServiceRuleDraftPublishesItsBodyThroughTheExistingUseCase(t *testing.T) {
	drafts := newDraftRegistryDouble()
	shell := draftShell(t, domain.CustomerServiceRuleObject, "csr-1", "v1")
	declaration := customerServiceRuleBodyWith(t, "contract-1")
	submitted, err := application.NewSubmitPublicationDraftHandler(drafts, fixedClock{at: draftSubmittedAt}).Handle(context.Background(),
		application.SubmitPublicationDraftCommand{
			Shell:     shell,
			Content:   customerServiceRuleContent(t, declaration),
			Submitter: pcValue(t, domain.NewOperatorSubjectReference, "op-submitter"),
		})
	if err != nil || submitted.Outcome() != application.PublicationDraftSubmitted {
		t.Fatalf("录入：%q, %v", submitted.Outcome(), err)
	}
	rules := approvalDutyRuleDouble{rule: dutyRule(t, true, ""), found: true}
	approved, err := application.NewApprovePublicationDraftHandler(drafts, rules, fixedClock{at: draftApprovedAt}).Handle(context.Background(),
		application.ApprovePublicationDraftCommand{
			Tenant: shell.TenantID, Kind: shell.Kind, ObjectID: shell.ObjectID, Version: shell.Version,
			Approver: operatorSubject(t, "op-approver"),
		})
	if err != nil || approved.Outcome() != application.PublicationDraftApproved {
		t.Fatalf("批准：%q, %v", approved.Outcome(), err)
	}

	registry := &publicationRegistryDouble{}
	publisher := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: draftPublishedAt}, &operatorRegistrationHandoffDouble{})
	result, err := application.NewPublishPublicationDraftHandler(drafts, publisher, fixedClock{at: draftPublishedAt}).Handle(context.Background(),
		application.PublishPublicationDraftCommand{Tenant: shell.TenantID, Kind: shell.Kind, ObjectID: shell.ObjectID, Version: shell.Version})
	if err != nil {
		t.Fatalf("发布：%v", err)
	}
	if result.Outcome() != application.PublicationDraftPublished {
		t.Fatalf("outcome = %q, want DRAFT_PUBLISHED", result.Outcome())
	}
	publication, ok := result.Publication()
	if !ok || publication.Outcome() != application.CommercialVersionPublishedEffective {
		t.Fatalf("publication = %q, %v; want PUBLISHED_EFFECTIVE", publication.Outcome(), ok)
	}
	if len(registry.savedVersions) != 1 || len(registry.savedServiceRules) != 1 {
		t.Fatalf("saved %d versions / %d rules, want 1 / 1", len(registry.savedVersions), len(registry.savedServiceRules))
	}
	saved := registry.savedServiceRules[0]
	if contract, applies := saved.Applicability().CustomerContract(); !applies || contract.String() != "contract-1" {
		t.Fatalf("applicability = %#v", saved.Applicability())
	}
	if saved.ResponsibleParty().String() != "operator-1" || saved.Scope().String() != "scope-1" {
		t.Fatalf("parent row = %s / %s", saved.ResponsibleParty(), saved.Scope())
	}
	if first, found := saved.ClaimDeadline(domain.FirstClaimDeadline); !found || first.DurationDays() != 30 || first.StartEvent().String() != "event-delivered" {
		t.Fatalf("first claim deadline = (%#v, %v)", first, found)
	}
	if review, found := saved.ClaimDeadline(domain.ConclusionReviewDeadline); !found || review.DurationDays() != 15 {
		t.Fatalf("conclusion review deadline = (%#v, %v)", review, found)
	}
	if loss, found := saved.MinimumMaterialsFor(pcValue(t, domain.NewClaimKindReference, "claim-loss")); !found || len(loss.Materials()) != 2 {
		t.Fatalf("claim-loss materials = (%#v, %v)", loss, found)
	}
	reports := publication.Declarations()
	if len(reports) != 1 || reports[0].Channel != application.CustomerServiceRuleBodyChannel || reports[0].Outcome != ports.DeclarationSaved {
		t.Fatalf("报告 = %#v, want CUSTOMER_SERVICE_RULE_BODY=SAVED 一条", reports)
	}
	stored, _, _ := drafts.LoadDraft(context.Background(), shell.TenantID, shell.Kind, shell.ObjectID, shell.Version)
	if registry.savedVersions[0].ContentDigest() != stored.Canonical().Digest() {
		t.Fatalf("入册摘要 %s ≠ 载体摘要 %s", registry.savedVersions[0].ContentDigest(), stored.Canonical().Digest())
	}

	t.Run("a shell naming another contract than the body is refused at publication", func(t *testing.T) {
		drafts := newDraftRegistryDouble()
		named := draftShell(t, domain.CustomerServiceRuleObject, "csr-2", "v1")
		named.References = map[domain.CommercialObjectKind]domain.CommercialObjectID{
			domain.CustomerContractObject: pcValue(t, domain.NewCommercialObjectID, "contract-OTHER"),
		}
		if submitted, err := application.NewSubmitPublicationDraftHandler(drafts, fixedClock{at: draftSubmittedAt}).Handle(context.Background(),
			application.SubmitPublicationDraftCommand{
				Shell:     named,
				Content:   customerServiceRuleContent(t, declaration),
				Submitter: pcValue(t, domain.NewOperatorSubjectReference, "op-submitter"),
			}); err != nil || submitted.Outcome() != application.PublicationDraftSubmitted {
			t.Fatalf("录入：%q, %v", submitted.Outcome(), err)
		}
		if approved, err := application.NewApprovePublicationDraftHandler(drafts, rules, fixedClock{at: draftApprovedAt}).Handle(context.Background(),
			application.ApprovePublicationDraftCommand{
				Tenant: named.TenantID, Kind: named.Kind, ObjectID: named.ObjectID, Version: named.Version,
				Approver: operatorSubject(t, "op-approver"),
			}); err != nil || approved.Outcome() != application.PublicationDraftApproved {
			t.Fatalf("批准：%q, %v", approved.Outcome(), err)
		}
		registry := &publicationRegistryDouble{}
		loaded := domain.NewCommercialRegistry()
		registeredEffective(t, loaded, publishSpec(t, domain.CustomerContractObject, "contract-OTHER", "v1"), publishApproval(t, "contract-OTHER"))
		registry.loaded = loaded
		publisher := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: draftPublishedAt}, &operatorRegistrationHandoffDouble{})
		_, err := application.NewPublishPublicationDraftHandler(drafts, publisher, fixedClock{at: draftPublishedAt}).Handle(context.Background(),
			application.PublishPublicationDraftCommand{Tenant: named.TenantID, Kind: named.Kind, ObjectID: named.ObjectID, Version: named.Version})
		if !errors.Is(err, domain.ErrCustomerServiceRuleApplicabilityMismatch) {
			t.Fatalf("err = %v, want ErrCustomerServiceRuleApplicabilityMismatch", err)
		}
		if len(registry.savedVersions) != 0 || len(registry.savedServiceRules) != 0 {
			t.Fatal("分歧的正文写了库")
		}
	})
}
