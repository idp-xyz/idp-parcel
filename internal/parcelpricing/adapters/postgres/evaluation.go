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

var _ ports.EvaluationByRequestView = (*Evaluations)(nil)

// evaluationRowColumns 是两条读法共用的列面：按标识读与按回指读都要把这些比对列与快照交叉核，列表写一处，
// 两条 SELECT 与 Scan 才不会各漏一列。
const evaluationRowColumns = `evaluation_id, tenant_id, status, semantic_digest, plan_content_digest,
		        canonicalization, request_reference, snapshot`

// evaluationRow 是评价表一行的比对列面与快照。
type evaluationRow struct {
	id, tenant, status, semanticDigest, planDigest, canonicalization string
	requestReference                                                 *string
	snapshot                                                         []byte
}

func scanEvaluationRow(scanner interface{ Scan(dest ...any) error }) (evaluationRow, error) {
	var row evaluationRow
	err := scanner.Scan(&row.id, &row.tenant, &row.status, &row.semanticDigest, &row.planDigest,
		&row.canonicalization, &row.requestReference, &row.snapshot)
	return row, err
}

// evaluation 从一行重建评价：快照经领域整图重验（含语义摘要自校），再与比对列交叉核——列与快照分岔说明这一行
// 被改过。回指那一格两边都可缺席，缺席与缺席同答、在与在同值才算合。
func (row evaluationRow) evaluation() (domain.PricingEvaluation, error) {
	evaluation, err := domain.RehydrateEvaluationSnapshot(row.snapshot)
	if err != nil {
		return domain.PricingEvaluation{}, err
	}
	reference, referenced := evaluation.RequestReference()
	if evaluation.ID().String() != row.id ||
		string(evaluation.Status()) != row.status ||
		evaluation.SemanticDigest() != row.semanticDigest ||
		evaluation.PlanContentDigest() != row.planDigest ||
		evaluation.PlanCanonicalizationVersion() != row.canonicalization ||
		evaluation.Input().TenantID().String() != row.tenant ||
		referenced != (row.requestReference != nil) ||
		(referenced && reference.String() != *row.requestReference) {
		return domain.PricingEvaluation{}, fmt.Errorf("comparison columns disagree with the snapshot for %s", row.id)
	}
	return evaluation, nil
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

	row, err := scanEvaluationRow(querier.QueryRow(ctx,
		`SELECT `+evaluationRowColumns+`
		   FROM parcel_pricing.evaluation
		  WHERE evaluation_id = $1`,
		id.String(),
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PricingEvaluation{}, false, nil
	}
	if err != nil {
		return domain.PricingEvaluation{}, false, fmt.Errorf("find evaluation: %w", err)
	}
	evaluation, err := row.evaluation()
	if err != nil {
		return domain.PricingEvaluation{}, false, fmt.Errorf("find evaluation: %w", err)
	}
	return evaluation, true, nil
}

// FindByRequestReference 按（租户、回指）取为那份 SA 评价请求形成的评价（ports.EvaluationByRequestView）。租户进
// 谓词（ADR-0003）：别的租户的评价对本租户不存在。至多一行由迁移 0010 的部分唯一索引守；读回同经整图重验与
// 比对列交叉核。
func (repository *Evaluations) FindByRequestReference(
	ctx context.Context,
	tenant domain.TenantID,
	reference domain.EvaluationRequestReference,
) (domain.PricingEvaluation, bool, error) {
	if tenant.String() == "" || reference.String() == "" {
		return domain.PricingEvaluation{}, false, fmt.Errorf("find evaluation by request: tenant and request reference are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PricingEvaluation{}, false, fmt.Errorf("find evaluation by request: %w", err)
	}

	row, err := scanEvaluationRow(querier.QueryRow(ctx,
		`SELECT `+evaluationRowColumns+`
		   FROM parcel_pricing.evaluation
		  WHERE tenant_id = $1 AND request_reference = $2`,
		tenant.String(), reference.String(),
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PricingEvaluation{}, false, nil
	}
	if err != nil {
		return domain.PricingEvaluation{}, false, fmt.Errorf("find evaluation by request: %w", err)
	}
	evaluation, err := row.evaluation()
	if err != nil {
		return domain.PricingEvaluation{}, false, fmt.Errorf("find evaluation by request: %w", err)
	}
	return evaluation, true, nil
}

// Save 写下一份评价。同标识已有记录时答`已有记录`——业务答案不是错误（ADR-0031）；
// ON CONFLICT DO NOTHING 保事务可用，编排拿到它还要同事务读回原评价作答。同租户同回指
// 已有一份时撞的是迁移 0010 的部分唯一索引，同样答`已有记录`——「同一请求不形成第二份
// 评价」在库上就是这样成立的；编排按回指读回先到者，不按新标识找。
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
	var requestReference *string
	if reference, referenced := evaluation.RequestReference(); referenced {
		value := reference.String()
		requestReference = &value
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_pricing.evaluation
			(evaluation_id, tenant_id, status, semantic_digest,
			 plan_content_digest, canonicalization, request_reference, snapshot)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT DO NOTHING`,
		evaluation.ID().String(),
		evaluation.Input().TenantID().String(),
		string(evaluation.Status()),
		evaluation.SemanticDigest(),
		evaluation.PlanContentDigest(),
		evaluation.PlanCanonicalizationVersion(),
		requestReference,
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
