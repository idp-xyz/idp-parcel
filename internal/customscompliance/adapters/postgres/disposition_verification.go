package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// DispositionVerifications 实现 ports.DispositionVerificationStore（写入代数同
// ADR-0031）。同一决定加同一事实集指纹只出一版。
type DispositionVerifications struct {
	db *bentopg.DB
}

func NewDispositionVerifications(db *bentopg.DB) (*DispositionVerifications, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &DispositionVerifications{db: db}, nil
}

type executionFactRow struct {
	Executor   string    `json:"executor"`
	Fact       string    `json:"fact"`
	Scope      string    `json:"scope"`
	Units      int       `json:"units"`
	OccurredAt time.Time `json:"occurredAt"`
}

func (repository *DispositionVerifications) FindByKey(
	ctx context.Context,
	key ports.VerificationKey,
) (domain.DispositionVerification, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.DispositionVerification{}, false, fmt.Errorf("find disposition verification: %w", err)
	}

	var (
		authority, action, scope, conclusion string
		quantityProvided                     bool
		quantityUnits, executed              int
		receivedAt, verifiedAt               time.Time
		factsRaw                             []byte
	)
	err = querier.QueryRow(ctx,
		`SELECT authority_ref, action_ref, scope_ref, quantity_provided, quantity_units,
		        received_at, facts, conclusion, executed_units, verified_at
		   FROM customs_compliance.disposition_verification
		  WHERE tenant_id = $1 AND decision_id = $2 AND facts_digest = $3`,
		key.TenantID.String(), key.Decision.String(), key.Digest,
	).Scan(&authority, &action, &scope, &quantityProvided, &quantityUnits,
		&receivedAt, &factsRaw, &conclusion, &executed, &verifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DispositionVerification{}, false, nil
	}
	if err != nil {
		return domain.DispositionVerification{}, false, fmt.Errorf("find disposition verification: %w", err)
	}

	verification, err := rebuildDispositionVerification(
		key.Decision, authority, action, scope, quantityProvided, quantityUnits,
		receivedAt, factsRaw, verifiedAt)
	if err != nil {
		return domain.DispositionVerification{}, false, fmt.Errorf("rebuild disposition verification: %w", err)
	}
	if verification.Conclusion().String() != conclusion || verification.ExecutedUnits() != executed {
		return domain.DispositionVerification{}, false,
			fmt.Errorf("rebuild disposition verification: stored conclusion or units disagree with rebuilt judgment")
	}
	return verification, true, nil
}

func (repository *DispositionVerifications) Save(
	ctx context.Context,
	key ports.VerificationKey,
	verification domain.DispositionVerification,
) (ports.VerificationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.VerificationSaveOutcomeInvalid, fmt.Errorf("save disposition verification: %w", err)
	}
	if key.Decision != verification.Decision().ID() {
		return ports.VerificationSaveOutcomeInvalid,
			fmt.Errorf("save disposition verification: key disagrees with the verification it claims to index")
	}

	decision := verification.Decision()
	quantity := decision.Quantity()
	factsRaw, err := marshalExecutionFacts(verification.Facts())
	if err != nil {
		return ports.VerificationSaveOutcomeInvalid, fmt.Errorf("save disposition verification: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.disposition_verification
			(tenant_id, decision_id, facts_digest, authority_ref, action_ref, scope_ref,
			 quantity_provided, quantity_units, received_at, facts, conclusion,
			 executed_units, verified_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT (tenant_id, decision_id, facts_digest) DO NOTHING`,
		key.TenantID.String(),
		key.Decision.String(),
		key.Digest,
		decision.Authority().String(),
		decision.Action().String(),
		decision.Scope().String(),
		quantity.Provided,
		quantity.Units,
		decision.ReceivedAt(),
		factsRaw,
		verification.Conclusion().String(),
		verification.ExecutedUnits(),
		verification.VerifiedAt(),
	)
	if err != nil {
		return ports.VerificationSaveOutcomeInvalid, fmt.Errorf("save disposition verification: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.VerificationAlreadyRecorded, nil
	}
	return ports.VerificationSaved, nil
}

func marshalExecutionFacts(facts []domain.ExecutionFact) ([]byte, error) {
	rows := make([]executionFactRow, 0, len(facts))
	for _, fact := range facts {
		rows = append(rows, executionFactRow{
			Executor:   fact.Executor().String(),
			Fact:       fact.Fact().String(),
			Scope:      fact.Scope().String(),
			Units:      fact.Units(),
			OccurredAt: fact.OccurredAt(),
		})
	}
	return json.Marshal(rows)
}

func rebuildDispositionVerification(
	decisionID domain.RegulatoryDecisionID,
	authority, action, scope string,
	quantityProvided bool,
	quantityUnits int,
	receivedAt time.Time,
	factsRaw []byte,
	verifiedAt time.Time,
) (domain.DispositionVerification, error) {
	spec := domain.RegulatoryDecisionSpec{
		ID:         decisionID,
		ReceivedAt: receivedAt,
		Quantity:   domain.RequiredQuantity{Provided: quantityProvided, Units: quantityUnits},
	}
	var err error
	if spec.Authority, err = domain.NewRegulatoryAuthorityReference(authority); err != nil {
		return domain.DispositionVerification{}, err
	}
	if spec.Action, err = domain.NewLegalActionReference(action); err != nil {
		return domain.DispositionVerification{}, err
	}
	if spec.Scope, err = domain.NewDecisionScopeReference(scope); err != nil {
		return domain.DispositionVerification{}, err
	}
	decision, err := domain.NewRegulatoryDecision(spec)
	if err != nil {
		return domain.DispositionVerification{}, err
	}

	var rows []executionFactRow
	if err := json.Unmarshal(factsRaw, &rows); err != nil {
		return domain.DispositionVerification{}, fmt.Errorf("facts: %w", err)
	}
	facts := make([]domain.ExecutionFact, 0, len(rows))
	for _, row := range rows {
		executor, err := domain.NewExecutorReference(row.Executor)
		if err != nil {
			return domain.DispositionVerification{}, err
		}
		factRef, err := domain.NewExecutionFactReference(row.Fact)
		if err != nil {
			return domain.DispositionVerification{}, err
		}
		factScope, err := domain.NewDecisionScopeReference(row.Scope)
		if err != nil {
			return domain.DispositionVerification{}, err
		}
		fact, err := domain.NewExecutionFact(domain.ExecutionFactSpec{
			Executor:   executor,
			Fact:       factRef,
			Scope:      factScope,
			Units:      row.Units,
			OccurredAt: row.OccurredAt,
		})
		if err != nil {
			return domain.DispositionVerification{}, err
		}
		facts = append(facts, fact)
	}
	return domain.VerifyDispositionExecution(decision, facts, verifiedAt)
}
