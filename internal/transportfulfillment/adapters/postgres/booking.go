package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// BookingRequests 实现 ports.BookingStore（写入代数同 ADR-0031）。
type BookingRequests struct {
	db *bentopg.DB
}

func NewBookingRequests(db *bentopg.DB) (*BookingRequests, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &BookingRequests{db: db}, nil
}

// FindByKey 按（租户+订舱）取回已提交申请。读回经 SubmitBookingRequest 重建，已取消
// 的再精确重放一次 Cancel。
func (repository *BookingRequests) FindByKey(
	ctx context.Context,
	key ports.BookingKey,
) (ports.BookingRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.BookingRecord{}, false, fmt.Errorf("find booking request: %w", err)
	}

	var commissionID, scheduleID, unit, digest string
	var quantity int64
	var requestedAt, recordedAt time.Time
	var cancelledAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT commission_id, schedule_id, quantity, unit_ref, requested_at, cancelled_at,
		        content_digest, recorded_at
		   FROM transport_fulfillment.booking_request
		  WHERE tenant_id = $1
		    AND booking_id = $2`,
		key.TenantID.String(),
		key.Booking.String(),
	).Scan(&commissionID, &scheduleID, &quantity, &unit, &requestedAt, &cancelledAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.BookingRecord{}, false, nil
	}
	if err != nil {
		return ports.BookingRecord{}, false, fmt.Errorf("find booking request: %w", err)
	}

	commissionRef, err := domain.NewTransportCommissionReference(commissionID)
	if err != nil {
		return ports.BookingRecord{}, false, fmt.Errorf("find booking request: %w", err)
	}
	scheduleRef, err := domain.NewScheduleReference(scheduleID)
	if err != nil {
		return ports.BookingRecord{}, false, fmt.Errorf("find booking request: %w", err)
	}
	unitRef, err := domain.NewQuantityUnitReference(unit)
	if err != nil {
		return ports.BookingRecord{}, false, fmt.Errorf("find booking request: %w", err)
	}
	booking, err := domain.SubmitBookingRequest(domain.BookingRequestSpec{
		TenantID:    key.TenantID,
		Booking:     key.Booking,
		Commission:  commissionRef,
		Schedule:    scheduleRef,
		Quantity:    quantity,
		Unit:        unitRef,
		RequestedAt: requestedAt,
	})
	if err != nil {
		return ports.BookingRecord{}, false, fmt.Errorf("find booking request: %w", err)
	}
	if cancelledAt != nil {
		booking, err = booking.Cancel(cancelledAt.UTC())
		if err != nil {
			return ports.BookingRecord{}, false, fmt.Errorf("find booking request: %w", err)
		}
	}
	return ports.BookingRecord{
		Key:           key,
		ContentDigest: digest,
		Booking:       booking,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次订舱申请。同键已有记录时答`已登记`。
func (repository *BookingRequests) Save(
	ctx context.Context,
	record ports.BookingRecord,
) (ports.BookingSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.BookingSaveOutcomeInvalid, fmt.Errorf("save booking request: %w", err)
	}

	var cancelledAt *time.Time
	if at, cancelled := record.Booking.Cancelled(); cancelled {
		utc := at.UTC()
		cancelledAt = &utc
	}
	quantity, unit := record.Booking.Quantity()

	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.booking_request
			(tenant_id, booking_id, commission_id, schedule_id, quantity, unit_ref,
			 requested_at, cancelled_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Booking.String(),
		record.Booking.Commission().String(),
		record.Booking.Schedule().String(),
		quantity,
		unit.String(),
		record.Booking.RequestedAt().UTC(),
		cancelledAt,
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.BookingSaveOutcomeInvalid, fmt.Errorf("save booking request: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.BookingAlreadyRegistered, nil
	}
	return ports.BookingSaved, nil
}

// BookingAnswers 实现 ports.BookingAnswerStore 与 ports.BookingAnswerLedger。一次订舱至多一份候补、至多一份定局（迁移
// 0029 的两条部分唯一索引）；INSERT 撞上任何一条唯一约束即`已有应答`，已接受的行不会被拒绝覆盖。两份答复之间的先后
// 索引钉不住，靠 FindAnswersForUpdate 那把锁把同一订舱的判定与写入串起来。
type BookingAnswers struct {
	db *bentopg.DB
}

var (
	_ ports.BookingAnswerStore  = (*BookingAnswers)(nil)
	_ ports.BookingAnswerLedger = (*BookingAnswers)(nil)
)

func NewBookingAnswers(db *bentopg.DB) (*BookingAnswers, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &BookingAnswers{db: db}, nil
}

const bookingAnswerColumns = `acceptance_id, outcome, quantity, basis, decided_at, content_digest, recorded_at`

// FindByKey 按订舱键取回当前那一份答复：有定局取定局，否则取候补。读回经重建门复验五值形状。
func (repository *BookingAnswers) FindByKey(
	ctx context.Context,
	key ports.BookingKey,
) (ports.BookingAnswerRecord, bool, error) {
	records, err := repository.query(ctx, key, "find booking answer",
		`SELECT `+bookingAnswerColumns+`
		   FROM transport_fulfillment.booking_answer
		  WHERE tenant_id = $1
		    AND booking_id = $2
		  ORDER BY (outcome <> 'WAITLISTED') DESC, decided_at DESC
		  LIMIT 1`)
	if err != nil || len(records) == 0 {
		return ports.BookingAnswerRecord{}, false, err
	}
	return records[0], true, nil
}

// FindAnswersForUpdate 先取（租户、订舱）这一键的事务级咨询锁，再按订舱键取回名下全部答复，按答复时刻先后；同刻按落库
// 先后。
//
// 锁键各维以 chr(31) 分隔后取 hashtext，口径同 ChargeOccurrences.FindCovering 那把锁；哈希撞上只多等一会儿，不错判。
// 事务隔离是读已提交，取到锁之后的下一条查询拿新快照，看得见先持锁那一笔已提交的答复。不在事务里调用由
// RequireExecutor 拒——无锁的读答不了「此刻还没有定局」。
func (repository *BookingAnswers) FindAnswersForUpdate(
	ctx context.Context,
	key ports.BookingKey,
) ([]ports.BookingAnswerRecord, error) {
	const operation = "find booking answers for update"
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('transport_fulfillment.booking_answer.booking'), hashtext($1 || chr(31) || $2))`,
		key.TenantID.String(), key.Booking.String(),
	); err != nil {
		return nil, fmt.Errorf("%s: 取订舱锁：%w", operation, err)
	}
	return repository.query(ctx, key, operation,
		`SELECT `+bookingAnswerColumns+`
		   FROM transport_fulfillment.booking_answer
		  WHERE tenant_id = $1
		    AND booking_id = $2
		  ORDER BY decided_at, inserted_at`)
}

