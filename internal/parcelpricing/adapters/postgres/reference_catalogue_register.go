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

// ReferenceCatalogueVersions 实现 ports.ReferenceCatalogueRegister 与 ports.ReferenceCatalogueVersionLoader
// （ADR-0109 Decision 二）。它拥有目录版本行的登记与按计价基准时点、邮编路线的解析，不生产任何映射——
// 列面只承担键、比对与包络检索，权威内容在领域折装的登记快照里，读回经领域整版重验（含摘要自校）。
type ReferenceCatalogueVersions struct {
	db *bentopg.DB
}

func NewReferenceCatalogueVersions(db *bentopg.DB) (*ReferenceCatalogueVersions, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel pricing postgres: db is nil")
	}
	return &ReferenceCatalogueVersions{db: db}, nil
}

// Register 登记一版目录。行只增不改：同键第二份由主键拦住，再按（规范化形状 + 内容摘要）比对译成结果
// 代数——同摘要是幂等重放，同形状不同摘要是内容冲突（映射更正必须走新版本），形状不同则摘要不可比。
func (register *ReferenceCatalogueVersions) Register(
	ctx context.Context,
	registration domain.ReferenceCatalogueRegistration,
) (ports.ReferenceCatalogueRegistrationOutcome, error) {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ReferenceCatalogueRegistrationOutcomeInvalid, fmt.Errorf("register reference catalogue: %w", err)
	}
	snapshot, err := domain.MarshalReferenceCatalogueRegistration(registration)
	if err != nil {
		return ports.ReferenceCatalogueRegistrationOutcomeInvalid, fmt.Errorf("register reference catalogue: %w", err)
	}

	var priorVersion, correctionBasis *string
	if prior, basis, corrected := registration.Correction(); corrected {
		version := prior.Version()
		priorVersion, correctionBasis = &version, &basis
	}
	var effectiveTo *time.Time
	if endsAt := registration.Period().EndsAt(); !endsAt.IsZero() {
		end := endsAt.UTC()
		effectiveTo = &end
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_pricing.reference_catalogue_version
			(tenant_id, catalogue_id, catalogue_version, kind, source_identifier, registrant,
			 origin_scope, prefix_length, entry_count, effective_from, effective_to,
			 prior_version, correction_basis, canonicalization, content_digest, snapshot)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 ON CONFLICT DO NOTHING`,
		registration.Tenant().String(),
		registration.Reference().ID(),
		registration.Reference().Version(),
		registration.Kind().String(),
		registration.SourceIdentifier(),
		registration.Registrant(),
		registration.Origin().Scope().String(),
		registration.PrefixLength(),
		len(registration.Entries()),
		registration.Period().StartsAt().UTC(),
		effectiveTo,
		priorVersion,
		correctionBasis,
		registration.Canonicalization(),
		registration.ContentDigest(),
		snapshot,
	)
	if err != nil {
		return ports.ReferenceCatalogueRegistrationOutcomeInvalid, fmt.Errorf("register reference catalogue: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.ReferenceCatalogueRegistered, nil
	}

	var canonicalization, digest string
	err = executor.QueryRow(ctx,
		`SELECT canonicalization, content_digest
		   FROM parcel_pricing.reference_catalogue_version
		  WHERE tenant_id = $1 AND catalogue_id = $2 AND catalogue_version = $3`,
		registration.Tenant().String(),
		registration.Reference().ID(),
		registration.Reference().Version(),
	).Scan(&canonicalization, &digest)
	if err != nil {
		return ports.ReferenceCatalogueRegistrationOutcomeInvalid, fmt.Errorf("register reference catalogue: 比对在册行：%w", err)
	}
	if canonicalization != registration.Canonicalization() {
		return ports.ReferenceCatalogueCanonicalizationDiffers, nil
	}
	if digest != registration.ContentDigest() {
		return ports.ReferenceCatalogueContentConflict, nil
	}
	return ports.ReferenceCatalogueAlreadyRegistered, nil
}

// ResolveAt 按（租户 + 目录版本引用 + 计价基准时点 + 邮编路线）解一个读数。版本不在册或时点不在生效
// 区间内答 false；查不到该邮编的读数值缺席而版本引用在——评价侧据以落待判断，不编默认。
func (register *ReferenceCatalogueVersions) ResolveAt(
	ctx context.Context,
	tenant domain.TenantID,
	reference domain.VersionReference,
	asOf time.Time,
	route domain.PostalRoute,
) (domain.ResolvedCatalogueValue, bool, error) {
	if asOf.IsZero() {
		return domain.ResolvedCatalogueValue{}, false, fmt.Errorf("resolve reference catalogue: asOf is required")
	}
	registration, found, err := register.LoadVersion(ctx, tenant, reference.ID(), reference.Version())
	if err != nil {
		return domain.ResolvedCatalogueValue{}, false, fmt.Errorf("resolve reference catalogue: %w", err)
	}
	if !found {
		return domain.ResolvedCatalogueValue{}, false, nil
	}
	reading, applicable := registration.ResolveAt(asOf, route)
	if !applicable {
		return domain.ResolvedCatalogueValue{}, false, nil
	}
	return reading, true, nil
}

// LoadVersion 实现 ports.ReferenceCatalogueVersionLoader：读回一版登记并整版重验，再与比对列交叉核——
// 列与快照分岔说明行被改过。ResolveAt 与复核用例都从这里取登记。
func (register *ReferenceCatalogueVersions) LoadVersion(
	ctx context.Context,
	tenant domain.TenantID,
	catalogueID string,
	catalogueVersion string,
) (domain.ReferenceCatalogueRegistration, bool, error) {
	if tenant.String() == "" || catalogueID == "" || catalogueVersion == "" {
		return domain.ReferenceCatalogueRegistration{}, false, fmt.Errorf(
			"load reference catalogue version: tenant, catalogue and version are required")
	}
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ReferenceCatalogueRegistration{}, false, fmt.Errorf("load reference catalogue version: %w", err)
	}

	var kind, canonicalization, digest string
	var entryCount int
	var snapshot []byte
	err = querier.QueryRow(ctx,
		`SELECT kind, entry_count, canonicalization, content_digest, snapshot
		   FROM parcel_pricing.reference_catalogue_version
		  WHERE tenant_id = $1 AND catalogue_id = $2 AND catalogue_version = $3`,
		tenant.String(), catalogueID, catalogueVersion,
	).Scan(&kind, &entryCount, &canonicalization, &digest, &snapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReferenceCatalogueRegistration{}, false, nil
	}
	if err != nil {
		return domain.ReferenceCatalogueRegistration{}, false, fmt.Errorf("load reference catalogue version: %w", err)
	}

	registration, err := domain.RehydrateReferenceCatalogueRegistration(snapshot)
	if err != nil {
		return domain.ReferenceCatalogueRegistration{}, false, fmt.Errorf(
			"load reference catalogue version: %s/%s：%w", catalogueID, catalogueVersion, err)
	}
	if registration.Tenant() != tenant ||
		registration.Reference().ID() != catalogueID ||
		registration.Reference().Version() != catalogueVersion ||
		registration.Kind().String() != kind ||
		len(registration.Entries()) != entryCount ||
		registration.Canonicalization() != canonicalization ||
		registration.ContentDigest() != digest {
		return domain.ReferenceCatalogueRegistration{}, false, fmt.Errorf(
			"load reference catalogue version: comparison columns disagree with the snapshot for %s/%s", catalogueID, catalogueVersion)
	}
	return registration, true, nil
}

var (
	_ ports.ReferenceCatalogueRegister      = (*ReferenceCatalogueVersions)(nil)
	_ ports.ReferenceCatalogueVersionLoader = (*ReferenceCatalogueVersions)(nil)
)
