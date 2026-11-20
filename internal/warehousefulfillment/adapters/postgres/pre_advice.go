package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 判重锁与预报锁的键各只写在下面两个函数里。提交只取判重锁，告知、撤回、收货缝与完结只取预报锁，两把不嵌套。
// 键若在调用处再写一遍，改了一处另一处不会报错，只是两边不再互斥：两笔提交可以同时取号，撤回也可以和收货缝同时改同一份预报的状态。

func lockPreAdviceSubmission(ctx context.Context, executor bentopg.Executor, tenant, path, source, key string) error {
	_, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(
		    hashtext('warehouse_fulfillment.pre_advice.submission'),
		    hashtext($1 || chr(31) || $2 || chr(31) || $3 || chr(31) || $4))`,
		tenant, path, source, key)
	if err != nil {
		return fmt.Errorf("lock pre-advice submission: %w", err)
	}
	return nil
}

func lockPreAdvice(ctx context.Context, executor bentopg.Executor, tenant, id string) error {
	_, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(
		    hashtext('warehouse_fulfillment.pre_advice'),
		    hashtext($1 || chr(31) || $2))`,
		tenant, id)
	if err != nil {
		return fmt.Errorf("lock pre-advice: %w", err)
	}
	return nil
}

// PreAdvices 是入库预报的 PostgreSQL 适配器。事务是 READ COMMITTED。
type PreAdvices struct {
	db *bentopg.DB
}

func NewPreAdvices(db *bentopg.DB) (*PreAdvices, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &PreAdvices{db: db}, nil
}

var _ ports.PreAdviceStore = (*PreAdvices)(nil)

