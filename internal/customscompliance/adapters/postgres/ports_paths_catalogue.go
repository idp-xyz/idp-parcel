package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// PortsPathsCatalogue 实现 ports.PortsPathsCatalogueRead：口岸目录与申报路径目录两册
// 的列表读面（ADR-0077，票 admin-remainder-mechanism-batch/03）。读的就是 0013 那两张
// 登记册本表，不是第二份数据；点读口（PortsPathsView）按键与时点单点作答，这里按租户
// 上列——两种读法各答各的问题，谁也不为对方改形状。
//
// 两册按 ports 里的声明（CandidatePortPages / DeclarationPathPages）取页，缺省序是登记册
// 键升序：口岸按（口岸标识），路径按（路径标识）；同一选择键下的各版按生效起点升序——
// 决胜键与排序同向（ADR-0144 决定三，同解释规则上列）。全部版本连同区间原样上列，区间
// 判读留给读者；limit 非正是调用方编程错误（判据同 RuleCatalogue 那句）。
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

// candidatePortPages 把 ports.CandidatePortPages 落到列上。q 在口岸标识与展示名上做字面包含。
var candidatePortPages = ports.CandidatePortPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"port": "port_ref", "appliesFrom": "applies_from"},
	Identity: []string{"port_ref", "applies_from"},
	Keyword:  []string{"port_ref", "display_name"},
})

// scannedCandidatePort 留着键（口岸、生效起点）的原值：游标位置取自 ORDER BY 用的列，不取重建后的领域对象。
type scannedCandidatePort struct {
	entry       ports.CandidatePortEntry
	port        string
	appliesFrom time.Time
}

func (catalogue *PortsPathsCatalogue) ListCandidatePorts(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.CandidatePortEntry], error) {
	fail := func(err error) (cataloguepage.Result[ports.CandidatePortEntry], error) {
		return cataloguepage.Result[ports.CandidatePortEntry]{}, fmt.Errorf("list candidate ports: %w", err)
	}
	const source = "customs_compliance.candidate_port"
	page, err := openPage(ctx, catalogue.db, candidatePortPages, query, limit, source, "tenant_id = $1", tenant.String())
	if err != nil {
		return fail(err)
	}
	rows, err := page.rows(ctx, "port_ref, applies_from, applies_until, display_name", source)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]scannedCandidatePort, 0, limit+1)
	for rows.Next() {
		var row scannedCandidatePort
		var appliesUntil *time.Time
		var displayName *string
		if err := rows.Scan(&row.port, &row.appliesFrom, &appliesUntil, &displayName); err != nil {
			return fail(err)
		}
		// 坏行走构造门拦下上抛，不进查阅答案（判据同 RuleCatalogue 的逐行重建）。
		port, err := domain.NewCustomsPortReference(row.port)
		if err != nil {
			return fail(fmt.Errorf("rebuild candidate port: %w", err))
		}
		name, err := portDisplayNameOf(displayName)
		if err != nil {
			return fail(fmt.Errorf("rebuild candidate port: %w", err))
		}
		row.entry = ports.CandidatePortEntry{Port: port, DisplayName: name, AppliesFrom: row.appliesFrom.UTC()}
		// NULL 终点即开放版：零值照端口约定透出，不代填「无限远」的编造时刻。
		if appliesUntil != nil {
			row.entry.AppliesUntil = appliesUntil.UTC()
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	result, err := cataloguepage.Finish(query, fetched, limit, page.total,
		func(row scannedCandidatePort, sortField string) cataloguepage.Position {
			appliesFrom := cataloguepage.FormatInstant(row.appliesFrom)
			value := row.port
			if sortField == "appliesFrom" {
				value = appliesFrom
			}
			return cataloguepage.Position{Value: value, Identity: []string{row.port, appliesFrom}}
		})
	if err != nil {
		return fail(err)
	}
	return pageEntries(result, func(row scannedCandidatePort) ports.CandidatePortEntry { return row.entry }), nil
}

// declarationPathPages 把 ports.DeclarationPathPages 落到列上。q 在路径标识、所属口岸与申报方式上做字面包含。
var declarationPathPages = ports.DeclarationPathPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"path": "path_ref", "appliesFrom": "applies_from"},
	Identity: []string{"path_ref", "applies_from"},
	Filters:  map[string]string{"direction": "direction"},
	Keyword:  []string{"path_ref", "port_ref", "declaration_mode"},
})

// scannedDeclarationPath 留着键（路径、生效起点）的原值，理由同 scannedCandidatePort。
type scannedDeclarationPath struct {
	entry       ports.DeclarationPathEntry
	path        string
	appliesFrom time.Time
}

func (catalogue *PortsPathsCatalogue) ListDeclarationPaths(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.DeclarationPathEntry], error) {
	fail := func(err error) (cataloguepage.Result[ports.DeclarationPathEntry], error) {
		return cataloguepage.Result[ports.DeclarationPathEntry]{}, fmt.Errorf("list declaration paths: %w", err)
	}
	const source = "customs_compliance.declaration_path"
	page, err := openPage(ctx, catalogue.db, declarationPathPages, query, limit, source, "tenant_id = $1", tenant.String())
	if err != nil {
		return fail(err)
	}
	rows, err := page.rows(ctx, "path_ref, port_ref, direction, declaration_mode, applies_from, applies_until", source)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]scannedDeclarationPath, 0, limit+1)
	for rows.Next() {
		var row scannedDeclarationPath
		var portRaw, directionRaw, modeRaw string
		var appliesUntil *time.Time
		if err := rows.Scan(
			&row.path, &portRaw, &directionRaw, &modeRaw, &row.appliesFrom, &appliesUntil,
		); err != nil {
			return fail(err)
		}
		path, err := domain.NewDeclarationPathReference(row.path)
		if err != nil {
			return fail(fmt.Errorf("rebuild declaration path: %w", err))
		}
		route, err := rebuildDeclarationPathRoute(portRaw, directionRaw, modeRaw)
		if err != nil {
			return fail(err)
		}
		row.entry = ports.DeclarationPathEntry{Path: path, Route: route, AppliesFrom: row.appliesFrom.UTC()}
		if appliesUntil != nil {
			row.entry.AppliesUntil = appliesUntil.UTC()
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	result, err := cataloguepage.Finish(query, fetched, limit, page.total,
		func(row scannedDeclarationPath, sortField string) cataloguepage.Position {
			appliesFrom := cataloguepage.FormatInstant(row.appliesFrom)
			value := row.path
			if sortField == "appliesFrom" {
				value = appliesFrom
			}
			return cataloguepage.Position{Value: value, Identity: []string{row.path, appliesFrom}}
		})
	if err != nil {
		return fail(err)
	}
	return pageEntries(result, func(row scannedDeclarationPath) ports.DeclarationPathEntry { return row.entry }), nil
}
