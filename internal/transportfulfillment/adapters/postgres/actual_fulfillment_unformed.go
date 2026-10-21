package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ActualFulfillmentUnformeds 实现实际履约未形成册（ADR-0231 决定六；迁移 0048）：补形成写册的三口与查阅页读的列表口。
//
// 写口一律要环境事务：册上那一行与补形成那一拍的其余写入同笔，否则一拍回滚了、册上那一条却留下来，就成了一件没人答过的未形成。读口走
// ReadExecutor。所有语句显式带租户条件。
type ActualFulfillmentUnformeds struct {
	db *bentopg.DB
}

var _ ports.ActualFulfillmentUnformedRegister = (*ActualFulfillmentUnformeds)(nil)

func NewActualFulfillmentUnformeds(db *bentopg.DB) (*ActualFulfillmentUnformeds, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &ActualFulfillmentUnformeds{db: db}, nil
}

// RecordUnformed 立一条，或把任一状态的那一条刷新为未形成，各格取这一次；首次答复时刻只在立的那一刻写。
func (register *ActualFulfillmentUnformeds) RecordUnformed(
	ctx context.Context,
	entry ports.ActualFulfillmentUnformedEntry,
	answeredAt time.Time,
) error {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return unformedWriteFailure("record actual fulfillment unformed", err)
	}
	var agreementObject, agreementVersion, defect *string
	if entry.Agreement.Object() != "" || entry.Agreement.Version() != "" {
		object, version := entry.Agreement.Object(), entry.Agreement.Version()
		agreementObject, agreementVersion = &object, &version
	}
	if entry.Defect != "" {
		defect = &entry.Defect
	}
	var revision *int
	if entry.Revision != 0 {
		revision = &entry.Revision
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.actual_fulfillment_unformed
		     (tenant_id, segment_ref, object_ref, commission_id, commission_revision, ended_at, reason,
		      agreement_object, agreement_version, defect, first_answered_at, last_answered_at, state)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $11, $10, $10, 'UNFORMED')
		 ON CONFLICT (tenant_id, segment_ref, object_ref) DO UPDATE
		    SET commission_id       = EXCLUDED.commission_id,
		        commission_revision = EXCLUDED.commission_revision,
		        ended_at            = EXCLUDED.ended_at,
		        reason              = EXCLUDED.reason,
		        agreement_object    = EXCLUDED.agreement_object,
		        agreement_version   = EXCLUDED.agreement_version,
		        defect              = EXCLUDED.defect,
		        last_answered_at    = EXCLUDED.last_answered_at,
		        state               = 'UNFORMED',
		        occurrence_ref      = NULL,
		        occurrence_version  = NULL,
		        not_charged_by      = NULL,
		        settled_at          = NULL`,
		entry.Key.TenantID.String(), entry.Key.Segment.String(), entry.Key.Object.String(),
		entry.Commission.String(), revision, entry.EndedAt.UTC(), entry.Reason.String(),
		agreementObject, agreementVersion, answeredAt.UTC(), defect,
	); err != nil {
		return unformedWriteFailure("record actual fulfillment unformed", err)
	}
	return nil
}

// SettleFormed 把一条改成已形成，记下盖住它的那一项；册上没有这一件就什么也不写。
func (register *ActualFulfillmentUnformeds) SettleFormed(
	ctx context.Context,
	key ports.ActualFulfillmentUnformedKey,
	occurrence ports.ChargeOccurrenceKey,
	at time.Time,
) error {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return unformedWriteFailure("settle actual fulfillment unformed", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE transport_fulfillment.actual_fulfillment_unformed
		    SET state = 'FORMED', occurrence_ref = $4, occurrence_version = $5, not_charged_by = NULL, settled_at = $6
		  WHERE tenant_id = $1 AND segment_ref = $2 AND object_ref = $3`,
		key.TenantID.String(), key.Segment.String(), key.Object.String(),
		occurrence.Occurrence.String(), occurrence.Validity.String(), at.UTC(),
	); err != nil {
		return unformedWriteFailure("settle actual fulfillment unformed", err)
	}
	return nil
}

