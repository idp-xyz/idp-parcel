package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// SigningEvidences 实现签署证据登记册：证据只追加，要求规则一户一行。
// 目标必须已经在商业版本或待批准载体上，发布前后都能登；都不在就答目标不存在，一行不写。
type SigningEvidences struct {
	db *bentopg.DB
}

func NewSigningEvidences(db *bentopg.DB) (*SigningEvidences, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &SigningEvidences{db: db}, nil
}

var _ ports.SigningEvidenceRegistry = (*SigningEvidences)(nil)

const pgUniqueViolation = "23505"

func (repository *SigningEvidences) SaveSigningEvidence(
	ctx context.Context,
	evidence domain.SigningEvidence,
) (ports.SigningEvidenceSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SigningEvidenceSaveOutcomeInvalid, fmt.Errorf("save signing evidence: %w", err)
	}
	if evidence.ID().String() == "" {
		return ports.SigningEvidenceSaveOutcomeInvalid, fmt.Errorf("save signing evidence: %w", domain.ErrInvalidSigningEvidence)
	}
	present, err := signingEvidenceTargetExists(ctx, executor, evidence)
	if err != nil {
		return ports.SigningEvidenceSaveOutcomeInvalid, err
	}
	if !present {
		return ports.SigningEvidenceTargetNotFound, nil
	}
	if _, corrects := evidence.Corrects(); corrects {
		history, loadErr := loadSigningEvidenceChain(ctx, executor, evidence)
		if loadErr != nil {
			return ports.SigningEvidenceSaveOutcomeInvalid, loadErr
		}
		if chainErr := evidence.AcceptCorrection(history); chainErr != nil {
			switch {
			case errors.Is(chainErr, domain.ErrSigningEvidenceAlreadyCorrected):
				return ports.SigningEvidenceAlreadyCorrected, nil
			case errors.Is(chainErr, domain.ErrSigningEvidenceCrossVersion):
				return ports.SigningEvidenceCrossVersion, nil
			case errors.Is(chainErr, domain.ErrSigningEvidenceCorrectsUnknown):
				return ports.SigningEvidenceCorrectsNotFound, nil
			default:
				return ports.SigningEvidenceSaveOutcomeInvalid, fmt.Errorf("save signing evidence: %w", chainErr)
			}
		}
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.signing_evidence (
		    tenant_id, evidence_id, object_kind, object_id, version_label, signed_on,
		    counterparty_signatory, our_signatory, file_name, file_sha256, library_index,
		    source, corrects_evidence_id, recorded_by, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 ON CONFLICT ON CONSTRAINT signing_evidence_pkey DO NOTHING`,
		evidence.Tenant().String(),
		evidence.ID().String(),
		int16(evidence.Kind()),
		evidence.ObjectID().String(),
		evidence.Version().String(),
		evidence.SignedOn(),
		nullIfEmpty(evidence.CounterpartySignatory()),
		nullIfEmpty(evidence.OurSignatory()),
		evidence.File().Name(),
		evidence.File().SHA256(),
		evidence.File().LibraryIndex(),
		evidence.Source().String(),
		correctsArgument(evidence),
		evidence.RecordedBy().String(),
		evidence.RecordedAt(),
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == "signing_evidence_corrects_once" {
			return ports.SigningEvidenceAlreadyCorrected, nil
		}
		return ports.SigningEvidenceSaveOutcomeInvalid, fmt.Errorf("save signing evidence: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.SigningEvidenceSaved, nil
	}
	stored, found, err := loadSigningEvidence(ctx, executor, evidence.Tenant(), evidence.ID())
	if err != nil {
		return ports.SigningEvidenceSaveOutcomeInvalid, err
	}
	if !found {
		return ports.SigningEvidenceSaveOutcomeInvalid, fmt.Errorf("save signing evidence: 撞键后读不回既有行")
	}
	if stored.SameContent(evidence) {
		return ports.SigningEvidenceAlreadyRegistered, nil
	}
	return ports.SigningEvidenceContentConflict, nil
}

func (repository *SigningEvidences) SaveSigningEvidenceRequirement(
	ctx context.Context,
	rule domain.SigningEvidenceRequirement,
) (ports.SigningEvidenceRequirementSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SigningEvidenceRequirementSaveOutcomeInvalid, fmt.Errorf("save signing evidence requirement: %w", err)
	}
	if rule.Tenant().String() == "" {
		return ports.SigningEvidenceRequirementSaveOutcomeInvalid, fmt.Errorf("save signing evidence requirement: %w", domain.ErrInvalidSigningEvidenceRequirement)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.signing_evidence_requirement
		    (tenant_id, requires_customer_contract, requires_supplier_agreement)
		 VALUES ($1, $2, $3)
		 ON CONFLICT DO NOTHING`,
		rule.Tenant().String(), rule.RequiresCustomerContract(), rule.RequiresSupplierAgreement(),
	)
	if err != nil {
		return ports.SigningEvidenceRequirementSaveOutcomeInvalid, fmt.Errorf("save signing evidence requirement: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.SigningEvidenceRequirementSaved, nil
	}
	stored, found, err := scanSigningEvidenceRequirement(ctx, executor, rule.Tenant())
	if err != nil {
		return ports.SigningEvidenceRequirementSaveOutcomeInvalid, err
	}
	if !found {
		return ports.SigningEvidenceRequirementSaveOutcomeInvalid, fmt.Errorf("save signing evidence requirement: 撞键后读不回既有行")
	}
	if stored.SameContent(rule) {
		return ports.SigningEvidenceRequirementAlreadyRegistered, nil
	}
	return ports.SigningEvidenceRequirementContentConflict, nil
}

func (repository *SigningEvidences) LoadSigningEvidenceRequirement(
	ctx context.Context,
	tenant domain.TenantID,
) (domain.SigningEvidenceRequirement, bool, error) {
	none := domain.SigningEvidenceRequirement{}
	if tenant.String() == "" {
		return none, false, fmt.Errorf("load signing evidence requirement: tenant is required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load signing evidence requirement: %w", err)
	}
	return scanSigningEvidenceRequirement(ctx, querier, tenant)
}

func (repository *SigningEvidences) HasSigningEvidence(
	ctx context.Context,
	tenant domain.TenantID,
	kind domain.CommercialObjectKind,
	objectID domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
) (bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("has signing evidence: %w", err)
	}
	var present bool
	err = querier.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1
		      FROM party_commercial.signing_evidence
		     WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4)`,
		tenant.String(), int16(kind), objectID.String(), version.String(),
	).Scan(&present)
	if err != nil {
		return false, fmt.Errorf("has signing evidence: %w", err)
	}
	return present, nil
}

func (repository *SigningEvidences) ListSigningEvidence(
	ctx context.Context,
	tenant domain.TenantID,
	kind domain.CommercialObjectKind,
	objectID domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
) ([]domain.SigningEvidence, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list signing evidence: %w", err)
	}
	rows, err := querier.Query(ctx, signingEvidenceSelect+`
		 WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4
		 ORDER BY recorded_at, evidence_id`,
		tenant.String(), int16(kind), objectID.String(), version.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list signing evidence: %w", err)
	}
	defer rows.Close()
	var listed []domain.SigningEvidence
	for rows.Next() {
		evidence, scanErr := scanSigningEvidence(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list signing evidence: %w", scanErr)
		}
		listed = append(listed, evidence)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list signing evidence: %w", err)
	}
	return listed, nil
}

const signingEvidenceSelect = `
	SELECT tenant_id, evidence_id, object_kind, object_id, version_label, signed_on,
	       counterparty_signatory, our_signatory, file_name, file_sha256, library_index,
	       source, corrects_evidence_id, recorded_by, recorded_at
	  FROM party_commercial.signing_evidence`

func signingEvidenceTargetExists(ctx context.Context, querier bentopg.Querier, evidence domain.SigningEvidence) (bool, error) {
	var present bool
	err := querier.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM party_commercial.commercial_version
		     WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4)
		 OR EXISTS (
		    SELECT 1 FROM party_commercial.publication_draft
		     WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4)`,
		evidence.Tenant().String(), int16(evidence.Kind()), evidence.ObjectID().String(), evidence.Version().String(),
	).Scan(&present)
	if err != nil {
		return false, fmt.Errorf("save signing evidence: %w", err)
	}
	return present, nil
}

