package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证待批准发布载体与审批职责规则两张表（0028；ADR-0126 Decision 三）：录入落点
// 四格（新行 / 同一次录入重放 / 待批准期间修订 / 已批准后内容固定）、往返经重建门、状态推进带前态条件、
// 租户是身份不是过滤器、CHECK 守住三格痕迹与摘要前缀；规则一租户一条、撞键不覆盖、缺行即未登记。

var (
	draftSubmittedAt = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	draftApprovedAt  = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	draftPublishedAt = time.Date(2026, 9, 1, 11, 0, 0, 0, time.UTC)
)

func newDraftRegistry(t *testing.T) (*adapter.PublicationDrafts, *adapter.ApprovalDutyRules, bentoapp.Transactor, *bentopg.DB) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	drafts, err := adapter.NewPublicationDrafts(db)
	if err != nil {
		t.Fatalf("构造载体册：%v", err)
	}
	rules, err := adapter.NewApprovalDutyRules(db)
	if err != nil {
		t.Fatalf("构造审批职责规则册：%v", err)
	}
	return drafts, rules, db.Transactor(), db
}

func draftShellIn(t *testing.T, tenant, scope, objectID, version string) domain.PublicationDraftShell {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	return domain.PublicationDraftShell{
		TenantID:  pcTenant(t, tenant),
		Kind:      domain.CreditPolicyObject,
		ObjectID:  pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:   pcValue(t, domain.NewCommercialVersionLabel, version),
		Scope:     pcValue(t, domain.NewCommercialScopeReference, scope),
		Effective: interval,
		References: map[domain.CommercialObjectKind]domain.CommercialObjectID{
			domain.ServiceProductObject: pcValue(t, domain.NewCommercialObjectID, "product-1"),
		},
	}
}

func creditDraftContent(t *testing.T, minor int64) domain.PublicationContent {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(effectiveAtRow, time.Time{})
	if err != nil {
		t.Fatalf("正文区间：%v", err)
	}
	return domain.PublicationContent{Kind: domain.CreditPolicyObject, CreditPolicy: &domain.CreditPolicyBody{
		LegalEntity: pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		Level:       pcValue(t, domain.NewAuthorityLevel, "level-commercial"),
		ChargeType:  pcValue(t, domain.NewChargeTypeReference, "charge-freight"),
		Limit:       creditAmountLimit(t, minor),
		Effective:   interval,
	}}
}

func pendingDraft(t *testing.T, shell domain.PublicationDraftShell, minor int64, submitter string) domain.PublicationDraft {
	t.Helper()
	draft, err := domain.SubmitPublicationDraft(shell, creditDraftContent(t, minor),
		pcValue(t, domain.NewOperatorSubjectReference, submitter), draftSubmittedAt)
	if err != nil {
		t.Fatalf("录入载体：%v", err)
	}
	return draft
}

func approveDraft(t *testing.T, draft domain.PublicationDraft, approver string) domain.PublicationDraft {
	t.Helper()
	subject, err := domain.NewOperatorSubject(pcValue(t, domain.NewOperatorSubjectReference, approver), nil)
	if err != nil {
		t.Fatalf("批准者：%v", err)
	}
	rule, err := domain.NewApprovalDutyRule(draft.Tenant(), true, domain.AuthorityLevel{})
	if err != nil {
		t.Fatalf("规则：%v", err)
	}
	approved, err := draft.Approve(subject, rule, draftApprovedAt)
	if err != nil {
		t.Fatalf("批准：%v", err)
	}
	return approved
}

func mustSubmitDraft(t *testing.T, transactor bentoapp.Transactor, ctx context.Context, drafts *adapter.PublicationDrafts, draft domain.PublicationDraft) ports.PublicationDraftSubmitOutcome {
	t.Helper()
	var outcome ports.PublicationDraftSubmitOutcome
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = drafts.SubmitDraft(txCtx, draft)
		return err
	})
	return outcome
}

