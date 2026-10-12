package main

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	visibilitydomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// Covers: 票 customer-service-rule-contract-applicability/04 完成判据 3（ADR-0176 决定二）——生产装配
// `buildClaimEligibilityRules` 在真库上接通层次读口：同一范围并存一版挂合同 SYN-CONTRACT-1 的客户服务规则（壳指名该合同，
// 正文只登首次索赔期限、起算事件定制）与一版产品版（壳空、正文挂服务产品，两行都登）。PC 真解析器按合同优先采纳合同版，
// 委托据此接受；索赔读回的首次索赔期限是合同版那一行、冻合同版的版本引用，最低材料是从产品底座继承的那一行、冻底座的
// 版本引用。闭包、回指、底座选法与正文都走真件，与生产路径同一条。测试输入是隔离合成，只记 `S`。
func TestTheWiredClaimsInheritTheRowsAContractTierRuleLeavesToTheProductBase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()

	tenant := mustValue(t, pcdomain.NewTenantID, "SYN-TENANT-1")
	scope := mustValue(t, pcdomain.NewCommercialScopeReference, "SYN-SCOPE-1")

	claims, err := buildClaimsOrchestration(db)
	if err != nil {
		t.Fatalf("装配索赔编排：%v", err)
	}
	rules, err := buildClaimEligibilityRules(db)
	if err != nil {
		t.Fatalf("装配资格规则读面：%v", err)
	}

	submission := submissionCommand(t)
	command := visibilityapp.ReceiveClaimCommand{
		TenantID:    mustValue(t, visibilitydomain.NewTenantID, submission.Identity.TenantID().String()),
		Batch:       mustValue(t, visibilitydomain.NewClaimBatchReference, "SYN-CLAIM-BATCH-1"),
		Item:        mustValue(t, visibilitydomain.NewClaimItemID, "SYN-CLAIM-ITEM-1"),
		Customer:    mustValue(t, visibilitydomain.NewCustomerAccountReference, submission.Identity.CustomerAccountID().String()),
		Applicant:   mustValue(t, visibilitydomain.NewApplicantReference, "SYN-APPLICANT-1"),
		Contract:    mustValue(t, visibilitydomain.NewContractScopeReference, "SYN-CONTRACT-1"),
		Target:      mustValue(t, visibilitydomain.NewRequestScopeReference, submission.DeclaredParcelIDs[0].String()),
		Kind:        mustValue(t, visibilitydomain.NewClaimKindReference, "SYN-KIND-LOSS"),
		SubmittedAt: time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
	}
	if received, err := claims.ReceiveClaim(ctx, command); err != nil || received.Outcome() != visibilityapp.ClaimReceived {
		t.Fatalf("受理索赔：outcome=%v err=%v", received.Outcome(), err)
	}
	registrar, err := vepostgres.NewCatalogRegistrar(db)
	if err != nil {
		t.Fatalf("构造目录写入方：%v", err)
	}
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		outcome, registerErr := registrar.RegisterClaimEligibility(txCtx, command.TenantID,
			veports.ClaimEligibilityRegistration{
				Header:       veports.CatalogApprovalHeader{Version: "SYN-CLAIM-RULES-1", ApprovedBy: "SYN-OPERATOR-1"},
				Contract:     command.Contract,
				CoveredKinds: []visibilitydomain.ClaimKindReference{command.Kind},
			})
		if registerErr == nil && outcome != veports.CatalogVersionRegistered {
			t.Errorf("登记索赔声明 = %s，应成功", outcome)
		}
		return registerErr
	}); err != nil {
		t.Fatalf("事务内登记索赔声明失败：%v", err)
	}

	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造商业发布登记册：%v", err)
	}
	contract := effectiveCommercialShell(t, tenant, pcdomain.CustomerContractObject, "SYN-CONTRACT-1", scope)
	productTier := effectiveCustomerServiceRule(t, tenant, scope)
	contractTier := effectiveContractTierCustomerServiceRule(t, tenant, scope, "SYN-CSR-CONTRACT-1", "SYN-CONTRACT-1")
	for _, shell := range []pcdomain.CommercialVersion{contract, productTier, contractTier} {
		var outcome pcports.PublicationSaveOutcome
		if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var saveErr error
			outcome, saveErr = publications.SaveVersion(txCtx, shell)
			return saveErr
		}); err != nil || outcome != pcports.PublicationSaved {
			t.Fatalf("发布壳 %s：outcome=%s err=%v", shell.ObjectID(), outcome, err)
		}
	}
	saveBody := func(body pcdomain.CustomerServiceRuleVersion) {
		t.Helper()
		var outcome pcports.CustomerServiceRuleSaveOutcome
		if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var saveErr error
			outcome, saveErr = publications.SaveCustomerServiceRule(txCtx, body)
			return saveErr
		}); err != nil || outcome != pcports.CustomerServiceRuleSaved {
			t.Fatalf("登记正文 %s：outcome=%s err=%v", body.Version().ObjectID(), outcome, err)
		}
	}
	productBody, err := pcdomain.NewCustomerServiceRuleVersion(productTier,
		pcdomain.CustomerServiceRuleAppliesToServiceProduct(mustValue(t, pcdomain.NewCommercialObjectID, "SYN-PRODUCT-1")),
		mustValue(t, pcdomain.NewPartyID, "SYN-OPERATOR-1"), scope,
		[]pcdomain.ClaimDeadlineRule{mustDeadlineRule(t, pcdomain.FirstClaimDeadline, "SYN-EVENT-DELIVERED", 30, "SYN-CALENDAR-1")},
		[]pcdomain.MinimumMaterialsRule{mustMaterialsRule(t, "SYN-KIND-LOSS", "SYN-MAT-PHOTO", "SYN-MAT-INVOICE")})
	if err != nil {
		t.Fatalf("产品版正文：%v", err)
	}
	contractBody, err := pcdomain.NewCustomerServiceRuleVersion(contractTier,
		pcdomain.CustomerServiceRuleAppliesToCustomerContract(mustValue(t, pcdomain.NewCommercialObjectID, "SYN-CONTRACT-1")),
		mustValue(t, pcdomain.NewPartyID, "SYN-OPERATOR-1"), scope,
		[]pcdomain.ClaimDeadlineRule{mustDeadlineRule(t, pcdomain.FirstClaimDeadline, "SYN-EVENT-HANDED-OVER", 15, "SYN-CALENDAR-1")},
		nil)
	if err != nil {
		t.Fatalf("合同版正文：%v", err)
	}
	saveBody(productBody)
	saveBody(contractBody)

	keyStore, err := pspostgres.NewCommercialResolutionKeyStore(db)
	if err != nil {
		t.Fatalf("构造解析键登记面：%v", err)
	}
	keys, err := pspartycommercial.NewCommercialResolutionKeys(keyStore)
	if err != nil {
		t.Fatalf("构造解析键适配器：%v", err)
	}
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		outcome, registerErr := keys.Register(txCtx, pspartycommercial.ResolutionKeyRegistration{
			TenantID:          submission.Identity.TenantID(),
			CustomerAccountID: submission.Identity.CustomerAccountID(),
			Scope:             scope,
			LegalEntity:       mustValue(t, pcdomain.NewLegalEntityReference, "SYN-LEGAL-1"),
			AnchorPolicy:      mustValue(t, pcdomain.NewAnchorPolicyVersion, "SYN-ANCHOR-POLICY-1"),
			AnchorAt:          time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC),
			RequiredBases:     []pcdomain.CommercialObjectKind{pcdomain.CustomerContractObject, pcdomain.CustomerServiceRuleObject},
		})
		if registerErr == nil && outcome != pspartycommercial.ResolutionKeySaved {
			t.Errorf("解析键行 outcome = %s，应成功", outcome)
		}
		return registerErr
	}); err != nil {
		t.Fatalf("登解析键行：%v", err)
	}
	key, formed, err := keys.FormResolutionKey(ctx, psports.CommercialBasisQuery{Identity: submission.Identity})
	if err != nil || !formed {
		t.Fatalf("按登记行成键：formed=%v err=%v", formed, err)
	}
	closure := resolveClosureOnRealAssembly(t, db, publications, key)
	if adopted, ok := closure.AdoptedFor(pcdomain.CustomerServiceRuleObject); !ok ||
		adopted.Version().ObjectID().String() != "SYN-CSR-CONTRACT-1" {
		t.Fatalf("闭包没按合同优先采纳合同版：ok=%v", ok)
	}
	acceptOnRealAssemblyWith(t, db, submission, closure.ResolutionID().String(), "SYN-DECISION-1")

	var answer veports.EligibilityRules
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		var readErr error
		var declared bool
		answer, declared, readErr = rules.RulesForClaim(txCtx, veports.EligibilityQuery{
			Tenant: command.TenantID, Batch: command.Batch, Item: command.Item, Customer: command.Customer,
			Contract: command.Contract, Target: command.Target, Kind: command.Kind, Applicant: command.Applicant,
		})
		if readErr == nil && !declared {
			t.Error("声明已登记却答不在场")
		}
		return readErr
	}); err != nil {
		t.Fatalf("读资格规则：%v", err)
	}
	deadline := answer.FilingDeadline
	if !deadline.Registered || deadline.RuleVersion != "SYN-TENANT-1/SYN-CSR-CONTRACT-1/v1" ||
		deadline.StartEvent != "SYN-EVENT-HANDED-OVER" {
		t.Fatalf("首次索赔期限维 = %#v，合同版有这一行，要答合同版的定制起算事件并冻合同版", deadline)
	}
	materials := answer.Materials
	if !materials.Registered || materials.RuleVersion != "SYN-TENANT-1/SYN-CSR-1/v1" ||
		len(materials.Required) != 2 ||
		materials.Required[0].String() != "SYN-MAT-INVOICE" || materials.Required[1].String() != "SYN-MAT-PHOTO" {
		t.Fatalf("最低材料维 = %#v，合同版无这一行，要继承产品底座那一行并冻底座", materials)
	}
}