// SettleNotCharged 把一条改成不计费，记下作答的那一版协议快照；册上没有这一件就什么也不写。
func (register *ActualFulfillmentUnformeds) SettleNotCharged(
	ctx context.Context,
	key ports.ActualFulfillmentUnformedKey,
	agreement domain.AgreementSnapshotReference,
	at time.Time,
) error {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return unformedWriteFailure("settle actual fulfillment unformed as not charged", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE transport_fulfillment.actual_fulfillment_unformed
		    SET state = 'NOT_CHARGED', not_charged_by = $4, occurrence_ref = NULL, occurrence_version = NULL, settled_at = $5
		  WHERE tenant_id = $1 AND segment_ref = $2 AND object_ref = $3`,
		key.TenantID.String(), key.Segment.String(), key.Object.String(), agreement.String(), at.UTC(),
	); err != nil {
		return unformedWriteFailure("settle actual fulfillment unformed as not charged", err)
	}
	return nil
}

// SettleInvalidatedByDecision 把一条改成运营决定失效，记下那一版失效版本；册上没有这一件就什么也不写。
func (register *ActualFulfillmentUnformeds) SettleInvalidatedByDecision(
	ctx context.Context,
	key ports.ActualFulfillmentUnformedKey,
	invalidated ports.ChargeOccurrenceKey,
	at time.Time,
) error {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return unformedWriteFailure("settle actual fulfillment unformed as invalidated by decision", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE transport_fulfillment.actual_fulfillment_unformed
		    SET state = 'INVALIDATED_BY_DECISION', occurrence_ref = $4, occurrence_version = $5, not_charged_by = NULL, settled_at = $6
		  WHERE tenant_id = $1 AND segment_ref = $2 AND object_ref = $3`,
		key.TenantID.String(), key.Segment.String(), key.Object.String(),
		invalidated.Occurrence.String(), invalidated.Validity.String(), at.UTC(),
	); err != nil {
		return unformedWriteFailure("settle actual fulfillment unformed as invalidated by decision", err)
	}
	return nil
}

// unformedWriteFailure 把写口的失败分成两种：确定性拒绝（没有环境事务、约束类 SQLSTATE 23xxx）带上
// ports.ErrActualFulfillmentUnformedWriteRefused，补形成据此按缺陷处置；其余照原样交出，补形成当「此刻读写不动」等重投。
func unformedWriteFailure(verb string, err error) error {
	var pgErr *pgconn.PgError
	if errors.Is(err, bentopg.ErrTransactionRequired) || (errors.As(err, &pgErr) && strings.HasPrefix(pgErr.Code, "23")) {
		return fmt.Errorf("%s: %w: %w", verb, ports.ErrActualFulfillmentUnformedWriteRefused, err)
	}
	return fmt.Errorf("%s: %w", verb, err)
}

const actualFulfillmentUnformedColumns = `tenant_id, segment_ref, object_ref, commission_id, commission_revision, ended_at, reason,
	agreement_object, agreement_version, defect, first_answered_at, last_answered_at, state,
	occurrence_ref, occurrence_version, not_charged_by, settled_at`

// FindByKey 按键读回一条。否定结果只回 false。
func (register *ActualFulfillmentUnformeds) FindByKey(
	ctx context.Context,
	key ports.ActualFulfillmentUnformedKey,
) (ports.ActualFulfillmentUnformedRecord, bool, error) {
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ActualFulfillmentUnformedRecord{}, false, fmt.Errorf("find actual fulfillment unformed: %w", err)
	}
	record, err := scanActualFulfillmentUnformed(querier.QueryRow(ctx,
		`SELECT `+actualFulfillmentUnformedColumns+`
		   FROM transport_fulfillment.actual_fulfillment_unformed
		  WHERE tenant_id = $1 AND segment_ref = $2 AND object_ref = $3`,
		key.TenantID.String(), key.Segment.String(), key.Object.String(),
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ActualFulfillmentUnformedRecord{}, false, nil
	}
	if err != nil {
		return ports.ActualFulfillmentUnformedRecord{}, false, fmt.Errorf("find actual fulfillment unformed: %w", err)
	}
	return record, true, nil
}