func mustAdvanceDraft(t *testing.T, transactor bentoapp.Transactor, ctx context.Context, drafts *adapter.PublicationDrafts, draft domain.PublicationDraft) ports.PublicationDraftAdvanceOutcome {
	t.Helper()
	var outcome ports.PublicationDraftAdvanceOutcome
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = drafts.AdvanceDraft(txCtx, draft)
		return err
	})
	return outcome
}

func loadDraft(t *testing.T, ctx context.Context, drafts *adapter.PublicationDrafts, tenant string, shell domain.PublicationDraftShell) (domain.PublicationDraft, bool) {
	t.Helper()
	draft, found, err := drafts.LoadDraft(ctx, pcTenant(t, tenant), shell.Kind, shell.ObjectID, shell.Version)
	if err != nil {
		t.Fatalf("读载体：%v", err)
	}
	return draft, found
}

// Covers: ADR-0126 Decision 三 — 录入往返经重建门：壳（含指名引用）、正文、算出的摘要、录入者与状态原样读回。
func TestADraftRoundTripsThroughTheRehydrationGate(t *testing.T) {
	drafts, _, transactor, _ := newDraftRegistry(t)
	ctx := t.Context()
	shell := draftShellIn(t, "tenant-1", "scope-1", "credit-1", "v1")
	submitted := pendingDraft(t, shell, 500_000, "op-submitter")

	if outcome := mustSubmitDraft(t, transactor, ctx, drafts, submitted); outcome != ports.PublicationDraftSaved {
		t.Fatalf("submit outcome = %s, want SAVED", outcome)
	}
	stored, found := loadDraft(t, ctx, drafts, "tenant-1", shell)
	if !found {
		t.Fatal("录入的载体读不回来")
	}
	if !stored.SameSubmissionAs(submitted) || stored.Status() != domain.PublicationDraftPendingApproval {
		t.Fatalf("读回的载体不是录入的那份：%#v", stored)
	}
	if stored.Submitter().String() != "op-submitter" || !stored.SubmittedAt().Equal(draftSubmittedAt) {
		t.Fatalf("录入者 = %s at %s", stored.Submitter(), stored.SubmittedAt())
	}
	if minor, ok := stored.Content().CreditPolicy.Limit.AmountMinor(); !ok || minor != 500_000 {
		t.Fatalf("正文额度 = %d, %v", minor, ok)
	}
	if reference, ok := stored.PublicationSpec().References[domain.ServiceProductObject]; !ok || reference.String() != "product-1" {
		t.Fatalf("指名引用没读回：%#v", stored.PublicationSpec().References)
	}
	if stored.Canonical().Digest() != submitted.Canonical().Digest() {
		t.Fatalf("摘要 %s ≠ %s", stored.Canonical().Digest(), submitted.Canonical().Digest())
	}
}