func (repository *BookingAnswers) query(
	ctx context.Context,
	key ports.BookingKey,
	operation string,
	statement string,
) ([]ports.BookingAnswerRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx, statement, key.TenantID.String(), key.Booking.String())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()
	var records []ports.BookingAnswerRecord
	for rows.Next() {
		var acceptanceID, outcomeName, digest string
		var quantity int64
		var basis *string
		var decidedAt, recordedAt time.Time
		if err := rows.Scan(&acceptanceID, &outcomeName, &quantity, &basis, &decidedAt, &digest, &recordedAt); err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		record, err := rehydrateBookingAnswer(key, acceptanceID, outcomeName, quantity, basis, decidedAt, digest, recordedAt)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return records, nil
}

func rehydrateBookingAnswer(
	key ports.BookingKey,
	acceptanceID, outcomeName string,
	quantity int64,
	basis *string,
	decidedAt time.Time,
	digest string,
	recordedAt time.Time,
) (ports.BookingAnswerRecord, error) {
	outcome, err := carrierOutcomeFrom(outcomeName)
	if err != nil {
		return ports.BookingAnswerRecord{}, err
	}
	acceptanceRef, err := domain.NewCarrierAcceptanceReference(acceptanceID)
	if err != nil {
		return ports.BookingAnswerRecord{}, err
	}
	spec := domain.RehydrateCarrierAcceptanceSpec{
		TenantID:   key.TenantID,
		Acceptance: acceptanceRef,
		Booking:    key.Booking,
		Outcome:    outcome,
		Quantity:   quantity,
		DecidedAt:  decidedAt,
	}
	if basis != nil {
		basisRef, err := domain.NewAcceptanceBasisReference(*basis)
		if err != nil {
			return ports.BookingAnswerRecord{}, err
		}
		spec.Basis = basisRef
	}
	acceptance, err := domain.RehydrateCarrierAcceptance(spec)
	if err != nil {
		return ports.BookingAnswerRecord{}, err
	}
	return ports.BookingAnswerRecord{
		Key:           key,
		ContentDigest: digest,
		Acceptance:    acceptance,
		RecordedAt:    recordedAt.UTC(),
	}, nil
}

// Save 写下一次承运答复。撞上同一答复标识、第二份候补或第二份定局时答`已有应答`，不覆盖——已接受不能再被拒绝。
func (repository *BookingAnswers) Save(
	ctx context.Context,
	record ports.BookingAnswerRecord,
) (ports.BookingAnswerSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.BookingAnswerSaveOutcomeInvalid, fmt.Errorf("save booking answer: %w", err)
	}

	var basis *string
	if value, present := record.Acceptance.Basis(); present {
		raw := value.String()
		basis = &raw
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.booking_answer
			(tenant_id, booking_id, acceptance_id, outcome, quantity, basis,
			 decided_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Booking.String(),
		record.Acceptance.Acceptance().String(),
		record.Acceptance.Outcome().String(),
		record.Acceptance.AcceptedQuantity(),
		basis,
		record.Acceptance.DecidedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.BookingAnswerSaveOutcomeInvalid, fmt.Errorf("save booking answer: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.BookingAlreadyAnswered, nil
	}
	return ports.BookingAnswerSaved, nil
}

func carrierOutcomeFrom(raw string) (domain.CarrierAcceptanceOutcome, error) {
	switch raw {
	case domain.BookingAccepted.String():
		return domain.BookingAccepted, nil
	case domain.BookingRefused.String():
		return domain.BookingRefused, nil
	case domain.BookingExpired.String():
		return domain.BookingExpired, nil
	case domain.BookingWithdrawn.String():
		return domain.BookingWithdrawn, nil
	case domain.BookingWaitlisted.String():
		return domain.BookingWaitlisted, nil
	default:
		return 0, fmt.Errorf("unknown carrier acceptance outcome %q", raw)
	}
}
