package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
)

// ScopeVersionRelations 实现 ports.ScopeVersionRelationStore。同一有序对至多一条边，
// 第二份由主键拦住译成已有记录（ON CONFLICT DO NOTHING，无 UPDATE）；外键指回
// stage_review——无 Go/No-Go 决定就无关系登记，由库面结构性担保。
type ScopeVersionRelations struct {
	db *bentopg.DB
}

func NewScopeVersionRelations(db *bentopg.DB) (*ScopeVersionRelations, error) {
	if db == nil {
		return nil, fmt.Errorf("pilot governance postgres: db is nil")
	}
	return &ScopeVersionRelations{db: db}, nil
}

var _ ports.ScopeVersionRelationStore = (*ScopeVersionRelations)(nil)

func (repository *ScopeVersionRelations) FindByPair(
	ctx context.Context,
	successor, predecessor domain.ScopeVersionReference,
) (domain.ScopeVersionRelation, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ScopeVersionRelation{}, false, fmt.Errorf("find scope relation: %w", err)
	}

	var kindName, objective, candidates string
	var registeredAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT relation_kind, objective, candidate_set_id, registered_at
		   FROM pilot_governance.scope_version_relation
		  WHERE successor_scope = $1
		    AND predecessor_scope = $2`,
		successor.String(),
		predecessor.String(),
	).Scan(&kindName, &objective, &candidates, &registeredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ScopeVersionRelation{}, false, nil
	}
	if err != nil {
		return domain.ScopeVersionRelation{}, false, fmt.Errorf("find scope relation: %w", err)
	}

	kind, err := relationKindFrom(kindName)
	if err != nil {
		return domain.ScopeVersionRelation{}, false, fmt.Errorf("rebuild scope relation: %w", err)
	}
	candidatesID, err := domain.NewCandidateVersionSetID(candidates)
	if err != nil {
		return domain.ScopeVersionRelation{}, false, fmt.Errorf("rebuild scope relation: %w", err)
	}
	relation, err := domain.RegisterScopeVersionRelation(domain.ScopeVersionRelationSpec{
		Successor:    successor,
		Predecessor:  predecessor,
		Kind:         kind,
		Objective:    objective,
		Candidates:   candidatesID,
		RegisteredAt: registeredAt,
	})
	if err != nil {
		return domain.ScopeVersionRelation{}, false, fmt.Errorf("rebuild scope relation: %w", err)
	}
	return relation, true, nil
}

func (repository *ScopeVersionRelations) Save(
	ctx context.Context,
	relation domain.ScopeVersionRelation,
) (ports.GovernanceSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.GovernanceSaveOutcomeInvalid, fmt.Errorf("save scope relation: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO pilot_governance.scope_version_relation
			(successor_scope, predecessor_scope, relation_kind,
			 objective, candidate_set_id, registered_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (successor_scope, predecessor_scope) DO NOTHING`,
		relation.Successor().String(),
		relation.Predecessor().String(),
		relation.Kind().String(),
		relation.Objective(),
		relation.Candidates().String(),
		relation.RegisteredAt().UTC(),
	)
	if err != nil {
		return ports.GovernanceSaveOutcomeInvalid, fmt.Errorf("save scope relation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.GovernanceAlreadyRecorded, nil
	}
	return ports.GovernanceSaved, nil
}

func relationKindFrom(raw string) (domain.ScopeVersionRelationKind, error) {
	switch raw {
	case domain.ScopeInheritsSuspensions.String():
		return domain.ScopeInheritsSuspensions, nil
	case domain.ScopeUnrelated.String():
		return domain.ScopeUnrelated, nil
	default:
		return 0, fmt.Errorf("unknown scope version relation kind %q", raw)
	}
}
