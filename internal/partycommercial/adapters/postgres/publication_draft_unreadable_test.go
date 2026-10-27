package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件证本构建折不回正文的载体行在真库上的读法（AT-PC-071、AT-PC-072 的持久化那一半）：点读与列表把它作为读不回一格交出，
// 不让一行读不回的载体挡住整页；录入撞上它时按状态答；行本身坏了是错误。

func readableDraftsOf(t *testing.T, open []ports.OpenPublicationDraft) []domain.PublicationDraft {
	t.Helper()
	drafts := make([]domain.PublicationDraft, 0, len(open))
	for _, entry := range open {
		draft, readable := entry.Draft()
		if !readable {
			t.Fatalf("列表里有读不回的一行：%v", entry)
		}
		drafts = append(drafts, draft)
	}
	return drafts
}

// unreadableDraftRegistry 返回载体册、事务器与同一个库的连接池（用来把行改成本构建读不回的样子）。
func unreadableDraftRegistry(t *testing.T) (*adapter.PublicationDrafts, bentoapp.Transactor, *pgxpool.Pool) {
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
	return drafts, db.Transactor(), pool
}

// makeUnreadable 把一行的规范化版本改成本构建不认得的 PCC-9（摘要串的前缀随之，库上 CHECK 要求两者一致）。
func makeUnreadable(t *testing.T, pool *pgxpool.Pool, objectID string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`UPDATE party_commercial.publication_draft
		    SET content_canonicalization = 'PCC-9', content_digest = 'PCC-9:future-build'
		  WHERE tenant_id = 'tenant-1' AND object_id = $1`, objectID); err != nil {
		t.Fatalf("改写 %s：%v", objectID, err)
	}
}

func asUnreadable(t *testing.T, err error) *domain.UnreadablePublicationDraft {
	t.Helper()
	var unreadable *domain.UnreadablePublicationDraft
	if !errors.As(err, &unreadable) {
		t.Fatalf("err = %v, want an *UnreadablePublicationDraft in the chain", err)
	}
	return unreadable
}

// Covers: AT-PC-071——本构建不认得规范化版本的载体行：点读交回「读不回」一格（带状态、列上摘要与成因，错误链认得
// ErrCanonicalizationUnsupported），列表照样列出它并标出读不回，别的行与顺序不受影响，已发布的不在列表里。
func TestAnUnreadableDraftIsListedAndReadBackAsNotReadableInsteadOfFailingThePage(t *testing.T) {
	drafts, transactor, pool := unreadableDraftRegistry(t)
	ctx := t.Context()
	at := func(minutes int) time.Time { return draftSubmittedAt.Add(time.Duration(minutes) * time.Minute) }
	submit := func(objectID string, minor int64, submittedAt time.Time) domain.PublicationDraft {
		shell := draftShellIn(t, "tenant-1", "scope-1", objectID, "v1")
		draft, err := domain.SubmitPublicationDraft(shell, creditDraftContent(t, minor),
			pcValue(t, domain.NewOperatorSubjectReference, "op-submitter"), submittedAt)
		if err != nil {
			t.Fatalf("录入载体：%v", err)
		}
		mustSubmitDraft(t, transactor, ctx, drafts, draft)
		return draft
	}
	readable := submit("credit-readable", 100, at(0))
	pending := submit("credit-pending", 200, at(1))
	approved := submit("credit-approved", 300, at(2))
	published := submit("credit-published", 400, at(3))
	approvedDraft := approveDraft(t, approved, "op-approver")
	mustAdvanceDraft(t, transactor, ctx, drafts, approvedDraft)
	publishedBase := approveDraft(t, published, "op-approver")
	mustAdvanceDraft(t, transactor, ctx, drafts, publishedBase)
	done, err := publishedBase.MarkPublished(draftPublishedAt)
	if err != nil {
		t.Fatalf("发布：%v", err)
	}
	mustAdvanceDraft(t, transactor, ctx, drafts, done)
	for _, objectID := range []string{"credit-pending", "credit-approved", "credit-published"} {
		makeUnreadable(t, pool, objectID)
	}

	shell := draftShellIn(t, "tenant-1", "scope-1", "credit-pending", "v1")
	_, found, err := drafts.LoadDraft(ctx, pcTenant(t, "tenant-1"), shell.Kind, shell.ObjectID, shell.Version)
	if found || !errors.Is(err, domain.ErrPublicationDraftNotReadable) || !errors.Is(err, domain.ErrCanonicalizationUnsupported) {
		t.Fatalf("点读 = found %v, err %v, want 读不回且错误链认得 ErrCanonicalizationUnsupported", found, err)
	}
	unreadable := asUnreadable(t, err)
	if unreadable.Status() != domain.PublicationDraftPendingApproval || unreadable.ContentDigest().String() != "PCC-9:future-build" ||
		unreadable.Canonicalization() != "PCC-9" || unreadable.ShellDigest() != pending.ShellDigest() {
		t.Fatalf("读不回一格的事实 = %#v", unreadable)
	}

	listed, err := drafts.ListOpenDrafts(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("列出未发布载体：%v", err)
	}
	var got []string
	for _, entry := range listed {
		if draft, ok := entry.Draft(); ok {
			got = append(got, draft.ObjectID().String()+"/readable")
		} else if row, ok := entry.Unreadable(); ok {
			got = append(got, row.ObjectID().String()+"/unreadable/"+row.Status().String())
		}
	}
	want := []string{"credit-approved/unreadable/APPROVED", "credit-pending/unreadable/PENDING_APPROVAL", "credit-readable/readable"}
	if len(got) != len(want) {
		t.Fatalf("列表 = %v, want %v（已发布的读不回行不在列表里）", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("列表 = %v, want %v", got, want)
		}
	}
	if first, _ := listed[2].Draft(); first.Canonical().Digest() != readable.Canonical().Digest() {
		t.Fatal("读得回的那一行被动过")
	}
	if row, _ := listed[0].Unreadable(); row.ShellDigest() != approvedDraft.ShellDigest() {
		t.Fatalf("已批准读不回行的壳摘要 = %s, want %s", row.ShellDigest(), approvedDraft.ShellDigest())
	}
}

