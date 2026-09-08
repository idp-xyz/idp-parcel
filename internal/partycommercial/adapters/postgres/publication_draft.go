package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 待批准发布载体与审批职责规则的持久化面（0028；ADR-0126 Decision 三）。
//
// 载体表是本上下文唯一一张就地更新的表：`待批准`期间的修订替换行，批准与发布推进状态。每一次写都带条件——
// 修订只在库上仍是`待批准`且这次录入与库上那份不同时落，推进只在库上仍是前一格状态、且行与推进所依据的那一次
// 录入逐列相同时落——条件不满足时零行命中，读回既有行把它翻成一格答案（重放 / 内容已固定 / 已被替换），不盖过去。
//
// content_document 列是 jsonb：库上存的是归一化后的形状，不是摘要盖住的那份原字节。摘要与快照仍是一样东西的两面，
// 靠的是重建门（domain.RehydratePublicationDraft）把文档折回正文再算一遍与列上摘要比，而不是靠字节相等。0028 头注
// 写的「就是……的字节」以此处为准；已施加的迁移不改，改了会让 migrate 的校验和漂移门把它挡下。

// PublicationDrafts 实现 ports.PublicationDraftRegistry。
type PublicationDrafts struct {
	db *bentopg.DB
}

func NewPublicationDrafts(db *bentopg.DB) (*PublicationDrafts, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &PublicationDrafts{db: db}, nil
}

var _ ports.PublicationDraftRegistry = (*PublicationDrafts)(nil)

// SubmitDraft 录入一版一行。INSERT … ON CONFLICT DO UPDATE 带 WHERE：库上是`待批准`且这次录入与库上那份不同
// （摘要或壳任一不同）才替换；RETURNING 用 xmax = 0 分辨插入与更新。零行命中时读回既有行：同一次录入是重放，
// 否则载体已过了`待批准`、内容固定。
func (repository *PublicationDrafts) SubmitDraft(
	ctx context.Context,
	draft domain.PublicationDraft,
) (ports.PublicationDraftSubmitOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PublicationDraftSubmitOutcomeInvalid, fmt.Errorf("submit publication draft: %w", err)
	}
	if draft.Status() != domain.PublicationDraftPendingApproval {
		return ports.PublicationDraftSubmitOutcomeInvalid, fmt.Errorf("submit publication draft: only a pending draft can be submitted, got %s", draft.Status())
	}
	references, err := json.Marshal(referenceDocumentsOf(draft.DeclaredReferences()))
	if err != nil {
		return ports.PublicationDraftSubmitOutcomeInvalid, fmt.Errorf("submit publication draft: %w", err)
	}
	startsAt, endsAt := intervalColumns(draft.Effective())

	var inserted bool
	err = executor.QueryRow(ctx,
		`INSERT INTO party_commercial.publication_draft
			(tenant_id, object_kind, object_id, version_label,
			 scope_ref, effective_starts_at, effective_ends_at, declared_references,
			 content_canonicalization, content_digest, content_document,
			 submitter_ref, submitted_at, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 ON CONFLICT (tenant_id, object_kind, object_id, version_label) DO UPDATE
		    SET scope_ref = EXCLUDED.scope_ref,
		        effective_starts_at = EXCLUDED.effective_starts_at,
		        effective_ends_at = EXCLUDED.effective_ends_at,
		        declared_references = EXCLUDED.declared_references,
		        content_canonicalization = EXCLUDED.content_canonicalization,
		        content_digest = EXCLUDED.content_digest,
		        content_document = EXCLUDED.content_document,
		        submitter_ref = EXCLUDED.submitter_ref,
		        submitted_at = EXCLUDED.submitted_at
		  WHERE publication_draft.status = $14
		    AND (publication_draft.content_digest <> EXCLUDED.content_digest
		         OR publication_draft.scope_ref <> EXCLUDED.scope_ref
		         OR publication_draft.effective_starts_at <> EXCLUDED.effective_starts_at
		         OR publication_draft.effective_ends_at IS DISTINCT FROM EXCLUDED.effective_ends_at
		         OR publication_draft.declared_references <> EXCLUDED.declared_references)
		 RETURNING (xmax = 0) AS inserted`,
		draft.Tenant().String(),
		uint8(draft.Kind()),
		draft.ObjectID().String(),
		draft.Version().String(),
		draft.Scope().String(),
		startsAt,
		endsAt,
		references,
		draft.Canonical().Canonicalization(),
		draft.Canonical().Digest().String(),
		draft.Canonical().Document(),
		draft.Submitter().String(),
		draft.SubmittedAt().UTC(),
		uint8(domain.PublicationDraftPendingApproval),
	).Scan(&inserted)
	switch {
	case err == nil && inserted:
		return ports.PublicationDraftSaved, nil
	case err == nil:
		return ports.PublicationDraftRevised, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return ports.PublicationDraftSubmitOutcomeInvalid, fmt.Errorf("submit publication draft: %w", err)
	}

	// 零行命中：库上有这一版、且要么同一次录入要么已不在`待批准`。读回判哪一格。
	existing, found, err := repository.LoadDraft(ctx, draft.Tenant(), draft.Kind(), draft.ObjectID(), draft.Version())
	if err != nil {
		return ports.PublicationDraftSubmitOutcomeInvalid, fmt.Errorf("submit publication draft: %w", err)
	}
	if !found {
		return ports.PublicationDraftSubmitOutcomeInvalid, fmt.Errorf("submit publication draft: 撞键后读不回既有行")
	}
	if existing.SameSubmissionAs(draft) {
		return ports.PublicationDraftReplayed, nil
	}
	return ports.PublicationDraftContentFixed, nil
}

