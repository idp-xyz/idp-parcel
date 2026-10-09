package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证价卡草稿册（0011；票 price-card-import/03）：一版一行、`已批准`之前换内容替换
// 那一行、之后答内容已固定，读回经领域整图重验再与比对列交叉核，查阅按租户列出、可按状态筛。批准与发布的写口
// 归票 04，这里用一条 UPDATE 造出那两格。夹具全部为 SYN 合成值（S 级）。

const draftOtherSourceSHA = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

var draftAt = time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC)

type draftStore struct {
	drafts     *adapter.PriceCardDrafts
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newDraftStore(t *testing.T) draftStore {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	drafts, err := adapter.NewPriceCardDrafts(db)
	if err != nil {
		t.Fatalf("构造草稿册：%v", err)
	}
	return draftStore{drafts: drafts, transactor: db.Transactor(), pool: pool}
}

func (store draftStore) submit(t *testing.T, draft domain.PriceCardDraft) ports.PriceCardDraftSubmitOutcome {
	t.Helper()
	var outcome ports.PriceCardDraftSubmitOutcome
	err := store.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		saved, submitErr := store.drafts.SubmitDraft(ctx, draft)
		outcome = saved
		return submitErr
	})
	if err != nil {
		t.Fatalf("录入草稿：%v", err)
	}
	return outcome
}

func (store draftStore) list(t *testing.T, tenant string, status domain.PriceCardDraftStatus) []domain.PriceCardDraft {
	t.Helper()
	drafts, err := store.drafts.ListPriceCardDrafts(t.Context(), evaluationValue(t, domain.NewTenantID, tenant), status, 50)
	if err != nil {
		t.Fatalf("列草稿：%v", err)
	}
	return drafts
}

// advance 造出票 04 推进后的那一格：状态与批准痕迹是列，推进只改这几列、不重写内容文档。
func (store draftStore) advance(t *testing.T, tenant, planID string, status domain.PriceCardDraftStatus) {
	t.Helper()
	var publishedAt *time.Time
	if status == domain.PriceCardDraftStatusPublished {
		at := draftAt.Add(2 * time.Hour)
		publishedAt = &at
	}
	tag, err := store.pool.Exec(t.Context(),
		`UPDATE parcel_pricing.price_card_draft
		    SET status = $3, approver = 'https://id.syn.example/dex#SYN-APPROVER', approved_at = $4, published_at = $5
		  WHERE tenant_id = $1 AND plan_id = $2`,
		tenant, planID, status.String(), draftAt.Add(time.Hour), publishedAt)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("推进草稿到 %s：%v（%d 行）", status, err, tag.RowsAffected())
	}
}

func draftSourceOf(t *testing.T, name, sha string) domain.SourceFileIdentity {
	t.Helper()
	source, err := domain.NewSourceFileIdentity(name, sha)
	if err != nil {
		t.Fatalf("构造源文件身份：%v", err)
	}
	return source
}

func validatedDraftOf(t *testing.T, tenant string, plan domain.PricingPlanVersion, source domain.SourceFileIdentity, at time.Time) domain.PriceCardDraft {
	t.Helper()
	authorization, err := domain.NewVersionReferenceIdentity(domain.ArtifactCommercialAuthorization, "SYN-AUTH-COST-DIR-01", "v1")
	if err != nil {
		t.Fatalf("构造授权引用：%v", err)
	}
	draft, err := domain.SubmitPriceCardDraft(domain.PriceCardDraftSubmission{
		Tenant:      evaluationValue(t, domain.NewTenantID, tenant),
		Plan:        plan.Reference(),
		Source:      source,
		Content:     &domain.PriceCardDraftContent{Plan: plan, DirectionAuthorization: authorization},
		Submitter:   "https://id.syn.example/dex#SYN-OPERATOR-01",
		SubmittedAt: at,
	})
	if err != nil {
		t.Fatalf("立已校验草稿：%v", err)
	}
	return draft
}

func problemDraftOf(t *testing.T, tenant, planID string, source domain.SourceFileIdentity, at time.Time, submitter string) domain.PriceCardDraft {
	t.Helper()
	plan, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, planID, "v1")
	if err != nil {
		t.Fatalf("构造方案引用：%v", err)
	}
	first, err := domain.NewPriceCardDraftProblem("tables", 2, "currency", "CELL_INVALID", "币种立不住")
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.NewPriceCardDraftProblem("rates_weight_zone", 0, "", "SHEET_MISSING", "缺表 rates_weight_zone")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := domain.SubmitPriceCardDraft(domain.PriceCardDraftSubmission{
		Tenant: evaluationValue(t, domain.NewTenantID, tenant), Plan: plan, Source: source,
		Problems: []domain.PriceCardDraftProblem{first, second}, Submitter: submitter, SubmittedAt: at,
	})
	if err != nil {
		t.Fatalf("立带问题的草稿：%v", err)
	}
	return draft
}