// Covers: AT-PC-071——价格规则载体带着体积系数（0041 迁移之前的快照）同样是读不回一格，错误链认得 ErrVolumetricFactorOnPricePolicy。
func TestAPricePolicyDraftCarryingAVolumetricFactorReadsBackAsNotReadable(t *testing.T) {
	drafts, _, pool := unreadableDraftRegistry(t)
	ctx := t.Context()
	document := `{"canonicalization":"PCC-1","kind":"PRICE_RULE","pricePolicy":{"direction":"SELL","pricingPlan":"plan-1@v1",` +
		`"planDirection":"SELL","conversion":"NONE","scope":"pricing-scope-1","effectiveStartsAt":"2026-01-03T00:00:00Z",` +
		`"caliber":{"taxDisposition":"TAX_EXCLUSIVE","taxClassification":"vat-standard","volumetricFactor":"sell-divisor-5000-cm"}}}`
	if _, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.publication_draft
			(tenant_id, object_kind, object_id, version_label, scope_ref, effective_starts_at,
			 content_canonicalization, content_digest, content_document, submitter_ref, submitted_at, status)
		 VALUES ('tenant-1', $1, 'price-1', 'v1', 'scope-1', now(), 'PCC-1', 'PCC-1:historical', $2::jsonb, 'op-1', now(), 1)`,
		uint8(domain.PriceRuleObject), document); err != nil {
		t.Fatalf("落一行带体积系数的价格规则载体：%v", err)
	}

	_, found, err := drafts.LoadDraft(ctx, pcTenant(t, "tenant-1"), domain.PriceRuleObject,
		pcValue(t, domain.NewCommercialObjectID, "price-1"), pcValue(t, domain.NewCommercialVersionLabel, "v1"))
	if found || !errors.Is(err, domain.ErrPublicationDraftNotReadable) || !errors.Is(err, domain.ErrVolumetricFactorOnPricePolicy) {
		t.Fatalf("点读 = found %v, err %v", found, err)
	}
}

// Covers: AT-PC-072——录入撞上读不回的既有行：`待批准`的按同一版本重录即被修订（之后读得回，照常推进）；`已批准`与`已发布`的
// 内容已固定，答内容已固定，行一字不动，不答错误。
func TestSubmittingOverAnUnreadableDraftRevisesAPendingOneAndFixesTheRest(t *testing.T) {
	drafts, transactor, pool := unreadableDraftRegistry(t)
	ctx := t.Context()
	submit := func(objectID string, minor int64) domain.PublicationDraft {
		shell := draftShellIn(t, "tenant-1", "scope-1", objectID, "v1")
		draft, err := domain.SubmitPublicationDraft(shell, creditDraftContent(t, minor),
			pcValue(t, domain.NewOperatorSubjectReference, "op-submitter"), draftSubmittedAt)
		if err != nil {
			t.Fatalf("录入载体：%v", err)
		}
		mustSubmitDraft(t, transactor, ctx, drafts, draft)
		return draft
	}
	pending := submit("credit-pending", 100)
	approved := submit("credit-approved", 200)
	published := submit("credit-published", 300)
	mustAdvanceDraft(t, transactor, ctx, drafts, approveDraft(t, approved, "op-approver"))
	publishedBase := approveDraft(t, published, "op-approver")
	mustAdvanceDraft(t, transactor, ctx, drafts, publishedBase)
	done, err := publishedBase.MarkPublished(draftPublishedAt)
	if err != nil {
		t.Fatalf("发布：%v", err)
	}
	mustAdvanceDraft(t, transactor, ctx, drafts, done)
	for _, objectID := range []string{"credit-pending", "credit-approved", "credit-published"} {
		makeUnreadable(t, pool, objectID)
	}

	if outcome := mustSubmitDraft(t, transactor, ctx, drafts, pending); outcome != ports.PublicationDraftRevised {
		t.Fatalf("待批准的读不回行按同版本重录 = %s, want REVISED", outcome)
	}
	if again, found := loadDraft(t, ctx, drafts, "tenant-1", draftShellIn(t, "tenant-1", "scope-1", "credit-pending", "v1")); !found ||
		again.Status() != domain.PublicationDraftPendingApproval || again.Canonical().Digest() != pending.Canonical().Digest() {
		t.Fatalf("修订之后应读得回且内容是新录的：%v %v", again.Status(), found)
	}
	for _, kept := range []domain.PublicationDraft{approved, published} {
		if outcome := mustSubmitDraft(t, transactor, ctx, drafts, kept); outcome != ports.PublicationDraftContentFixed {
			t.Fatalf("%s 重录 = %s, want CONTENT_FIXED", kept.ObjectID(), outcome)
		}
	}
	var digest, status string
	if err := pool.QueryRow(ctx,
		`SELECT content_digest, status::text FROM party_commercial.publication_draft WHERE object_id = 'credit-approved'`).Scan(&digest, &status); err != nil ||
		digest != "PCC-9:future-build" || status != "2" {
		t.Fatalf("已批准的读不回行被动过：%s %s %v", digest, status, err)
	}
}

// Covers: AT-PC-071 的反面——文档自身立不住或与摘要对不上的坏行不被收进读不回一格：点读与列表答错误（列表整次未形成，不交缺了行的
// 列表），已批准的坏行录入也按错误交回；而`待批准`的坏行同内容按同版本重录能被改好（修订条件含快照文档，重放仍一字不动）。
func TestABrokenDraftRowStillFailsAndAPendingOneIsHealedByResubmittingTheSameContent(t *testing.T) {
	drafts, transactor, pool := unreadableDraftRegistry(t)
	ctx := t.Context()
	shell := draftShellIn(t, "tenant-1", "scope-1", "credit-broken", "v1")
	draft := pendingDraft(t, shell, 100, "op-submitter")
	mustSubmitDraft(t, transactor, ctx, drafts, draft)

	flaws := map[string]string{
		"正文缺席":      `'{"canonicalization":"PCC-1","kind":"CREDIT_POLICY"}'::jsonb`,
		"不是对象":      `'[]'::jsonb`,
		"是字符串":      `'"x"'::jsonb`,
		"册名不认得":     `jsonb_set(content_document, '{kind}', '"NOT_A_KIND"')`,
		"文档自称的版本不符": `jsonb_set(content_document, '{canonicalization}', '"PCC-2"')`,
		"必填格缺失":     `content_document #- '{creditPolicy,legalEntity}'`,
		"额度被改而摘要没动": `jsonb_set(content_document, '{creditPolicy,limitMinor}', '999')`,
		"册被换成别册":    `jsonb_set(content_document, '{kind}', '"SETTLEMENT_POLICY"')`,
	}
	for name, document := range flaws {
		if _, err := pool.Exec(ctx,
			`UPDATE party_commercial.publication_draft SET content_document = `+document+` WHERE object_id = 'credit-broken'`); err != nil {
			t.Fatalf("%s：改写快照：%v", name, err)
		}
		_, found, err := drafts.LoadDraft(ctx, pcTenant(t, "tenant-1"), shell.Kind, shell.ObjectID, shell.Version)
		if found || !errors.Is(err, domain.ErrInvalidRehydratedPublicationDraft) || errors.Is(err, domain.ErrPublicationDraftNotReadable) {
			t.Fatalf("%s：点读 found %v, err %v, want 行坏了而不是读不回", name, found, err)
		}
		if _, err := drafts.ListOpenDrafts(ctx, pcTenant(t, "tenant-1"), 10); !errors.Is(err, domain.ErrInvalidRehydratedPublicationDraft) {
			t.Fatalf("%s：列表遇到坏行 err = %v, want 整次未形成", name, err)
		}
		// 同内容按同版本重录：快照文档与库上不同，修订落下，行回到读得回的样子。
		if outcome := mustSubmitDraft(t, transactor, ctx, drafts, draft); outcome != ports.PublicationDraftRevised {
			t.Fatalf("%s：同内容重录 = %s, want REVISED", name, outcome)
		}
		if healed, found := loadDraft(t, ctx, drafts, "tenant-1", shell); !found || healed.Canonical().Digest() != draft.Canonical().Digest() {
			t.Fatalf("%s：重录之后应读得回原内容", name)
		}
	}
	// 没有变化的同一次录入是重放：文档比较不会把重放误当修订。
	if outcome := mustSubmitDraft(t, transactor, ctx, drafts, draft); outcome != ports.PublicationDraftReplayed {
		t.Fatalf("同一次录入 = %s, want REPLAYED", outcome)
	}

	// 已批准的坏行内容已固定、不能就地替换：录入按错误交回，不装成读不回，也不装成内容已固定。
	mustAdvanceDraft(t, transactor, ctx, drafts, approveDraft(t, draft, "op-approver"))
	if _, err := pool.Exec(ctx,
		`UPDATE party_commercial.publication_draft SET content_document = '[]'::jsonb WHERE object_id = 'credit-broken'`); err != nil {
		t.Fatalf("改写已批准行的快照：%v", err)
	}
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		outcome, err := drafts.SubmitDraft(txCtx, draft)
		if err == nil {
			t.Errorf("已批准的坏行录入 = %s, want 错误", outcome)
		}
		return err
	})
	if !errors.Is(err, domain.ErrInvalidRehydratedPublicationDraft) {
		t.Fatalf("err = %v, want the broken-row refusal", err)
	}
}

