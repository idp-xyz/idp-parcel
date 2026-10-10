package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证价卡草稿的推进口与审批职责规则表（0011、0012；票 price-card-import/04）：批准与发布各推进
// 一格、只改状态与痕迹列、内容原样；从陈旧的读推进答`已被替换`、行一字不动；规则一租户一条，重放与冲突分格。夹具全部
// 为 SYN 合成值（S 级）。

const draftProgressApprover = "https://id.syn.example/dex#SYN-OPERATOR-02"

type progressStore struct {
	drafts     *adapter.PriceCardDrafts
	rules      *adapter.PriceCardApprovalDutyRules
	transactor bentoapp.Transactor
}

func newProgressStore(t *testing.T) progressStore {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	drafts, err := adapter.NewPriceCardDrafts(db)
	if err != nil {
		t.Fatalf("构造草稿册：%v", err)
	}
	rules, err := adapter.NewPriceCardApprovalDutyRules(db)
	if err != nil {
		t.Fatalf("构造审批职责规则册：%v", err)
	}
	return progressStore{drafts: drafts, rules: rules, transactor: db.Transactor()}
}

func (store progressStore) within(t *testing.T, work func(ctx context.Context) error) {
	t.Helper()
	if err := store.transactor.WithinTransaction(t.Context(), work); err != nil {
		t.Fatalf("事务：%v", err)
	}
}

func (store progressStore) submit(t *testing.T, draft domain.PriceCardDraft) ports.PriceCardDraftSubmitOutcome {
	t.Helper()
	var outcome ports.PriceCardDraftSubmitOutcome
	store.within(t, func(ctx context.Context) (err error) {
		outcome, err = store.drafts.SubmitDraft(ctx, draft)
		return err
	})
	return outcome
}

func (store progressStore) load(t *testing.T, draft domain.PriceCardDraft) domain.PriceCardDraft {
	t.Helper()
	loaded, found, err := store.drafts.LoadPriceCardDraft(t.Context(), draft.Tenant(), draft.Plan())
	if err != nil || !found {
		t.Fatalf("读草稿：found=%v err=%v", found, err)
	}
	return loaded
}

func (store progressStore) advance(t *testing.T, draft domain.PriceCardDraft) ports.PriceCardDraftAdvanceOutcome {
	t.Helper()
	var outcome ports.PriceCardDraftAdvanceOutcome
	store.within(t, func(ctx context.Context) (err error) {
		outcome, err = store.drafts.AdvancePriceCardDraft(ctx, draft)
		return err
	})
	return outcome
}

func approvedBy(t *testing.T, draft domain.PriceCardDraft, approver string, at time.Time) domain.PriceCardDraft {
	t.Helper()
	subject, err := domain.NewOperatorSubject(approver, nil)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := domain.NewPriceCardApprovalDutyRule(draft.Tenant(), true, domain.OperatorGrant{})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := draft.Approve(subject, rule, at)
	if err != nil {
		t.Fatalf("批准：%v", err)
	}
	return approved
}

// 批准推进一格、发布再推进一格：读回的那一份经整图重验，状态与三格痕迹是推进时交的，内容摘要与录入痕迹原样。
func TestApprovalAndPublicationAdvanceTheDraftRow(t *testing.T) {
	store := newProgressStore(t)
	plan := catalogPlan(t, "SYN-PLAN-DRAFT-01", "v1", domain.PricingDirectionBuy, "30")
	submitted := validatedDraftOf(t, "SYN-TENANT-01", plan, draftSourceOf(t, "SYN-CARD-DRAFT-01.xlsx", catalogSourceSHA), draftAt)
	if outcome := store.submit(t, submitted); outcome != ports.PriceCardDraftSaved {
		t.Fatalf("首录 = %v", outcome)
	}

	approvedAt := draftAt.Add(time.Hour)
	if outcome := store.advance(t, approvedBy(t, store.load(t, submitted), draftProgressApprover, approvedAt)); outcome != ports.PriceCardDraftAdvanced {
		t.Fatalf("批准推进 = %v", outcome)
	}
	approved := store.load(t, submitted)
	if approved.Status() != domain.PriceCardDraftStatusApproved {
		t.Fatalf("status = %v，want APPROVED", approved.Status())
	}
	if approver, _ := approved.Approver(); approver != draftProgressApprover {
		t.Fatalf("approver = %q", approver)
	}
	if at, _ := approved.ApprovedAt(); !at.Equal(approvedAt) {
		t.Fatalf("approvedAt = %v", at)
	}

	publishedAt := approvedAt.Add(time.Minute)
	published, err := approved.MarkPublished(publishedAt)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := store.advance(t, published); outcome != ports.PriceCardDraftAdvanced {
		t.Fatalf("发布推进 = %v", outcome)
	}
	read := store.load(t, submitted)
	content, has := read.Content()
	if read.Status() != domain.PriceCardDraftStatusPublished || !has || content.Plan.ContentDigest() != plan.ContentDigest() ||
		read.Submitter() != submitted.Submitter() || !read.SubmittedAt().Equal(draftAt) {
		t.Fatalf("read = %+v", read)
	}
	if at, _ := read.PublishedAt(); !at.Equal(publishedAt) {
		t.Fatalf("publishedAt = %v", at)
	}
}

