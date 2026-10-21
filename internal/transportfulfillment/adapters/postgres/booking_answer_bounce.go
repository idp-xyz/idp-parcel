package postgres

import (
	"context"
	"fmt"
	"strings"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// BookingAnswerBounces 实现 ports.BookingAnswerBounceLedger：承运答复挡回册（迁移 0050）。
type BookingAnswerBounces struct {
	db *bentopg.DB
}

var _ ports.BookingAnswerBounceLedger = (*BookingAnswerBounces)(nil)

func NewBookingAnswerBounces(db *bentopg.DB) (*BookingAnswerBounces, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &BookingAnswerBounces{db: db}, nil
}

// RecordBounce 立一行或给同键那一行加一次。只在事务里写：挡回与那一次答复同笔，答复回滚时这一笔留痕也不该留下。
//
// 两个时刻各取极值而不是照写：并发的两次同键挡回按落库先后到达，后落的那一笔不一定是后记的那一刻。
func (repository *BookingAnswerBounces) RecordBounce(ctx context.Context, bounce ports.BookingAnswerBounce) error {
	const operation = "record booking answer bounce"
	basis := bounce.Basis.String()
	if basis == "" {
		return fmt.Errorf("%s: bounce basis %d is not a basis", operation, bounce.Basis)
	}
	if strings.TrimSpace(bounce.Source) == "" {
		return fmt.Errorf("%s: answering source is blank", operation)
	}
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	bouncedAt := bounce.BouncedAt.UTC()
	if _, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.booking_answer_bounce AS bounce
			(tenant_id, source_ref, booking_id, acceptance_id, basis,
			 first_bounced_at, last_bounced_at, bounce_count)
		 VALUES ($1, $2, $3, $4, $5, $6, $6, 1)
		 ON CONFLICT (tenant_id, source_ref, booking_id, acceptance_id, basis) DO UPDATE
		    SET first_bounced_at = LEAST(bounce.first_bounced_at, EXCLUDED.first_bounced_at),
		        last_bounced_at  = GREATEST(bounce.last_bounced_at, EXCLUDED.last_bounced_at),
		        bounce_count    = bounce.bounce_count + 1`,
		bounce.Key.TenantID.String(),
		bounce.Source,
		bounce.Key.Booking.String(),
		bounce.Acceptance.String(),
		basis,
		bouncedAt,
	); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}
