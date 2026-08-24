package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	visibilitydomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// Covers: `/claims` 的第二参是真编排——索赔库、追偿库、标识签发与责任结论 Outbox
// 在真实 PostgreSQL 上装得起来；受理真实落库且重放走已有项（证首笔事务真的提交了）；
// 资格缝显式未配置时审核入口的答案是指名到缝的未决——不是 error（那会被端点折成
// 5xx），也不是`不予受理`或`不受理`（那是业务否定），索赔项一字不动（接线票 04 的
// 验收钉）。测试输入是隔离合成，只记 `S`，不进生产装配。
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

	// 接线票 04 的验收钉：资格缝显式未配置时，审核入口必须停在携带续办指名的未决。
	// 三个都不是它的答案——error（端点会折成 5xx NO_ANSWER_FORMED）、`不予受理`
	// （ADR-0051 的永久格，默认拒赔）、`已过审`（默认放行）。
	screened, err := claims.ScreenClaim(t.Context(), visibilityapp.ScreenClaimCommand{
		TenantID: command.TenantID,
		Batch:    command.Batch,
		Item:     command.Item,
	})
	if err != nil {
		t.Fatalf("资格缝未配置不该以 error 交回（那是 5xx，不是未决）：%v", err)
	}
	if got := screened.Outcome(); got != visibilityapp.HandleClaimUndecided {
		t.Fatalf("outcome = %v, want UNDECIDED——资格未配置既不是业务否定也不是放行", got)
	}
	if got := screened.UndecidedReason(); got != visibilityapp.EligibilityRulesUnavailable {
		t.Fatalf("reason = %v, want ELIGIBILITY_RULES_UNAVAILABLE——未决要指名到缝，人才知道去接哪一条", got)
	}

	// 审核在资格缝就停了，证据缝经公开入口走不到；直接钉它的形状：未配置答「归集
	// 无从查起」（端口自设的核不了格），不是 error，更不是「查过了零件」的空清单。
	materials, known, err := unconfiguredClaimEvidence{}.ReceivedMaterials(
		t.Context(), command.TenantID, command.Batch, command.Item)
	if err != nil {
		t.Fatalf("证据缝未配置不该以 error 交回：%v", err)
	}
	if known {
		t.Fatal("证据缝未配置却答 known=true——那是「归集查过了」的有效事实，会替客户立下补充义务")
	}
	if len(materials) != 0 {
		t.Fatalf("证据缝未配置却交回材料清单：%v", materials)
	}
}