// 从陈旧的读推进答`已被替换`、行一字不动：读后被修订（录入痕迹换了）、读后被别人先批准（状态换了）都是这一格；册上没有
// 这一版答不在。
func TestAnAdvanceFromAStaleReadIsSuperseded(t *testing.T) {
	store := newProgressStore(t)
	plan := catalogPlan(t, "SYN-PLAN-DRAFT-01", "v1", domain.PricingDirectionBuy, "30")
	source := draftSourceOf(t, "SYN-CARD-DRAFT-01.xlsx", catalogSourceSHA)
	first := validatedDraftOf(t, "SYN-TENANT-01", plan, source, draftAt)
	store.submit(t, first)
	stale := store.load(t, first)

	revisedPlan := catalogPlan(t, "SYN-PLAN-DRAFT-01", "v1", domain.PricingDirectionBuy, "31")
	revised := validatedDraftOf(t, "SYN-TENANT-01", revisedPlan, source, draftAt.Add(30*time.Minute))
	if outcome := store.submit(t, revised); outcome != ports.PriceCardDraftRevised {
		t.Fatalf("修订 = %v", outcome)
	}
	if outcome := store.advance(t, approvedBy(t, stale, draftProgressApprover, draftAt.Add(time.Hour))); outcome != ports.PriceCardDraftAdvanceSuperseded {
		t.Fatalf("修订后从旧读批准 = %v，want SUPERSEDED", outcome)
	}
	if read := store.load(t, first); read.Status() != domain.PriceCardDraftStatusValidated {
		t.Fatalf("被替换的推进动了那一行：status = %v", read.Status())
	}

	current := store.load(t, first)
	if outcome := store.advance(t, approvedBy(t, current, draftProgressApprover, draftAt.Add(time.Hour))); outcome != ports.PriceCardDraftAdvanced {
		t.Fatalf("先到的批准 = %v", outcome)
	}
	if outcome := store.advance(t, approvedBy(t, current, "https://id.syn.example/dex#SYN-OPERATOR-03", draftAt.Add(2*time.Hour))); outcome != ports.PriceCardDraftAdvanceSuperseded {
		t.Fatalf("后到的批准 = %v，want SUPERSEDED", outcome)
	}
	if approver, _ := store.load(t, first).Approver(); approver != draftProgressApprover {
		t.Fatalf("后到的批准顶替了先到的：approver = %q", approver)
	}

	never := validatedDraftOf(t, "SYN-TENANT-01", catalogPlan(t, "SYN-PLAN-DRAFT-02", "v1", domain.PricingDirectionBuy, "30"), source, draftAt)
	if outcome := store.advance(t, approvedBy(t, never, draftProgressApprover, draftAt.Add(time.Hour))); outcome != ports.PriceCardDraftAdvanceNotFound {
		t.Fatalf("册上没有的一版 = %v，want NOT_FOUND", outcome)
	}
	if _, found, err := store.drafts.LoadPriceCardDraft(t.Context(), never.Tenant(), never.Plan()); err != nil || found {
		t.Fatalf("读册上没有的一版：found=%v err=%v", found, err)
	}
}

