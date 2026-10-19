package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// PreAdviceWindows 实现 ports.PreAdviceWindowRegister（迁移 0013；PAR-NET-18）。只增：唯一的写口是带排他约束的插入，重叠由约束拒在
// 库面，没有 UPDATE 或 DELETE，也没有「给开放前版落终点」那一步。
type PreAdviceWindows struct {
	db *bentopg.DB
}

func NewPreAdviceWindows(db *bentopg.DB) (*PreAdviceWindows, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &PreAdviceWindows{db: db}, nil
}

var _ ports.PreAdviceWindowRegister = (*PreAdviceWindows)(nil)

const windowColumns = `effective_from, effective_until, lead_minutes, grace_minutes, basis_ref, registered_at`

// RegisterPreAdviceWindow 落一行时窗。无 conflict target 的 DO NOTHING 同时吃主键撞键与排他约束撞重叠，事务不因此中止；没落时读回
// 撞上的那一行（起点最早的那一行）。
func (register *PreAdviceWindows) RegisterPreAdviceWindow(
	ctx context.Context,
	window domain.PreAdviceValidityWindow,
) (domain.PreAdviceValidityWindow, ports.DeliveryScopeAppendOutcome, error) {
	none := domain.PreAdviceValidityWindow{}
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("register pre-advice window: %w", err)
	}
	var until *time.Time
	if bound, bounded := window.EffectiveUntil(); bounded {
		until = &bound
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.pre_advice_window
			(tenant_id, `+windowColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT DO NOTHING`,
		window.TenantID().String(), window.EffectiveFrom(), until,
		int64(window.Lead()/time.Minute), int64(window.Grace()/time.Minute), window.Basis(), window.RegisteredAt(),
	)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("register pre-advice window: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return window, ports.DeliveryScopeAppended, nil
	}
	recorded, found, err := scanWindow(executor.QueryRow(ctx,
		`SELECT `+windowColumns+` FROM node_operations.pre_advice_window
		  WHERE tenant_id = $1 AND tstzrange(effective_from, effective_until) && tstzrange($2::timestamptz, $3::timestamptz)
		  ORDER BY effective_from
		  LIMIT 1`,
		window.TenantID().String(), window.EffectiveFrom(), until,
	), window.TenantID())
	if err != nil || !found {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("register pre-advice window: read back: found=%v: %w", found, err)
	}
	return recorded, ports.DeliveryScopeAlreadyRecorded, nil
}

// FindPreAdviceWindowAt 读租户在时刻 at 生效的那一行；排他约束保证至多一行。空时点是调用方的编程错误，不查成「没有生效行」——那会把
// 坏输入折成一个像样的未配置。
func (register *PreAdviceWindows) FindPreAdviceWindowAt(
	ctx context.Context,
	tenant domain.TenantID,
	at time.Time,
) (domain.PreAdviceValidityWindow, bool, error) {
	if at.IsZero() {
		return domain.PreAdviceValidityWindow{}, false, fmt.Errorf("find pre-advice window: the instant is zero")
	}
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PreAdviceValidityWindow{}, false, fmt.Errorf("find pre-advice window: %w", err)
	}
	window, found, err := scanWindow(querier.QueryRow(ctx,
		`SELECT `+windowColumns+` FROM node_operations.pre_advice_window
		  WHERE tenant_id = $1 AND tstzrange(effective_from, effective_until) @> $2::timestamptz`,
		tenant.String(), at.UTC(),
	), tenant)
	if err != nil {
		return domain.PreAdviceValidityWindow{}, false, fmt.Errorf("find pre-advice window: %w", err)
	}
	return window, found, nil
}

func scanWindow(row pgx.Row, tenant domain.TenantID) (domain.PreAdviceValidityWindow, bool, error) {
	var (
		from         time.Time
		until        *time.Time
		lead, grace  int64
		basis        string
		registeredAt time.Time
	)
	err := row.Scan(&from, &until, &lead, &grace, &basis, &registeredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PreAdviceValidityWindow{}, false, nil
	}
	if err != nil {
		return domain.PreAdviceValidityWindow{}, false, err
	}
	spec := domain.PreAdviceValidityWindowSpec{
		TenantID:      tenant,
		Lead:          time.Duration(lead) * time.Minute,
		Grace:         time.Duration(grace) * time.Minute,
		EffectiveFrom: from,
		Basis:         basis,
		RegisteredAt:  registeredAt,
	}
	if until != nil {
		spec.EffectiveUntil = *until
	}
	window, err := domain.FormPreAdviceValidityWindow(spec)
	if err != nil {
		return domain.PreAdviceValidityWindow{}, false, err
	}
	return window, true, nil
}