// LoadDraft 按版本身份点读。显式租户是键的一部分（ADR-0003/0040）；读回的行经 RehydratePublicationDraft 重建。
func (repository *PublicationDrafts) LoadDraft(
	ctx context.Context,
	tenant domain.TenantID,
	kind domain.CommercialObjectKind,
	objectID domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
) (domain.PublicationDraft, bool, error) {
	none := domain.PublicationDraft{}
	if tenant.String() == "" || objectID.String() == "" || version.String() == "" {
		return none, false, fmt.Errorf("load publication draft: tenant and version identity are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load publication draft: %w", err)
	}

	var row scannedDraft
	err = querier.QueryRow(ctx,
		`SELECT scope_ref, effective_starts_at, effective_ends_at, declared_references,
		        content_canonicalization, content_digest, content_document,
		        submitter_ref, submitted_at, status, approver_ref, approved_at, published_at
		   FROM party_commercial.publication_draft
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant.String(), uint8(kind), objectID.String(), version.String(),
	).Scan(&row.scope, &row.startsAt, &row.endsAt, &row.references,
		&row.canonicalization, &row.digest, &row.document,
		&row.submitter, &row.submittedAt, &row.status, &row.approver, &row.approvedAt, &row.publishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load publication draft: %w", err)
	}
	draft, err := row.draft(tenant, kind, objectID, version)
	if err != nil {
		return none, false, fmt.Errorf("load publication draft: %w", err)
	}
	return draft, true, nil
}

// AdvanceDraft 写回一份已在领域推进过状态的载体。UPDATE 带前态条件——`已批准`要求库上是`待批准`，`已发布`要求库上
// 是`已批准`——并要求行与这份载体所依据的那一次录入**逐列相同**：摘要、壳（范围、区间、指名引用）、录入者、录入时刻，
// 即 SubmitDraft 的修订会重写的每一列。只钉摘要不够：领域把换壳不换正文也定为修订（SameSubmissionAs），批准者读到与
// 写回之间录入者换范围重录，批准就会落在批准者没看过的壳上，自批门也是对旧录入者判的。零行命中时读回分「不在」与
// 「已被替换」。
func (repository *PublicationDrafts) AdvanceDraft(
	ctx context.Context,
	draft domain.PublicationDraft,
) (ports.PublicationDraftAdvanceOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PublicationDraftAdvanceOutcomeInvalid, fmt.Errorf("advance publication draft: %w", err)
	}
	var prior domain.PublicationDraftStatus
	switch draft.Status() {
	case domain.PublicationDraftApproved:
		prior = domain.PublicationDraftPendingApproval
	case domain.PublicationDraftPublished:
		prior = domain.PublicationDraftApproved
	default:
		return ports.PublicationDraftAdvanceOutcomeInvalid, fmt.Errorf("advance publication draft: %s is not an advanced state", draft.Status())
	}
	approver, _ := draft.Approver()
	approvedAt, _ := draft.ApprovedAt()
	var publishedAt *time.Time
	if at, published := draft.PublishedAt(); published {
		utc := at.UTC()
		publishedAt = &utc
	}
	references, err := json.Marshal(referenceDocumentsOf(draft.DeclaredReferences()))
	if err != nil {
		return ports.PublicationDraftAdvanceOutcomeInvalid, fmt.Errorf("advance publication draft: %w", err)
	}
	startsAt, endsAt := intervalColumns(draft.Effective())

	tag, err := executor.Exec(ctx,
		`UPDATE party_commercial.publication_draft
		    SET status = $5, approver_ref = $6, approved_at = $7, published_at = $8
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4
		    AND status = $9 AND content_digest = $10
		    AND submitter_ref = $11 AND submitted_at = $12
		    AND scope_ref = $13 AND effective_starts_at = $14
		    AND effective_ends_at IS NOT DISTINCT FROM $15
		    AND declared_references = $16::jsonb`,
		draft.Tenant().String(), uint8(draft.Kind()), draft.ObjectID().String(), draft.Version().String(),
		uint8(draft.Status()), approver.String(), approvedAt.UTC(), publishedAt,
		uint8(prior), draft.Canonical().Digest().String(),
		draft.Submitter().String(), draft.SubmittedAt().UTC(),
		draft.Scope().String(), startsAt, endsAt, references,
	)
	if err != nil {
		return ports.PublicationDraftAdvanceOutcomeInvalid, fmt.Errorf("advance publication draft: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.PublicationDraftAdvanced, nil
	}

	var exists bool
	err = executor.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM party_commercial.publication_draft
		     WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4)`,
		draft.Tenant().String(), uint8(draft.Kind()), draft.ObjectID().String(), draft.Version().String(),
	).Scan(&exists)
	if err != nil {
		return ports.PublicationDraftAdvanceOutcomeInvalid, fmt.Errorf("advance publication draft: %w", err)
	}
	if !exists {
		return ports.PublicationDraftAdvanceNotFound, nil
	}
	return ports.PublicationDraftAdvanceSuperseded, nil
}

