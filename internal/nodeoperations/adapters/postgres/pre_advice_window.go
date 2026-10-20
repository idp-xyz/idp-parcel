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

// PreAdviceWindows 实现 ports.PreAdviceWindowRegister（迁移 0013、0020；PAR-NET-18；票 ps-external-mark-relations/14）。只增：写口
// 只有登记一行与给开放终点的一行追加收口两种插入，没有 UPDATE 或 DELETE，在册的行不被改写。
//
// 同租户生效区间不重叠按收口之后的有效终点判。排他约束只能比一行登记时给的区间、读不到收口（迁移 0020 因此撤了它），所以由本类型
// 的两个写口交付：先取同一把以租户为键的事务级咨询锁，再在下一条语句里查重叠并插入。事务是读已提交，取锁之后的语句看得见先到那
// 一笔已提交的行与收口；咨询锁不要已有行，首插并发同样串行。两个写口之外不得往这两张表里写——绕过锁的写不受这条不变量保护。
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

// windowSource 是带着收口读时窗行的来源，windowSelect 是对应的列；effectiveSpan 是一行收口之后的有效区间：收过口的终点是收口
// 时刻，否则是登记时给的终点（收口只落给登记时没有终点的行，两者不会同时在场）。查重叠与按时点读都经它，两处的「终点」才是同一个。
const (
	windowSource = `node_operations.pre_advice_window AS w
		LEFT JOIN node_operations.pre_advice_window_closure AS c
		  ON c.tenant_id = w.tenant_id AND c.window_effective_from = w.effective_from`
	windowSelect  = `w.effective_from, w.effective_until, w.lead_minutes, w.grace_minutes, w.basis_ref, w.registered_at, c.closed_at, c.basis_ref, c.registered_at`
	effectiveSpan = `tstzrange(w.effective_from, COALESCE(c.closed_at, w.effective_until))`
)

// lockTenantWindows 取本租户时窗册的事务级咨询锁，两个写口同一把。哈希撞上只多等一会儿，不错判；锁键口径同本包别的咨询锁。
func lockTenantWindows(ctx context.Context, executor bentopg.Executor, tenant domain.TenantID) error {
	_, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('node_operations.pre_advice_window'), hashtext($1))`,
		tenant.String(),
	)
	return err
}

// RegisterPreAdviceWindow 落一行时窗：取租户锁之后，与在册各行收口之后的有效区间都不相交才插入；没落时读回撞上的那一行（起点最早
// 的那一行），带着它的收口。同起点必相交，主键撞键因此走不到。
func (register *PreAdviceWindows) RegisterPreAdviceWindow(
	ctx context.Context,
	window domain.PreAdviceValidityWindow,
) (domain.PreAdviceValidityWindow, ports.DeliveryScopeAppendOutcome, error) {
	none := domain.PreAdviceValidityWindow{}
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("register pre-advice window: %w", err)
	}
	if err := lockTenantWindows(ctx, executor, window.TenantID()); err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("register pre-advice window: 取租户锁：%w", err)
	}
	var until *time.Time
	if bound, bounded := window.EffectiveUntil(); bounded {
		until = &bound
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.pre_advice_window
			(tenant_id, effective_from, effective_until, lead_minutes, grace_minutes, basis_ref, registered_at)
		 SELECT $1, $2, $3, $4, $5, $6, $7
		  WHERE NOT EXISTS (
			SELECT 1 FROM `+windowSource+`
			 WHERE w.tenant_id = $1 AND `+effectiveSpan+` && tstzrange($2::timestamptz, $3::timestamptz))`,
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
		`SELECT `+windowSelect+` FROM `+windowSource+`
		  WHERE w.tenant_id = $1 AND `+effectiveSpan+` && tstzrange($2::timestamptz, $3::timestamptz)
		  ORDER BY w.effective_from
		  LIMIT 1`,
		window.TenantID().String(), window.EffectiveFrom(), until,
	), window.TenantID())
	if err != nil || !found {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("register pre-advice window: read back: found=%v: %w", found, err)
	}
	return recorded, ports.DeliveryScopeAlreadyRecorded, nil
}