// 一版一行：首录落新行，同内容再录是重放、行数不变；读回的那一份经整图重验，内容摘要、源文件身份与录入者都原样。
func TestADraftVersionIsOneRowAndTheSameContentReplays(t *testing.T) {
	store := newDraftStore(t)
	plan := catalogPlan(t, "SYN-PLAN-DRAFT-01", "v1", domain.PricingDirectionBuy, "30")
	source := draftSourceOf(t, "SYN-CARD-DRAFT-01.xlsx", catalogSourceSHA)

	if outcome := store.submit(t, validatedDraftOf(t, "SYN-TENANT-01", plan, source, draftAt)); outcome != ports.PriceCardDraftSaved {
		t.Fatalf("首录 = %v", outcome)
	}
	if outcome := store.submit(t, validatedDraftOf(t, "SYN-TENANT-01", plan, source, draftAt.Add(time.Hour))); outcome != ports.PriceCardDraftReplayed {
		t.Fatalf("同内容再录 = %v", outcome)
	}

	drafts := store.list(t, "SYN-TENANT-01", domain.PriceCardDraftStatusInvalid)
	if len(drafts) != 1 {
		t.Fatalf("drafts = %d, want 1", len(drafts))
	}
	read := drafts[0]
	content, has := read.Content()
	if read.Status() != domain.PriceCardDraftStatusValidated || !has || content.Plan.ContentDigest() != plan.ContentDigest() ||
		content.DirectionAuthorization.ID() != "SYN-AUTH-COST-DIR-01" || read.SourceFile() != source ||
		read.Submitter() != "https://id.syn.example/dex#SYN-OPERATOR-01" || !read.SubmittedAt().Equal(draftAt) {
		t.Fatalf("read = %+v", read)
	}
}

// `已批准`之前换内容是修订：那一行被替换，状态随新内容走，录入者与录入时刻随之更新；换回带问题的也一样。
func TestARevisionReplacesTheRowBeforeApproval(t *testing.T) {
	store := newDraftStore(t)
	plan := catalogPlan(t, "SYN-PLAN-DRAFT-01", "v1", domain.PricingDirectionBuy, "30")

	broken := problemDraftOf(t, "SYN-TENANT-01", "SYN-PLAN-DRAFT-01", draftSourceOf(t, "card.xlsx", catalogSourceSHA), draftAt, "https://id.syn.example/dex#SYN-OPERATOR-01")
	if outcome := store.submit(t, broken); outcome != ports.PriceCardDraftSaved {
		t.Fatalf("首录 = %v", outcome)
	}
	fixed := validatedDraftOf(t, "SYN-TENANT-01", plan, draftSourceOf(t, "card.xlsx", draftOtherSourceSHA), draftAt.Add(time.Hour))
	if outcome := store.submit(t, fixed); outcome != ports.PriceCardDraftRevised {
		t.Fatalf("改好了再录 = %v", outcome)
	}
	read := store.list(t, "SYN-TENANT-01", domain.PriceCardDraftStatusInvalid)
	if len(read) != 1 || read[0].Status() != domain.PriceCardDraftStatusValidated || len(read[0].Problems()) != 0 ||
		read[0].SourceFile().SHA256() != draftOtherSourceSHA || !read[0].SubmittedAt().Equal(draftAt.Add(time.Hour)) {
		t.Fatalf("修订后 = %+v", read)
	}

	again := problemDraftOf(t, "SYN-TENANT-01", "SYN-PLAN-DRAFT-01", draftSourceOf(t, "card.xlsx", catalogSourceSHA), draftAt.Add(2*time.Hour), "https://id.syn.example/dex#SYN-OPERATOR-02")
	if outcome := store.submit(t, again); outcome != ports.PriceCardDraftRevised {
		t.Fatalf("改坏了再录 = %v", outcome)
	}
	read = store.list(t, "SYN-TENANT-01", domain.PriceCardDraftStatusInvalid)
	if len(read) != 1 || read[0].Status() != domain.PriceCardDraftStatusDraft || len(read[0].Problems()) != 2 ||
		read[0].Submitter() != "https://id.syn.example/dex#SYN-OPERATOR-02" {
		t.Fatalf("再修订后 = %+v", read)
	}
	var digest *string
	if err := store.pool.QueryRow(t.Context(),
		`SELECT content_digest FROM parcel_pricing.price_card_draft WHERE tenant_id = 'SYN-TENANT-01'`).Scan(&digest); err != nil || digest != nil {
		t.Fatalf("回到`草稿`后比对列没清：%v %v", digest, err)
	}
}

