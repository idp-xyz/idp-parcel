package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// PortsPathsCatalogue 实现 ports.PortsPathsCatalogueRead：口岸目录与申报路径目录两册
// 的列表读面（ADR-0077，票 admin-remainder-mechanism-batch/03）。读的就是 0013 那两张
// 登记册本表，不是第二份数据；点读口（PortsPathsView）按键与时点单点作答，这里按租户
// 上列——两种读法各答各的问题，谁也不为对方改形状。
//
// 排序按登记册键升序保证分页可重复：口岸按（口岸标识），路径按（路径标识）；同一
// 选择键内按生效起点倒序——当前与最近的版本在前，历史版本随后（同解释规则上列）。
// 全部版本连同区间原样上列，区间判读留给读者；limit 非正是调用方编程错误（判据同
// RuleCatalogue 那句）。
type PortsPathsCatalogue struct {
	db *bentopg.DB
}

func NewPortsPathsCatalogue(db *bentopg.DB) (*PortsPathsCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &PortsPathsCatalogue{db: db}, nil
}

var _ ports.PortsPathsCatalogueRead = (*PortsPathsCatalogue)(nil)

func (catalogue *PortsPathsCatalogue) ListCandidatePorts(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.CandidatePortEntry, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list candidate ports: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list candidate ports: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT port_ref, applies_from, applies_until
		   FROM customs_compliance.candidate_port
		  WHERE tenant_id = $1
		  ORDER BY port_ref, applies_from DESC
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list candidate ports: %w", err)
	}
	defer rows.Close()

	entries := make([]ports.CandidatePortEntry, 0, limit)
	for rows.Next() {
		var portRaw string
		var appliesFrom time.Time
		var appliesUntil *time.Time
		if err := rows.Scan(&portRaw, &appliesFrom, &appliesUntil); err != nil {
			return nil, fmt.Errorf("list candidate ports: %w", err)
		}
		// 坏行走构造门拦下上抛，不进查阅答案（判据同 RuleCatalogue 的逐行重建）。
		port, err := domain.NewCustomsPortReference(portRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild candidate port: %w", err)
		}
		entry := ports.CandidatePortEntry{Port: port, AppliesFrom: appliesFrom.UTC()}
		// NULL 终点即开放版：零值照端口约定透出，不代填「无限远」的编造时刻。
		if appliesUntil != nil {
			entry.AppliesUntil = appliesUntil.UTC()
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list candidate ports: %w", err)
	}
	return entries, nil
}

func (catalogue *PortsPathsCatalogue) ListDeclarationPaths(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.DeclarationPathEntry, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list declaration paths: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list declaration paths: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT path_ref, port_ref, direction, declaration_mode, applies_from, applies_until
		   FROM customs_compliance.declaration_path
		  WHERE tenant_id = $1
		  ORDER BY path_ref, applies_from DESC
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list declaration paths: %w", err)
	}
	defer rows.Close()

	entries := make([]ports.DeclarationPathEntry, 0, limit)
	for rows.Next() {
		var pathRaw, portRaw, directionRaw, modeRaw string
		var appliesFrom time.Time
		var appliesUntil *time.Time
		if err := rows.Scan(
			&pathRaw, &portRaw, &directionRaw, &modeRaw, &appliesFrom, &appliesUntil,
		); err != nil {
			return nil, fmt.Errorf("list declaration paths: %w", err)
		}
		path, err := domain.NewDeclarationPathReference(pathRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild declaration path: %w", err)
		}
		route, err := rebuildDeclarationPathRoute(portRaw, directionRaw, modeRaw)
		if err != nil {
			return nil, err
		}
		entry := ports.DeclarationPathEntry{Path: path, Route: route, AppliesFrom: appliesFrom.UTC()}
		if appliesUntil != nil {
			entry.AppliesUntil = appliesUntil.UTC()
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list declaration paths: %w", err)
	}
	return entries, nil
}