func (store *PreAdvices) HoldSubmission(ctx context.Context, tenant domain.TenantID, submission domain.PreAdviceSubmission) error {
	const operation = "hold pre-advice submission"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if err := lockPreAdviceSubmission(ctx, executor, tenant.String(), submission.Path().String(), submissionSource(submission), submission.Key().String()); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (store *PreAdvices) HoldPreAdvice(ctx context.Context, tenant domain.TenantID, id domain.PreAdviceID) error {
	const operation = "hold pre-advice"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if err := lockPreAdvice(ctx, executor, tenant.String(), id.String()); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (store *PreAdvices) FindBySubmission(
	ctx context.Context,
	tenant domain.TenantID,
	submission domain.PreAdviceSubmission,
) (domain.PreAdvice, bool, error) {
	return store.loadWhere(ctx, "load pre-advice by submission",
		`tenant_id = $1 AND submission_path = $2 AND submission_source = $3 AND request_key = $4`,
		tenant.String(), submission.Path().String(), submissionSource(submission), submission.Key().String())
}

func (store *PreAdvices) FindForOwner(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	id domain.PreAdviceID,
) (domain.PreAdvice, bool, error) {
	return store.loadWhere(ctx, "load pre-advice for owner",
		`tenant_id = $1 AND pre_advice_id = $2 AND owner_account_id = $3`,
		tenant.String(), id.String(), owner.String())
}

func (store *PreAdvices) Find(ctx context.Context, tenant domain.TenantID, id domain.PreAdviceID) (domain.PreAdvice, bool, error) {
	return store.loadWhere(ctx, "load pre-advice",
		`tenant_id = $1 AND pre_advice_id = $2`,
		tenant.String(), id.String())
}

func (store *PreAdvices) FindNoticeBySubmission(
	ctx context.Context,
	tenant domain.TenantID,
	submission domain.PreAdviceSubmission,
) (domain.PreAdviceID, domain.PreAdviceNotice, bool, error) {
	const operation = "load pre-advice notice by submission"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PreAdviceID{}, domain.PreAdviceNotice{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	id, notice, found, err := loadNoticeBySubmission(ctx, querier, tenant, submission)
	if err != nil {
		return domain.PreAdviceID{}, domain.PreAdviceNotice{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return id, notice, found, nil
}

func (store *PreAdvices) FindWithdrawalBySubmission(
	ctx context.Context,
	tenant domain.TenantID,
	submission domain.PreAdviceSubmission,
) (domain.PreAdviceID, bool, error) {
	const operation = "load pre-advice withdrawal by submission"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PreAdviceID{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	var preAdviceID string
	err = querier.QueryRow(ctx,
		`SELECT pre_advice_id FROM warehouse_fulfillment.inbound_pre_advice_withdrawal
		  WHERE tenant_id = $1 AND submission_path = $2 AND submission_source = $3 AND request_key = $4`,
		tenant.String(), submission.Path().String(), submissionSource(submission), submission.Key().String(),
	).Scan(&preAdviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PreAdviceID{}, false, nil
	}
	if err != nil {
		return domain.PreAdviceID{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	id, err := domain.NewPreAdviceID(preAdviceID)
	if err != nil {
		return domain.PreAdviceID{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return id, true, nil
}

func (store *PreAdvices) Insert(ctx context.Context, advice domain.PreAdvice) (ports.PreAdviceWriteOutcome, error) {
	const operation = "insert pre-advice"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PreAdviceWriteOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if err := lockPreAdviceSubmission(ctx, executor, advice.Tenant().String(), advice.Submission().Path().String(), submissionSource(advice.Submission()), advice.Submission().Key().String()); err != nil {
		return ports.PreAdviceWriteOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	inserted, err := insertPreAdvice(ctx, executor, advice)
	if err != nil {
		return ports.PreAdviceWriteOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if !inserted {
		return ports.PreAdviceWriteKeyTaken, nil
	}
	return ports.PreAdviceWritten, nil
}

func (store *PreAdvices) AppendNotice(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.PreAdviceID,
	notice domain.PreAdviceNotice,
) (ports.PreAdviceWriteOutcome, error) {
	const operation = "append pre-advice notice"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PreAdviceWriteOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if err := lockPreAdvice(ctx, executor, tenant.String(), id.String()); err != nil {
		return ports.PreAdviceWriteOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	inserted, err := insertNotice(ctx, executor, tenant, id, notice)
	if err != nil {
		return ports.PreAdviceWriteOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if !inserted {
		return ports.PreAdviceWriteKeyTaken, nil
	}
	return ports.PreAdviceWritten, nil
}

func (store *PreAdvices) Cancel(ctx context.Context, advice domain.PreAdvice) (ports.PreAdviceWriteOutcome, error) {
	const operation = "cancel pre-advice"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PreAdviceWriteOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	withdrawal, recorded := advice.Withdrawal()
	if advice.Status() != domain.PreAdviceCancelled || !recorded {
		return ports.PreAdviceWriteOutcomeInvalid, fmt.Errorf("%s: advice is not cancelled", operation)
	}
	if err := lockPreAdvice(ctx, executor, advice.Tenant().String(), advice.ID().String()); err != nil {
		return ports.PreAdviceWriteOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	inserted, err := insertWithdrawal(ctx, executor, advice.Tenant(), advice.ID(), withdrawal)
	if err != nil {
		return ports.PreAdviceWriteOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if !inserted {
		return ports.PreAdviceWriteKeyTaken, nil
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.inbound_pre_advice
		    SET status = 'CANCELLED'
		  WHERE tenant_id = $1 AND pre_advice_id = $2 AND status = 'ADVISED'`,
		advice.Tenant().String(), advice.ID().String())
	if err != nil {
		return ports.PreAdviceWriteOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() != 1 {
		return ports.PreAdviceWriteOutcomeInvalid, fmt.Errorf("%s: status was not advised", operation)
	}
	return ports.PreAdviceWritten, nil
}

func (store *PreAdvices) MarkReceiving(ctx context.Context, tenant domain.TenantID, id domain.PreAdviceID) error {
	return store.markStatus(ctx, "mark pre-advice receiving", tenant, id, "RECEIVING", "ADVISED")
}

func (store *PreAdvices) MarkClosed(ctx context.Context, tenant domain.TenantID, id domain.PreAdviceID) error {
	return store.markStatus(ctx, "mark pre-advice closed", tenant, id, "CLOSED", "ADVISED", "RECEIVING")
}

func (store *PreAdvices) markStatus(
	ctx context.Context,
	operation string,
	tenant domain.TenantID,
	id domain.PreAdviceID,
	next string,
	from ...string,
) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if err := lockPreAdvice(ctx, executor, tenant.String(), id.String()); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.inbound_pre_advice
		    SET status = $3
		  WHERE tenant_id = $1 AND pre_advice_id = $2 AND status = ANY($4)`,
		tenant.String(), id.String(), next, from)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%s: status was not %v", operation, from)
	}
	return nil
}

func (store *PreAdvices) loadWhere(ctx context.Context, operation, where string, args ...any) (domain.PreAdvice, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PreAdvice{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	advice, found, err := loadPreAdviceWhere(ctx, querier, where, args...)
	if err != nil {
		return domain.PreAdvice{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return advice, found, nil
}

func submissionSource(submission domain.PreAdviceSubmission) string {
	source, ok := submission.Source()
	if !ok {
		return ""
	}
	return source.String()
}
