package main

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

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

// Covers: `/claims` 的第二参是真编排——索赔库、追偿库、标识签发与责任结论 Outbox
// 在真实 PostgreSQL 上装得起来；受理真实落库且重放走已有项（证首笔事务真的提交了）；
// 资格缝接真后（票 ve-claims-read-seams/01）审核入口按租户的册作答且两态分明——
// 未登记租户停在「声明待登记」的业务格（不是 error，那会被端点折成 5xx；也不是
// `不予受理`或`不受理`，那是业务否定），别的租户登了册也不改这一答；本租户登册后
// 审核走进逐维核对、停在册上真实缺的那一维。证据缝接真后（票 ve-claims-read-seams/02）
// 零收讫答 known=true 的零件、登记后按行作答。索赔项全程一字不动。测试输入是隔离
// 合成，只记 `S`，不进生产装配。
func TestTheWiredClaimsAnswerHonestlyAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	claims, err := buildClaimsOrchestration(db)
	if err != nil {
		t.Fatalf("装配索赔编排：%v", err)
	}

	command := visibilityapp.ReceiveClaimCommand{
		TenantID:    mustValue(t, visibilitydomain.NewTenantID, "SYN-TENANT-1"),
		Batch:       mustValue(t, visibilitydomain.NewClaimBatchReference, "SYN-CLAIM-BATCH-1"),
		Item:        mustValue(t, visibilitydomain.NewClaimItemID, "SYN-CLAIM-ITEM-1"),
		Customer:    mustValue(t, visibilitydomain.NewCustomerAccountReference, "SYN-CUSTOMER-1"),
		Applicant:   mustValue(t, visibilitydomain.NewApplicantReference, "SYN-APPLICANT-1"),
		Contract:    mustValue(t, visibilitydomain.NewContractScopeReference, "SYN-CONTRACT-1"),
		Target:      mustValue(t, visibilitydomain.NewRequestScopeReference, "SYN-PARCEL-1"),
		Kind:        mustValue(t, visibilitydomain.NewClaimKindReference, "SYN-KIND-LOSS"),
		SubmittedAt: time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
	}

	received, err := claims.ReceiveClaim(t.Context(), command)
	if err != nil {
		t.Fatalf("受理索赔：%v", err)
	}
	if got := received.Outcome(); got != visibilityapp.ClaimReceived {
		t.Fatalf("outcome = %v, want CLAIM_RECEIVED", got)
	}
	if _, has := received.Claim(); !has {
		t.Fatal("受理成功却没带回索赔项")
	}

	replay, err := claims.ReceiveClaim(t.Context(), command)
	if err != nil {
		t.Fatalf("重放同一份受理：%v", err)
	}
	if got := replay.Outcome(); got != visibilityapp.ClaimExistingResult {
		t.Fatalf("outcome = %v, want CLAIM_EXISTING_RESULT——重放没走已有项，首笔事务的落库没有提交", got)
	}

	// 票 ve-claims-read-seams/01 的验收钉（换下接线票 04 的 UNAVAILABLE 钉）：资格缝
	// 已接真，未登记租户的审核停在「声明待登记」的业务格。三个都不是它的答案——error
	// （端点会折成 5xx NO_ANSWER_FORMED）、`不予受理`（ADR-0051 的永久格，默认拒赔）、
	// `已过审`（默认放行）。
	screenCommand := visibilityapp.ScreenClaimCommand{
		TenantID: command.TenantID,
		Batch:    command.Batch,
		Item:     command.Item,
	}
	screened, err := claims.ScreenClaim(t.Context(), screenCommand)
	if err != nil {
		t.Fatalf("未登记租户的审核不该以 error 交回（那是 5xx，不是未决）：%v", err)
	}
	if got := screened.Outcome(); got != visibilityapp.HandleClaimUndecided {
		t.Fatalf("outcome = %v, want UNDECIDED——声明待登记既不是业务否定也不是放行", got)
	}
	if got := screened.UndecidedReason(); got != visibilityapp.EligibilityCatalogueNotConfigured {
		t.Fatalf("reason = %v, want ELIGIBILITY_CATALOGUE_NOT_CONFIGURED——恢复动作是登记声明，不再是接缝", got)
	}

	// 别的租户登了册也不改这一答：读的若不是查询租户自己的册，这一格就会串。
	registrar, err := vepostgres.NewCatalogRegistrar(db)
	if err != nil {
		t.Fatalf("构造目录写入方：%v", err)
	}
	registerEligibility := func(tenant visibilitydomain.TenantID) {
		t.Helper()
		var outcome veports.CatalogRegistrationOutcome
		if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			var registerErr error
			outcome, registerErr = registrar.RegisterClaimEligibility(txCtx, tenant,
				veports.ClaimEligibilityRegistration{
					Header:       veports.CatalogApprovalHeader{Version: "SYN-CLAIM-RULES-1", ApprovedBy: "SYN-OPERATOR-1"},
					Contract:     command.Contract,
					CoveredKinds: []visibilitydomain.ClaimKindReference{command.Kind},
				})
			return registerErr
		}); err != nil {
			t.Fatalf("事务内登记失败：%v", err)
		}
		if outcome != veports.CatalogVersionRegistered {
			t.Fatalf("登记索赔声明应成功，实得 %s", outcome)
		}
	}
	registerEligibility(mustValue(t, visibilitydomain.NewTenantID, "SYN-TENANT-2"))
	crossTenant, err := claims.ScreenClaim(t.Context(), screenCommand)
	if err != nil {
		t.Fatalf("跨租户审核：%v", err)
	}
	if got := crossTenant.UndecidedReason(); got != visibilityapp.EligibilityCatalogueNotConfigured {
		t.Fatalf("reason = %v——别的租户登册改了本租户的答案，租户维没真的进查询", got)
	}

	// 本租户登册后按册作答：声明在场、类型在保，审核走进逐维核对，停在册上真实缺的
	// 第一维（首次索赔期限属 `PAR-VIS-08` 待登记实例参数）——证明答案确实来自这租户
	// 的册，而不是任何一格顶位。
	registerEligibility(command.TenantID)
	registered, err := claims.ScreenClaim(t.Context(), screenCommand)
	if err != nil {
		t.Fatalf("登册后的审核：%v", err)
	}
	if got := registered.Outcome(); got != visibilityapp.HandleClaimUndecided {
		t.Fatalf("outcome = %v, want UNDECIDED——期限维还没登记", got)
	}
	if got := registered.UndecidedReason(); got != visibilityapp.EligibilityFilingDeadlineNotRegistered {
		t.Fatalf("reason = %v, want ELIGIBILITY_FILING_DEADLINE_NOT_REGISTERED——登册后要按册答到真实缺的那一维", got)
	}

	// 证据缝经公开入口仍走不到：材料清单维未登记时，逐维核对在读证据之前就停下
	// （judgeMinimumMaterials 先看 Registered）。直接钉装配所接的真读适配器（票
	// ve-claims-read-seams/02 换下显式未配置桩）：没有收讫行答 known=true 的零件
	// ——「查过了，一件都没收到」的有效事实，与退役桩的「无从查起」（known=false）
	// 语义相反；经受控写入口登记一笔后按行作答。
	evidence, err := vepostgres.NewClaimMaterialReceipts(db)
	if err != nil {
		t.Fatalf("构造证据视图：%v", err)
	}
	materials, known, err := evidence.ReceivedMaterials(
		t.Context(), command.TenantID, command.Batch, command.Item)
	if err != nil {
		t.Fatalf("读空归集不该以 error 交回：%v", err)
	}
	if !known {
		t.Fatal("归集面已接真却答 known=false——「无从查起」那格已随桩退役")
	}
	if len(materials) != 0 {
		t.Fatalf("零收讫却交回材料清单：%v", materials)
	}

	receipts, err := vepostgres.NewMaterialReceiptRegistrar(db)
	if err != nil {
		t.Fatalf("构造归集面写入方：%v", err)
	}
	photo := mustValue(t, visibilitydomain.NewMaterialRequirementReference, "SYN-MAT-PHOTO")
	var receiptOutcome veports.MaterialReceiptWriteOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var registerErr error
		receiptOutcome, registerErr = receipts.RegisterReceipt(txCtx, command.TenantID, veports.MaterialReceipt{
			Batch:      command.Batch,
			Item:       command.Item,
			Material:   photo,
			ReceivedAt: time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC),
			ReceivedBy: "SYN-OPERATOR-1",
		})
		return registerErr
	}); err != nil {
		t.Fatalf("事务内登记收讫失败：%v", err)
	}
	if receiptOutcome != veports.MaterialReceiptRecorded {
		t.Fatalf("登记收讫应成功，实得 %d", receiptOutcome)
	}
	materials, known, err = evidence.ReceivedMaterials(
		t.Context(), command.TenantID, command.Batch, command.Item)
	if err != nil || !known {
		t.Fatalf("登记后读现存集：err=%v known=%v", err, known)
	}
	if len(materials) != 1 || materials[0].String() != "SYN-MAT-PHOTO" {
		t.Fatalf("现存集 = %v，要登记的那一件", materials)
	}
}

