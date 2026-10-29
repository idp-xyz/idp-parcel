package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// ReferenceEnablements 实现 ports.ReferenceEnablementRegistry：启用范围登记册的只增登记、最新修订装载与按种类整册读出
// （0043 迁移）。撞键不覆盖：同内容重放，异内容冲突（ADR-0031）。内容指状态、生效时点与来源单据；登记责任方与登记时间
// 是审计，不进比对。
type ReferenceEnablements struct {
	db *bentopg.DB
}

func NewReferenceEnablements(db *bentopg.DB) (*ReferenceEnablements, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &ReferenceEnablements{db: db}, nil
}

var _ ports.ReferenceEnablementRegistry = (*ReferenceEnablements)(nil)

const referenceEnablementColumns = `tenant_id, kind, code, revision, status, effective_from, source_ref, registered_by, recorded_at`

func (repository *ReferenceEnablements) SaveReferenceEnablement(
	ctx context.Context,
	registration domain.ReferenceEnablementRegistration,
) (ports.ReferenceEnablementSaveOutcome, error) {
	const operation = "save reference enablement"
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ReferenceEnablementSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.reference_enablement_registration
			(`+referenceEnablementColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT DO NOTHING`,
		registration.Tenant().String(), registration.Kind().String(), registration.Code().String(), registration.Revision(),
		registration.Status().String(), registration.EffectiveFrom(), registration.Source().String(),
		registration.Registrant().String(), registration.RecordedAt(),
	)
	if err != nil {
		return ports.ReferenceEnablementSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return ports.ReferenceEnablementSaved, nil
	}

	existing, err := scanReferenceEnablement(executor.QueryRow(ctx,
		`SELECT `+referenceEnablementColumns+`
		   FROM party_commercial.reference_enablement_registration
		  WHERE tenant_id = $1 AND kind = $2 AND code = $3 AND revision = $4`,
		registration.Tenant().String(), registration.Kind().String(), registration.Code().String(), registration.Revision(),
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ReferenceEnablementSaveOutcomeInvalid, fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return ports.ReferenceEnablementSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if existing.SameContent(registration) {
		return ports.ReferenceEnablementAlreadyRegistered, nil
	}
	return ports.ReferenceEnablementContentConflict, nil
}

func (repository *ReferenceEnablements) LoadLatestReferenceEnablement(
	ctx context.Context,
	tenant domain.TenantID,
	kind domain.ReferenceKind,
	code domain.ReferenceCode,
) (domain.ReferenceEnablementRegistration, bool, error) {
	const operation = "load latest reference enablement"
	if tenant.String() == "" || kind.String() == "" || code.String() == "" {
		return domain.ReferenceEnablementRegistration{}, false,
			fmt.Errorf("%s: tenant, kind and code are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ReferenceEnablementRegistration{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	registration, err := scanReferenceEnablement(querier.QueryRow(ctx,
		`SELECT `+referenceEnablementColumns+`
		   FROM party_commercial.reference_enablement_registration
		  WHERE tenant_id = $1 AND kind = $2 AND code = $3
		  ORDER BY revision DESC
		  LIMIT 1`,
		tenant.String(), kind.String(), code.String(),
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReferenceEnablementRegistration{}, false, nil
	}
	if err != nil {
		return domain.ReferenceEnablementRegistration{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return registration, true, nil
}

func (repository *ReferenceEnablements) ListReferenceEnablements(
	ctx context.Context,
	tenant domain.TenantID,
	kind domain.ReferenceKind,
) ([]domain.ReferenceEnablementRegistration, error) {
	const operation = "list reference enablements"
	if tenant.String() == "" || kind.String() == "" {
		return nil, fmt.Errorf("%s: tenant and kind are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+referenceEnablementColumns+`
		   FROM party_commercial.reference_enablement_registration
		  WHERE tenant_id = $1 AND kind = $2
		  ORDER BY code, revision`,
		tenant.String(), kind.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()

	var registrations []domain.ReferenceEnablementRegistration
	for rows.Next() {
		registration, err := scanReferenceEnablement(rows)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		registrations = append(registrations, registration)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return registrations, nil
}

// scanReferenceEnablement 经重建门读回一行：形状与不变量照核，目录成员不核（目录随发版变，册上的老行要读得回）。
// 任何一格过不了门即坏数据，上抛不吞。
func scanReferenceEnablement(row pgx.Row) (domain.ReferenceEnablementRegistration, error) {
	var (
		tenantID, kindName, codeValue, statusName, source, registeredBy string
		revision                                                        int
		effectiveFrom, recordedAt                                       time.Time
	)
	if err := row.Scan(&tenantID, &kindName, &codeValue, &revision, &statusName,
		&effectiveFrom, &source, &registeredBy, &recordedAt); err != nil {
		return domain.ReferenceEnablementRegistration{}, err
	}
	tenant, err := domain.NewTenantID(tenantID)
	if err != nil {
		return domain.ReferenceEnablementRegistration{}, err
	}
	kind, known := domain.ReferenceKindNamed(kindName)
	if !known {
		return domain.ReferenceEnablementRegistration{}, fmt.Errorf("册面里的种类 %q 不在封闭集内", kindName)
	}
	code, err := domain.NewReferenceCode(codeValue)
	if err != nil {
		return domain.ReferenceEnablementRegistration{}, err
	}
	status, known := domain.ReferenceEnablementStatusNamed(statusName)
	if !known {
		return domain.ReferenceEnablementRegistration{}, fmt.Errorf("册面里的状态 %q 不在封闭集内", statusName)
	}
	sourceReference, err := domain.NewReferenceEnablementSource(source)
	if err != nil {
		return domain.ReferenceEnablementRegistration{}, err
	}
	registrant, err := domain.NewOperatorSubjectReference(registeredBy)
	if err != nil {
		return domain.ReferenceEnablementRegistration{}, err
	}
	return domain.RehydrateReferenceEnablementRegistration(tenant, kind, code, revision,
		domain.ReferenceEnablementSpec{Status: status, EffectiveFrom: effectiveFrom, Source: sourceReference},
		registrant, recordedAt)
}