// ClosePreAdviceWindow 给一行开放终点的时窗追加收口：取租户锁之后，只在那一行登记时没有终点时插入；那一行已收过口由主键接住，
// 什么也不落。之后一律按那一行读回：落了的带着这份收口，没落的带着它已有的终点或收口，读不到即本租户没有那一行。收口只让有效
// 区间变短，不会造出重叠，所以这里不查重叠。
func (register *PreAdviceWindows) ClosePreAdviceWindow(
	ctx context.Context,
	closure domain.PreAdviceWindowClosure,
) (domain.PreAdviceValidityWindow, ports.DeliveryScopeAppendOutcome, error) {
	none := domain.PreAdviceValidityWindow{}
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("close pre-advice window: %w", err)
	}
	if err := lockTenantWindows(ctx, executor, closure.TenantID()); err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("close pre-advice window: 取租户锁：%w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.pre_advice_window_closure
			(tenant_id, window_effective_from, closed_at, basis_ref, registered_at)
		 SELECT w.tenant_id, w.effective_from, $3, $4, $5
		   FROM node_operations.pre_advice_window AS w
		  WHERE w.tenant_id = $1 AND w.effective_from = $2 AND w.effective_until IS NULL
		 ON CONFLICT DO NOTHING`,
		closure.TenantID().String(), closure.WindowFrom(), closure.ClosedAt(), closure.Basis(), closure.RegisteredAt(),
	)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("close pre-advice window: %w", err)
	}
	recorded, found, err := scanWindow(executor.QueryRow(ctx,
		`SELECT `+windowSelect+` FROM `+windowSource+`
		  WHERE w.tenant_id = $1 AND w.effective_from = $2`,
		closure.TenantID().String(), closure.WindowFrom(),
	), closure.TenantID())
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("close pre-advice window: read back: %w", err)
	}
	switch {
	case !found && tag.RowsAffected() == 1:
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("close pre-advice window: read back: the closed row is gone")
	case !found:
		return none, ports.DeliveryScopeTargetNotFound, nil
	case tag.RowsAffected() == 1:
		return recorded, ports.DeliveryScopeAppended, nil
	default:
		return recorded, ports.DeliveryScopeAlreadyRecorded, nil
	}
}

// FindPreAdviceWindowAt 读租户在时刻 at 生效的那一行，收过口的按收口时刻当终点；写口的不重叠保证至多一行。空时点是调用方的编程
// 错误，不查成「没有生效行」——那会把坏输入折成一个像样的未配置。
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
		`SELECT `+windowSelect+` FROM `+windowSource+`
		  WHERE w.tenant_id = $1 AND `+effectiveSpan+` @> $2::timestamptz`,
		tenant.String(), at.UTC(),
	), tenant)
	if err != nil {
		return domain.PreAdviceValidityWindow{}, false, fmt.Errorf("find pre-advice window: %w", err)
	}
	return window, found, nil
}

// scanWindow 按 windowSelect 的列还原一行，带收口的经领域的 Close 挂上——还原与当初落册过同一道门，册上若有一份收口挂在登记时带
// 终点的行上，读出来是错误而不是一行像样的时窗。
func scanWindow(row pgx.Row, tenant domain.TenantID) (domain.PreAdviceValidityWindow, bool, error) {
	var (
		from                time.Time
		until               *time.Time
		lead, grace         int64
		basis               string
		registeredAt        time.Time
		closedAt            *time.Time
		closureBasis        *string
		closureRegisteredAt *time.Time
	)
	err := row.Scan(&from, &until, &lead, &grace, &basis, &registeredAt, &closedAt, &closureBasis, &closureRegisteredAt)
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
	if closedAt == nil {
		return window, true, nil
	}
	if closureBasis == nil || closureRegisteredAt == nil {
		return domain.PreAdviceValidityWindow{}, false, fmt.Errorf("pre-advice window closure is incomplete")
	}
	closure, err := domain.FormPreAdviceWindowClosure(domain.PreAdviceWindowClosureSpec{
		TenantID:     tenant,
		WindowFrom:   from,
		ClosedAt:     *closedAt,
		Basis:        *closureBasis,
		RegisteredAt: *closureRegisteredAt,
	})
	if err != nil {
		return domain.PreAdviceValidityWindow{}, false, err
	}
	closed, err := window.Close(closure)
	if err != nil {
		return domain.PreAdviceValidityWindow{}, false, err
	}
	return closed, true, nil
}
