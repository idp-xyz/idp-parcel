package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// PortsPathsSnapshots 实现 ports.PortsPathsSnapshotView：按租户取两本目录的全量在册行
// （票 routing-first-cut/12）。它不按点读——点读供登记编排与判断链按键取当刻版本，这里
// 供关务适用性判断折「目录为空 / 口岸未登记 / 口岸未生效」三格，全量快照才折得开；
// 目录换版只在前版终点上落一下，全量读回的行集就是判断依据的全貌。
//
// 行序按（引用，生效起点）排定：同一份数据两次读回同一行序，出处里的版本引用与判断
// 标识才可重算（判据三靠它）。
type PortsPathsSnapshots struct {
	db *bentopg.DB
}

func NewPortsPathsSnapshots(db *bentopg.DB) (*PortsPathsSnapshots, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &PortsPathsSnapshots{db: db}, nil
}

var _ ports.PortsPathsSnapshotView = (*PortsPathsSnapshots)(nil)

func (view *PortsPathsSnapshots) LoadPortsPathsSnapshot(
	ctx context.Context,
	tenant domain.TenantID,
) (ports.PortsPathsSnapshot, error) {
	none := ports.PortsPathsSnapshot{}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, fmt.Errorf("load ports paths snapshot: %w", err)
	}

	portRows, err := querier.Query(ctx,
		`SELECT port_ref, applies_from, applies_until
		   FROM customs_compliance.candidate_port
		  WHERE tenant_id = $1
		  ORDER BY port_ref, applies_from`,
		tenant.String(),
	)
	if err != nil {
		return none, fmt.Errorf("load ports paths snapshot: %w", err)
	}
	defer portRows.Close()

	snapshot := ports.PortsPathsSnapshot{}
	for portRows.Next() {
		var (
			portRaw      string
			appliesFrom  time.Time
			appliesUntil *time.Time
		)
		if err := portRows.Scan(&portRaw, &appliesFrom, &appliesUntil); err != nil {
			return none, fmt.Errorf("load ports paths snapshot: %w", err)
		}
		port, err := domain.NewCustomsPortReference(portRaw)
		if err != nil {
			return none, fmt.Errorf("load ports paths snapshot: %w", err)
		}
		entry := ports.CandidatePortEntry{Port: port, AppliesFrom: appliesFrom.UTC()}
		if appliesUntil != nil {
			entry.AppliesUntil = appliesUntil.UTC()
		}
		snapshot.Ports = append(snapshot.Ports, entry)
	}
	if err := portRows.Err(); err != nil {
		return none, fmt.Errorf("load ports paths snapshot: %w", err)
	}

	pathRows, err := querier.Query(ctx,
		`SELECT path_ref, port_ref, direction, declaration_mode, applies_from, applies_until
		   FROM customs_compliance.declaration_path
		  WHERE tenant_id = $1
		  ORDER BY path_ref, applies_from`,
		tenant.String(),
	)
	if err != nil {
		return none, fmt.Errorf("load ports paths snapshot: %w", err)
	}
	defer pathRows.Close()

	for pathRows.Next() {
		var (
			pathRaw, portRaw, directionRaw, modeRaw string
			appliesFrom                             time.Time
			appliesUntil                            *time.Time
		)
		if err := pathRows.Scan(
			&pathRaw, &portRaw, &directionRaw, &modeRaw, &appliesFrom, &appliesUntil); err != nil {
			return none, fmt.Errorf("load ports paths snapshot: %w", err)
		}
		path, err := domain.NewDeclarationPathReference(pathRaw)
		if err != nil {
			return none, fmt.Errorf("load ports paths snapshot: %w", err)
		}
		route, err := rebuildDeclarationPathRoute(portRaw, directionRaw, modeRaw)
		if err != nil {
			return none, fmt.Errorf("load ports paths snapshot: %w", err)
		}
		entry := ports.DeclarationPathEntry{Path: path, Route: route, AppliesFrom: appliesFrom.UTC()}
		if appliesUntil != nil {
			entry.AppliesUntil = appliesUntil.UTC()
		}
		snapshot.Paths = append(snapshot.Paths, entry)
	}
	if err := pathRows.Err(); err != nil {
		return none, fmt.Errorf("load ports paths snapshot: %w", err)
	}
	return snapshot, nil
}
