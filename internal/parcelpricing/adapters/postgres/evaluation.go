// Package postgres 是 parcel-pricing 自有语义端口的 PostgreSQL 适配器。
//
// 评价的序列化形状归领域（MarshalEvaluationSnapshot / RehydrateEvaluationSnapshot
// ——那里说明了为什么这是行模型纪律的刻意例外），本包只负责列面：键、比对列与
// 写入代数。租户条件进每条语句（ADR-0003）。
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

// Evaluations 实现 ports.EvaluationStore（写入代数同 ADR-0031）。
type Evaluations struct {
	db *bentopg.DB
}

func NewEvaluations(db *bentopg.DB) (*Evaluations, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel pricing postgres: db is nil")
	}
	return &Evaluations{db: db}, nil
}

// FindByID 按评价标识取回评价。评价标识全局唯一由签发方保证，租户列仍随行核对：
// 读回的快照经领域整图重验后，再与比对列交叉核——列与快照分岔说明这一行被改过。
func (repository *Evaluations) FindByID(
	ctx context.Context,
	id domain.EvaluationID,
) (domain.PricingEvaluation, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PricingEvaluation{}, false, fmt.Errorf("find evaluation: %w", err)
	}

	var tenant, status, semanticDigest, planDigest, canonicalization string
	var snapshot []byte
	err = querier.QueryRow(ctx,
		`SELECT tenant_id, status, semantic_digest, plan_content_digest, canonicalization, snapshot
		   FROM parcel_pricing.evaluation
		  WHERE evaluation_id = $1`,
		id.String(),
	).Scan(&tenant, &status, &semanticDigest, &planDigest, &canonicalization, &snapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PricingEvaluation{}, false, nil
	}
	if err != nil {
		return domain.PricingEvaluation{}, false, fmt.Errorf("find evaluation: %w", err)
	}

	evaluation, err := domain.RehydrateEvaluationSnapshot(snapshot)
	if err != nil {
		return domain.PricingEvaluation{}, false, fmt.Errorf("find evaluation: %w", err)
	}
	if evaluation.ID() != id ||
		string(evaluation.Status()) != status ||
		evaluation.SemanticDigest() != semanticDigest ||
		evaluation.PlanContentDigest() != planDigest ||
		evaluation.PlanCanonicalizationVersion() != canonicalization ||
		evaluation.Input().TenantID().String() != tenant {
		return domain.PricingEvaluation{}, false, fmt.Errorf(
			"find evaluation: comparison columns disagree with the snapshot for %s", id)
	}
	return evaluation, true, nil
}

// Save 写下一份评价。同标识已有记录时答`已有记录`——业务答案不是错误（ADR-0031）；
// ON CONFLICT DO NOTHING 保事务可用，编排拿到它还要同事务读回原评价作答。
//
// 走 RequireExecutor：评价落库与将来同一步的意图发布必须同生共死。
func (repository *Evaluations) Save(
	ctx context.Context,
	evaluation domain.PricingEvaluation,
) (ports.EvaluationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.EvaluationSaveOutcomeInvalid, fmt.Errorf("save evaluation: %w", err)
	}

	snapshot, err := domain.MarshalEvaluationSnapshot(evaluation)
	if err != nil {
		return ports.EvaluationSaveOutcomeInvalid, fmt.Errorf("save evaluation: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_pricing.evaluation
			(evaluation_id, tenant_id, status, semantic_digest,
			 plan_content_digest, canonicalization, snapshot)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT DO NOTHING`,
		evaluation.ID().String(),
		evaluation.Input().TenantID().String(),
		string(evaluation.Status()),
		evaluation.SemanticDigest(),
		evaluation.PlanContentDigest(),
		evaluation.PlanCanonicalizationVersion(),
		snapshot,
	)
	if err != nil {
		return ports.EvaluationSaveOutcomeInvalid, fmt.Errorf("save evaluation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.EvaluationAlreadyRecorded, nil
	}
	// 问题项落子表，与父行同一事务（ADR-0105 Decision 三）：子表是检索列面，权威内容仍在快照。只在父行真写
	// 下时写——`已有记录`那一格的子行由先到的那次写入留下，这里不重复也不覆盖。
	for ordinal, issue := range evaluation.Issues() {
		var seriesKind, seriesID *string
		if subject, ok := issue.Series(); ok {
			kind := subject.Kind().String()
			seriesKind = &kind
			if id, declared := subject.SeriesID(); declared {
				seriesID = &id
			}
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO parcel_pricing.evaluation_issue (evaluation_id, ordinal, code, series_kind, series_id)
			 VALUES ($1, $2, $3, $4, $5)`,
			evaluation.ID().String(), ordinal, issue.Code(), seriesKind, seriesID,
		); err != nil {
			return ports.EvaluationSaveOutcomeInvalid, fmt.Errorf("save evaluation issue %d: %w", ordinal, err)
		}
	}
	return ports.EvaluationSaved, nil
}
