package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// EvidenceItems 实现 ports.EvidenceStore：证据项与披露版本两张表（0024）。
//
// 证据项读回经 RehydrateEvidenceItem 重验（评价三值、依据与评价成对）；披露版本读回
// 先取证据项再经 PrepareDisclosure 重过构造门（脱敏指纹不得与原件相同）——两张表都有
// CHECK 守着，Go 侧再验一次，一次坏写入不得变成看起来合法的对象。
type EvidenceItems struct {
	db *bentopg.DB
}

func NewEvidenceItems(db *bentopg.DB) (*EvidenceItems, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &EvidenceItems{db: db}, nil
}

var _ ports.EvidenceStore = (*EvidenceItems)(nil)

const evidenceItemColumns = `evidence_id, provider_ref, content_digest, submitted_at, appraisal, appraisal_basis`

func (repository *EvidenceItems) FindByID(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.EvidenceItemID,
) (domain.EvidenceItem, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.EvidenceItem{}, false, fmt.Errorf("find evidence item: %w", err)
	}
	row := querier.QueryRow(ctx,
		`SELECT `+evidenceItemColumns+`
		   FROM visibility_exception.evidence_item
		  WHERE tenant_id = $1 AND evidence_id = $2`,
		tenant.String(), id.String(),
	)
	return scanEvidenceItem(row)
}

func (repository *EvidenceItems) FindByProviderDigest(
	ctx context.Context,
	tenant domain.TenantID,
	provider domain.EvidenceProviderReference,
	digest domain.EvidenceContentDigest,
) (domain.EvidenceItem, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.EvidenceItem{}, false, fmt.Errorf("find evidence item: %w", err)
	}
	row := querier.QueryRow(ctx,
		`SELECT `+evidenceItemColumns+`
		   FROM visibility_exception.evidence_item
		  WHERE tenant_id = $1 AND provider_ref = $2 AND content_digest = $3`,
		tenant.String(), provider.String(), digest.String(),
	)
	return scanEvidenceItem(row)
}