// `已批准`之后内容固定：换内容答内容已固定且那一行一字不动，同内容仍是重放；`已发布`同。
func TestAnApprovedOrPublishedDraftHasFixedContent(t *testing.T) {
	for _, status := range []domain.PriceCardDraftStatus{domain.PriceCardDraftStatusApproved, domain.PriceCardDraftStatusPublished} {
		store := newDraftStore(t)
		plan := catalogPlan(t, "SYN-PLAN-DRAFT-01", "v1", domain.PricingDirectionBuy, "30")
		source := draftSourceOf(t, "card.xlsx", catalogSourceSHA)
		if outcome := store.submit(t, validatedDraftOf(t, "SYN-TENANT-01", plan, source, draftAt)); outcome != ports.PriceCardDraftSaved {
			t.Fatalf("%s: 首录 = %v", status, outcome)
		}
		store.advance(t, "SYN-TENANT-01", "SYN-PLAN-DRAFT-01", status)

		changed := catalogPlan(t, "SYN-PLAN-DRAFT-01", "v1", domain.PricingDirectionBuy, "31")
		if outcome := store.submit(t, validatedDraftOf(t, "SYN-TENANT-01", changed, draftSourceOf(t, "card.xlsx", draftOtherSourceSHA), draftAt.Add(3*time.Hour))); outcome != ports.PriceCardDraftContentFixed {
			t.Fatalf("%s: 换内容 = %v", status, outcome)
		}
		if outcome := store.submit(t, validatedDraftOf(t, "SYN-TENANT-01", plan, source, draftAt.Add(3*time.Hour))); outcome != ports.PriceCardDraftReplayed {
			t.Fatalf("%s: 同内容 = %v", status, outcome)
		}
		read := store.list(t, "SYN-TENANT-01", status)
		if len(read) != 1 {
			t.Fatalf("%s: drafts = %d, want 1", status, len(read))
		}
		if content, _ := read[0].Content(); content.Plan.ContentDigest() != plan.ContentDigest() || !read[0].SubmittedAt().Equal(draftAt) {
			t.Fatalf("%s: 内容已固定却动了那一行：%+v", status, read)
		}
		if approver, has := read[0].Approver(); !has || approver != "https://id.syn.example/dex#SYN-APPROVER" {
			t.Fatalf("%s: approver = %q", status, approver)
		}
	}
}

// 查阅读口按租户列出（租户是键的一部分，不是过滤器），按录入时刻倒序；状态筛选只列那一格。
func TestTheDraftListIsPerTenantNewestFirstAndFiltersByStatus(t *testing.T) {
	store := newDraftStore(t)
	cost := catalogPlan(t, "SYN-PLAN-DRAFT-01", "v1", domain.PricingDirectionBuy, "30")
	sell := catalogPlan(t, "SYN-PLAN-DRAFT-02", "v1", domain.PricingDirectionSell, "45")
	source := draftSourceOf(t, "card.xlsx", catalogSourceSHA)

	store.submit(t, validatedDraftOf(t, "SYN-TENANT-01", cost, source, draftAt))
	store.submit(t, validatedDraftOf(t, "SYN-TENANT-01", sell, source, draftAt.Add(2*time.Hour)))
	store.submit(t, problemDraftOf(t, "SYN-TENANT-01", "SYN-PLAN-DRAFT-03", source, draftAt.Add(time.Hour), "https://id.syn.example/dex#SYN-OPERATOR-01"))
	store.submit(t, validatedDraftOf(t, "SYN-TENANT-02", cost, source, draftAt.Add(3*time.Hour)))

	all := store.list(t, "SYN-TENANT-01", domain.PriceCardDraftStatusInvalid)
	if len(all) != 3 || all[0].Plan().ID() != "SYN-PLAN-DRAFT-02" || all[1].Plan().ID() != "SYN-PLAN-DRAFT-03" || all[2].Plan().ID() != "SYN-PLAN-DRAFT-01" {
		t.Fatalf("all = %+v", planIDsOf(all))
	}
	problems := all[1].Problems()
	if len(problems) != 2 || problems[0].Code() != "CELL_INVALID" || problems[0].Message() != "币种立不住" || problems[1].Column() != "" {
		t.Fatalf("problems = %+v", problems)
	}
	if drafts := store.list(t, "SYN-TENANT-01", domain.PriceCardDraftStatusDraft); len(drafts) != 1 || drafts[0].Plan().ID() != "SYN-PLAN-DRAFT-03" {
		t.Fatalf("DRAFT = %+v", planIDsOf(drafts))
	}
	if drafts := store.list(t, "SYN-TENANT-01", domain.PriceCardDraftStatusApproved); len(drafts) != 0 {
		t.Fatalf("APPROVED = %+v", planIDsOf(drafts))
	}
	if drafts := store.list(t, "SYN-TENANT-03", domain.PriceCardDraftStatusInvalid); drafts == nil || len(drafts) != 0 {
		t.Fatalf("空册应交回空列表：%v", drafts)
	}

	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "SYN-TENANT-01")
	for name, call := range map[string]func() error{
		"页大小为零": func() error {
			_, err := store.drafts.ListPriceCardDrafts(ctx, tenant, domain.PriceCardDraftStatusInvalid, 0)
			return err
		},
		"状态不在四格": func() error {
			_, err := store.drafts.ListPriceCardDrafts(ctx, tenant, domain.PriceCardDraftStatus(9), 10)
			return err
		},
		"缺租户": func() error {
			_, err := store.drafts.ListPriceCardDrafts(ctx, domain.TenantID{}, domain.PriceCardDraftStatusInvalid, 10)
			return err
		},
	} {
		if call() == nil {
			t.Errorf("%s: 读口照答了", name)
		}
	}
}

