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

// 本文件证销售价格约定随客户合同版本发布的编排（票 commercial-four-module-requirements/16，ADR-0380 决定一、七）：
// 它是合同正文的一层，折进同一个内容摘要，各写各的通道。

func (double *publicationRegistryDouble) SaveContractSellPriceArrangement(
	_ context.Context,
	arrangement domain.ContractSellPriceArrangement,
) (ports.DeclarationSaveOutcome, error) {
	if double.declarationErr == nil {
		double.savedSellPrice = append(double.savedSellPrice, arrangement)
	}
	return double.declarationAnswer("contract-sell-price")
}

func sellPriceScopes(t *testing.T, names ...string) []domain.PricingScopeReference {
	t.Helper()
	scopes := make([]domain.PricingScopeReference, 0, len(names))
	for _, name := range names {
		scopes = append(scopes, pcValue(t, domain.NewPricingScopeReference, name))
	}
	return scopes
}

// contractWithSellPrice 造一份带销售价格约定的客户合同正文，并交回它该折成的正文面（摘要按它算）。
func contractWithSellPrice(
	t *testing.T,
	arrangement application.SellPriceArrangementDeclaration,
) (application.CommercialDeclarations, domain.PublicationContent) {
	t.Helper()
	declarations := contractDeclarations(t, "rules-1")
	declarations.SellPriceArrangement = &arrangement
	content := customerContractContent(t, declarations)
	content.CustomerContract.SellPrice = &domain.SellPriceArrangementBody{
		Sources:          arrangement.Sources,
		NoSellPriceBasis: arrangement.NoSellPriceBasis,
	}
	return declarations, content
}

func digestOf(t *testing.T, content domain.PublicationContent) domain.CommercialContentDigest {
	t.Helper()
	canonical, err := domain.CanonicalizePublicationContent(content)
	if err != nil {
		t.Fatalf("规范化客户合同正文：%v", err)
	}
	return canonical.Digest()
}

// Covers: 票面「录入、批准与发布面带上这一格…批准与发布绑定的内容摘要随之包含它」（ADR-0380 决定一）——受控批文的
// 客户合同项带销售价格约定时，摘要按含这一层的正文算（PCC-3），约定经自己的通道落库、顺序照声明；摘要若是按不含
// 这一层的正文算的，对账门答`未受理`、一个字节不写。
func TestACustomerContractPublishesItsSellPriceArrangementUnderTheSameDigest(t *testing.T) {
	declarations, content := contractWithSellPrice(t, application.SellPriceArrangementDeclaration{
		Sources: sellPriceScopes(t, "SYN-SCOPE-EXCLUSIVE", "SYN-SCOPE-STANDARD"),
	})

	t.Run("the arrangement is saved through its own channel in the declared order", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		spec := publishSpec(t, domain.CustomerContractObject, "contract-1", "v1")
		spec.ContentDigest = digestOf(t, content)
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         spec,
			Approval:     publishApproval(t, "contract-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: declarations,
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.CommercialVersionPublishedEffective {
			t.Fatalf("outcome = %q, want PUBLISHED_EFFECTIVE", result.Outcome())
		}
		if len(registry.savedSellPrice) != 1 {
			t.Fatalf("saved sell price arrangements = %#v", registry.savedSellPrice)
		}
		sources, priced := registry.savedSellPrice[0].Sources()
		if !priced || len(sources) != 2 || sources[0].String() != "SYN-SCOPE-EXCLUSIVE" || sources[1].String() != "SYN-SCOPE-STANDARD" {
			t.Fatalf("saved sources = %v priced = %v", sources, priced)
		}
		if !strings.Contains(strings.Join(registry.declarationLog, ","), "contract-sell-price") {
			t.Fatalf("declaration log = %v, want the sell price channel", registry.declarationLog)
		}
		if !strings.HasPrefix(registry.savedVersions[0].ContentDigest().String(), "PCC-3:") {
			t.Fatalf("registered digest = %s, want a PCC-3 digest", registry.savedVersions[0].ContentDigest())
		}
		reported := false
		for _, report := range result.Declarations() {
			if report.Channel == application.ContractSellPriceChannel {
				reported = report.Outcome == ports.DeclarationSaved && report.Channel.String() == "CONTRACT_SELL_PRICE"
			}
		}
		if !reported {
			t.Fatalf("declaration reports = %#v, want CONTRACT_SELL_PRICE saved", result.Declarations())
		}
	})

	t.Run("a digest computed without the arrangement is not accepted", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		spec := publishSpec(t, domain.CustomerContractObject, "contract-1", "v1")
		spec.ContentDigest = digestOf(t, customerContractContent(t, contractDeclarations(t, "rules-1")))
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         spec,
			Approval:     publishApproval(t, "contract-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: declarations,
		})
		if err != nil {
			t.Fatalf("Handle：%v——未受理不是 error", err)
		}
		if result.Outcome() != application.CommercialPublicationNotAccepted || !errors.Is(result.RefusalCause(), domain.ErrDeclaredDigestMismatch) {
			t.Fatalf("outcome = %q, cause = %v; want NOT_ACCEPTED / ErrDeclaredDigestMismatch", result.Outcome(), result.RefusalCause())
		}
		if _, computed, ok := result.DigestReconciliation(); !ok || !strings.HasPrefix(computed, "PCC-3:") {
			t.Fatalf("computed digest = %q (%v), want a PCC-3 digest", computed, ok)
		}
		if len(registry.savedVersions) != 0 || len(registry.savedSellPrice) != 0 {
			t.Fatal("未受理写了库")
		}
	})
}

