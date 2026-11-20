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

// OutboundNumbers 把出库请求接到编号组件的取号编排上。是否已用查出库请求表。
type OutboundNumbers struct {
	db       *bentopg.DB
	assigner PreAdviceNumberAssigner
}

func NewOutboundNumbers(db *bentopg.DB, assigner PreAdviceNumberAssigner) (*OutboundNumbers, error) {
	if db == nil || assigner == nil {
		return nil, errors.New("warehouse fulfillment postgres: outbound numbers need a db and an assigner")
	}
	return &OutboundNumbers{db: db, assigner: assigner}, nil
}

var _ ports.OutboundNumberIssuer = (*OutboundNumbers)(nil)

const outboundNumberSavepoint = "warehouse_fulfillment_outbound_number"

func (numbers *OutboundNumbers) IssueOutboundNumber(ctx context.Context, tenant domain.TenantID, issuerZone domain.BusinessTimeZone) (ports.PreAdviceNumberIssue, error) {
	const operation = "issue outbound request number"
	executor, err := numbers.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT "+outboundNumberSavepoint); err != nil {
		return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: %w", operation, err)
	}
	zone := ""
	if issuerZone.Registered() {
		zone = issuerZone.String()
	}
	assignment, err := numbers.assigner.Assign(ctx, docnumber.IssueRequest{
		Tenant: tenant.String(), Type: domain.OutboundRequestDocumentNumberType.Code, IssuerTimeZone: zone,
		IsUsed: func(ctx context.Context, number string) (bool, error) {
			var used bool
			err := executor.QueryRow(ctx,
				`SELECT EXISTS (
				    SELECT 1 FROM warehouse_fulfillment.outbound_request
				     WHERE tenant_id = $1 AND document_number = $2)`,
				tenant.String(), number).Scan(&used)
			return used, err
		},
	})
	if err != nil {
		_, _ = executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+outboundNumberSavepoint)
		_, _ = executor.Exec(ctx, "RELEASE SAVEPOINT "+outboundNumberSavepoint)
		return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: %w", operation, err)
	}
	var issue ports.PreAdviceNumberIssue
	switch assignment.Outcome {
	case docnumber.NumberAssigned:
		number, numberErr := domain.NewDocumentNumber(assignment.Number)
		if numberErr != nil {
			_, _ = executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+outboundNumberSavepoint)
			_, _ = executor.Exec(ctx, "RELEASE SAVEPOINT "+outboundNumberSavepoint)
			return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: %w", operation, numberErr)
		}
		issue = ports.PreAdviceNumberIssue{Outcome: ports.PreAdviceNumberAssigned, Number: number}
	case docnumber.NumberingTimeZoneNotConfigured:
		issue = ports.PreAdviceNumberIssue{Outcome: ports.PreAdviceNumberTimeZoneNotConfigured}
	case docnumber.NumberingRuleCollision:
		if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+outboundNumberSavepoint); err != nil {
			return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: %w", operation, err)
		}
		issue = ports.PreAdviceNumberIssue{Outcome: ports.PreAdviceNumberRuleCollision}
	default:
		_, _ = executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+outboundNumberSavepoint)
		_, _ = executor.Exec(ctx, "RELEASE SAVEPOINT "+outboundNumberSavepoint)
		return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: unexpected assignment outcome %q", operation, assignment.Outcome)
	}
	if _, err := executor.Exec(ctx, "RELEASE SAVEPOINT "+outboundNumberSavepoint); err != nil {
		return ports.PreAdviceNumberIssue{}, fmt.Errorf("%s: %w", operation, err)
	}
	return issue, nil
}