// Covers: ADR-0126 Decision 三 — 同一次录入再录是重放（行一字不动）；`待批准`期间换内容或换壳是修订（行被替换，
// 录入者随之更新）；`已批准`之后换内容答`内容已固定`、同一次录入仍是重放。
func TestSubmittingAgainDistinguishesReplayRevisionAndFixedContent(t *testing.T) {
	drafts, _, transactor, _ := newDraftRegistry(t)
	ctx := t.Context()
	shell := draftShellIn(t, "tenant-1", "scope-1", "credit-1", "v1")
	mustSubmitDraft(t, transactor, ctx, drafts, pendingDraft(t, shell, 500_000, "op-submitter"))

	if outcome := mustSubmitDraft(t, transactor, ctx, drafts, pendingDraft(t, shell, 500_000, "op-other")); outcome != ports.PublicationDraftReplayed {
		t.Fatalf("replay outcome = %s, want REPLAYED", outcome)
	}
	if stored, _ := loadDraft(t, ctx, drafts, "tenant-1", shell); stored.Submitter().String() != "op-submitter" {
		t.Fatalf("重放改动了原行的录入者：%s", stored.Submitter())
	}

	revised := pendingDraft(t, shell, 1, "op-reviser")
	if outcome := mustSubmitDraft(t, transactor, ctx, drafts, revised); outcome != ports.PublicationDraftRevised {
		t.Fatalf("revision outcome = %s, want REVISED", outcome)
	}
	stored, _ := loadDraft(t, ctx, drafts, "tenant-1", shell)
	if !stored.SameSubmissionAs(revised) || stored.Submitter().String() != "op-reviser" {
		t.Fatalf("修订没有替换行：%#v", stored)
	}

	rescoped := pendingDraft(t, draftShellIn(t, "tenant-1", "scope-2", "credit-1", "v1"), 1, "op-reviser")
	if outcome := mustSubmitDraft(t, transactor, ctx, drafts, rescoped); outcome != ports.PublicationDraftRevised {
		t.Fatalf("rescope outcome = %s, want REVISED（换壳也是修订）", outcome)
	}
	if stored, _ := loadDraft(t, ctx, drafts, "tenant-1", shell); stored.Scope().String() != "scope-2" {
		t.Fatalf("换范围没落进行：%s", stored.Scope())
	}

	if outcome := mustAdvanceDraft(t, transactor, ctx, drafts, approveDraft(t, rescoped, "op-approver")); outcome != ports.PublicationDraftAdvanced {
		t.Fatalf("advance outcome = %s, want ADVANCED", outcome)
	}
	if outcome := mustSubmitDraft(t, transactor, ctx, drafts, pendingDraft(t, shell, 2, "op-late")); outcome != ports.PublicationDraftContentFixed {
		t.Fatalf("outcome after approval = %s, want CONTENT_FIXED", outcome)
	}
	if outcome := mustSubmitDraft(t, transactor, ctx, drafts, rescoped); outcome != ports.PublicationDraftReplayed {
		t.Fatalf("same submission after approval = %s, want REPLAYED", outcome)
	}
	if stored, _ := loadDraft(t, ctx, drafts, "tenant-1", shell); stored.Status() != domain.PublicationDraftApproved {
		t.Fatalf("已批准的载体被再录改回了：%s", stored.Status())
	}
}

// Covers: ADR-0126 Decision 三 — 推进带前态条件：批准写回后读回`已批准`带批准者；发布写回后`已发布`带三刻；
// 基于过期状态的推进答`已被替换`而不是盖过去；不存在的载体答`未找到`。
func TestAdvancingADraftRequiresThePriorStateItWasBasedOn(t *testing.T) {
	drafts, _, transactor, _ := newDraftRegistry(t)
	ctx := t.Context()
	shell := draftShellIn(t, "tenant-1", "scope-1", "credit-1", "v1")
	pending := pendingDraft(t, shell, 500_000, "op-submitter")
	mustSubmitDraft(t, transactor, ctx, drafts, pending)

	approved := approveDraft(t, pending, "op-approver")
	if outcome := mustAdvanceDraft(t, transactor, ctx, drafts, approved); outcome != ports.PublicationDraftAdvanced {
		t.Fatalf("approve advance = %s", outcome)
	}
	stored, _ := loadDraft(t, ctx, drafts, "tenant-1", shell)
	approver, ok := stored.Approver()
	if stored.Status() != domain.PublicationDraftApproved || !ok || approver.String() != "op-approver" {
		t.Fatalf("读回 = %#v", stored)
	}
	if at, _ := stored.ApprovedAt(); !at.Equal(draftApprovedAt) {
		t.Fatalf("approvedAt = %s", at)
	}

	// 第二个批准者基于同一份`待批准`快照动手：库上已是`已批准`，后到者答`已被替换`。
	if outcome := mustAdvanceDraft(t, transactor, ctx, drafts, approveDraft(t, pending, "op-second")); outcome != ports.PublicationDraftAdvanceSuperseded {
		t.Fatalf("stale approve advance = %s, want SUPERSEDED", outcome)
	}
	stored, _ = loadDraft(t, ctx, drafts, "tenant-1", shell)
	if approver, _ := stored.Approver(); approver.String() != "op-approver" {
		t.Fatalf("后到的批准盖掉了先到的：%s", approver)
	}

	published, err := approved.MarkPublished(draftPublishedAt)
	if err != nil {
		t.Fatalf("mark published: %v", err)
	}
	if outcome := mustAdvanceDraft(t, transactor, ctx, drafts, published); outcome != ports.PublicationDraftAdvanced {
		t.Fatalf("publish advance = %s", outcome)
	}
	stored, _ = loadDraft(t, ctx, drafts, "tenant-1", shell)
	if at, ok := stored.PublishedAt(); stored.Status() != domain.PublicationDraftPublished || !ok || !at.Equal(draftPublishedAt) {
		t.Fatalf("发布后读回 = %#v", stored)
	}
	if outcome := mustAdvanceDraft(t, transactor, ctx, drafts, published); outcome != ports.PublicationDraftAdvanceSuperseded {
		t.Fatalf("publishing twice = %s, want SUPERSEDED", outcome)
	}

	unknown := approveDraft(t, pendingDraft(t, draftShellIn(t, "tenant-1", "scope-1", "credit-9", "v1"), 1, "op-submitter"), "op-approver")
	if outcome := mustAdvanceDraft(t, transactor, ctx, drafts, unknown); outcome != ports.PublicationDraftAdvanceNotFound {
		t.Fatalf("advancing an unknown draft = %s, want NOT_FOUND", outcome)
	}
}

