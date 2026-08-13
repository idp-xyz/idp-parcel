package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// DispositionRequests 实现 ports.DispositionRequestStore。「（案件+动作+范围）当前
// 至多一份」由部分唯一索引结构性承担；SaveSupersession 先更旧行（让出部分索引）再插
// 新行、同一事务越过提交边界——只落一半，替代关系与新意图会各说各话。
type DispositionRequests struct {
	db *bentopg.DB
}

func NewDispositionRequests(db *bentopg.DB) (*DispositionRequests, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &DispositionRequests{db: db}, nil
}

func (repository *DispositionRequests) FindByID(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.DispositionRequestID,
) (*domain.DispositionRequest, bool, error) {
	return repository.findWhere(ctx,
		`tenant_id = $1 AND request_id = $2`,
		tenant.String(), id.String())
}

// FindCurrent 按（租户+案件+动作+范围）交回当前那份——未被替代的请求。部分唯一
// 索引保证至多一行。
func (repository *DispositionRequests) FindCurrent(
	ctx context.Context,
	tenant domain.TenantID,
	caseID domain.CaseID,
	action domain.RequestedActionReference,
	scope domain.RequestScopeReference,
) (*domain.DispositionRequest, bool, error) {
	return repository.findWhere(ctx,
		`tenant_id = $1 AND case_id = $2 AND action_ref = $3 AND scope_ref = $4
		    AND superseded_by IS NULL`,
		tenant.String(), caseID.String(), action.String(), scope.String())
}

func (repository *DispositionRequests) findWhere(
	ctx context.Context,
	where string,
	args ...any,
) (*domain.DispositionRequest, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("find disposition request: %w", err)
	}

	var (
		requestID, caseID, target, action, scope, reason, evidence string
		intentVersion                                              int
		sentAt                                                     time.Time
		acceptanceWindow, judgedAt                                 *time.Time
		judgment, cancellation, supersededBy                       *string
	)
	err = querier.QueryRow(ctx,
		`SELECT request_id, case_id, target_context, action_ref, scope_ref, reason,
		        evidence_ref, intent_version, sent_at, acceptance_window,
		        judgment, judged_at, cancellation, superseded_by
		   FROM visibility_exception.disposition_request
		  WHERE `+where,
		args...,
	).Scan(&requestID, &caseID, &target, &action, &scope, &reason,
		&evidence, &intentVersion, &sentAt, &acceptanceWindow,
		&judgment, &judgedAt, &cancellation, &supersededBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find disposition request: %w", err)
	}

	snapshot := domain.DispositionRequestSnapshot{
		Reason:        reason,
		IntentVersion: intentVersion,
		SentAt:        sentAt,
	}
	if snapshot.ID, err = domain.NewDispositionRequestID(requestID); err != nil {
		return nil, false, fmt.Errorf("rebuild disposition request: %w", err)
	}
	if snapshot.Case, err = domain.NewCaseID(caseID); err != nil {
		return nil, false, fmt.Errorf("rebuild disposition request: %w", err)
	}
	if snapshot.Target, err = sourceContextFrom(target); err != nil {
		return nil, false, err
	}
	if snapshot.Action, err = domain.NewRequestedActionReference(action); err != nil {
		return nil, false, fmt.Errorf("rebuild disposition request: %w", err)
	}
	if snapshot.Scope, err = domain.NewRequestScopeReference(scope); err != nil {
		return nil, false, fmt.Errorf("rebuild disposition request: %w", err)
	}
	if snapshot.Evidence, err = domain.NewRequestEvidenceReference(evidence); err != nil {
		return nil, false, fmt.Errorf("rebuild disposition request: %w", err)
	}
	if acceptanceWindow != nil {
		snapshot.AcceptanceWindow = *acceptanceWindow
	}
	if judgment != nil {
		if snapshot.Judgment, err = sourceJudgmentFrom(*judgment); err != nil {
			return nil, false, err
		}
	}
	if judgedAt != nil {
		snapshot.JudgedAt = *judgedAt
	}
	if cancellation != nil {
		if snapshot.Cancellation, err = cancellationAnswerFrom(*cancellation); err != nil {
			return nil, false, err
		}
	}
	if supersededBy != nil {
		if snapshot.SupersededBy, err = domain.NewDispositionRequestID(*supersededBy); err != nil {
			return nil, false, fmt.Errorf("rebuild disposition request: %w", err)
		}
	}

	request, err := domain.RehydrateDispositionRequest(snapshot)
	if err != nil {
		return nil, false, fmt.Errorf("rebuild disposition request: %w", err)
	}
	return request, true, nil
}

