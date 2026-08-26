package main

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

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
