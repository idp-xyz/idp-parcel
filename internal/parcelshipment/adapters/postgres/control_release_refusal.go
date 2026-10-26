package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ControlReleaseRefusals 实现 ports.ControlReleaseRefusalRegister：释放未受理册（ADR-0282 决定二）。只追加，主键是（租户，
// 委托，提交版本，控制结果），同一笔占用再被拒收撞主键答已有记录（ADR-0031 的写入代数）。
type ControlReleaseRefusals struct {
	db *bentopg.DB
}

func NewControlReleaseRefusals(db *bentopg.DB) (*ControlReleaseRefusals, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &ControlReleaseRefusals{db: db}, nil
}

var _ ports.ControlReleaseRefusalRegister = (*ControlReleaseRefusals)(nil)

// controlReleaseRefusalSavepoint 是登记那一句在调用方事务里开的 savepoint 名，与释放口外那一层的点分开取名：两者不嵌套，
// 分开取名免得哪天嵌套时退到别人的点上。
const controlReleaseRefusalSavepoint = "parcel_shipment_control_release_refusal"

// Record 记下一笔释放确定未受理的占用。要环境事务：登记与那件业务事实同笔落地。
//
// INSERT 隔在一个 savepoint 里：登记写不进时只能让那一笔改留补偿续办，不能连累调用方事务里已经成立的撤回、拒绝或暂停
// （ADR-0282 决定二；UC-PS-005「不得把释放失败改写为撤回失败」）。一条 SQL 级错误不退回 savepoint，整笔事务就进了中止态，
// 编排改留的补偿引用随提交失败一起蒸发、业务事实一并回滚。开、退、收 savepoint 本身失败同样交回错误，由编排落
// `CONTROL_RELEASE_REFUSAL_NOT_RECORDED`；退不回时事务仍在中止态，提交时整笔回滚，那是库此刻答不出，不是本口能救的。
func (register *ControlReleaseRefusals) Record(
	ctx context.Context,
	refusal ports.ControlReleaseRefusal,
) (ports.ControlReleaseRefusalSaveOutcome, error) {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ControlReleaseRefusalSaveOutcomeInvalid, fmt.Errorf("record control release refusal: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT "+controlReleaseRefusalSavepoint); err != nil {
		return ports.ControlReleaseRefusalSaveOutcomeInvalid, fmt.Errorf("record control release refusal: savepoint: %w", err)
	}
	outcome, err := register.insert(ctx, executor, refusal)
	if err != nil {
		if _, rollbackErr := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+controlReleaseRefusalSavepoint); rollbackErr != nil {
			return ports.ControlReleaseRefusalSaveOutcomeInvalid, errors.Join(err,
				fmt.Errorf("record control release refusal: savepoint rollback: %w", rollbackErr))
		}
		return ports.ControlReleaseRefusalSaveOutcomeInvalid, err
	}
	if _, err := executor.Exec(ctx, "RELEASE SAVEPOINT "+controlReleaseRefusalSavepoint); err != nil {
		return ports.ControlReleaseRefusalSaveOutcomeInvalid, fmt.Errorf("record control release refusal: savepoint release: %w", err)
	}
	return outcome, nil
}

func (register *ControlReleaseRefusals) insert(
	ctx context.Context,
	executor bentopg.Executor,
	refusal ports.ControlReleaseRefusal,
) (ports.ControlReleaseRefusalSaveOutcome, error) {
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_shipment.control_release_refusal
			(tenant_id, shipment_request_id, submission_version, control_result_id,
			 customer_account_id, occupation_legal_entity, occupation_settlement_account, occupation_currency,
			 reason_ref, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT DO NOTHING`,
		refusal.TenantID.String(),
		refusal.ShipmentRequestID.String(),
		refusal.SubmissionVersion.String(),
		refusal.ControlResultID.String(),
		refusal.CustomerAccountID.String(),
		nullableText(refusal.Scope.LegalEntity()),
		nullableText(refusal.Scope.Account()),
		nullableText(refusal.Scope.Currency()),
		refusal.Reason.String(),
		refusal.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.ControlReleaseRefusalSaveOutcomeInvalid, fmt.Errorf("record control release refusal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ControlReleaseRefusalAlreadyRecorded, nil
	}
	return ports.ControlReleaseRefusalRecorded, nil
}

// ListByShipmentRequest 按委托交回册上各笔，按提交版本、登记时间排序。否定结果只回空，不区分「没有」与「属于另一个
// 租户」。
func (register *ControlReleaseRefusals) ListByShipmentRequest(
	ctx context.Context,
	tenant domain.TenantID,
	requestID domain.ShipmentRequestID,
) ([]ports.ControlReleaseRefusal, error) {
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list control release refusals: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT submission_version, control_result_id, customer_account_id,
		        occupation_legal_entity, occupation_settlement_account, occupation_currency,
		        reason_ref, recorded_at
		   FROM parcel_shipment.control_release_refusal
		  WHERE tenant_id = $1 AND shipment_request_id = $2
		  ORDER BY submission_version, recorded_at, control_result_id`,
		tenant.String(), requestID.String())
	if err != nil {
		return nil, fmt.Errorf("list control release refusals: %w", err)
	}
	defer rows.Close()

	var refusals []ports.ControlReleaseRefusal
	for rows.Next() {
		var (
			version, resultID, account, reason string
			scope                              occupationScopeRow
			recordedAt                         time.Time
		)
		if err := rows.Scan(&version, &resultID, &account,
			&scope.legalEntity, &scope.account, &scope.currency, &reason, &recordedAt); err != nil {
			return nil, fmt.Errorf("list control release refusals: %w", err)
		}
		refusal, err := controlReleaseRefusal(tenant, requestID, version, resultID, account, scope, reason, recordedAt)
		if err != nil {
			return nil, fmt.Errorf("list control release refusals: %w", err)
		}
		refusals = append(refusals, refusal)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list control release refusals: %w", err)
	}
	return refusals, nil
}

func controlReleaseRefusal(
	tenant domain.TenantID,
	requestID domain.ShipmentRequestID,
	versionRaw, resultRaw, accountRaw string,
	scopeRow occupationScopeRow,
	reasonRaw string,
	recordedAt time.Time,
) (ports.ControlReleaseRefusal, error) {
	version, err := domain.NewSubmissionVersionID(versionRaw)
	if err != nil {
		return ports.ControlReleaseRefusal{}, err
	}
	resultID, err := domain.NewFinancialControlResultID(resultRaw)
	if err != nil {
		return ports.ControlReleaseRefusal{}, err
	}
	account, err := domain.NewCustomerAccountID(accountRaw)
	if err != nil {
		return ports.ControlReleaseRefusal{}, err
	}
	scope, err := scopeRow.occupationScope()
	if err != nil {
		return ports.ControlReleaseRefusal{}, err
	}
	reason, err := domain.NewCheckReason(reasonRaw)
	if err != nil {
		return ports.ControlReleaseRefusal{}, err
	}
	return ports.ControlReleaseRefusal{
		TenantID:          tenant,
		CustomerAccountID: account,
		ShipmentRequestID: requestID,
		SubmissionVersion: version,
		ControlResultID:   resultID,
		Scope:             scope,
		Reason:            reason,
		RecordedAt:        recordedAt.UTC(),
	}, nil
}