// Covers: ADR-0003 — 租户是身份不是过滤器：他租户读不到本租户的载体，两租户同号版本各自一行。
func TestDraftsAreBoundToTheirTenant(t *testing.T) {
	drafts, _, transactor, _ := newDraftRegistry(t)
	ctx := t.Context()
	mine := draftShellIn(t, "tenant-1", "scope-1", "credit-1", "v1")
	theirs := draftShellIn(t, "tenant-2", "scope-1", "credit-1", "v1")
	mustSubmitDraft(t, transactor, ctx, drafts, pendingDraft(t, mine, 500_000, "op-1"))
	if outcome := mustSubmitDraft(t, transactor, ctx, drafts, pendingDraft(t, theirs, 1, "op-2")); outcome != ports.PublicationDraftSaved {
		t.Fatalf("他租户的同号版本应是另一行：%s", outcome)
	}

	if _, found := loadDraft(t, ctx, drafts, "tenant-2", mine); !found {
		t.Fatal("tenant-2 自己的载体读不到")
	}
	stored, _ := loadDraft(t, ctx, drafts, "tenant-2", mine)
	if minor, _ := stored.Content().CreditPolicy.Limit.AmountMinor(); minor != 1 {
		t.Fatalf("tenant-2 读到了 tenant-1 的正文：%d", minor)
	}
	if _, found := loadDraft(t, ctx, drafts, "tenant-3", mine); found {
		t.Fatal("没有载体的租户读到了别人的")
	}
}

