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

	var commissionID, unit, digest string
	var quantity int64
	var requestedAt, recordedAt time.Time
	var cancelledAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT commission_id, quantity, unit_ref, requested_at, cancelled_at,
		        content_digest, recorded_at
		   FROM transport_fulfillment.booking_request
		  WHERE tenant_id = $1
		    AND booking_id = $2`,
		key.TenantID.String(),
		key.Booking.String(),
	).Scan(&commissionID, &quantity, &unit, &requestedAt, &cancelledAt, &digest, &recordedAt)
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
	unitRef, err := domain.NewQuantityUnitReference(unit)
	if err != nil {
		return ports.BookingRecord{}, false, fmt.Errorf("find booking request: %w", err)
	}
	booking, err := domain.SubmitBookingRequest(domain.BookingRequestSpec{
		TenantID:    key.TenantID,
		Booking:     key.Booking,
		Commission:  commissionRef,
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
			(tenant_id, booking_id, commission_id, quantity, unit_ref,
			 requested_at, cancelled_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Booking.String(),
		record.Booking.Commission().String(),
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

// BookingAnswers 实现 ports.BookingAnswerStore。一订舱一应答：INSERT 撞键即
// `已有应答`，已接受的行不会被拒绝覆盖。
type BookingAnswers struct {
	db *bentopg.DB
}

func NewBookingAnswers(db *bentopg.DB) (*BookingAnswers, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &BookingAnswers{db: db}, nil
}

// FindByKey 按订舱键取回承运应答。读回经重建门复验四值形状。
func (repository *BookingAnswers) FindByKey(
	ctx context.Context,
	key ports.BookingKey,
) (ports.BookingAnswerRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.BookingAnswerRecord{}, false, fmt.Errorf("find booking answer: %w", err)
	}

	var acceptanceID, outcomeName, digest string
	var quantity int64
	var basis *string
	var decidedAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT acceptance_id, outcome, quantity, basis, decided_at, content_digest, recorded_at
		   FROM transport_fulfillment.booking_answer
		  WHERE tenant_id = $1
		    AND booking_id = $2`,
		key.TenantID.String(),
		key.Booking.String(),
	).Scan(&acceptanceID, &outcomeName, &quantity, &basis, &decidedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.BookingAnswerRecord{}, false, nil
	}
	if err != nil {
		return ports.BookingAnswerRecord{}, false, fmt.Errorf("find booking answer: %w", err)
	}

	outcome, err := carrierOutcomeFrom(outcomeName)
	if err != nil {
		return ports.BookingAnswerRecord{}, false, fmt.Errorf("find booking answer: %w", err)
	}
	acceptanceRef, err := domain.NewCarrierAcceptanceReference(acceptanceID)
	if err != nil {
		return ports.BookingAnswerRecord{}, false, fmt.Errorf("find booking answer: %w", err)
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
			return ports.BookingAnswerRecord{}, false, fmt.Errorf("find booking answer: %w", err)
		}
		spec.Basis = basisRef
	}
	acceptance, err := domain.RehydrateCarrierAcceptance(spec)
	if err != nil {
		return ports.BookingAnswerRecord{}, false, fmt.Errorf("find booking answer: %w", err)
	}
	return ports.BookingAnswerRecord{
		Key:           key,
		ContentDigest: digest,
		Acceptance:    acceptance,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次承运应答。同订舱已有应答时答`已有应答`，不覆盖——已接受不能再被拒绝。
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
	default:
		return 0, fmt.Errorf("unknown carrier acceptance outcome %q", raw)
	}
}