func loadSigningEvidenceChain(ctx context.Context, querier bentopg.Querier, evidence domain.SigningEvidence) ([]domain.SigningEvidence, error) {
	corrects, _ := evidence.Corrects()
	rows, err := querier.Query(ctx, signingEvidenceSelect+`
		 WHERE tenant_id = $1 AND (evidence_id = $2 OR corrects_evidence_id = $2)`,
		evidence.Tenant().String(), corrects.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("save signing evidence: %w", err)
	}
	defer rows.Close()
	var history []domain.SigningEvidence
	for rows.Next() {
		item, scanErr := scanSigningEvidence(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("save signing evidence: %w", scanErr)
		}
		history = append(history, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("save signing evidence: %w", err)
	}
	return history, nil
}

func loadSigningEvidence(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	id domain.SigningEvidenceID,
) (domain.SigningEvidence, bool, error) {
	rows, err := querier.Query(ctx, signingEvidenceSelect+` WHERE tenant_id = $1 AND evidence_id = $2`, tenant.String(), id.String())
	if err != nil {
		return domain.SigningEvidence{}, false, fmt.Errorf("save signing evidence: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return domain.SigningEvidence{}, false, fmt.Errorf("save signing evidence: %w", err)
		}
		return domain.SigningEvidence{}, false, nil
	}
	evidence, err := scanSigningEvidence(rows)
	if err != nil {
		return domain.SigningEvidence{}, false, fmt.Errorf("save signing evidence: %w", err)
	}
	return evidence, true, nil
}

type signingEvidenceScanner interface {
	Scan(dest ...any) error
}

func scanSigningEvidence(row signingEvidenceScanner) (domain.SigningEvidence, error) {
	var (
		tenantID, id, objectID, version, fileName, digest, libraryIndex, source, recordedBy string
		kind                                                                                int16
		signedOn, recordedAt                                                                time.Time
		counterparty, ourSide, corrects                                                     *string
	)
	if err := row.Scan(
		&tenantID, &id, &kind, &objectID, &version, &signedOn,
		&counterparty, &ourSide, &fileName, &digest, &libraryIndex,
		&source, &corrects, &recordedBy, &recordedAt,
	); err != nil {
		return domain.SigningEvidence{}, err
	}
	tenant, err := domain.NewTenantID(tenantID)
	if err != nil {
		return domain.SigningEvidence{}, err
	}
	evidenceID, err := domain.NewSigningEvidenceID(id)
	if err != nil {
		return domain.SigningEvidence{}, err
	}
	object, err := domain.NewCommercialObjectID(objectID)
	if err != nil {
		return domain.SigningEvidence{}, err
	}
	label, err := domain.NewCommercialVersionLabel(version)
	if err != nil {
		return domain.SigningEvidence{}, err
	}
	year, month, day := signedOn.UTC().Date()
	file, err := domain.NewSigningFile(fileName, digest, libraryIndex)
	if err != nil {
		return domain.SigningEvidence{}, err
	}
	named, known := domain.SigningEvidenceSourceNamed(source)
	if !known {
		return domain.SigningEvidence{}, fmt.Errorf("%w: source %q", domain.ErrInvalidSigningEvidence, source)
	}
	var prior domain.SigningEvidenceID
	if corrects != nil {
		prior, err = domain.NewSigningEvidenceID(*corrects)
		if err != nil {
			return domain.SigningEvidence{}, err
		}
	}
	recorder, err := domain.NewOperatorSubjectReference(recordedBy)
	if err != nil {
		return domain.SigningEvidence{}, err
	}
	kindValue := domain.CommercialObjectKind(kind)
	return domain.NewSigningEvidence(
		evidenceID, tenant, kindValue, object, label,
		time.Date(year, month, day, 0, 0, 0, 0, time.UTC),
		stringOrEmpty(counterparty), stringOrEmpty(ourSide),
		file, named, prior, recorder, recordedAt.UTC(),
	)
}

func scanSigningEvidenceRequirement(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
) (domain.SigningEvidenceRequirement, bool, error) {
	var contract, agreement bool
	err := querier.QueryRow(ctx,
		`SELECT requires_customer_contract, requires_supplier_agreement
		   FROM party_commercial.signing_evidence_requirement
		  WHERE tenant_id = $1`,
		tenant.String(),
	).Scan(&contract, &agreement)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SigningEvidenceRequirement{}, false, nil
	}
	if err != nil {
		return domain.SigningEvidenceRequirement{}, false, fmt.Errorf("load signing evidence requirement: %w", err)
	}
	rule, err := domain.NewSigningEvidenceRequirement(tenant, contract, agreement)
	if err != nil {
		return domain.SigningEvidenceRequirement{}, false, fmt.Errorf("load signing evidence requirement: %w", err)
	}
	return rule, true, nil
}

func correctsArgument(evidence domain.SigningEvidence) any {
	corrects, ok := evidence.Corrects()
	if !ok {
		return nil
	}
	return corrects.String()
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