// Covers: 票面完成判据「一份带销售价格约定的合同版本经录入、批准、发布后读回，列表顺序与录入相同」在编排这一层
// ——载体上的正文快照带着销售价格约定（PCC-3），发布时折回发布用例的同一条通道、顺序不变，入册摘要就是载体上
// 算出的那一个；不计销售价的约定带着依据走同一条路。
func TestACustomerContractDraftCarriesItsSellPriceArrangementThroughApprovalAndPublication(t *testing.T) {
	publish := func(t *testing.T, arrangement application.SellPriceArrangementDeclaration) (*publicationRegistryDouble, domain.PublicationDraft) {
		t.Helper()
		_, content := contractWithSellPrice(t, arrangement)
		drafts := newDraftRegistryDouble()
		shell := draftShell(t, domain.CustomerContractObject, "contract-1", "v1")
		submitted, err := application.NewSubmitPublicationDraftHandler(drafts, fixedClock{at: draftSubmittedAt}).Handle(context.Background(),
			application.SubmitPublicationDraftCommand{
				Shell:     shell,
				Content:   content,
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
		result, err := application.NewPublishPublicationDraftHandler(drafts, publisher, fixedClock{at: draftPublishedAt}).
			Handle(context.Background(), application.PublishPublicationDraftCommand{
				Tenant: shell.TenantID, Kind: shell.Kind, ObjectID: shell.ObjectID, Version: shell.Version,
			})
		if err != nil || result.Outcome() != application.PublicationDraftPublished {
			t.Fatalf("发布 = %q, %v", result.Outcome(), err)
		}
		stored, _, _ := drafts.LoadDraft(context.Background(), shell.TenantID, shell.Kind, shell.ObjectID, shell.Version)
		return registry, stored
	}

	t.Run("sources keep the order they were submitted in", func(t *testing.T) {
		registry, stored := publish(t, application.SellPriceArrangementDeclaration{
			Sources: sellPriceScopes(t, "SYN-SCOPE-EXCLUSIVE", "SYN-SCOPE-STANDARD"),
		})
		if stored.Canonical().Canonicalization() != "PCC-3" {
			t.Fatalf("draft snapshot canonicalized as %s, want PCC-3", stored.Canonical().Canonicalization())
		}
		if len(registry.savedVersions) != 1 || registry.savedVersions[0].ContentDigest() != stored.Canonical().Digest() {
			t.Fatalf("入册摘要与载体摘要不一致：%#v vs %s", registry.savedVersions, stored.Canonical().Digest())
		}
		if len(registry.savedSellPrice) != 1 {
			t.Fatalf("saved sell price arrangements = %#v", registry.savedSellPrice)
		}
		sources, priced := registry.savedSellPrice[0].Sources()
		if !priced || len(sources) != 2 || sources[0].String() != "SYN-SCOPE-EXCLUSIVE" || sources[1].String() != "SYN-SCOPE-STANDARD" {
			t.Fatalf("published sources = %v priced = %v, want the submitted order", sources, priced)
		}
	})

	t.Run("an explicit no-sell-price arrangement keeps its basis", func(t *testing.T) {
		registry, _ := publish(t, application.SellPriceArrangementDeclaration{
			NoSellPriceBasis: pcValue(t, domain.NewNoSellPriceBasis, "SYN-NO-SELL-PRICE-01"),
		})
		if len(registry.savedSellPrice) != 1 {
			t.Fatalf("saved sell price arrangements = %#v", registry.savedSellPrice)
		}
		basis, notPriced := registry.savedSellPrice[0].NoSellPriceBasis()
		if !notPriced || basis.String() != "SYN-NO-SELL-PRICE-01" {
			t.Fatalf("published basis = %q notPriced = %v", basis, notPriced)
		}
	})
}

// Covers: ADR-0380 决定一「二选一」与归属——两种都声明的约定折不成文档，受控批文那一项答`未受理`带成因、一个字节
// 不写；挂在客户合同之外的版本上整项拒，不静默丢弃。没有销售价格约定的合同照旧发布，不写这一通道、不补默认。
func TestASellPriceArrangementThatDoesNotStandIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	t.Run("both sources and a no-sell-price basis", func(t *testing.T) {
		declarations := contractDeclarations(t, "rules-1")
		declarations.SellPriceArrangement = &application.SellPriceArrangementDeclaration{
			Sources:          sellPriceScopes(t, "SYN-SCOPE-STANDARD"),
			NoSellPriceBasis: pcValue(t, domain.NewNoSellPriceBasis, "SYN-NO-SELL-PRICE-01"),
		}
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.CustomerContractObject, "contract-1", "v1"),
			Approval:     publishApproval(t, "contract-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: declarations,
		})
		if err != nil {
			t.Fatalf("Handle：%v——未受理不是 error", err)
		}
		if result.Outcome() != application.CommercialPublicationNotAccepted || !errors.Is(result.RefusalCause(), domain.ErrInvalidSellPriceArrangement) {
			t.Fatalf("outcome = %q, cause = %v; want NOT_ACCEPTED / ErrInvalidSellPriceArrangement", result.Outcome(), result.RefusalCause())
		}
		if len(registry.savedVersions) != 0 || len(registry.declarationLog) != 0 || registry.loads != 0 {
			t.Fatal("未受理写了库或读了整册")
		}
	})

	t.Run("on a version that is not a customer contract", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		_, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.ServiceProductObject, "product-1", "v1"),
			Approval:     publishApproval(t, "product-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{
				SellPriceArrangement: &application.SellPriceArrangementDeclaration{Sources: sellPriceScopes(t, "SYN-SCOPE-STANDARD")},
			},
		})
		if !errors.Is(err, domain.ErrUnusableContract) {
			t.Fatalf("err = %v, want ErrUnusableContract", err)
		}
		if len(registry.savedVersions) != 0 || len(registry.savedSellPrice) != 0 {
			t.Fatal("挂错拥有者的约定写了库")
		}
	})

	t.Run("a contract without the arrangement publishes as before", func(t *testing.T) {
		declarations := contractDeclarations(t, "rules-1")
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         customerContractSpec(t, "contract-1", "v1", declarations),
			Approval:     publishApproval(t, "contract-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: declarations,
		})
		if err != nil || result.Outcome() != application.CommercialVersionPublishedEffective {
			t.Fatalf("outcome = %q, %v", result.Outcome(), err)
		}
		if len(registry.savedSellPrice) != 0 || strings.Contains(strings.Join(registry.declarationLog, ","), "contract-sell-price") {
			t.Fatalf("没有销售价格约定的合同写了这一通道：%v", registry.declarationLog)
		}
		if !strings.HasPrefix(registry.savedVersions[0].ContentDigest().String(), "PCC-1:") {
			t.Fatalf("registered digest = %s, want the unchanged PCC-1 digest", registry.savedVersions[0].ContentDigest())
		}
	})
}