// syntheticRuleKeys 是装配测试注入的解析键来源。生产装配传 nil——VE 词到 PC 闭包键的映射属实例
// 半边、今天没有登记面（票 ve-claims-read-seams/03「裁决」）；这里用一份 SYN 键把「已登记」态
// 钉出来，只在本测试里存在，不进生产装配，只记 `S`。别的租户形不成键，与登记面「无行即未配置」
// 同形。
type syntheticRuleKeys struct{ key pcdomain.ClosureResolutionKey }

func (keys syntheticRuleKeys) FormRuleResolutionKey(
	_ context.Context,
	query veports.EligibilityQuery,
) (pcdomain.ClosureResolutionKey, bool, error) {
	if query.Tenant.String() != keys.key.TenantID.String() {
		return pcdomain.ClosureResolutionKey{}, false, nil
	}
	return keys.key, true, nil
}

// Covers: 票 ve-claims-read-seams/03 完成标准——资格缝的首次索赔期限与最低材料两维经消费侧适配器
// 从 PC 客户服务规则正文读（ADR-0104 Consequences），对真库钉两态：本租户在 PC 没有生效的规则版本时
// 两维照旧未登记、编排停的格不变；PC 登了正文之后两维 Registered 为真、RuleVersion 是 PC 三段版本引用
// （Decision 五）、Required 与 PC 材料条目逐项相等，而票面留格的 Deadline / SupplementDeadline / Notice
// 仍是零值。另钉一格：生产装配（键来源 nil）在 PC 登了正文之后**行为一字不变**——那是显式未配置，
// 不是接错。测试输入是隔离合成，只记 `S`。
func TestTheWiredClaimsReadCustomerServiceRulesFromPartyCommercial(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()

	tenant := mustValue(t, pcdomain.NewTenantID, "SYN-TENANT-1")
	scope := mustValue(t, pcdomain.NewCommercialScopeReference, "SYN-SCOPE-1")
	anchor, err := pcdomain.NewSelectionAnchor(
		time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC),
		mustValue(t, pcdomain.NewAnchorPolicyVersion, "SYN-ANCHOR-POLICY-1"))
	if err != nil {
		t.Fatalf("选择锚点：%v", err)
	}
	keys := syntheticRuleKeys{key: pcdomain.ClosureResolutionKey{
		TenantID:             tenant,
		CustomerAccountID:    mustValue(t, pcdomain.NewCustomerAccountID, "SYN-CUSTOMER-1"),
		LegalEntityCandidate: mustValue(t, pcdomain.NewLegalEntityReference, "SYN-LEGAL-1"),
		Scope:                scope,
		Purpose:              pcdomain.AcceptanceControlPurpose,
		Anchor:               anchor,
		RequiredBases:        []pcdomain.CommercialObjectKind{pcdomain.CustomerServiceRuleObject},
	}}

	claims, err := buildClaimsOrchestrationWith(db, keys)
	if err != nil {
		t.Fatalf("装配索赔编排：%v", err)
	}
	rules, err := buildClaimEligibilityRules(db, systemClock{}, keys)
	if err != nil {
		t.Fatalf("装配资格规则读面：%v", err)
	}

	command := visibilityapp.ReceiveClaimCommand{
		TenantID:    mustValue(t, visibilitydomain.NewTenantID, "SYN-TENANT-1"),
		Batch:       mustValue(t, visibilitydomain.NewClaimBatchReference, "SYN-CLAIM-BATCH-1"),
		Item:        mustValue(t, visibilitydomain.NewClaimItemID, "SYN-CLAIM-ITEM-1"),
		Customer:    mustValue(t, visibilitydomain.NewCustomerAccountReference, "SYN-CUSTOMER-1"),
		Applicant:   mustValue(t, visibilitydomain.NewApplicantReference, "SYN-APPLICANT-1"),
		Contract:    mustValue(t, visibilitydomain.NewContractScopeReference, "SYN-CONTRACT-1"),
		Target:      mustValue(t, visibilitydomain.NewRequestScopeReference, "SYN-PARCEL-1"),
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
		if registerErr != nil {
			return registerErr
		}
		if outcome != veports.CatalogVersionRegistered {
			t.Fatalf("登记索赔声明应成功，实得 %s", outcome)
		}
		return nil
	}); err != nil {
		t.Fatalf("事务内登记失败：%v", err)
	}

	query := veports.EligibilityQuery{
		Tenant:    command.TenantID,
		Batch:     command.Batch,
		Item:      command.Item,
		Customer:  command.Customer,
		Contract:  command.Contract,
		Target:    command.Target,
		Kind:      command.Kind,
		Applicant: command.Applicant,
	}
	// 解析要固定闭包进 PC 解析库（ADR-0027），读面因此只在事务内可用——生产路径上索赔编排
	// 本来就包在一笔事务里（transactionalClaims）。
	readRules := func() veports.EligibilityRules {
		t.Helper()
		var answer veports.EligibilityRules
		if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			got, declared, readErr := rules.RulesForClaim(txCtx, query)
			if readErr != nil {
				return readErr
			}
			if !declared {
				t.Fatal("声明已登记却答不在场")
			}
			answer = got
			return nil
		}); err != nil {
			t.Fatalf("读资格规则：%v", err)
		}
		return answer
	}
	screenCommand := visibilityapp.ScreenClaimCommand{TenantID: command.TenantID, Batch: command.Batch, Item: command.Item}

	// 态一：VE 的册在场、PC 这个范围里没有生效的客户服务规则版本——两维照旧未登记，编排停的格不变。
	before := readRules()
	if before.FilingDeadline.Registered || before.Materials.Registered {
		t.Fatalf("PC 没登规则却答了登记：%#v", before)
	}
	if !before.KindCovered {
		t.Fatal("VE 自己的册说类型在保，叠两维之后丢了")
	}
	screened, err := claims.ScreenClaim(ctx, screenCommand)
	if err != nil || screened.Outcome() != visibilityapp.HandleClaimUndecided ||
		screened.UndecidedReason() != visibilityapp.EligibilityFilingDeadlineNotRegistered {
		t.Fatalf("PC 没登规则时的审核：outcome=%v reason=%v err=%v，要照旧停在 ELIGIBILITY_FILING_DEADLINE_NOT_REGISTERED",
			screened.Outcome(), screened.UndecidedReason(), err)
	}

	// 态二：PC 发布一版客户服务规则壳并登记正文（首次索赔期限一行 + 本索赔类型一份材料清单）。
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造商业发布登记册：%v", err)
	}
	version := effectiveCustomerServiceRule(t, tenant, scope)
	content, err := pcdomain.NewCustomerServiceRuleVersion(
		version,
		pcdomain.CustomerServiceRuleAppliesToCustomerContract(mustValue(t, pcdomain.NewCommercialObjectID, "SYN-CONTRACT-1")),
		mustValue(t, pcdomain.NewPartyID, "SYN-OPERATOR-1"),
		scope,
		[]pcdomain.ClaimDeadlineRule{mustDeadlineRule(t, pcdomain.FirstClaimDeadline, "SYN-EVENT-DELIVERED", 30, "SYN-CALENDAR-1")},
		[]pcdomain.MinimumMaterialsRule{mustMaterialsRule(t, "SYN-KIND-LOSS", "SYN-MAT-PHOTO", "SYN-MAT-INVOICE")},
	)
	if err != nil {
		t.Fatalf("客户服务规则正文：%v", err)
	}
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		saved, saveErr := publications.SaveVersion(txCtx, version)
		if saveErr != nil {
			return saveErr
		}
		if saved != pcports.PublicationSaved {
			t.Fatalf("发布版本壳应成功，实得 %s", saved)
		}
		registered, saveErr := publications.SaveCustomerServiceRule(txCtx, content)
		if saveErr != nil {
			return saveErr
		}
		if registered != pcports.CustomerServiceRuleSaved {
			t.Fatalf("登记正文应成功，实得 %s", registered)
		}
		return nil
	}); err != nil {
		t.Fatalf("事务内发布客户服务规则失败：%v", err)
	}

	after := readRules()
	deadline := after.FilingDeadline
	if !deadline.Registered || deadline.RuleVersion != "SYN-TENANT-1/SYN-CSR-1/v1" ||
		deadline.StartEvent != "SYN-EVENT-DELIVERED" || deadline.Calendar != "SYN-CALENDAR-1" ||
		deadline.Scope != "SYN-PARCEL-1" {
		t.Fatalf("首次索赔期限维 = %#v，要 Registered、PC 三段版本引用、照引用转写的起算事件与日历、索赔目标范围", deadline)
	}
	if !deadline.Deadline.IsZero() {
		t.Fatalf("Deadline = %v；起算事实源与业务日历今天都没有，截止时刻只能留格", deadline.Deadline)
	}
	materials := after.Materials
	if !materials.Registered || materials.RuleVersion != "SYN-TENANT-1/SYN-CSR-1/v1" ||
		len(materials.Required) != 2 ||
		materials.Required[0].String() != "SYN-MAT-INVOICE" || materials.Required[1].String() != "SYN-MAT-PHOTO" {
		t.Fatalf("最低材料维 = %#v，要 Registered、同一版本引用、与 PC 材料条目逐项相等", materials)
	}
	if materials.Notice.String() != "" || !materials.SupplementDeadline.IsZero() {
		t.Fatalf("Notice=%q SupplementDeadline=%v；两样今天都没有来源，只能留格", materials.Notice, materials.SupplementDeadline)
	}

	// 材料维接通之后，零收讫的索赔走进「差材料」那一支；补充截止是留格的零值，applyScreen 据既有守卫停在
	// ELIGIBILITY_SUPPLEMENT_WINDOW_CLOSED——停在未决、不记第三态、不拒赔。这一格的名字对「截止算不出」
	// 已经不准，改名归 application（票 ve-claims-read-seams/05）；那一票落地时这一行随之换名。
	screened, err = claims.ScreenClaim(ctx, screenCommand)
	if err != nil || screened.Outcome() != visibilityapp.HandleClaimUndecided {
		t.Fatalf("PC 登了正文后的审核：outcome=%v err=%v，要停在未决而不是拒赔或放行", screened.Outcome(), err)
	}
	if got := screened.UndecidedReason(); got != visibilityapp.EligibilitySupplementWindowClosed {
		t.Fatalf("reason = %v, want ELIGIBILITY_SUPPLEMENT_WINDOW_CLOSED——材料维已按 PC 清单核出缺口、补充截止留格", got)
	}
	if _, has := screened.Claim(); has {
		t.Fatal("停在未决却交回了索赔项——未决时索赔项该一字不动、不随答案交出")
	}

	// 生产装配：键来源 nil 是显式未配置，PC 登了正文也不改答案——两维仍未登记，编排停的格与本票之前一字不变。
	production, err := buildClaimsOrchestration(db)
	if err != nil {
		t.Fatalf("生产装配：%v", err)
	}
	screened, err = production.ScreenClaim(ctx, screenCommand)
	if err != nil || screened.UndecidedReason() != visibilityapp.EligibilityFilingDeadlineNotRegistered {
		t.Fatalf("生产装配的审核：reason=%v err=%v，键来源未配置时要照旧停在 ELIGIBILITY_FILING_DEADLINE_NOT_REGISTERED",
			screened.UndecidedReason(), err)
	}
}

