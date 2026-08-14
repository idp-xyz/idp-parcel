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

// StatementDisputes 实现 ports.StatementDisputeStore。Save 开立；Replace 只写裁定
// 三列，不改对账单号、费用与争议金额。
type StatementDisputes struct {
	db *bentopg.DB
}

func NewStatementDisputes(db *bentopg.DB) (*StatementDisputes, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &StatementDisputes{db: db}, nil
}

// FindByKey 按（租户+异议）取回。否定结果只回 false。读回经重建门复验裁定三件，
// 不重审费用是否仍在对账单内。
func (repository *StatementDisputes) FindByKey(
	ctx context.Context,
	key ports.DisputeKey,
) (ports.DisputeRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.DisputeRecord{}, false, fmt.Errorf("find statement dispute: %w", err)
	}

	var statement, charge, reason, digest string
	var disputed int64
	var openedAt, recordedAt time.Time
	var resolution, resolutionRef *string
	var resolvedAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT statement_number, charge_id, disputed_minor, reason_ref, opened_at,
		        resolution, resolution_ref, resolved_at, content_digest, recorded_at
		   FROM settlement_accounting.statement_dispute
		  WHERE tenant_id = $1
		    AND dispute_id = $2`,
		key.TenantID.String(),
		key.Dispute.String(),
	).Scan(&statement, &charge, &disputed, &reason, &openedAt,
		&resolution, &resolutionRef, &resolvedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.DisputeRecord{}, false, nil
	}
	if err != nil {
		return ports.DisputeRecord{}, false, fmt.Errorf("find statement dispute: %w", err)
	}

	dispute, err := rebuildStatementDispute(key.Dispute, statement, charge, disputed, reason, openedAt, resolution, resolutionRef, resolvedAt)
	if err != nil {
		return ports.DisputeRecord{}, false, fmt.Errorf("find statement dispute: %w", err)
	}
	return ports.DisputeRecord{
		Key:           key,
		ContentDigest: digest,
		Dispute:       dispute,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次开立。同标识已有记录时答`已开立`，不覆盖先到者。
func (repository *StatementDisputes) Save(
	ctx context.Context,
	record ports.DisputeRecord,
) (ports.DisputeSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DisputeSaveOutcomeInvalid, fmt.Errorf("save statement dispute: %w", err)
	}

	kind, basis, resolvedAt, resolved := record.Dispute.Resolution()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.statement_dispute
			(tenant_id, dispute_id, statement_number, charge_id, disputed_minor,
			 reason_ref, opened_at, resolution, resolution_ref, resolved_at,
			 content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Dispute.String(),
		record.Dispute.Statement().String(),
		record.Dispute.Charge().String(),
		record.Dispute.DisputedMinor(),
		record.Dispute.Reason().String(),
		record.Dispute.OpenedAt().UTC(),
		optionalRef(kind, resolved),
		optionalRef(basis, resolved),
		optionalTime(resolvedAt, resolved),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.DisputeSaveOutcomeInvalid, fmt.Errorf("save statement dispute: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.DisputeAlreadyOpened, nil
	}
	return ports.DisputeSaved, nil
}

// Replace 只承担裁定：UPDATE 只写裁定三列与 digest/recorded_at。对账单号、费用、
// 争议金额不在语句里。WHERE 允许未裁定或`待复核`再处理；终局三格答 false。
func (repository *StatementDisputes) Replace(
	ctx context.Context,
	record ports.DisputeRecord,
) (bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("replace statement dispute: %w", err)
	}

	kind, basis, resolvedAt, resolved := record.Dispute.Resolution()
	if !resolved {
		return false, fmt.Errorf("replace statement dispute: replace requires a resolution")
	}

	tag, err := executor.Exec(ctx,
		`UPDATE settlement_accounting.statement_dispute
		    SET resolution = $3, resolution_ref = $4, resolved_at = $5,
		        content_digest = $6, recorded_at = $7
		  WHERE tenant_id = $1
		    AND dispute_id = $2
		    AND statement_number = $8
		    AND charge_id = $9
		    AND disputed_minor = $10
		    AND (resolved_at IS NULL OR resolution IS NOT DISTINCT FROM 'PENDING_REVIEW')`,
		record.Key.TenantID.String(),
		record.Key.Dispute.String(),
		kind.String(),
		basis.String(),
		resolvedAt.UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
		record.Dispute.Statement().String(),
		record.Dispute.Charge().String(),
		record.Dispute.DisputedMinor(),
	)
	if err != nil {
		return false, fmt.Errorf("replace statement dispute: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func rebuildStatementDispute(
	id domain.DisputeID,
	statement, charge string,
	disputed int64,
	reason string,
	openedAt time.Time,
	resolution, resolutionRef *string,
	resolvedAt *time.Time,
) (domain.StatementDispute, error) {
	statementNumber, err := domain.NewStatementNumber(statement)
	if err != nil {
		return domain.StatementDispute{}, err
	}
	chargeID, err := domain.NewCustomerChargeID(charge)
	if err != nil {
		return domain.StatementDispute{}, err
	}
	reasonRef, err := domain.NewDisputeBasisReference(reason)
	if err != nil {
		return domain.StatementDispute{}, err
	}
	spec := domain.RehydrateStatementDisputeSpec{
		Dispute:       id,
		Statement:     statementNumber,
		Charge:        chargeID,
		DisputedMinor: disputed,
		Reason:        reasonRef,
		OpenedAt:      openedAt,
	}
	if resolution != nil {
		spec.Resolution, err = disputeResolutionFrom(*resolution)
		if err != nil {
			return domain.StatementDispute{}, err
		}
	}
	if resolutionRef != nil {
		spec.ResolutionRef, err = domain.NewDisputeBasisReference(*resolutionRef)
		if err != nil {
			return domain.StatementDispute{}, err
		}
	}
	if resolvedAt != nil {
		spec.ResolvedAt = *resolvedAt
	}
	return domain.RehydrateStatementDispute(spec)
}

func disputeResolutionFrom(raw string) (domain.DisputeResolutionKind, error) {
	switch raw {
	case domain.DisputeAccepted.String():
		return domain.DisputeAccepted, nil
	case domain.DisputePartiallyAccepted.String():
		return domain.DisputePartiallyAccepted, nil
	case domain.DisputeRejected.String():
		return domain.DisputeRejected, nil
	case domain.DisputePendingReview.String():
		return domain.DisputePendingReview, nil
	default:
		return 0, fmt.Errorf("unknown dispute resolution %q", raw)
	}
}