// Save 落一项证据。撞（提供方+指纹）唯一约束交回 AlreadyRecorded（ON CONFLICT DO NOTHING，
// 事务保持可用），编排读回赢家；证据项标识是新键新行，撞主键同样归入 DO NOTHING——两个
// 约束任一命中都表示「这一行不是本轮写的」，编排都按已有处理。没有 UPDATE 路径：评价的
// 推进（Appraise）今天没有编排，等它有入口时再开专门的回填口，不在这里预留。
func (repository *EvidenceItems) Save(
	ctx context.Context,
	tenant domain.TenantID,
	item domain.EvidenceItem,
) (ports.EvidenceSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.EvidenceSaveOutcomeInvalid, fmt.Errorf("save evidence item: %w", err)
	}
	snapshot := item.Snapshot()
	if snapshot.ID.String() == "" {
		return ports.EvidenceSaveOutcomeInvalid, fmt.Errorf("save evidence item: item is zero")
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.evidence_item
			(tenant_id, `+evidenceItemColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT DO NOTHING`,
		tenant.String(),
		snapshot.ID.String(),
		snapshot.Provider.String(),
		snapshot.Digest.String(),
		snapshot.SubmittedAt,
		snapshot.Appraisal.String(),
		nullIfBlank(snapshot.AppraisalBasis),
	)
	if err != nil {
		return ports.EvidenceSaveOutcomeInvalid, fmt.Errorf("save evidence item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.EvidenceAlreadyRecorded, nil
	}
	return ports.EvidenceSaved, nil
}

func (repository *EvidenceItems) FindDisclosure(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.EvidenceItemID,
	redacted domain.EvidenceContentDigest,
) (domain.EvidenceDisclosureVersion, bool, error) {
	item, found, err := repository.FindByID(ctx, tenant, id)
	if err != nil {
		return domain.EvidenceDisclosureVersion{}, false, fmt.Errorf("find evidence disclosure: %w", err)
	}
	if !found {
		return domain.EvidenceDisclosureVersion{}, false, nil
	}

	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.EvidenceDisclosureVersion{}, false, fmt.Errorf("find evidence disclosure: %w", err)
	}
	var (
		originalRaw, scope string
		preparedAt         time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT original_digest, scope, prepared_at
		   FROM visibility_exception.evidence_disclosure_version
		  WHERE tenant_id = $1 AND evidence_id = $2 AND redacted_digest = $3`,
		tenant.String(), id.String(), redacted.String(),
	).Scan(&originalRaw, &scope, &preparedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.EvidenceDisclosureVersion{}, false, nil
	}
	if err != nil {
		return domain.EvidenceDisclosureVersion{}, false, fmt.Errorf("find evidence disclosure: %w", err)
	}
	// 行上的原件指纹必须就是证据项当前的指纹：对不上说明证据项被人改过指纹或行错挂了
	// 证据项，两种都不是一份能对回原件的披露版本。
	if originalRaw != item.Digest().String() {
		return domain.EvidenceDisclosureVersion{}, false, fmt.Errorf(
			"rebuild evidence disclosure: 版本 %s/%s 锚定的原件指纹与证据项不一致", id, redacted)
	}
	version, err := domain.PrepareDisclosure(item, redacted, scope, preparedAt)
	if err != nil {
		return domain.EvidenceDisclosureVersion{}, false, fmt.Errorf("rebuild evidence disclosure: %w", err)
	}
	return version, true, nil
}

// SaveDisclosure 落一个披露版本。撞（证据项+脱敏指纹）主键交回 AlreadyRecorded；版本只增
// 不改。外键要求证据项在场：给一个不存在的证据项造披露版本，正是「来源不明的附件」。
func (repository *EvidenceItems) SaveDisclosure(
	ctx context.Context,
	tenant domain.TenantID,
	version domain.EvidenceDisclosureVersion,
) (ports.EvidenceSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.EvidenceSaveOutcomeInvalid, fmt.Errorf("save evidence disclosure: %w", err)
	}
	if version.Item().String() == "" {
		return ports.EvidenceSaveOutcomeInvalid, fmt.Errorf("save evidence disclosure: version is zero")
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.evidence_disclosure_version
			(tenant_id, evidence_id, redacted_digest, original_digest, scope, prepared_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING`,
		tenant.String(),
		version.Item().String(),
		version.Redacted().String(),
		version.Original().String(),
		version.Scope(),
		version.PreparedAt(),
	)
	if err != nil {
		return ports.EvidenceSaveOutcomeInvalid, fmt.Errorf("save evidence disclosure: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.EvidenceAlreadyRecorded, nil
	}
	return ports.EvidenceSaved, nil
}

func scanEvidenceItem(row pgx.Row) (domain.EvidenceItem, bool, error) {
	var (
		idRaw, providerRaw, digestRaw, appraisalRaw string
		basisRaw                                    *string
		submittedAt                                 time.Time
	)
	err := row.Scan(&idRaw, &providerRaw, &digestRaw, &submittedAt, &appraisalRaw, &basisRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.EvidenceItem{}, false, nil
	}
	if err != nil {
		return domain.EvidenceItem{}, false, fmt.Errorf("find evidence item: %w", err)
	}

	snapshot := domain.EvidenceItemSnapshot{SubmittedAt: submittedAt}
	if snapshot.ID, err = domain.NewEvidenceItemID(idRaw); err != nil {
		return domain.EvidenceItem{}, false, fmt.Errorf("rebuild evidence item: %w", err)
	}
	if snapshot.Provider, err = domain.NewEvidenceProviderReference(providerRaw); err != nil {
		return domain.EvidenceItem{}, false, fmt.Errorf("rebuild evidence item: %w", err)
	}
	if snapshot.Digest, err = domain.NewEvidenceContentDigest(digestRaw); err != nil {
		return domain.EvidenceItem{}, false, fmt.Errorf("rebuild evidence item: %w", err)
	}
	if snapshot.Appraisal, err = evidenceAppraisalFrom(appraisalRaw); err != nil {
		return domain.EvidenceItem{}, false, fmt.Errorf("rebuild evidence item: %w", err)
	}
	if basisRaw != nil {
		snapshot.AppraisalBasis = *basisRaw
	}
	item, err := domain.RehydrateEvidenceItem(snapshot)
	if err != nil {
		return domain.EvidenceItem{}, false, fmt.Errorf("rebuild evidence item: %w", err)
	}
	return item, true, nil
}

// evidenceAppraisalFrom 逐格译回封闭三值；库上有 CHECK，这里的兜底挡的是 CHECK 被后续
// 迁移放宽而 Go 侧没跟上。
func evidenceAppraisalFrom(raw string) (domain.EvidenceAppraisal, error) {
	for _, candidate := range []domain.EvidenceAppraisal{
		domain.EvidenceReceived,
		domain.EvidenceCredited,
		domain.EvidenceDiscredited,
	} {
		if candidate.String() == raw {
			return candidate, nil
		}
	}
	return domain.EvidenceAppraisalInvalid, fmt.Errorf("未知证据评价 %q", raw)
}
