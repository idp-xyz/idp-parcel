package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// PriceCardDrafts 实现草稿册的写口与查阅读口（0011；ADR-0101 决定三，票 price-card-import/03）。
//
// 同版再录的落点由领域 PriceCardDraft.ResubmissionOf 判，这里只管落库与并发：首录 INSERT … ON CONFLICT DO NOTHING；
// 撞键时在同一笔事务里 SELECT … FOR UPDATE 锁住册上那一行、经重建门读回、交领域判，判成修订才 UPDATE。锁住之后
// 到 UPDATE 之间没有别人改得了那一行，所以落点规则不必在 SQL 的 WHERE 里再写一遍——写两遍就是两个口径。
type PriceCardDrafts struct {
	db *bentopg.DB
}

var (
	_ ports.PriceCardDraftRegister = (*PriceCardDrafts)(nil)
	_ ports.PriceCardDraftRead     = (*PriceCardDrafts)(nil)
	_ ports.PriceCardDraftProgress = (*PriceCardDrafts)(nil)
)

func NewPriceCardDrafts(db *bentopg.DB) (*PriceCardDrafts, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel pricing postgres: db is nil")
	}
	return &PriceCardDrafts{db: db}, nil
}

// priceCardDraftColumns 是写回与读回共用的列面，次序与 scanPriceCardDraftRow 对齐。
const priceCardDraftColumns = `plan_id, plan_version, status, source_file_name, source_file_sha256,
		        canonicalization, content_digest, authorization_id, authorization_version,
		        content_document, submitter, submitted_at, approver, approved_at, published_at`