// 推进与规则登记都是写：无事务即拒（框架合同 RequireExecutor）。推进口只收批准或发布推进出来的那一份。
func TestTheProgressWritesNeedATransaction(t *testing.T) {
	store := newProgressStore(t)
	plan := catalogPlan(t, "SYN-PLAN-DRAFT-01", "v1", domain.PricingDirectionBuy, "30")
	submitted := validatedDraftOf(t, "SYN-TENANT-01", plan, draftSourceOf(t, "SYN-CARD-DRAFT-01.xlsx", catalogSourceSHA), draftAt)
	store.submit(t, submitted)

	if _, err := store.drafts.AdvancePriceCardDraft(t.Context(), approvedBy(t, store.load(t, submitted), draftProgressApprover, draftAt.Add(time.Hour))); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务推进：err = %v，want ErrTransactionRequired", err)
	}
	if _, err := store.rules.SavePriceCardApprovalDutyRule(t.Context(), domain.PriceCardApprovalDutyRule{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务登记规则：err = %v，want ErrTransactionRequired", err)
	}
	store.within(t, func(ctx context.Context) error {
		if _, err := store.drafts.AdvancePriceCardDraft(ctx, store.load(t, submitted)); err == nil {
			t.Error("拿一份没推进过的`已校验`去推进应被拒")
		}
		return nil
	})
}

// 规则一租户一条：没登即不在（批准门据此答`未配置`）；首登入册、读回原样（不要求授予存成 NULL、读回仍是不要求）；同内容
// 再登是重放；异内容答冲突、原行不被顶替；别的租户各登各的。
func TestThePriceCardApprovalDutyRuleIsRegisteredOncePerTenant(t *testing.T) {
	store := newProgressStore(t)
	tenant := evaluationValue(t, domain.NewTenantID, "SYN-TENANT-01")
	if _, found, err := store.rules.LoadPriceCardApprovalDutyRule(t.Context(), tenant); err != nil || found {
		t.Fatalf("没登的规则：found=%v err=%v", found, err)
	}
	supervisor := evaluationValue(t, domain.NewOperatorGrant, "SYN-GRANT-PRICING-SUPERVISOR")
	strict, err := domain.NewPriceCardApprovalDutyRule(tenant, true, supervisor)
	if err != nil {
		t.Fatal(err)
	}
	save := func(rule domain.PriceCardApprovalDutyRule) ports.PriceCardApprovalDutyRuleSaveOutcome {
		var outcome ports.PriceCardApprovalDutyRuleSaveOutcome
		store.within(t, func(ctx context.Context) (err error) {
			outcome, err = store.rules.SavePriceCardApprovalDutyRule(ctx, rule)
			return err
		})
		return outcome
	}
	if outcome := save(strict); outcome != ports.PriceCardApprovalDutyRuleSaved {
		t.Fatalf("首登 = %v", outcome)
	}
	loaded, found, err := store.rules.LoadPriceCardApprovalDutyRule(t.Context(), tenant)
	if grant, required := loaded.RequiredGrant(); err != nil || !found || !loaded.RequiresDistinctSubjects() || !required || grant != supervisor {
		t.Fatalf("读回 = %+v found=%v err=%v", loaded, found, err)
	}
	if outcome := save(strict); outcome != ports.PriceCardApprovalDutyRuleAlreadyRegistered {
		t.Fatalf("同内容再登 = %v", outcome)
	}
	loose, err := domain.NewPriceCardApprovalDutyRule(tenant, false, domain.OperatorGrant{})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := save(loose); outcome != ports.PriceCardApprovalDutyRuleContentConflict {
		t.Fatalf("异内容再登 = %v", outcome)
	}
	if kept, _, _ := store.rules.LoadPriceCardApprovalDutyRule(t.Context(), tenant); !kept.RequiresDistinctSubjects() {
		t.Fatal("冲突顶替了原行")
	}

	other := evaluationValue(t, domain.NewTenantID, "SYN-TENANT-02")
	permissive, err := domain.NewPriceCardApprovalDutyRule(other, false, domain.OperatorGrant{})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := save(permissive); outcome != ports.PriceCardApprovalDutyRuleSaved {
		t.Fatalf("别的租户首登 = %v", outcome)
	}
	read, found, err := store.rules.LoadPriceCardApprovalDutyRule(t.Context(), other)
	if _, required := read.RequiredGrant(); err != nil || !found || read.RequiresDistinctSubjects() || required {
		t.Fatalf("两格都不要求的规则读回 = %+v found=%v err=%v", read, found, err)
	}
}
