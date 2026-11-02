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
// 三列，不改对账单号、费用、争议金额与争议号。
type StatementDisputes struct {
	db *bentopg.DB
}

var _ ports.StatementDisputeStore = (*StatementDisputes)(nil)

const statementDisputeTable = "settlement_accounting.statement_dispute"

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
	var resolution, resolutionRef, openedBy, resolvedBy, number, absence *string
	var resolvedAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT statement_number, charge_id, disputed_minor, reason_ref, opened_at,
		        resolution, resolution_ref, resolved_at, content_digest, recorded_at,
		        operation_evidence, resolution_evidence, document_number, document_number_absence
		   FROM settlement_accounting.statement_dispute
		  WHERE tenant_id = $1
		    AND dispute_id = $2`,
		key.TenantID.String(),
		key.Dispute.String(),
	).Scan(&statement, &charge, &disputed, &reason, &openedAt,
		&resolution, &resolutionRef, &resolvedAt, &digest, &recordedAt, &openedBy, &resolvedBy, &number, &absence)
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
	numbering, err := numberingFrom(number, absence)
	if err != nil {
		return ports.DisputeRecord{}, false, fmt.Errorf("find statement dispute: %w", err)
	}
	return ports.DisputeRecord{
		Key:                key,
		ContentDigest:      digest,
		Dispute:            dispute,
		RecordedAt:         recordedAt.UTC(),
		OperationEvidence:  catalogueText(openedBy),
		ResolutionEvidence: catalogueText(resolvedBy),
		Numbering:          numbering,
	}, true, nil
}

// LockRegistration 以事务级咨询锁把同一争议标识的登记排成一队（见 ports.StatementDisputeStore）。
func (repository *StatementDisputes) LockRegistration(ctx context.Context, key ports.DisputeKey) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("lock statement dispute registration: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('settlement_accounting.statement_dispute.registration'), hashtext($1 || chr(31) || $2))`,
		key.TenantID.String(), key.Dispute.String()); err != nil {
		return fmt.Errorf("lock statement dispute registration: %w", err)
	}
	return nil
}

// DocumentNumberUsed 答一个争议号是否已在本租户的争议册上。
func (repository *StatementDisputes) DocumentNumberUsed(ctx context.Context, tenant domain.TenantID, number string) (bool, error) {
	used, err := documentNumberUsed(ctx, repository.db, statementDisputeTable, tenant, number)
	if err != nil {
		return false, fmt.Errorf("statement dispute number used: %w", err)
	}
	return used, nil
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

	number, absence, err := numberingColumns(record.Numbering)
	if err != nil {
		return ports.DisputeSaveOutcomeInvalid, fmt.Errorf("save statement dispute: %w", err)
	}
	kind, basis, resolvedAt, resolved := record.Dispute.Resolution()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.statement_dispute
			(tenant_id, dispute_id, statement_number, charge_id, disputed_minor,
			 reason_ref, opened_at, resolution, resolution_ref, resolved_at,
			 content_digest, recorded_at, operation_evidence, document_number, document_number_absence)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
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
		optionalText(record.OperationEvidence),
		number,
		absence,
	)
	if err != nil {
		return ports.DisputeSaveOutcomeInvalid, fmt.Errorf("save statement dispute: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.DisputeAlreadyOpened, nil
	}
	return ports.DisputeSaved, nil
}

// finalDisputeResolutions 是终局走向的库上词形：从领域给的全集里按 domain.DisputeResolutionKind.Final 挑，不在 SQL 里另抄一份，
// 也不依赖常量的次序。Replace 的「还能不能再处理」与挂着未了结争议的单那一读都从这一份判，两处说的是同一件事。
var finalDisputeResolutions = func() []string {
	var words []string
	for _, kind := range domain.DisputeResolutionKinds() {
		if kind.Final() {
			words = append(words, kind.String())
		}
	}
	return words
}()

// Replace 只承担裁定：UPDATE 只写裁定三列、处理人的操作证据与 digest/recorded_at。对账单号、费用、
// 争议金额不在语句里。WHERE 允许未裁定或非终局的处理（待复核）再处理；终局答 false。
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
		        content_digest = $6, recorded_at = $7, resolution_evidence = $12
		  WHERE tenant_id = $1
		    AND dispute_id = $2
		    AND statement_number = $8
		    AND charge_id = $9
		    AND disputed_minor = $10
		    AND (resolved_at IS NULL OR resolution <> ALL($11))`,
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
		finalDisputeResolutions,
		optionalText(record.ResolutionEvidence),
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
