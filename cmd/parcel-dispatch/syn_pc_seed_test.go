package main

import (
	"context"
	"testing"
	"time"

	nrpartycommercial "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/partycommercial"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件是 SYN-PC-SEED / SYN-PC-PRODUCT：隔离 S 夹具，给已接受委托的
// ResolutionID=`SYN-RES-01` 配上一份真实 PC 闭包——同一次首次 Save 同时采用接单规则包
// 与可观察的 NetworkServiceForm 服务产品。生产路径与 synSCommercialBasis 都不动：
// formDecision 继续发明快照；种子只补持久化面。禁止第二次 Save 同标识，禁止
// INSERT network_definition，禁止 SaveServiceProduct。
//
// 空资格清单会让 JudgeIntakeEligibility 直接 ESTABLISHED 并形成承诺；只种一种来源会
// 让另一条链走 NOT_APPLICABLE 并入账。两件都禁止。

const (
	synPCResolutionID   = "SYN-RES-01"
	synPCRuleObject     = "SYN-RULES-01"
	synPCRuleVersion    = "v1"
	synPCProductObject  = "SYN-PRODUCT-01"
	synPCProductVersion = "v1"
	synPCQualification  = "INTAKE-QUAL/customs-precheck"
	synPCUnprovenBasis  = "INTAKE_QUALIFICATION_UNPROVEN/" + synPCQualification
	synPCViewRevision   = "SYN-VIEW-01"
	acceptanceRuleKind  = 4 // pcdomain.AcceptanceRulePackageObject
)

// seedSYNPCEligibility 一次种完闭包与收寄资格。必须用 fixture 同一份 pool：另开
// pgtest.Pool 会种到另一个库，beat 读不到。
func seedSYNPCEligibility(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	if fixture.pool == nil {
		t.Fatal("fixture.pool 是空的——种子必须落在 beat 用的那份库上")
	}

	live := synPCEffectiveRulePackage(t, fixture)
	product := synPCEffectiveServiceProduct(t, fixture)
	seedIntakeQualification(t, fixture)
	seedResolvedClosure(t, fixture, live, product)
}

func synPCEffectiveRulePackage(t *testing.T, fixture *synVerticalFixture) pcdomain.CommercialVersion {
	t.Helper()
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		mustPC(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-RULES-01"),
		mustPC(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-RULES-01"),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	live, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      mustPC(t, pcdomain.NewTenantID, fixture.identity.TenantID().String()),
		Kind:          pcdomain.AcceptanceRulePackageObject,
		ObjectID:      mustPC(t, pcdomain.NewCommercialObjectID, synPCRuleObject),
		Version:       mustPC(t, pcdomain.NewCommercialVersionLabel, synPCRuleVersion),
		Scope:         mustPC(t, pcdomain.NewCommercialScopeReference, "SYN-PC-SCOPE-01"),
		ContentDigest: mustPC(t, pcdomain.NewCommercialContentDigest, "sha256:SYN-RULES-01"),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   approvedAt,
		EffectiveAt:   approvedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("重建已生效接单规则包：%v", err)
	}
	return live
}

func synPCEffectiveServiceProduct(t *testing.T, fixture *synVerticalFixture) pcdomain.ServiceProduct {
	t.Helper()
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("产品有效区间：%v", err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		mustPC(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-PRODUCT-01"),
		mustPC(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-PRODUCT-01"),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("产品批准依据：%v", err)
	}
	live, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      mustPC(t, pcdomain.NewTenantID, fixture.identity.TenantID().String()),
		Kind:          pcdomain.ServiceProductObject,
		ObjectID:      mustPC(t, pcdomain.NewCommercialObjectID, synPCProductObject),
		Version:       mustPC(t, pcdomain.NewCommercialVersionLabel, synPCProductVersion),
		Scope:         mustPC(t, pcdomain.NewCommercialScopeReference, "SYN-PC-SCOPE-01"),
		ContentDigest: mustPC(t, pcdomain.NewCommercialContentDigest, "sha256:SYN-PRODUCT-01"),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   approvedAt,
		EffectiveAt:   approvedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("重建已生效服务产品版本：%v", err)
	}
	product, err := pcdomain.NewServiceProduct(live, pcdomain.NetworkServiceForm)
	if err != nil {
		t.Fatalf("构造 NetworkServiceForm 服务产品：%v", err)
	}
	return product
}

func seedIntakeQualification(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	tenant := fixture.identity.TenantID().String()
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO party_commercial.intake_qualification_content
			(tenant_id, object_kind, object_id, version_label)
		 VALUES ($1, $2, $3, $4)`,
		tenant, acceptanceRuleKind, synPCRuleObject, synPCRuleVersion); err != nil {
		t.Fatalf("登记收寄资格父行：%v", err)
	}
	for _, source := range []string{"NODE_INTAKE", "OFFSITE_PICKUP"} {
		if _, err := fixture.pool.Exec(t.Context(),
			`INSERT INTO party_commercial.intake_allowed_source
				(tenant_id, object_kind, object_id, version_label, source_kind)
			 VALUES ($1, $2, $3, $4, $5)`,
			tenant, acceptanceRuleKind, synPCRuleObject, synPCRuleVersion, source); err != nil {
			t.Fatalf("登记允许来源 %s：%v", source, err)
		}
	}
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO party_commercial.intake_qualification_ref
			(tenant_id, object_kind, object_id, version_label, rule_reference)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenant, acceptanceRuleKind, synPCRuleObject, synPCRuleVersion, synPCQualification); err != nil {
		t.Fatalf("登记硬资格引用：%v", err)
	}
}

func seedResolvedClosure(
	t *testing.T,
	fixture *synVerticalFixture,
	rules pcdomain.CommercialVersion,
	product pcdomain.ServiceProduct,
) {
	t.Helper()
	anchorAt := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	anchor, err := pcdomain.NewSelectionAnchor(anchorAt, mustPC(t, pcdomain.NewAnchorPolicyVersion, "SYN-ANCHOR-POLICY-1"))
	if err != nil {
		t.Fatalf("选用锚点：%v", err)
	}
	key := pcdomain.ClosureResolutionKey{
		TenantID:             mustPC(t, pcdomain.NewTenantID, fixture.identity.TenantID().String()),
		CustomerAccountID:    mustPC(t, pcdomain.NewCustomerAccountID, fixture.identity.CustomerAccountID().String()),
		LegalEntityCandidate: mustPC(t, pcdomain.NewLegalEntityReference, "SYN-LEGAL-01"),
		Scope:                mustPC(t, pcdomain.NewCommercialScopeReference, "SYN-PC-SCOPE-01"),
		Purpose:              pcdomain.AcceptanceControlPurpose,
		Anchor:               anchor,
		RequiredBases: []pcdomain.CommercialObjectKind{
			pcdomain.AcceptanceRulePackageObject,
			pcdomain.ServiceProductObject,
		},
	}
	closure, err := pcdomain.RehydrateCommercialClosure(pcdomain.RehydrateCommercialClosureSpec{
		Outcome:      pcdomain.UniquelyResolved,
		ResolutionID: mustPC(t, pcdomain.NewResolutionID, synPCResolutionID),
		Key:          key,
		Anchor:       anchor,
		ViewRevision: mustPC(t, pcdomain.NewAuthorityViewRevision, synPCViewRevision),
		Adopted: []pcdomain.RehydrateAdoptedBasisSpec{
			{Kind: pcdomain.AcceptanceRulePackageObject, Version: rules},
			{
				Kind:              pcdomain.ServiceProductObject,
				Version:           product.Version(),
				ServiceProduct:    product,
				HasServiceProduct: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("重建唯一已解析闭包：%v", err)
	}

	store, err := pcpostgres.NewCommercialResolutions(fixture.db)
	if err != nil {
		t.Fatalf("构造解析库：%v", err)
	}
	var outcome pcports.ResolutionSaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = store.Save(txCtx, closure)
		return saveErr
	})
	if outcome != pcports.ResolutionSaved {
		t.Fatalf("save outcome = %s, want 已写入", outcome)
	}
}

// assertSYNPCEligibilitySeeded 用真读口钉死种子：闭包在、规则包对、产品形态可观察、
// 两种来源都允许、硬资格非空。缺产品会让接受链停在适用性未决而不是证据未配置；
// 缺硬资格会让收寄链 ESTABLISHED。
func assertSYNPCEligibilitySeeded(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	tenant := mustPC(t, pcdomain.NewTenantID, fixture.identity.TenantID().String())

	resolutions, err := pcpostgres.NewCommercialResolutions(fixture.db)
	if err != nil {
		t.Fatalf("构造解析库：%v", err)
	}
	closure, found, err := resolutions.LoadResolution(t.Context(), tenant, mustPC(t, pcdomain.NewResolutionID, synPCResolutionID))
	if err != nil || !found {
		t.Fatalf("LoadResolution found = %v err = %v，种子没把 SYN-RES-01 写进解析库", found, err)
	}
	adopted, ok := closure.AdoptedFor(pcdomain.AcceptanceRulePackageObject)
	if !ok || adopted.Version().ObjectID().String() != synPCRuleObject {
		t.Fatalf("闭包采用的规则包 object = %q ok = %v, want %s", adopted.Version().ObjectID(), ok, synPCRuleObject)
	}
	productBasis, ok := closure.AdoptedFor(pcdomain.ServiceProductObject)
	if !ok {
		t.Fatal("闭包没采用服务产品——ADR-0064 回指后适用性仍会停在产品不可观察")
	}
	product, ok := productBasis.ServiceProduct()
	if !ok || product.Form() != pcdomain.NetworkServiceForm {
		t.Fatalf("服务产品形态不可观察或不是 NETWORK_SERVICE：ok=%v form=%q", ok, product.Form())
	}

	declarations, err := pcpostgres.NewStageContentDeclarations(fixture.db)
	if err != nil {
		t.Fatalf("构造阶段内容读口：%v", err)
	}
	content, found, err := declarations.LoadIntakeQualification(t.Context(), tenant, adopted.Version())
	if err != nil || !found {
		t.Fatalf("LoadIntakeQualification found = %v err = %v", found, err)
	}
	if !content.Allows(pcdomain.DeclaredNodeIntake) || !content.Allows(pcdomain.DeclaredOffsitePickup) {
		t.Fatal("允许来源缺 NODE_INTAKE 或 OFFSITE_PICKUP——缺一种会让对应链走 NOT_APPLICABLE 并入账")
	}
	if quals := content.Qualifications(); len(quals) == 0 || quals[0].String() != synPCQualification {
		t.Fatalf("qualifications = %#v, want 头一项 %s（空清单会 ESTABLISHED）", quals, synPCQualification)
	}
}

// assertIntakeEligibilityUnproven 用生产形状的资格适配器直接判一次，把未决原因钉在
// NOT_ESTABLISHED 而不是 UNCONFIGURED。失败码分不出这两格。
func assertIntakeEligibilityUnproven(t *testing.T, fixture *synVerticalFixture, kind psdomain.IntakeSourceKind) {
	t.Helper()
	resolutions, err := pcpostgres.NewCommercialResolutions(fixture.db)
	if err != nil {
		t.Fatalf("构造解析库：%v", err)
	}
	owners, err := pspartycommercial.NewResolvedAdoptedStageOwner(fixture.requests, resolutions)
	if err != nil {
		t.Fatalf("构造采用 owner：%v", err)
	}
	stageContent, err := pcpostgres.NewStageContentDeclarations(fixture.db)
	if err != nil {
		t.Fatalf("构造阶段内容读口：%v", err)
	}
	declared := pspartycommercial.NewDeclaredStageContent(stageContent, stageContent, stageContent, owners)
	eligibility := pspartycommercial.NewServiceStageRulesAdapter(
		declared, declared, declared, nil,
		pspartycommercial.UnconfiguredIntakeQualificationEvidence{},
	)

	judged, configured, err := eligibility.JudgeIntakeEligibility(
		t.Context(), fixture.identity, fixture.requestID, synPCIntakeSource(t, kind),
	)
	if err != nil {
		t.Fatalf("JudgeIntakeEligibility：%v", err)
	}
	if !configured {
		t.Fatal("configured = false——停在 UNCONFIGURED，种子没被 owner 看见")
	}
	if judged.Outcome != psports.IntakeEligibilityNotEstablished {
		t.Fatalf("outcome = %q, want NOT_ESTABLISHED（空清单会 ESTABLISHED）", judged.Outcome)
	}
	if judged.Basis.String() != synPCUnprovenBasis {
		t.Fatalf("basis = %q, want %s", judged.Basis, synPCUnprovenBasis)
	}
}

// assertRoutingApplicabilityRequired 用生产适用性视图钉死：闭包回指后形态译成要求判断。
// 失败码分不出适用性未决与证据未配置，拍前必须另证这一格已经越过。
func assertRoutingApplicabilityRequired(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	closures, err := pcpostgres.NewCommercialResolutions(fixture.db)
	if err != nil {
		t.Fatalf("构造解析库：%v", err)
	}
	view, err := nrpartycommercial.NewRoutingApplicability(closures)
	if err != nil {
		t.Fatalf("构造适用性视图：%v", err)
	}
	resolution, err := nrdomain.NewCommercialResolutionReference(synPCResolutionID)
	if err != nil {
		t.Fatalf("解析引用：%v", err)
	}
	eligibility, err := view.AssessRoutingApplicability(t.Context(), synInitialRouteKey(t, fixture), resolution)
	if err != nil {
		t.Fatalf("AssessRoutingApplicability：%v——种子没让形态可观察", err)
	}
	if !eligibility.JudgmentRequired() {
		t.Fatal("NETWORK_SERVICE 被译成了不要求——那是把唯一合法形态做成了不适用默认值")
	}
}

// assertRouteEvidenceUnconfigured 用生产登记册钉死下一诚实停点：这个范围没有网络定义。
func assertRouteEvidenceUnconfigured(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	definitions, err := nrpostgres.NewNetworkDefinitions(fixture.db)
	if err != nil {
		t.Fatalf("构造网络定义登记册：%v", err)
	}
	_, configured, err := definitions.LoadInitialRouteEvidence(t.Context(), synInitialRouteKey(t, fixture))
	if err != nil {
		t.Fatalf("LoadInitialRouteEvidence：%v", err)
	}
	if configured {
		t.Fatal("网络定义已配置——种子不得 INSERT network_definition")
	}
}

func synInitialRouteKey(t *testing.T, fixture *synVerticalFixture) nrdomain.InitialRouteJudgmentKey {
	t.Helper()
	return nrdomain.InitialRouteJudgmentKey{
		TenantID:           mustNR(t, nrdomain.NewTenantID, fixture.identity.TenantID().String()),
		CustomerAccountID:  mustNR(t, nrdomain.NewCustomerAccountID, fixture.identity.CustomerAccountID().String()),
		ShipmentRequestID:  mustNR(t, nrdomain.NewShipmentRequestID, fixture.requestID.String()),
		AcceptanceBaseline: mustNR(t, nrdomain.NewAcceptanceBaselineReference, "SYN-VER-01"),
		DeclaredParcelID:   mustNR(t, nrdomain.NewDeclaredParcelID, "SYN-PARCEL-01"),
		ServicePurpose:     mustNR(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"),
	}
}

func mustNR[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func synPCIntakeSource(t *testing.T, kind psdomain.IntakeSourceKind) psdomain.IntakeSource {
	t.Helper()
	source, err := psdomain.NewIntakeSource(psdomain.IntakeSourceSpec{
		Kind:       kind,
		Object:     mustPS(t, psdomain.NewSourceObjectReference, "SYN-UNIT-01"),
		Parcel:     mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01"),
		Place:      mustPS(t, psdomain.NewIntakePlaceReference, "SYN-PLACE-01"),
		Control:    mustPS(t, psdomain.NewIntakeControlReference, "SYN-CONTROL-01"),
		Version:    mustPS(t, psdomain.NewSourceResultVersion, "SYN-SOURCE-V1"),
		OccurredAt: time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造来源：%v", err)
	}
	return source
}

func mustPC[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}