// Covers: 库上 CHECK 是重建门不变量的镜像——`待批准`带批准痕迹、摘要串不以规范化版本为前缀、状态集外都进不来。
// 领域构造门拦经它进来的，CHECK 拦绕开它的那条路。
func TestDraftTableRejectsRowsTheRehydrationGateWouldRefuse(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()

	insert := func(status string, approver string, digest string) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO party_commercial.publication_draft
				(tenant_id, object_kind, object_id, version_label, scope_ref, effective_starts_at,
				 content_canonicalization, content_digest, content_document,
				 submitter_ref, submitted_at, status, approver_ref, approved_at)
			 VALUES ('tenant-1', 8, 'credit-x', 'v1', 'scope-1', now(),
			         'PCC-1', `+digest+`, '{}'::jsonb,
			         'op-1', now(), `+status+`, `+approver+`, CASE WHEN `+approver+` IS NULL THEN NULL ELSE now() END)`)
		return err
	}
	if err := insert("1", "'op-2'", "'PCC-1:abc'"); err == nil {
		t.Fatal("待批准的行带着批准者进了表")
	}
	if err := insert("2", "NULL", "'PCC-1:abc'"); err == nil {
		t.Fatal("已批准的行没有批准者进了表")
	}
	if err := insert("1", "NULL", "'sha256:abc'"); err == nil {
		t.Fatal("摘要串不带规范化版本前缀进了表")
	}
	if err := insert("4", "'op-2'", "'PCC-1:abc'"); err == nil {
		t.Fatal("状态集外的行进了表")
	}
	if err := insert("1", "NULL", "'PCC-1:abc'"); err != nil {
		t.Fatalf("一行合法的待批准载体被拒：%v", err)
	}
}

// Covers: PAR-COM-18 / ADR-0126 Decision 三 — 规则一租户一条：登记后读回逐格相同；缺行即未登记（found=false，
// 不是 error）；同内容重放、异内容冲突且原行不动；他租户读不到。
func TestApprovalDutyRuleIsRegisteredOncePerTenant(t *testing.T) {
	_, rules, transactor, _ := newDraftRegistry(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	if _, found, err := rules.LoadApprovalDutyRule(ctx, tenant); err != nil || found {
		t.Fatalf("未登记应答 found=false：found=%v err=%v", found, err)
	}

	strict, err := domain.NewApprovalDutyRule(tenant, true, pcValue(t, domain.NewAuthorityLevel, "level-approver"))
	if err != nil {
		t.Fatalf("规则：%v", err)
	}
	save := func(rule domain.ApprovalDutyRule) ports.ApprovalDutyRuleSaveOutcome {
		var outcome ports.ApprovalDutyRuleSaveOutcome
		mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			var err error
			outcome, err = rules.SaveApprovalDutyRule(txCtx, rule)
			return err
		})
		return outcome
	}
	if outcome := save(strict); outcome != ports.ApprovalDutyRuleSaved {
		t.Fatalf("save outcome = %s", outcome)
	}
	loaded, found, err := rules.LoadApprovalDutyRule(ctx, tenant)
	if err != nil || !found {
		t.Fatalf("读回：found=%v err=%v", found, err)
	}
	level, required := loaded.RequiredApproverLevel()
	if !loaded.RequiresDistinctSubjects() || !required || level.String() != "level-approver" {
		t.Fatalf("读回的规则变了：%#v", loaded)
	}

	if outcome := save(strict); outcome != ports.ApprovalDutyRuleAlreadyRegistered {
		t.Fatalf("replay outcome = %s", outcome)
	}
	loose, err := domain.NewApprovalDutyRule(tenant, false, domain.AuthorityLevel{})
	if err != nil {
		t.Fatalf("规则：%v", err)
	}
	if outcome := save(loose); outcome != ports.ApprovalDutyRuleContentConflict {
		t.Fatalf("conflict outcome = %s", outcome)
	}
	if loaded, _, _ := rules.LoadApprovalDutyRule(ctx, tenant); !loaded.RequiresDistinctSubjects() {
		t.Fatal("冲突写入改动了原行")
	}

	if _, found, err := rules.LoadApprovalDutyRule(ctx, pcTenant(t, "tenant-2")); err != nil || found {
		t.Fatalf("他租户读到了本租户的规则：found=%v err=%v", found, err)
	}

	// 两格都不要求的规则是一条合法声明，读回时 required=false 而不是空串等级。
	other := pcTenant(t, "tenant-2")
	permissive, err := domain.NewApprovalDutyRule(other, false, domain.AuthorityLevel{})
	if err != nil {
		t.Fatalf("规则：%v", err)
	}
	if outcome := save(permissive); outcome != ports.ApprovalDutyRuleSaved {
		t.Fatalf("permissive save = %s", outcome)
	}
	loaded, found, err = rules.LoadApprovalDutyRule(ctx, other)
	if err != nil || !found || loaded.RequiresDistinctSubjects() {
		t.Fatalf("permissive 读回：%#v %v %v", loaded, found, err)
	}
	if _, required := loaded.RequiredApproverLevel(); required {
		t.Fatal("没要求等级的规则读回来带了等级")
	}
}
