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

// 证据等级列的两个取值：任何一期缺可复核凭证，整版只有断言强度（CONTEXT：缺凭证的
// 期次可用于隔离验证，不得支撑生产金额）。
const (
	seriesGradeVerifiable = "VERIFIABLE"
	seriesGradeAsserted   = "ASSERTED"
)

// ReferenceSeriesVersions 实现 ports.ReferenceSeriesRegister（票 08 件①②）。它拥有
// 序列版本行的登记与按计价基准时点的解析，不生产数值也不选口径——列面只承担键、比对
// 与包络检索，权威内容在领域折装的登记快照里，读回经领域整版重验（含摘要自校）。
type ReferenceSeriesVersions struct {
	db *bentopg.DB
}

func NewReferenceSeriesVersions(db *bentopg.DB) (*ReferenceSeriesVersions, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel pricing postgres: db is nil")
	}
	return &ReferenceSeriesVersions{db: db}, nil
}

// Register 登记一版序列取值。行只增不改：同键第二份由主键拦住，再按（规范化形状 +
// 内容摘要）比对译成结果代数——同摘要是幂等重放，同形状不同摘要是内容冲突（取值更正
// 必须走新版本），形状不同则摘要不可比，三种都不顶替原行。
//
// 走 RequireExecutor：登记与将来同一步的治理记录必须同生共死。
func (register *ReferenceSeriesVersions) Register(
	ctx context.Context,
	registration domain.ReferenceSeriesRegistration,
) (ports.ReferenceSeriesRegistrationOutcome, error) {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ReferenceSeriesRegistrationOutcomeInvalid, fmt.Errorf("register reference series: %w", err)
	}

	snapshot, err := domain.MarshalReferenceSeriesRegistration(registration)
	if err != nil {
		return ports.ReferenceSeriesRegistrationOutcomeInvalid, fmt.Errorf("register reference series: %w", err)
	}

	grade := seriesGradeAsserted
	if registration.Verifiable() {
		grade = seriesGradeVerifiable
	}
	var quoteBasisID, quoteBasisVersion *string
	if basis, declared := registration.QuoteBasis(); declared {
		id, version := basis.ID(), basis.Version()
		quoteBasisID, quoteBasisVersion = &id, &version
	}
	var priorVersion, correctionBasis *string
	if prior, basis, corrected := registration.Correction(); corrected {
		version := prior.Version()
		priorVersion, correctionBasis = &version, &basis
	}
	var effectiveTo *time.Time
	if endsAt, bounded := registration.EffectiveTo(); bounded {
		end := endsAt.UTC()
		effectiveTo = &end
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_pricing.reference_series_version
			(tenant_id, series_id, series_version, kind, source_identifier, registrant,
			 quote_basis_id, quote_basis_version, effective_from, effective_to,
			 evidence_grade, prior_version, correction_basis,
			 canonicalization, content_digest, snapshot)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 ON CONFLICT DO NOTHING`,
		registration.Tenant().String(),
		registration.Reference().ID(),
		registration.Reference().Version(),
		registration.Kind().String(),
		registration.SourceIdentifier(),
		registration.Registrant(),
		quoteBasisID,
		quoteBasisVersion,
		registration.EffectiveFrom().UTC(),
		effectiveTo,
		grade,
		priorVersion,
		correctionBasis,
		registration.Canonicalization(),
		registration.ContentDigest(),
		snapshot,
	)
	if err != nil {
		return ports.ReferenceSeriesRegistrationOutcomeInvalid, fmt.Errorf("register reference series: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.ReferenceSeriesRegistered, nil
	}

	var canonicalization, digest string
	err = executor.QueryRow(ctx,
		`SELECT canonicalization, content_digest
		   FROM parcel_pricing.reference_series_version
		  WHERE tenant_id = $1 AND series_id = $2 AND series_version = $3`,
		registration.Tenant().String(),
		registration.Reference().ID(),
		registration.Reference().Version(),
	).Scan(&canonicalization, &digest)
	if err != nil {
		return ports.ReferenceSeriesRegistrationOutcomeInvalid, fmt.Errorf("register reference series: 比对在册行：%w", err)
	}
	if canonicalization != registration.Canonicalization() {
		return ports.ReferenceSeriesCanonicalizationDiffers, nil
	}
	if digest != registration.ContentDigest() {
		return ports.ReferenceSeriesContentConflict, nil
	}
	return ports.ReferenceSeriesAlreadyRegistered, nil
}

// ResolveAt 按（租户 + 序列版本引用 + 计价基准时点）解析一期取值。序列版本不在册或
// 时点落在期次缺口都答未解析——评价侧据以挂起，不编数值。读回经领域整版重验（含摘要
// 自校），再与比对列交叉核——列与快照分岔说明行被改过。
func (register *ReferenceSeriesVersions) ResolveAt(
	ctx context.Context,
	tenant domain.TenantID,
	reference domain.VersionReference,
	asOf time.Time,
) (domain.ResolvedSeriesReading, bool, error) {
	if asOf.IsZero() {
		return domain.ResolvedSeriesReading{}, false, fmt.Errorf(
			"resolve reference series: tenant, reference and asOf are required")
	}
	registration, found, err := register.LoadVersion(ctx, tenant, reference.ID(), reference.Version())
	if err != nil {
		return domain.ResolvedSeriesReading{}, false, fmt.Errorf("resolve reference series: %w", err)
	}
	if !found {
		return domain.ResolvedSeriesReading{}, false, nil
	}
	reading, found := registration.ResolveAt(asOf)
	if !found {
		return domain.ResolvedSeriesReading{}, false, nil
	}
	return reading, true, nil
}

// LoadVersion 实现 ports.ReferenceSeriesVersionLoader：读回一版登记并整版重验，再与比对
// 列交叉核。ResolveAt 与复核用例都从这里取登记——一处读回门。
func (register *ReferenceSeriesVersions) LoadVersion(
	ctx context.Context,
	tenant domain.TenantID,
	seriesID string,
	seriesVersion string,
) (domain.ReferenceSeriesRegistration, bool, error) {
	if tenant.String() == "" || seriesID == "" || seriesVersion == "" {
		return domain.ReferenceSeriesRegistration{}, false, fmt.Errorf(
			"load reference series version: tenant, series and version are required")
	}
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ReferenceSeriesRegistration{}, false, fmt.Errorf("load reference series version: %w", err)
	}

	var kind, grade, canonicalization, digest string
	var snapshot []byte
	err = querier.QueryRow(ctx,
		`SELECT kind, evidence_grade, canonicalization, content_digest, snapshot
		   FROM parcel_pricing.reference_series_version
		  WHERE tenant_id = $1 AND series_id = $2 AND series_version = $3`,
		tenant.String(), seriesID, seriesVersion,
	).Scan(&kind, &grade, &canonicalization, &digest, &snapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReferenceSeriesRegistration{}, false, nil
	}
	if err != nil {
		return domain.ReferenceSeriesRegistration{}, false, fmt.Errorf("load reference series version: %w", err)
	}

	registration, err := domain.RehydrateReferenceSeriesRegistration(snapshot)
	if err != nil {
		return domain.ReferenceSeriesRegistration{}, false, fmt.Errorf(
			"load reference series version: %s/%s：%w", seriesID, seriesVersion, err)
	}
	expectedGrade := seriesGradeAsserted
	if registration.Verifiable() {
		expectedGrade = seriesGradeVerifiable
	}
	if registration.Tenant() != tenant ||
		registration.Reference().ID() != seriesID ||
		registration.Reference().Version() != seriesVersion ||
		registration.Kind().String() != kind ||
		expectedGrade != grade ||
		registration.Canonicalization() != canonicalization ||
		registration.ContentDigest() != digest {
		return domain.ReferenceSeriesRegistration{}, false, fmt.Errorf(
			"load reference series version: comparison columns disagree with the snapshot for %s/%s",
			seriesID, seriesVersion)
	}
	return registration, true, nil
}

var _ ports.ReferenceSeriesVersionLoader = (*ReferenceSeriesVersions)(nil)