// SubmitDraft 录入一版。只收新录入（`草稿`或`已校验`）：后两格只从推进而来。
func (drafts *PriceCardDrafts) SubmitDraft(ctx context.Context, draft domain.PriceCardDraft) (ports.PriceCardDraftSubmitOutcome, error) {
	executor, err := drafts.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PriceCardDraftSubmitOutcomeInvalid, fmt.Errorf("submit price card draft: %w", err)
	}
	if status := draft.Status(); status != domain.PriceCardDraftStatusDraft && status != domain.PriceCardDraftStatusValidated {
		return ports.PriceCardDraftSubmitOutcomeInvalid, fmt.Errorf("submit price card draft: only a fresh submission can be submitted, got %q", status)
	}
	document, err := domain.MarshalPriceCardDraftContent(draft)
	if err != nil {
		return ports.PriceCardDraftSubmitOutcomeInvalid, fmt.Errorf("submit price card draft: %w", err)
	}
	content := contentColumnsOf(draft)

	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_pricing.price_card_draft
			(tenant_id, plan_id, plan_version, status, source_file_name, source_file_sha256,
			 canonicalization, content_digest, authorization_id, authorization_version,
			 content_document, submitter, submitted_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT DO NOTHING`,
		draft.Tenant().String(), draft.Plan().ID(), draft.Plan().Version(), draft.Status().String(),
		draft.SourceFile().Name(), draft.SourceFile().SHA256(),
		content.canonicalization, content.digest, content.authorizationID, content.authorizationVersion,
		document, draft.Submitter(), draft.SubmittedAt().UTC(),
	)
	if err != nil {
		return ports.PriceCardDraftSubmitOutcomeInvalid, fmt.Errorf("submit price card draft: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.PriceCardDraftSaved, nil
	}

	row, err := scanPriceCardDraftRow(executor.QueryRow(ctx,
		`SELECT `+priceCardDraftColumns+`
		   FROM parcel_pricing.price_card_draft
		  WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3
		    FOR UPDATE`,
		draft.Tenant().String(), draft.Plan().ID(), draft.Plan().Version(),
	))
	if err != nil {
		return ports.PriceCardDraftSubmitOutcomeInvalid, fmt.Errorf("submit price card draft: 撞键后读册上那一行：%w", err)
	}
	existing, err := row.draft(draft.Tenant())
	if err != nil {
		return ports.PriceCardDraftSubmitOutcomeInvalid, fmt.Errorf("submit price card draft: %w", err)
	}
	resubmission, err := existing.ResubmissionOf(draft)
	if err != nil {
		return ports.PriceCardDraftSubmitOutcomeInvalid, fmt.Errorf("submit price card draft: %w", err)
	}
	switch resubmission {
	case domain.PriceCardDraftResubmissionReplay:
		return ports.PriceCardDraftReplayed, nil
	case domain.PriceCardDraftResubmissionContentFixed:
		return ports.PriceCardDraftContentFixed, nil
	case domain.PriceCardDraftResubmissionRevision:
		return revisePriceCardDraft(ctx, executor, draft, document, content)
	default:
		return ports.PriceCardDraftSubmitOutcomeInvalid, fmt.Errorf("submit price card draft: unexpected resubmission %q", resubmission)
	}
}

// revisePriceCardDraft 替换册上那一行：内容、源文件身份、录入者与录入时刻一并换成这次录入的；那一行已被 SubmitDraft
// 锁住，又只在`已批准`之前才判得出修订，所以批准与发布三列此刻必为空、不必再写。
func revisePriceCardDraft(
	ctx context.Context,
	executor bentopg.Executor,
	draft domain.PriceCardDraft,
	document []byte,
	content draftContentColumns,
) (ports.PriceCardDraftSubmitOutcome, error) {
	tag, err := executor.Exec(ctx,
		`UPDATE parcel_pricing.price_card_draft
		    SET status = $4, source_file_name = $5, source_file_sha256 = $6,
		        canonicalization = $7, content_digest = $8, authorization_id = $9, authorization_version = $10,
		        content_document = $11, submitter = $12, submitted_at = $13
		  WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3`,
		draft.Tenant().String(), draft.Plan().ID(), draft.Plan().Version(), draft.Status().String(),
		draft.SourceFile().Name(), draft.SourceFile().SHA256(),
		content.canonicalization, content.digest, content.authorizationID, content.authorizationVersion,
		document, draft.Submitter(), draft.SubmittedAt().UTC(),
	)
	if err != nil {
		return ports.PriceCardDraftSubmitOutcomeInvalid, fmt.Errorf("submit price card draft: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ports.PriceCardDraftSubmitOutcomeInvalid, fmt.Errorf("submit price card draft: 锁住的那一行替换时不在了")
	}
	return ports.PriceCardDraftRevised, nil
}

// ListPriceCardDrafts 按租户列草稿，按录入时刻倒序、同刻按方案标识与版本正序收尾，分页可重复。status 为零值即四格都列。
// 每一行经重建门读回再与比对列交叉核：读不回是答案没形成，不跳过那一行——跳过会让一份坏行看起来像不存在。
func (drafts *PriceCardDrafts) ListPriceCardDrafts(
	ctx context.Context,
	tenant domain.TenantID,
	status domain.PriceCardDraftStatus,
	limit int,
) ([]domain.PriceCardDraft, error) {
	if tenant.String() == "" {
		return nil, fmt.Errorf("list price card drafts: tenant is required")
	}
	if err := catalogueLimit("list price card drafts", limit); err != nil {
		return nil, err
	}
	filter := ""
	if status != domain.PriceCardDraftStatusInvalid {
		if status.String() == "" {
			return nil, fmt.Errorf("list price card drafts: status %d is not one of the four", uint8(status))
		}
		filter = status.String()
	}
	querier, err := drafts.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list price card drafts: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT `+priceCardDraftColumns+`
		   FROM parcel_pricing.price_card_draft
		  WHERE tenant_id = $1 AND ($2::text = '' OR status = $2::text)
		  ORDER BY submitted_at DESC, plan_id, plan_version
		  LIMIT $3`,
		tenant.String(), filter, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list price card drafts: %w", err)
	}
	defer rows.Close()

	listed := make([]domain.PriceCardDraft, 0, limit)
	for rows.Next() {
		row, err := scanPriceCardDraftRow(rows)
		if err != nil {
			return nil, fmt.Errorf("list price card drafts: %w", err)
		}
		draft, err := row.draft(tenant)
		if err != nil {
			return nil, fmt.Errorf("list price card drafts: %w", err)
		}
		listed = append(listed, draft)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list price card drafts: %w", err)
	}
	return listed, nil
}

