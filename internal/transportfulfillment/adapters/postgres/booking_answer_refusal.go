package postgres

import (
	"context"
	"fmt"
	"strings"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// BookingAnswerRefusals 实现 ports.BookingAnswerRefusalLedger：承运答复拒收册（迁移 0050）。
type BookingAnswerRefusals struct {
	db *bentopg.DB
}

var _ ports.BookingAnswerRefusalLedger = (*BookingAnswerRefusals)(nil)

func NewBookingAnswerRefusals(db *bentopg.DB) (*BookingAnswerRefusals, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &BookingAnswerRefusals{db: db}, nil
}

// RecordRefusal 立一行或给同键那一行加一次。只在事务里写：拒收与那一次答复同笔，答复回滚时这一笔留痕也不该留下。
//
// 两个时刻各取极值而不是照写：并发的两次同键拒收按落库先后到达，后落的那一笔不一定是后记的那一刻。
func (repository *BookingAnswerRefusals) RecordRefusal(ctx context.Context, refusal ports.BookingAnswerRefusal) error {
	const operation = "record booking answer refusal"
	basis := refusal.Basis.String()
	if basis == "" {
		return fmt.Errorf("%s: refusal basis %d is not a basis", operation, refusal.Basis)
	}
	if strings.TrimSpace(refusal.Source) == "" {
		return fmt.Errorf("%s: answering source is blank", operation)
	}
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	refusedAt := refusal.RefusedAt.UTC()
	if _, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.booking_answer_refusal AS refusal
			(tenant_id, source_ref, booking_id, acceptance_id, refusal_basis,
			 first_refused_at, last_refused_at, refusal_count)
		 VALUES ($1, $2, $3, $4, $5, $6, $6, 1)
		 ON CONFLICT (tenant_id, source_ref, booking_id, acceptance_id, refusal_basis) DO UPDATE
		    SET first_refused_at = LEAST(refusal.first_refused_at, EXCLUDED.first_refused_at),
		        last_refused_at  = GREATEST(refusal.last_refused_at, EXCLUDED.last_refused_at),
		        refusal_count    = refusal.refusal_count + 1`,
		refusal.Key.TenantID.String(),
		refusal.Source,
		refusal.Key.Booking.String(),
		refusal.Acceptance.String(),
		basis,
		refusedAt,
	); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}