// effectiveCustomerServiceRule 用导出 API 造一版已生效的客户服务规则壳，范围与锚点落在同一处。
func effectiveCustomerServiceRule(t *testing.T, tenant pcdomain.TenantID, scope pcdomain.CommercialScopeReference) pcdomain.CommercialVersion {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	draft, err := pcdomain.NewCommercialDraft(pcdomain.CommercialVersionSpec{
		TenantID:      tenant,
		Kind:          pcdomain.CustomerServiceRuleObject,
		ObjectID:      mustValue(t, pcdomain.NewCommercialObjectID, "SYN-CSR-1"),
		Version:       mustValue(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         scope,
		ContentDigest: mustValue(t, pcdomain.NewCommercialContentDigest, "sha256:syn-csr-1"),
		Effective:     interval,
	})
	if err != nil {
		t.Fatalf("商业草稿：%v", err)
	}
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	basis, err := pcdomain.NewApprovalBasis(
		mustValue(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-CSR-1"),
		mustValue(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-CSR-1"),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	published, err := draft.Publish(basis, pcdomain.ApprovalRoleConfirmed, approvedAt, nil)
	if err != nil {
		t.Fatalf("发布：%v", err)
	}
	live, err := published.TakeEffect(approvedAt)
	if err != nil {
		t.Fatalf("生效：%v", err)
	}
	return live
}

func mustDeadlineRule(t *testing.T, kind pcdomain.ClaimDeadlineKind, event string, days int, calendar string) pcdomain.ClaimDeadlineRule {
	t.Helper()
	rule, err := pcdomain.NewClaimDeadlineRule(kind,
		mustValue(t, pcdomain.NewDeadlineStartEventReference, event), days,
		mustValue(t, pcdomain.NewBusinessCalendarReference, calendar))
	if err != nil {
		t.Fatalf("索赔期限规则：%v", err)
	}
	return rule
}

func mustMaterialsRule(t *testing.T, claimKind string, materials ...string) pcdomain.MinimumMaterialsRule {
	t.Helper()
	references := make([]pcdomain.MaterialRequirementReference, 0, len(materials))
	for _, material := range materials {
		references = append(references, mustValue(t, pcdomain.NewMaterialRequirementReference, material))
	}
	rule, err := pcdomain.NewMinimumMaterialsRule(mustValue(t, pcdomain.NewClaimKindReference, claimKind), references)
	if err != nil {
		t.Fatalf("最低材料规则：%v", err)
	}
	return rule
}
