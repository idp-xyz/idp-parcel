package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// BookingAnswerManualEntries 是人工录入的承运答复留痕册，实现 ports.ManualBookingAnswerLedger。
type BookingAnswerManualEntries struct {
	db *bentopg.DB
}

func NewBookingAnswerManualEntries(db *bentopg.DB) (*BookingAnswerManualEntries, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &BookingAnswerManualEntries{db: db}, nil
}

var _ ports.ManualBookingAnswerLedger = (*BookingAnswerManualEntries)(nil)

// RecordManualBookingAnswer 记一份承运答复是谁照着哪份原件录的。只在事务里写：与那一份答复同笔。一份答复至多一条，撞键
// 照错误上抛。
func (repository *BookingAnswerManualEntries) RecordManualBookingAnswer(ctx context.Context, entry ports.ManualBookingAnswer) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record manual booking answer: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.booking_answer_manual_entry
		     (tenant_id, booking_id, acceptance_ref, entered_by, original_ref, entered_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		entry.Key.TenantID.String(), entry.Key.Booking.String(), entry.Acceptance.String(), entry.EnteredBy, entry.Original, entry.EnteredAt.UTC(),
	); err != nil {
		return fmt.Errorf("record manual booking answer: %w", err)
	}
	return nil
}
