package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// AdvanceAssessments 实现 ports.AdvanceAssessmentStore（写入代数同 ADR-0031）。
// 同一评估标识只登一次。
type AdvanceAssessments struct {
	db *bentopg.DB
}

func NewAdvanceAssessments(db *bentopg.DB) (*AdvanceAssessments, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &AdvanceAssessments{db: db}, nil
}

// FindByKey 按（租户+评估）取回。否定结果只回 false。读回经公开构造门复验四值形状。
func (repository *AdvanceAssessments) FindByKey(
	ctx context.Context,
	key ports.AdvanceAssessmentKey,
) (ports.AdvanceAssessmentRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.AdvanceAssessmentRecord{}, false, fmt.Errorf("find advance assessment: %w", err)
	}

	var obligation, verdictName, currency, version, digest string
	var fundsFact, payer, responsibility, basis *string
	var amount int64
	var judgedAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT obligation_ref, verdict, funds_fact, payer_ref, responsibility_ref, basis,
		        currency, amount_minor, version, judged_at, content_digest, recorded_at
		   FROM settlement_accounting.advance_assessment
		  WHERE tenant_id = $1
		    AND assessment_id = $2`,
		key.TenantID.String(),
		key.Assessment.String(),
	).Scan(&obligation, &verdictName, &fundsFact, &payer, &responsibility, &basis,
		&currency, &amount, &version, &judgedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.AdvanceAssessmentRecord{}, false, nil
	}
	if err != nil {
		return ports.AdvanceAssessmentRecord{}, false, fmt.Errorf("find advance assessment: %w", err)
	}

	assessment, err := rebuildAdvanceAssessment(key.Assessment, obligation, verdictName, fundsFact, payer, responsibility, basis, currency, amount, version, judgedAt)
	if err != nil {
		return ports.AdvanceAssessmentRecord{}, false, fmt.Errorf("find advance assessment: %w", err)
	}
	return ports.AdvanceAssessmentRecord{
		Key:           key,
		ContentDigest: digest,
		Assessment:    assessment,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次代垫评估。同标识已有记录时答`已有记录`，不覆盖先到者。
func (repository *AdvanceAssessments) Save(
	ctx context.Context,
	record ports.AdvanceAssessmentRecord,
) (ports.AdvanceAssessmentSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.AdvanceAssessmentSaveOutcomeInvalid, fmt.Errorf("save advance assessment: %w", err)
	}

	currency, amount := record.Assessment.Amount()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.advance_assessment
			(tenant_id, assessment_id, obligation_ref, verdict, funds_fact, payer_ref,
			 responsibility_ref, basis, currency, amount_minor, version, judged_at,
			 content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Assessment.String(),
		record.Assessment.Obligation().String(),
		record.Assessment.Verdict().String(),
		optionalRef(record.Assessment.FundsFact()),
		optionalRef(record.Assessment.Party()),
		optionalRef(record.Assessment.Responsibility()),
		optionalRef(record.Assessment.Basis()),
		currency.String(),
		amount,
		record.Assessment.Version().String(),
		record.Assessment.JudgedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.AdvanceAssessmentSaveOutcomeInvalid, fmt.Errorf("save advance assessment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.AdvanceAssessmentAlreadyRecorded, nil
	}
	return ports.AdvanceAssessmentSaved, nil
}

func rebuildAdvanceAssessment(
	id domain.AdvanceAssessmentID,
	obligation, verdictName string,
	fundsFact, payer, responsibility, basis *string,
	currency string,
	amount int64,
	version string,
	judgedAt time.Time,
) (domain.ActualAdvanceAssessment, error) {
	obligationRef, err := domain.NewTaxObligationReference(obligation)
	if err != nil {
		return domain.ActualAdvanceAssessment{}, err
	}
	verdict, err := advanceVerdictFrom(verdictName)
	if err != nil {
		return domain.ActualAdvanceAssessment{}, err
	}
	currencyCode, err := domain.NewCurrencyCode(currency)
	if err != nil {
		return domain.ActualAdvanceAssessment{}, err
	}
	versionRef, err := domain.NewAdvanceAssessmentVersion(version)
	if err != nil {
		return domain.ActualAdvanceAssessment{}, err
	}
	spec := domain.ActualAdvanceAssessmentSpec{
		ID:          id,
		Obligation:  obligationRef,
		Verdict:     verdict,
		Currency:    currencyCode,
		AmountMinor: amount,
		Version:     versionRef,
		JudgedAt:    judgedAt,
	}
	if fundsFact != nil {
		spec.FundsFact, err = domain.NewFundsFactReference(*fundsFact)
		if err != nil {
			return domain.ActualAdvanceAssessment{}, err
		}
	}
	if payer != nil {
		spec.Payer, err = domain.NewAdvancePayerReference(*payer)
		if err != nil {
			return domain.ActualAdvanceAssessment{}, err
		}
	}
	if responsibility != nil {
		spec.Responsibility, err = domain.NewAdvanceResponsibilityReference(*responsibility)
		if err != nil {
			return domain.ActualAdvanceAssessment{}, err
		}
	}
	if basis != nil {
		spec.Basis, err = domain.NewAssessmentBasisReference(*basis)
		if err != nil {
			return domain.ActualAdvanceAssessment{}, err
		}
	}
	return domain.AssessActualAdvance(spec)
}

func advanceVerdictFrom(raw string) (domain.AdvanceVerdict, error) {
	switch raw {
	case domain.AdvanceEstablished.String():
		return domain.AdvanceEstablished, nil
	case domain.AdvanceNotEstablishedVerdict.String():
		return domain.AdvanceNotEstablishedVerdict, nil
	case domain.AdvanceUndecided.String():
		return domain.AdvanceUndecided, nil
	case domain.AdvanceConflicting.String():
		return domain.AdvanceConflicting, nil
	default:
		return 0, fmt.Errorf("unknown advance verdict %q", raw)
	}
}

func optionalRef[T interface{ String() string }](value T, present bool) *string {
	if !present {
		return nil
	}
	raw := value.String()
	return &raw
}