// Save 落请求的当前交互历史（首发、判断回填与取消答复都经同一入口）。同键整行
// UPSERT——交互形状由迁移 CHECK 与领域重建口两头把门。
func (repository *DispositionRequests) Save(
	ctx context.Context,
	tenant domain.TenantID,
	request *domain.DispositionRequest,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save disposition request: %w", err)
	}
	if request == nil {
		return fmt.Errorf("save disposition request: request is nil")
	}
	return repository.upsert(ctx, executor, tenant, request)
}

// SaveSupersession 把被替代者与后继同一事务写入。先更旧行：被替代者让出部分唯一
// 索引（(案件+动作+范围) 当前至多一份），后继才插得进去——反过来两行会在索引上相撞。
func (repository *DispositionRequests) SaveSupersession(
	ctx context.Context,
	tenant domain.TenantID,
	prior, successor *domain.DispositionRequest,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save disposition supersession: %w", err)
	}
	if prior == nil || successor == nil {
		return fmt.Errorf("save disposition supersession: prior or successor is nil")
	}
	successorID, superseded := prior.SupersededBy()
	if !superseded || successorID != successor.ID() {
		return fmt.Errorf("save disposition supersession: the prior does not point at the successor")
	}

	if err := repository.upsert(ctx, executor, tenant, prior); err != nil {
		return fmt.Errorf("save disposition supersession: prior: %w", err)
	}
	if err := repository.upsert(ctx, executor, tenant, successor); err != nil {
		return fmt.Errorf("save disposition supersession: successor: %w", err)
	}
	return nil
}

func (repository *DispositionRequests) upsert(
	ctx context.Context,
	executor bentopg.Executor,
	tenant domain.TenantID,
	request *domain.DispositionRequest,
) error {
	snapshot := request.Snapshot()

	var judgment, cancellation, supersededBy *string
	if value := snapshot.Judgment.String(); value != "" {
		judgment = &value
	}
	if value := snapshot.Cancellation.String(); value != "" {
		cancellation = &value
	}
	if snapshot.SupersededBy.String() != "" {
		value := snapshot.SupersededBy.String()
		supersededBy = &value
	}

	_, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.disposition_request
			(tenant_id, request_id, case_id, target_context, action_ref, scope_ref,
			 reason, evidence_ref, intent_version, sent_at, acceptance_window,
			 judgment, judged_at, cancellation, superseded_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 ON CONFLICT (tenant_id, request_id) DO UPDATE SET
			judgment = EXCLUDED.judgment,
			judged_at = EXCLUDED.judged_at,
			cancellation = EXCLUDED.cancellation,
			superseded_by = EXCLUDED.superseded_by`,
		tenant.String(),
		snapshot.ID.String(),
		snapshot.Case.String(),
		snapshot.Target.String(),
		snapshot.Action.String(),
		snapshot.Scope.String(),
		snapshot.Reason,
		snapshot.Evidence.String(),
		snapshot.IntentVersion,
		snapshot.SentAt,
		nullIfZeroTime(snapshot.AcceptanceWindow),
		judgment,
		nullIfZeroTime(snapshot.JudgedAt),
		cancellation,
		supersededBy,
	)
	if err != nil {
		return fmt.Errorf("upsert disposition request: %w", err)
	}
	return nil
}

func sourceJudgmentFrom(value string) (domain.SourceJudgment, error) {
	switch value {
	case "ACCEPTED":
		return domain.RequestAccepted, nil
	case "PARTIALLY_ACCEPTED":
		return domain.RequestPartiallyAccepted, nil
	case "REFUSED":
		return domain.RequestRefused, nil
	case "SUPPLEMENT_REQUIRED":
		return domain.SupplementRequired, nil
	default:
		return domain.SourceJudgmentInvalid,
			fmt.Errorf("visibility exception postgres: unknown source judgment %q", value)
	}
}

func cancellationAnswerFrom(value string) (domain.CancellationAnswer, error) {
	switch value {
	case "CANCELLATION_ACCEPTED":
		return domain.CancellationAcceptedByTarget, nil
	case "PARTIALLY_CANCELLED":
		return domain.PartiallyCancelled, nil
	case "NO_LONGER_CANCELLABLE":
		return domain.NoLongerCancellable, nil
	case "CANCELLATION_REFUSED":
		return domain.CancellationRefusedByTarget, nil
	default:
		return domain.CancellationAnswerInvalid,
			fmt.Errorf("visibility exception postgres: unknown cancellation answer %q", value)
	}
}
