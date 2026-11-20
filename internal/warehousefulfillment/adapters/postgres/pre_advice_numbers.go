package postgres

import (
	"context"
	"errors"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// PreAdviceNumberAssigner 是编号组件的取号编排在本适配器这一侧看得见的那一面。
type PreAdviceNumberAssigner interface {
	Assign(ctx context.Context, request docnumber.IssueRequest) (docnumber.Assignment, error)
}

// PreAdviceNumbers 把入库预报接到编号组件的取号编排上。是否已用查本表。
// 取号包在保存点里：撞号退回保存点，取过的流水不留；成功则释放保存点，号留在调用方的受理事务里，
// 预报插入失败时整笔回滚，号也不留。
type PreAdviceNumbers struct {
	db       *bentopg.DB
	assigner PreAdviceNumberAssigner
}

func NewPreAdviceNumbers(db *bentopg.DB, assigner PreAdviceNumberAssigner) (*PreAdviceNumbers, error) {
	if db == nil || assigner == nil {
		return nil, errors.New("warehouse fulfillment postgres: pre-advice numbers need a db and an assigner")
	}
	return &PreAdviceNumbers{db: db, assigner: assigner}, nil
}

var _ ports.PreAdviceNumberIssuer = (*PreAdviceNumbers)(nil)

const preAdviceNumberSavepoint = "warehouse_fulfillment_pre_advice_number"

func (numbers *PreAdviceNumbers) IssuePreAdviceNumber(
	ctx context.Context,
	tenant domain.TenantID,
	issuerZone domain.BusinessTimeZone,
) (ports.PreAdviceNumberIssue, error) {
	const operation = "issue inbound pre-advice number"
	if tenant.String() == "" {
		return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: tenant is empty", operation)
	}
	executor, err := numbers.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT "+preAdviceNumberSavepoint); err != nil {
		return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: %w", operation, err)
	}
	// 仓的业务时区没登记就留空：编号组件改用租户的单据编号时区。两者都没有时答时区未配置，不在这里填 UTC。
	zone := ""
	if issuerZone.Registered() {
		zone = issuerZone.String()
	}
	assignment, err := numbers.assigner.Assign(ctx, docnumber.IssueRequest{
		Tenant:         tenant.String(),
		Type:           domain.InboundPreAdviceDocumentNumberType.Code,
		IssuerTimeZone: zone,
		IsUsed: func(ctx context.Context, number string) (bool, error) {
			var used bool
			err := executor.QueryRow(ctx,
				`SELECT EXISTS (
				    SELECT 1 FROM warehouse_fulfillment.inbound_pre_advice
				     WHERE tenant_id = $1 AND document_number = $2)`,
				tenant.String(), number).Scan(&used)
			return used, err
		},
	})
	if err != nil {
		if _, rollbackErr := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+preAdviceNumberSavepoint); rollbackErr != nil {
			return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: %w (and the savepoint did not roll back: %v)", operation, err, rollbackErr)
		}
		if _, releaseErr := executor.Exec(ctx, "RELEASE SAVEPOINT "+preAdviceNumberSavepoint); releaseErr != nil {
			return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: %w (and the savepoint did not release: %v)", operation, err, releaseErr)
		}
		return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: %w", operation, err)
	}

	var issue ports.PreAdviceNumberIssue
	switch assignment.Outcome {
	case docnumber.NumberAssigned:
		number, numberErr := domain.NewDocumentNumber(assignment.Number)
		if numberErr != nil {
			_, _ = executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+preAdviceNumberSavepoint)
			_, _ = executor.Exec(ctx, "RELEASE SAVEPOINT "+preAdviceNumberSavepoint)
			return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: %w", operation, numberErr)
		}
		issue = ports.PreAdviceNumberIssue{Outcome: ports.PreAdviceNumberAssigned, Number: number}
	case docnumber.NumberingTimeZoneNotConfigured:
		issue = ports.PreAdviceNumberIssue{Outcome: ports.PreAdviceNumberTimeZoneNotConfigured}
	case docnumber.NumberingRuleCollision:
		if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+preAdviceNumberSavepoint); err != nil {
			return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: release the collided sequence values: %w", operation, err)
		}
		issue = ports.PreAdviceNumberIssue{Outcome: ports.PreAdviceNumberRuleCollision}
	default:
		_, _ = executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+preAdviceNumberSavepoint)
		_, _ = executor.Exec(ctx, "RELEASE SAVEPOINT "+preAdviceNumberSavepoint)
		return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: unexpected assignment outcome %q", operation, assignment.Outcome)
	}
	if _, err := executor.Exec(ctx, "RELEASE SAVEPOINT "+preAdviceNumberSavepoint); err != nil {
		return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: %w", operation, err)
	}
	return issue, nil
}