// effectiveContractTierCustomerServiceRule 造一版壳上指名客户合同的客户服务规则（PC 闭包按壳把它归合同层）。被指名的合同
// 由调用方另行发布；这里的发布门只问它在不在册，按已发布答。
func effectiveContractTierCustomerServiceRule(
	t *testing.T, tenant pcdomain.TenantID, scope pcdomain.CommercialScopeReference, objectID, contract string,
) pcdomain.CommercialVersion {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	draft, err := pcdomain.NewCommercialDraft(pcdomain.CommercialVersionSpec{
		TenantID:      tenant,
		Kind:          pcdomain.CustomerServiceRuleObject,
		ObjectID:      mustValue(t, pcdomain.NewCommercialObjectID, objectID),
		Version:       mustValue(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         scope,
		ContentDigest: mustValue(t, pcdomain.NewCommercialContentDigest, "sha256:"+objectID),
		Effective:     interval,
		References: map[pcdomain.CommercialObjectKind]pcdomain.CommercialObjectID{
			pcdomain.CustomerContractObject: mustValue(t, pcdomain.NewCommercialObjectID, contract),
		},
	})
	if err != nil {
		t.Fatalf("商业草稿：%v", err)
	}
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	basis, err := pcdomain.NewApprovalBasis(
		mustValue(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-"+objectID),
		mustValue(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-"+objectID),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	contractPublished := func(pcdomain.CommercialObjectKind, pcdomain.CommercialObjectID) pcdomain.NamedReferenceStanding {
		return pcdomain.NamedReferencePublished
	}
	published, err := draft.Publish(basis, pcdomain.ApprovalRoleConfirmed, approvedAt, contractPublished)
	if err != nil {
		t.Fatalf("发布：%v", err)
	}
	live, err := published.TakeEffect(approvedAt)
	if err != nil {
		t.Fatalf("生效：%v", err)
	}
	return live
}