type scannedDraft struct {
	scope            string
	startsAt         time.Time
	endsAt           *time.Time
	references       []byte
	canonicalization string
	digest           string
	document         []byte
	submitter        string
	submittedAt      time.Time
	status           uint8
	approver         *string
	approvedAt       *time.Time
	publishedAt      *time.Time
}

func (row scannedDraft) draft(
	tenant domain.TenantID,
	kind domain.CommercialObjectKind,
	objectID domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
) (domain.PublicationDraft, error) {
	scope, err := domain.NewCommercialScopeReference(row.scope)
	if err != nil {
		return domain.PublicationDraft{}, err
	}
	effective, err := intervalFrom(row.startsAt, row.endsAt)
	if err != nil {
		return domain.PublicationDraft{}, err
	}
	var references []referenceDocument
	if err := json.Unmarshal(row.references, &references); err != nil {
		return domain.PublicationDraft{}, fmt.Errorf("declared references: %w", err)
	}
	referenceMap, err := referenceMapOf(references)
	if err != nil {
		return domain.PublicationDraft{}, err
	}
	digest, err := domain.NewCommercialContentDigest(row.digest)
	if err != nil {
		return domain.PublicationDraft{}, err
	}
	submitter, err := domain.NewOperatorSubjectReference(row.submitter)
	if err != nil {
		return domain.PublicationDraft{}, err
	}
	spec := domain.RehydratePublicationDraftSpec{
		Shell: domain.PublicationDraftShell{
			TenantID:   tenant,
			Kind:       kind,
			ObjectID:   objectID,
			Version:    version,
			Scope:      scope,
			Effective:  effective,
			References: referenceMap,
		},
		Canonicalization: row.canonicalization,
		Document:         row.document,
		Digest:           digest,
		Submitter:        submitter,
		SubmittedAt:      row.submittedAt,
		Status:           domain.PublicationDraftStatus(row.status),
	}
	if row.approver != nil {
		if spec.Approver, err = domain.NewOperatorSubjectReference(*row.approver); err != nil {
			return domain.PublicationDraft{}, err
		}
	}
	if row.approvedAt != nil {
		spec.ApprovedAt = *row.approvedAt
	}
	if row.publishedAt != nil {
		spec.PublishedAt = *row.publishedAt
	}
	return domain.RehydratePublicationDraft(spec)
}

// referenceDocumentsOf 与 referenceMapOf 是壳上指名引用在 jsonb 列里的形状，键名与版本册快照里的 referenceDocument
// 同一份——同一样东西在两张表里不换形。
func referenceDocumentsOf(references []domain.DeclaredReference) []referenceDocument {
	documents := make([]referenceDocument, 0, len(references))
	for _, reference := range references {
		documents = append(documents, referenceDocument{Kind: uint8(reference.Kind()), ObjectID: reference.ObjectID().String()})
	}
	return documents
}