// Covers: AT-PC-071——读不回只收两类：列上的规范化版本不认得时，不论文档长什么样（这里是 `[]`）都是读不回，文档无从判坏。
func TestAnUnknownColumnVersionIsNotReadableWhateverTheDocumentLooksLike(t *testing.T) {
	drafts, transactor, pool := unreadableDraftRegistry(t)
	ctx := t.Context()
	shell := draftShellIn(t, "tenant-1", "scope-1", "credit-future", "v1")
	mustSubmitDraft(t, transactor, ctx, drafts, pendingDraft(t, shell, 100, "op-submitter"))
	if _, err := pool.Exec(ctx,
		`UPDATE party_commercial.publication_draft
		    SET content_canonicalization = 'PCC-9', content_digest = 'PCC-9:future-build', content_document = '[]'::jsonb
		  WHERE object_id = 'credit-future'`); err != nil {
		t.Fatalf("改写：%v", err)
	}
	_, found, err := drafts.LoadDraft(ctx, pcTenant(t, "tenant-1"), shell.Kind, shell.ObjectID, shell.Version)
	if found || !errors.Is(err, domain.ErrPublicationDraftNotReadable) || !errors.Is(err, domain.ErrCanonicalizationUnsupported) {
		t.Fatalf("点读 = found %v, err %v", found, err)
	}
}