// LoadPriceCardDraft 按键读一版，经重建门读回再与比对列交叉核（票 price-card-import/04）。册上没有即 found=false。
func (drafts *PriceCardDrafts) LoadPriceCardDraft(
	ctx context.Context,
	tenant domain.TenantID,
	plan domain.VersionReference,
) (domain.PriceCardDraft, bool, error) {
	querier, err := drafts.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PriceCardDraft{}, false, fmt.Errorf("load price card draft: %w", err)
	}
	row, err := scanPriceCardDraftRow(querier.QueryRow(ctx,
		`SELECT `+priceCardDraftColumns+`
		   FROM parcel_pricing.price_card_draft
		  WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3`,
		tenant.String(), plan.ID(), plan.Version(),
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PriceCardDraft{}, false, nil
	}
	if err != nil {
		return domain.PriceCardDraft{}, false, fmt.Errorf("load price card draft: %w", err)
	}
	draft, err := row.draft(tenant)
	if err != nil {
		return domain.PriceCardDraft{}, false, fmt.Errorf("load price card draft: %w", err)
	}
	return draft, true, nil
}

// AdvancePriceCardDraft 把领域推进后的那一份写回：只改状态与批准、发布三格痕迹，不重写内容文档。「还是读到时那一行」
// 由 WHERE 判——前一格、录入者、录入时刻与内容摘要都对得上，推进成发布时连批准者与批准时刻也要对得上；修订会换掉录入
// 痕迹，别人先推进会换掉状态，两者都落成`已被替换`、行一字不动。不在这里加锁：批准与发布各在自己的一笔事务里读、判、
// 写，判不上的那一笔由调用方按答案或回滚处置。
func (drafts *PriceCardDrafts) AdvancePriceCardDraft(ctx context.Context, draft domain.PriceCardDraft) (ports.PriceCardDraftAdvanceOutcome, error) {
	executor, err := drafts.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PriceCardDraftAdvanceOutcomeInvalid, fmt.Errorf("advance price card draft: %w", err)
	}
	var previous domain.PriceCardDraftStatus
	switch draft.Status() {
	case domain.PriceCardDraftStatusApproved:
		previous = domain.PriceCardDraftStatusValidated
	case domain.PriceCardDraftStatusPublished:
		previous = domain.PriceCardDraftStatusApproved
	default:
		return ports.PriceCardDraftAdvanceOutcomeInvalid, fmt.Errorf("advance price card draft: only an approval or a publication advances a draft, got %q", draft.Status())
	}
	content, has := draft.Content()
	approver, hasApprover := draft.Approver()
	approvedAt, hasApprovedAt := draft.ApprovedAt()
	if !has || !hasApprover || !hasApprovedAt {
		return ports.PriceCardDraftAdvanceOutcomeInvalid, fmt.Errorf("advance price card draft: the advanced draft carries no content or approval traces")
	}
	var publishedAt *time.Time
	if at, published := draft.PublishedAt(); published {
		publishedAt = &at
	}

	tag, err := executor.Exec(ctx,
		`UPDATE parcel_pricing.price_card_draft
		    SET status = $4, approver = $5, approved_at = $6, published_at = $7
		  WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3
		    AND status = $8 AND submitter = $9 AND submitted_at = $10 AND content_digest = $11
		    AND ($8 <> 'APPROVED' OR (approver = $5 AND approved_at = $6))`,
		draft.Tenant().String(), draft.Plan().ID(), draft.Plan().Version(),
		draft.Status().String(), approver, approvedAt.UTC(), publishedAt,
		previous.String(), draft.Submitter(), draft.SubmittedAt().UTC(), content.Plan.ContentDigest(),
	)
	if err != nil {
		return ports.PriceCardDraftAdvanceOutcomeInvalid, fmt.Errorf("advance price card draft: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.PriceCardDraftAdvanced, nil
	}
	var present bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM parcel_pricing.price_card_draft
		                 WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3)`,
		draft.Tenant().String(), draft.Plan().ID(), draft.Plan().Version(),
	).Scan(&present); err != nil {
		return ports.PriceCardDraftAdvanceOutcomeInvalid, fmt.Errorf("advance price card draft: %w", err)
	}
	if !present {
		return ports.PriceCardDraftAdvanceNotFound, nil
	}
	return ports.PriceCardDraftAdvanceSuperseded, nil
}

// draftContentColumns 是已校验起才有的四列比对列；`草稿`四列全空。
type draftContentColumns struct {
	canonicalization, digest, authorizationID, authorizationVersion *string
}

func contentColumnsOf(draft domain.PriceCardDraft) draftContentColumns {
	content, has := draft.Content()
	if !has {
		return draftContentColumns{}
	}
	canonicalization, digest := content.Plan.CanonicalizationVersion(), content.Plan.ContentDigest()
	authorizationID, authorizationVersion := content.DirectionAuthorization.ID(), content.DirectionAuthorization.Version()
	return draftContentColumns{
		canonicalization: &canonicalization, digest: &digest,
		authorizationID: &authorizationID, authorizationVersion: &authorizationVersion,
	}
}

func (columns draftContentColumns) equal(other draftContentColumns) bool {
	return sameOptional(columns.canonicalization, other.canonicalization) &&
		sameOptional(columns.digest, other.digest) &&
		sameOptional(columns.authorizationID, other.authorizationID) &&
		sameOptional(columns.authorizationVersion, other.authorizationVersion)
}

func sameOptional(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// priceCardDraftRow 是草稿表一行的列面。
type priceCardDraftRow struct {
	planID, planVersion, status string
	sourceName, sourceSHA       string
	content                     draftContentColumns
	document                    []byte
	submitter                   string
	submittedAt                 time.Time
	approver                    *string
	approvedAt, publishedAt     *time.Time
}

func scanPriceCardDraftRow(scanner interface{ Scan(dest ...any) error }) (priceCardDraftRow, error) {
	var row priceCardDraftRow
	err := scanner.Scan(&row.planID, &row.planVersion, &row.status, &row.sourceName, &row.sourceSHA,
		&row.content.canonicalization, &row.content.digest, &row.content.authorizationID, &row.content.authorizationVersion,
		&row.document, &row.submitter, &row.submittedAt, &row.approver, &row.approvedAt, &row.publishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return priceCardDraftRow{}, fmt.Errorf("草稿行不在：%w", err)
	}
	return row, err
}

// draft 经领域重建门读回一行，再与比对列交叉核——列与文档分岔说明行被改过。规范化版本不被本构建支持时把领域那枚
// 哨兵原样包出去，调用方据以分格（ADR-0014）。
func (row priceCardDraftRow) draft(tenant domain.TenantID) (domain.PriceCardDraft, error) {
	status, known := domain.ParsePriceCardDraftStatus(row.status)
	if !known {
		return domain.PriceCardDraft{}, fmt.Errorf("%s/%s：状态 %q 不在四格里", row.planID, row.planVersion, row.status)
	}
	plan, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, row.planID, row.planVersion)
	if err != nil {
		return domain.PriceCardDraft{}, fmt.Errorf("%s/%s：%w", row.planID, row.planVersion, err)
	}
	source, err := domain.NewSourceFileIdentity(row.sourceName, row.sourceSHA)
	if err != nil {
		return domain.PriceCardDraft{}, fmt.Errorf("%s/%s：%w", row.planID, row.planVersion, err)
	}
	spec := domain.RehydratePriceCardDraftSpec{
		Tenant: tenant, Plan: plan, Status: status, Source: source, Document: row.document,
		Submitter: row.submitter, SubmittedAt: row.submittedAt,
	}
	if row.approver != nil {
		spec.Approver = *row.approver
	}
	if row.approvedAt != nil {
		spec.ApprovedAt = *row.approvedAt
	}
	if row.publishedAt != nil {
		spec.PublishedAt = *row.publishedAt
	}
	draft, err := domain.RehydratePriceCardDraft(spec)
	if err != nil {
		return domain.PriceCardDraft{}, fmt.Errorf("%s/%s：%w", row.planID, row.planVersion, err)
	}
	if !contentColumnsOf(draft).equal(row.content) {
		return domain.PriceCardDraft{}, fmt.Errorf("comparison columns disagree with the content document for %s/%s", row.planID, row.planVersion)
	}
	return draft, nil
}