func referenceMapOf(documents []referenceDocument) (map[domain.CommercialObjectKind]domain.CommercialObjectID, error) {
	if len(documents) == 0 {
		return nil, nil
	}
	references := make(map[domain.CommercialObjectKind]domain.CommercialObjectID, len(documents))
	for _, document := range documents {
		objectID, err := domain.NewCommercialObjectID(document.ObjectID)
		if err != nil {
			return nil, fmt.Errorf("declared reference: %w", err)
		}
		references[domain.CommercialObjectKind(document.Kind)] = objectID
	}
	return references, nil
}

// ApprovalDutyRules 实现 ports.ApprovalDutyRuleRegistry：一租户一条审批职责规则（PAR-COM-18）。
//
// 写口撞键不覆盖（同内容重放、异内容冲突）；缺行即未登记，读口如实答 found=false——批准门据此答`未配置`。
type ApprovalDutyRules struct {
	db *bentopg.DB
}

func NewApprovalDutyRules(db *bentopg.DB) (*ApprovalDutyRules, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &ApprovalDutyRules{db: db}, nil
}

var _ ports.ApprovalDutyRuleRegistry = (*ApprovalDutyRules)(nil)

func (repository *ApprovalDutyRules) LoadApprovalDutyRule(
	ctx context.Context,
	tenant domain.TenantID,
) (domain.ApprovalDutyRule, bool, error) {
	none := domain.ApprovalDutyRule{}
	if tenant.String() == "" {
		return none, false, fmt.Errorf("load approval duty rule: tenant is required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load approval duty rule: %w", err)
	}
	var distinct bool
	var level *string
	err = querier.QueryRow(ctx,
		`SELECT requires_distinct_subjects, required_approver_level
		   FROM party_commercial.publication_approval_duty_rule
		  WHERE tenant_id = $1`,
		tenant.String(),
	).Scan(&distinct, &level)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load approval duty rule: %w", err)
	}
	rule, err := approvalDutyRuleFrom(tenant, distinct, level)
	if err != nil {
		return none, false, fmt.Errorf("load approval duty rule: %w", err)
	}
	return rule, true, nil
}

func (repository *ApprovalDutyRules) SaveApprovalDutyRule(
	ctx context.Context,
	rule domain.ApprovalDutyRule,
) (ports.ApprovalDutyRuleSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ApprovalDutyRuleSaveOutcomeInvalid, fmt.Errorf("save approval duty rule: %w", err)
	}
	if rule.Tenant().String() == "" {
		return ports.ApprovalDutyRuleSaveOutcomeInvalid, fmt.Errorf("save approval duty rule: %w", domain.ErrInvalidApprovalDutyRule)
	}
	var level *string
	if required, ok := rule.RequiredApproverLevel(); ok {
		value := required.String()
		level = &value
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.publication_approval_duty_rule
			(tenant_id, requires_distinct_subjects, required_approver_level)
		 VALUES ($1, $2, $3)
		 ON CONFLICT DO NOTHING`,
		rule.Tenant().String(), rule.RequiresDistinctSubjects(), level,
	)
	if err != nil {
		return ports.ApprovalDutyRuleSaveOutcomeInvalid, fmt.Errorf("save approval duty rule: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.ApprovalDutyRuleSaved, nil
	}

	var existingDistinct bool
	var existingLevel *string
	err = executor.QueryRow(ctx,
		`SELECT requires_distinct_subjects, required_approver_level
		   FROM party_commercial.publication_approval_duty_rule
		  WHERE tenant_id = $1`,
		rule.Tenant().String(),
	).Scan(&existingDistinct, &existingLevel)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ApprovalDutyRuleSaveOutcomeInvalid, fmt.Errorf("save approval duty rule: 撞键后读不回既有行")
	}
	if err != nil {
		return ports.ApprovalDutyRuleSaveOutcomeInvalid, fmt.Errorf("save approval duty rule: %w", err)
	}
	if existingDistinct == rule.RequiresDistinctSubjects() && sameOptionalString(existingLevel, level) {
		return ports.ApprovalDutyRuleAlreadyRegistered, nil
	}
	return ports.ApprovalDutyRuleContentConflict, nil
}

func approvalDutyRuleFrom(tenant domain.TenantID, distinct bool, level *string) (domain.ApprovalDutyRule, error) {
	var required domain.AuthorityLevel
	if level != nil {
		var err error
		if required, err = domain.NewAuthorityLevel(*level); err != nil {
			return domain.ApprovalDutyRule{}, err
		}
	}
	return domain.NewApprovalDutyRule(tenant, distinct, required)
}
