package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// SourceConnectorBindings 实现 ports.SourceConnectorBindingRegister 与
// ports.SourceConnectorBindingLoader（迁移 0005；ADR-0099 决定六）。绑定行只增不改；「当前绑定」
// 不是列，是按登记时刻派生的结论——本适配器取最近登记的一版，选择规则写在 SQL 的排序里因为它
// 只有这一句、没有第二处口径要对齐。
type SourceConnectorBindings struct {
	db *bentopg.DB
}

func NewSourceConnectorBindings(db *bentopg.DB) (*SourceConnectorBindings, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel pricing postgres: db is nil")
	}
	return &SourceConnectorBindings{db: db}, nil
}

const sourceConnectorBindingColumns = `tenant_id, series_id, binding_version, connector_kind, source_identifier,
	source_locator, series_kind, quote_basis_id, quote_basis_version, quote_basis_digest, registrant, cadence,
	review_exemption`

// Register 登记一版绑定。同键第二份由主键拦住，再按声明（Spec）比对译成结果代数：同即幂等重放，
// 异即冲突，原行不顶替。走 RequireExecutor：绑定登记是治理写入，与同一步的其他治理记录同生共死。
func (register *SourceConnectorBindings) Register(
	ctx context.Context,
	binding domain.SourceConnectorBinding,
) (ports.SourceConnectorBindingOutcome, error) {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SourceConnectorBindingOutcomeInvalid, fmt.Errorf("register source connector binding: %w", err)
	}
	if binding.Tenant().String() == "" || binding.SeriesID() == "" {
		return ports.SourceConnectorBindingOutcomeInvalid, fmt.Errorf("register source connector binding: binding is not constructed")
	}

	var quoteBasisID, quoteBasisVersion, quoteBasisDigest *string
	if basis, declared := binding.QuoteBasis(); declared {
		id, version, digest := basis.ID(), basis.Version(), basis.Digest()
		quoteBasisID, quoteBasisVersion, quoteBasisDigest = &id, &version, &digest
	}
	var cadence *string
	if declared, scheduled := binding.Cadence(); scheduled {
		cadence = &declared
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_pricing.source_connector_binding (`+sourceConnectorBindingColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT DO NOTHING`,
		binding.Tenant().String(),
		binding.SeriesID(),
		binding.Version(),
		binding.ConnectorKind(),
		binding.SourceIdentifier(),
		binding.SourceLocator(),
		binding.SeriesKind().String(),
		quoteBasisID,
		quoteBasisVersion,
		quoteBasisDigest,
		binding.Registrant(),
		cadence,
		binding.ReviewExemption().String(),
	)
	if err != nil {
		return ports.SourceConnectorBindingOutcomeInvalid, fmt.Errorf("register source connector binding: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.SourceConnectorBindingRegistered, nil
	}

	existing, err := scanSourceConnectorBinding(executor.QueryRow(ctx,
		`SELECT `+sourceConnectorBindingColumns+`
		   FROM parcel_pricing.source_connector_binding
		  WHERE tenant_id = $1 AND series_id = $2 AND binding_version = $3`,
		binding.Tenant().String(), binding.SeriesID(), binding.Version()))
	if err != nil {
		return ports.SourceConnectorBindingOutcomeInvalid, fmt.Errorf("register source connector binding: 比对在册行：%w", err)
	}
	if existing.Spec() != binding.Spec() {
		return ports.SourceConnectorBindingConflict, nil
	}
	return ports.SourceConnectorBindingAlreadyRegistered, nil
}

// LoadCurrentBinding 取（租户、序列）最近登记的一版：按登记时刻、同刻按版本号字典序，判定确定。
// 读回经领域构造门——列上溜进的坏声明在这里暴露，不会变成一个看起来合法的绑定。
func (register *SourceConnectorBindings) LoadCurrentBinding(
	ctx context.Context,
	tenant domain.TenantID,
	seriesID string,
) (domain.SourceConnectorBinding, bool, error) {
	if tenant.String() == "" || seriesID == "" {
		return domain.SourceConnectorBinding{}, false, fmt.Errorf("load source connector binding: tenant and series are required")
	}
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SourceConnectorBinding{}, false, fmt.Errorf("load source connector binding: %w", err)
	}
	binding, err := scanSourceConnectorBinding(querier.QueryRow(ctx,
		`SELECT `+sourceConnectorBindingColumns+`
		   FROM parcel_pricing.source_connector_binding
		  WHERE tenant_id = $1 AND series_id = $2
		  ORDER BY registered_at DESC, binding_version DESC
		  LIMIT 1`,
		tenant.String(), seriesID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SourceConnectorBinding{}, false, nil
	}
	if err != nil {
		return domain.SourceConnectorBinding{}, false, fmt.Errorf("load source connector binding: %w", err)
	}
	return binding, true, nil
}

// scanSourceConnectorBinding 是本包唯一的绑定读回门：列 → 声明 → 领域构造门。
func scanSourceConnectorBinding(row pgx.Row) (domain.SourceConnectorBinding, error) {
	var (
		tenant, seriesID, version, connectorKind, sourceIdentifier, sourceLocator, seriesKind, registrant, exemption string
		quoteBasisID, quoteBasisVersion, quoteBasisDigest, cadence                                                   *string
	)
	if err := row.Scan(&tenant, &seriesID, &version, &connectorKind, &sourceIdentifier, &sourceLocator, &seriesKind,
		&quoteBasisID, &quoteBasisVersion, &quoteBasisDigest, &registrant, &cadence, &exemption); err != nil {
		return domain.SourceConnectorBinding{}, err
	}
	tenantID, err := domain.NewTenantID(tenant)
	if err != nil {
		return domain.SourceConnectorBinding{}, fmt.Errorf("%s/%s：%w", seriesID, version, err)
	}
	spec := domain.SourceConnectorBindingSpec{
		Tenant:           tenantID,
		SeriesID:         seriesID,
		Version:          version,
		ConnectorKind:    connectorKind,
		SourceIdentifier: sourceIdentifier,
		SourceLocator:    sourceLocator,
		SeriesKind:       domain.ReferenceSeriesKind(seriesKind),
		Registrant:       registrant,
		ReviewExemption:  domain.ReviewExemption(exemption),
	}
	if quoteBasisID != nil && quoteBasisVersion != nil && quoteBasisDigest != nil {
		basis, err := domain.NewVersionReference(domain.ArtifactCommercialPolicy, *quoteBasisID, *quoteBasisVersion, *quoteBasisDigest)
		if err != nil {
			return domain.SourceConnectorBinding{}, fmt.Errorf("%s/%s quote basis：%w", seriesID, version, err)
		}
		spec.QuoteBasis = basis
	}
	if cadence != nil {
		spec.Cadence = *cadence
	}
	binding, err := domain.NewSourceConnectorBinding(spec)
	if err != nil {
		return domain.SourceConnectorBinding{}, fmt.Errorf("%s/%s：%w", seriesID, version, err)
	}
	return binding, nil
}

var (
	_ ports.SourceConnectorBindingRegister = (*SourceConnectorBindings)(nil)
	_ ports.SourceConnectorBindingLoader   = (*SourceConnectorBindings)(nil)
)
