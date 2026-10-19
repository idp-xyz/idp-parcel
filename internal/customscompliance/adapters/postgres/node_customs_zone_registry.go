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

// NodeCustomsZoneRegistrations 实现 ports.NodeCustomsZoneRegistry（0024 建表，ADR-0206）。版本代数逐句照
// PortsPathsRegistrations：一律不 UPSERT，撞键与撞重叠都由无 conflict target 的 DO NOTHING 折成`已登记`；唯一的
// UPDATE 是换版给开放前版落终点，起点与区域列没有任何改写路径。
type NodeCustomsZoneRegistrations struct {
	db *bentopg.DB
}

func NewNodeCustomsZoneRegistrations(db *bentopg.DB) (*NodeCustomsZoneRegistrations, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &NodeCustomsZoneRegistrations{db: db}, nil
}

var _ ports.NodeCustomsZoneRegistry = (*NodeCustomsZoneRegistrations)(nil)

// RegisterNodeCustomsZone 登记一版节点区域。起点在键上且无默认可言（判据同口岸目录写口的零起点门）。
func (registry *NodeCustomsZoneRegistrations) RegisterNodeCustomsZone(
	ctx context.Context,
	tenant domain.TenantID,
	node domain.NetworkNodeReference,
	zone domain.CustomsZoneReference,
	appliesFrom time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	if appliesFrom.IsZero() {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register node customs zone: the applicable interval has no start")
	}
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register node customs zone: %w", err)
	}

	// 换版：先给同支下起点更早的开放版落终点，后继才插得进——开放区间与任何更晚起点在排他约束上相斥。
	if _, err := executor.Exec(ctx,
		`UPDATE customs_compliance.node_customs_zone
		    SET applies_until = $3
		  WHERE tenant_id = $1 AND node_ref = $2
		    AND applies_until IS NULL AND applies_from < $3`,
		tenant.String(), node.String(), appliesFrom.UTC(),
	); err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register node customs zone: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.node_customs_zone
			(tenant_id, node_ref, applies_from, zone_ref)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), node.String(), appliesFrom.UTC(), zone.String(),
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register node customs zone: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

// NodeCustomsZonePointView 实现 ports.NodeCustomsZoneView：按（租户，节点）在评估时点上解析区域版本，半开区间与
// 口岸目录读口同形，至多一行。found=false 即那一刻没有在册版本——不拿开放版或系统当前时间顶替业务时点。
type NodeCustomsZonePointView struct {
	db *bentopg.DB
}

func NewNodeCustomsZonePointView(db *bentopg.DB) (*NodeCustomsZonePointView, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &NodeCustomsZonePointView{db: db}, nil
}

var _ ports.NodeCustomsZoneView = (*NodeCustomsZonePointView)(nil)

func (view *NodeCustomsZonePointView) LoadNodeCustomsZone(
	ctx context.Context,
	tenant domain.TenantID,
	node domain.NetworkNodeReference,
	evaluatedAt time.Time,
) (ports.NodeCustomsZoneEntry, bool, error) {
	none := ports.NodeCustomsZoneEntry{}
	// 空键与零时点是调用方编程错误，不查成「未登记」——那会把坏输入折成一个像样的业务答案。
	if strings.TrimSpace(node.String()) == "" {
		return none, false, fmt.Errorf("load node customs zone: the node reference is blank")
	}
	if evaluatedAt.IsZero() {
		return none, false, fmt.Errorf("load node customs zone: the evaluation instant is zero")
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load node customs zone: %w", err)
	}

	var (
		zoneRaw      string
		appliesFrom  time.Time
		appliesUntil *time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT zone_ref, applies_from, applies_until
		   FROM customs_compliance.node_customs_zone
		  WHERE tenant_id = $1 AND node_ref = $2
		    AND applies_from <= $3
		    AND (applies_until IS NULL OR applies_until > $3)`,
		tenant.String(), node.String(), evaluatedAt.UTC(),
	).Scan(&zoneRaw, &appliesFrom, &appliesUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load node customs zone: %w", err)
	}
	zone, err := domain.NewCustomsZoneReference(zoneRaw)
	if err != nil {
		return none, false, fmt.Errorf("load node customs zone: %w", err)
	}
	entry := ports.NodeCustomsZoneEntry{Node: node, Zone: zone, AppliesFrom: appliesFrom.UTC()}
	if appliesUntil != nil {
		entry.AppliesUntil = appliesUntil.UTC()
	}
	return entry, true, nil
}