// ListActualFulfillmentUnformed 列一个租户未形成的那几件：参与终点最早的在前，同刻按（段，对象）稳定排序。了结的不上列：这一页是要人去
// 补的清单，不是发生项的历史。
func (register *ActualFulfillmentUnformeds) ListActualFulfillmentUnformed(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ActualFulfillmentUnformedRecord, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list actual fulfillment unformed: limit must be positive, got %d", limit)
	}
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list actual fulfillment unformed: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+actualFulfillmentUnformedColumns+`
		   FROM transport_fulfillment.actual_fulfillment_unformed
		  WHERE tenant_id = $1 AND state = 'UNFORMED'
		  ORDER BY ended_at, segment_ref, object_ref
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list actual fulfillment unformed: %w", err)
	}
	defer rows.Close()
	list := make([]ports.ActualFulfillmentUnformedRecord, 0)
	for rows.Next() {
		record, err := scanActualFulfillmentUnformed(rows)
		if err != nil {
			return nil, fmt.Errorf("list actual fulfillment unformed: %w", err)
		}
		list = append(list, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list actual fulfillment unformed: %w", err)
	}
	return list, nil
}

func scanActualFulfillmentUnformed(row pgx.Row) (ports.ActualFulfillmentUnformedRecord, error) {
	var (
		tenant, segment, object, commission, reason, state string
		revision                                           *int
		endedAt, firstAnsweredAt, lastAnsweredAt           time.Time
		agreementObject, agreementVersion, defect          *string
		occurrenceRef, occurrenceVersion, notChargedBy     *string
		settledAt                                          *time.Time
	)
	if err := row.Scan(&tenant, &segment, &object, &commission, &revision, &endedAt, &reason,
		&agreementObject, &agreementVersion, &defect, &firstAnsweredAt, &lastAnsweredAt, &state,
		&occurrenceRef, &occurrenceVersion, &notChargedBy, &settledAt); err != nil {
		return ports.ActualFulfillmentUnformedRecord{}, err
	}
	record := ports.ActualFulfillmentUnformedRecord{
		FirstAnsweredAt: firstAnsweredAt.UTC(),
		LastAnsweredAt:  lastAnsweredAt.UTC(),
	}
	entry := &record.Entry
	entry.EndedAt = endedAt.UTC()
	if revision != nil {
		entry.Revision = *revision
	}
	if defect != nil {
		entry.Defect = *defect
	}
	var err error
	if entry.Key.TenantID, err = domain.NewTenantID(tenant); err != nil {
		return ports.ActualFulfillmentUnformedRecord{}, err
	}
	if entry.Key.Segment, err = domain.NewFulfillmentSegmentReference(segment); err != nil {
		return ports.ActualFulfillmentUnformedRecord{}, err
	}
	if entry.Key.Object, err = domain.NewCarriedObjectReference(object); err != nil {
		return ports.ActualFulfillmentUnformedRecord{}, err
	}
	if entry.Commission, err = domain.NewTransportCommissionReference(commission); err != nil {
		return ports.ActualFulfillmentUnformedRecord{}, err
	}
	if entry.Reason = unformedReasonOf(reason); entry.Reason == ports.ActualFulfillmentUnformedReasonInvalid {
		return ports.ActualFulfillmentUnformedRecord{}, fmt.Errorf("unknown unformed reason %q", reason)
	}
	if agreementObject != nil && agreementVersion != nil {
		if entry.Agreement, err = domain.NewSupplierAgreementVersion(*agreementObject, *agreementVersion); err != nil {
			return ports.ActualFulfillmentUnformedRecord{}, err
		}
	}
	switch state {
	case ports.ActualFulfillmentUnformed.String():
		record.State = ports.ActualFulfillmentUnformed
	case ports.ActualFulfillmentUnformedFormed.String(), ports.ActualFulfillmentUnformedInvalidatedByDecision.String():
		record.State = ports.ActualFulfillmentUnformedFormed
		if state == ports.ActualFulfillmentUnformedInvalidatedByDecision.String() {
			record.State = ports.ActualFulfillmentUnformedInvalidatedByDecision
		}
		if occurrenceRef == nil || occurrenceVersion == nil {
			return ports.ActualFulfillmentUnformedRecord{}, fmt.Errorf("%s unformed entry without its occurrence", state)
		}
		record.Occurrence.TenantID = entry.Key.TenantID
		if record.Occurrence.Occurrence, err = domain.NewChargeOccurrenceReference(*occurrenceRef); err != nil {
			return ports.ActualFulfillmentUnformedRecord{}, err
		}
		if record.Occurrence.Validity, err = domain.NewOccurrenceValidityVersion(*occurrenceVersion); err != nil {
			return ports.ActualFulfillmentUnformedRecord{}, err
		}
	case ports.ActualFulfillmentUnformedNotCharged.String():
		record.State = ports.ActualFulfillmentUnformedNotCharged
		if notChargedBy == nil {
			return ports.ActualFulfillmentUnformedRecord{}, fmt.Errorf("not charged unformed entry without its agreement")
		}
		record.NotChargedBy = *notChargedBy
	default:
		return ports.ActualFulfillmentUnformedRecord{}, fmt.Errorf("unknown unformed state %q", state)
	}
	if settledAt != nil {
		record.SettledAt = settledAt.UTC()
	}
	return record, nil
}

func unformedReasonOf(raw string) ports.ActualFulfillmentUnformedReason {
	for _, reason := range []ports.ActualFulfillmentUnformedReason{
		ports.UnformedNotConfigured, ports.UnformedAgreementNotEffective, ports.UnformedAgreementTerminated,
		ports.UnformedProcurementContextMissing, ports.UnformedFormationDefect,
	} {
		if reason.String() == raw {
			return reason
		}
	}
	return ports.ActualFulfillmentUnformedReasonInvalid
}

// ListActualFulfillmentUnformed 是查阅页那一格（ports.ReviewCatalogueRead）。不另写一条查询：筛法与排序只在册适配器的同名方法一处。
func (catalogue *ReviewCatalogue) ListActualFulfillmentUnformed(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ActualFulfillmentUnformedRecord, error) {
	return (&ActualFulfillmentUnformeds{db: catalogue.db}).ListActualFulfillmentUnformed(ctx, tenant, limit)
}