// 读回门：比对列与内容文档分岔、或文档里的摘要被改过，读口交回错误而不是一份看起来如实的草稿。
func TestTheDraftListRefusesARowThatWasTamperedWith(t *testing.T) {
	for name, tamper := range map[string]string{
		"比对列被改": `UPDATE parcel_pricing.price_card_draft SET content_digest = 'sha256:tampered'`,
		"授权列被改": `UPDATE parcel_pricing.price_card_draft SET authorization_version = 'v9'`,
		"文档被改":  `UPDATE parcel_pricing.price_card_draft SET content_document = jsonb_set(content_document, '{plan,contentDigest}', '"sha256:tampered"')`,
	} {
		store := newDraftStore(t)
		plan := catalogPlan(t, "SYN-PLAN-DRAFT-01", "v1", domain.PricingDirectionBuy, "30")
		store.submit(t, validatedDraftOf(t, "SYN-TENANT-01", plan, draftSourceOf(t, "card.xlsx", catalogSourceSHA), draftAt))
		if _, err := store.pool.Exec(t.Context(), tamper); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := store.drafts.ListPriceCardDrafts(t.Context(), evaluationValue(t, domain.NewTenantID, "SYN-TENANT-01"), domain.PriceCardDraftStatusInvalid, 10); err == nil {
			t.Errorf("%s: 读口照答了", name)
		}
	}
}

// 库上的 CHECK 是领域不变量的镜像：内容比对列与状态对不上、已批准缺批准者，这两种行直接写也写不进去。
func TestTheDraftTableRefusesRowsNoPathCouldWrite(t *testing.T) {
	store := newDraftStore(t)
	for name, statement := range map[string]string{
		"已校验缺摘要": `INSERT INTO parcel_pricing.price_card_draft
			(tenant_id, plan_id, plan_version, status, source_file_name, source_file_sha256,
			 canonicalization, content_digest, authorization_id, authorization_version, content_document, submitter, submitted_at)
			VALUES ('SYN-TENANT-01', 'SYN-PLAN-X', 'v1', 'VALIDATED', 'card.xlsx', '` + catalogSourceSHA + `',
			 'PPC-5', NULL, 'SYN-AUTH', 'v1', '{}', 'SYN-OPERATOR', now())`,
		"已批准缺批准者": `INSERT INTO parcel_pricing.price_card_draft
			(tenant_id, plan_id, plan_version, status, source_file_name, source_file_sha256,
			 canonicalization, content_digest, authorization_id, authorization_version, content_document, submitter, submitted_at, approved_at)
			VALUES ('SYN-TENANT-01', 'SYN-PLAN-Y', 'v1', 'APPROVED', 'card.xlsx', '` + catalogSourceSHA + `',
			 'PPC-5', 'sha256:x', 'SYN-AUTH', 'v1', '{}', 'SYN-OPERATOR', now(), now())`,
	} {
		if _, err := store.pool.Exec(t.Context(), statement); err == nil {
			t.Errorf("%s: 写进去了", name)
		}
	}
}

func planIDsOf(drafts []domain.PriceCardDraft) []string {
	ids := make([]string, 0, len(drafts))
	for _, draft := range drafts {
		ids = append(ids, draft.Plan().ID())
	}
	return ids
}
