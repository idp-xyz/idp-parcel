package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ObligationInventoryView 实现 ports.ObligationInventoryView：按案件与业务截点盘出
// 关闭依据项。
//
// 目录在场与明细行数是两个独立信号，所以读两张表：configured 取自目录行，清单取自
// 明细。压成「有没有明细行」一个信号，就会把「这个程序的义务目录还没登记」读成
// 「没有义务所以可关」——而 CONTEXT 硬句 218 要求关闭前逐项盘点全部适用义务。
type ObligationInventoryView struct {
	db *bentopg.DB
}

func NewObligationInventoryView(db *bentopg.DB) (*ObligationInventoryView, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &ObligationInventoryView{db: db}, nil
}

var _ ports.ObligationInventoryView = (*ObligationInventoryView)(nil)

// LoadObligationItems 先确认目录已登记，再按业务截点盘出当时适用的义务项。截点比较
// 用半开区间：applies_until 等于截点的义务在该时刻已不再适用，算进来会让一份早该关
// 的案件永远关不掉。
func (view *ObligationInventoryView) LoadObligationItems(
	ctx context.Context,
	tenant domain.TenantID,
	caseRef domain.CustomsCaseID,
	cutoffAt time.Time,
) ([]domain.ClosureObligationItem, bool, error) {
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("load obligation items: %w", err)
	}

	var registered bool
	err = querier.QueryRow(ctx,
		`SELECT true
		   FROM customs_compliance.closure_obligation_catalog
		  WHERE tenant_id = $1 AND case_ref = $2`,
		tenant.String(), caseRef.String(),
	).Scan(&registered)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load obligation items: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT obligation, scope, item_state, basis, handed_to
		   FROM customs_compliance.closure_obligation_item
		  WHERE tenant_id = $1
		    AND case_ref = $2
		    AND applies_from <= $3
		    AND (applies_until IS NULL OR applies_until > $3)
		  ORDER BY obligation`,
		tenant.String(), caseRef.String(), cutoffAt.UTC(),
	)
	if err != nil {
		return nil, false, fmt.Errorf("load obligation items: %w", err)
	}
	defer rows.Close()

	items := make([]domain.ClosureObligationItem, 0)
	for rows.Next() {
		var (
			obligation, scope, stateText, basis string
			handedTo                            *string
		)
		if err := rows.Scan(&obligation, &scope, &stateText, &basis, &handedTo); err != nil {
			return nil, false, fmt.Errorf("load obligation items: %w", err)
		}
		state, err := obligationItemStateOf(stateText)
		if err != nil {
			return nil, false, fmt.Errorf("load obligation items: %w", err)
		}
		item := domain.ClosureObligationItem{
			Obligation: obligation,
			Scope:      scope,
			State:      state,
			Basis:      basis,
		}
		if handedTo != nil {
			item.HandedTo = *handedTo
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("load obligation items: %w", err)
	}
	return items, true, nil
}

// obligationItemStateOf 把库里的封闭三值译回领域取值。default 报错不吸收：多出来的
// 取值只可能来自库被旁路写入，静默当作`未解决`会让一份不该关的案件看起来只差一步。
func obligationItemStateOf(raw string) (domain.ObligationItemState, error) {
	switch raw {
	case "CONCLUDED":
		return domain.ObligationConcluded, nil
	case "HANDED_OVER":
		return domain.ObligationHandedOver, nil
	case "UNRESOLVED":
		return domain.ObligationUnresolved, nil
	default:
		return domain.ObligationItemStateInvalid,
			fmt.Errorf("unknown obligation item state %q", raw)
	}
}
