package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// PortsPathsPointView 实现 ports.PortsPathsView：按（租户，口岸）或（租户，路径）在
// 评估时点上解析目录版本。半开区间与解释规则解析同形（applies_until 等于评估时点的
// 版本已不再适用）；区间不重叠由 0013 的排他约束守着，至多一行。
//
// found=false 即该键该时点无已登记版本——实例半边未提供时停在未决。这里没有任何
// 兜底口径：拿开放版或系统当前时间顶替业务时点，正是解释规则读口那两句禁令挡的事。
type PortsPathsPointView struct {
	db *bentopg.DB
}

func NewPortsPathsPointView(db *bentopg.DB) (*PortsPathsPointView, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &PortsPathsPointView{db: db}, nil
}

var _ ports.PortsPathsView = (*PortsPathsPointView)(nil)

func (view *PortsPathsPointView) LoadCandidatePort(
	ctx context.Context,
	tenant domain.TenantID,
	port domain.CustomsPortReference,
	evaluatedAt time.Time,
) (ports.CandidatePortEntry, bool, error) {
	none := ports.CandidatePortEntry{}
	// 空键与零时点是调用方编程错误，与「实例还没登记」是两回事（判据同解释规则
	// 读口）：悄悄查成「未配置」会把编排的坏输入折成一个像样的业务答案。
	if strings.TrimSpace(port.String()) == "" {
		return none, false, fmt.Errorf("load candidate port: the port reference is blank")
	}
	if evaluatedAt.IsZero() {
		return none, false, fmt.Errorf("load candidate port: the evaluation instant is zero")
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load candidate port: %w", err)
	}

	var (
		appliesFrom  time.Time
		appliesUntil *time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT applies_from, applies_until
		   FROM customs_compliance.candidate_port
		  WHERE tenant_id = $1 AND port_ref = $2
		    AND applies_from <= $3
		    AND (applies_until IS NULL OR applies_until > $3)`,
		tenant.String(), port.String(), evaluatedAt.UTC(),
	).Scan(&appliesFrom, &appliesUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load candidate port: %w", err)
	}

	entry := ports.CandidatePortEntry{Port: port, AppliesFrom: appliesFrom.UTC()}
	if appliesUntil != nil {
		entry.AppliesUntil = appliesUntil.UTC()
	}
	return entry, true, nil
}

func (view *PortsPathsPointView) LoadDeclarationPath(
	ctx context.Context,
	tenant domain.TenantID,
	path domain.DeclarationPathReference,
	evaluatedAt time.Time,
) (ports.DeclarationPathEntry, bool, error) {
	none := ports.DeclarationPathEntry{}
	if strings.TrimSpace(path.String()) == "" {
		return none, false, fmt.Errorf("load declaration path: the path reference is blank")
	}
	if evaluatedAt.IsZero() {
		return none, false, fmt.Errorf("load declaration path: the evaluation instant is zero")
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load declaration path: %w", err)
	}

	var (
		portRaw, directionRaw, modeRaw string
		appliesFrom                    time.Time
		appliesUntil                   *time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT port_ref, direction, declaration_mode, applies_from, applies_until
		   FROM customs_compliance.declaration_path
		  WHERE tenant_id = $1 AND path_ref = $2
		    AND applies_from <= $3
		    AND (applies_until IS NULL OR applies_until > $3)`,
		tenant.String(), path.String(), evaluatedAt.UTC(),
	).Scan(&portRaw, &directionRaw, &modeRaw, &appliesFrom, &appliesUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load declaration path: %w", err)
	}

	route, err := rebuildDeclarationPathRoute(portRaw, directionRaw, modeRaw)
	if err != nil {
		return none, false, err
	}
	entry := ports.DeclarationPathEntry{Path: path, Route: route, AppliesFrom: appliesFrom.UTC()}
	if appliesUntil != nil {
		entry.AppliesUntil = appliesUntil.UTC()
	}
	return entry, true, nil
}

// rebuildDeclarationPathRoute 把路径三维读回领域路由：逐维走同一道构造门，坏行在
// 读口拦下上抛，不进查阅答案（判据同案件册读口的逐行重建）。
func rebuildDeclarationPathRoute(portRaw, directionRaw, modeRaw string) (domain.DeclarationPathRoute, error) {
	port, err := domain.NewCustomsPortReference(portRaw)
	if err != nil {
		return domain.DeclarationPathRoute{}, fmt.Errorf("rebuild declaration path: %w", err)
	}
	direction, err := manifestDirectionFrom(directionRaw)
	if err != nil {
		return domain.DeclarationPathRoute{}, fmt.Errorf("rebuild declaration path: %w", err)
	}
	mode, err := domain.NewDeclarationModeReference(modeRaw)
	if err != nil {
		return domain.DeclarationPathRoute{}, fmt.Errorf("rebuild declaration path: %w", err)
	}
	route, err := domain.NewDeclarationPathRoute(port, direction, mode)
	if err != nil {
		return domain.DeclarationPathRoute{}, fmt.Errorf("rebuild declaration path: %w", err)
	}
	return route, nil
}
