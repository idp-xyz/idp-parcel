package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ConsolidationUnits 实现 ports.ConsolidationStore 与 ports.ContainmentIndex。
// 当前容纳行与单元行同事务改写：加入先插容纳（撞主键说明已在别处），再改单元，
// 最后删本单元不再持有的行——这一顺序保证调用方即便把错误吞掉并提交，也不会
// 留下「单元已含成员、容纳索引还指着别人」或「删光索引却没写成关闭」的半截。
type ConsolidationUnits struct {
	db *bentopg.DB
}

func NewConsolidationUnits(db *bentopg.DB) (*ConsolidationUnits, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &ConsolidationUnits{db: db}, nil
}

type snapshotRow struct {
	Members  []string  `json:"members"`
	Seal     string    `json:"seal"`
	Basis    string    `json:"basis"`
	SealedAt time.Time `json:"sealedAt"`
}

// FindByID 按（租户+实例）取回集运单元。否定结果只回 false。读回经重建门复验。
func (repository *ConsolidationUnits) FindByID(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.ConsolidationUnitID,
) (*domain.ConsolidationUnit, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("find consolidation: %w", err)
	}

	var assetRef, phase string
	var membersJSON, snapshotsJSON []byte
	var closedAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT asset_ref, phase, members, snapshots, closed_at
		   FROM node_operations.consolidation_unit
		  WHERE tenant_id = $1
		    AND unit_id = $2`,
		tenant.String(),
		id.String(),
	).Scan(&assetRef, &phase, &membersJSON, &snapshotsJSON, &closedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find consolidation: %w", err)
	}

	unit, err := rebuildConsolidation(id, assetRef, phase, membersJSON, snapshotsJSON, closedAt)
	if err != nil {
		return nil, false, fmt.Errorf("find consolidation: %w", err)
	}
	return unit, true, nil
}

// Save 开启一个集运单元实例。同 ID 已有记录时答`已有记录`，不覆盖先到者。
func (repository *ConsolidationUnits) Save(
	ctx context.Context,
	tenant domain.TenantID,
	unit *domain.ConsolidationUnit,
) (ports.ConsolidationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ConsolidationSaveOutcomeInvalid, fmt.Errorf("save consolidation: %w", err)
	}

	membersJSON, snapshotsJSON, err := marshalConsolidation(unit)
	if err != nil {
		return ports.ConsolidationSaveOutcomeInvalid, fmt.Errorf("save consolidation: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.consolidation_unit
			(tenant_id, unit_id, asset_ref, phase, members, snapshots, closed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT DO NOTHING`,
		tenant.String(),
		unit.ID().String(),
		unit.Asset().String(),
		consolidationPhaseOf(unit),
		membersJSON,
		snapshotsJSON,
		closedAtColumn(unit),
	)
	if err != nil {
		return ports.ConsolidationSaveOutcomeInvalid, fmt.Errorf("save consolidation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ConsolidationAlreadyRecorded, nil
	}
	if err := repository.syncContainment(ctx, executor, tenant, unit); err != nil {
		return ports.ConsolidationSaveOutcomeInvalid, fmt.Errorf("save consolidation: %w", err)
	}
	return ports.ConsolidationSaved, nil
}

// Update 落加入/移出/封装/开封/关闭这一步状态推进：身份与载具在开启时固定，重写
// 等值也不给入口。行不存在如实报错；租户条件进语句（ADR-0003）。
func (repository *ConsolidationUnits) Update(
	ctx context.Context,
	tenant domain.TenantID,
	unit *domain.ConsolidationUnit,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("update consolidation: %w", err)
	}

	// 未关闭时先占容纳行：撞别人的主键在改单元之前失败，调用方提交也不会留下半截成员。
	if !unit.Closed() {
		if err := repository.insertDesiredContainment(ctx, executor, tenant, unit); err != nil {
			return fmt.Errorf("update consolidation: %w", err)
		}
	}

	membersJSON, snapshotsJSON, err := marshalConsolidation(unit)
	if err != nil {
		return fmt.Errorf("update consolidation: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE node_operations.consolidation_unit
		    SET phase = $3, members = $4, snapshots = $5, closed_at = $6, updated_at = now()
		  WHERE tenant_id = $1 AND unit_id = $2`,
		tenant.String(),
		unit.ID().String(),
		consolidationPhaseOf(unit),
		membersJSON,
		snapshotsJSON,
		closedAtColumn(unit),
	)
	if err != nil {
		return fmt.Errorf("update consolidation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update consolidation: %s not found", unit.ID())
	}

	if err := repository.dropReleasedContainment(ctx, executor, tenant, unit); err != nil {
		return fmt.Errorf("update consolidation: %w", err)
	}
	return nil
}

// CurrentParent 查未关闭单元里谁直接包含这件实物。关闭后的成员不再占据当前父级。
func (repository *ConsolidationUnits) CurrentParent(
	ctx context.Context,
	tenant domain.TenantID,
	member domain.HandlingUnitID,
) (domain.ConsolidationUnitID, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ConsolidationUnitID{}, false, fmt.Errorf("current parent: %w", err)
	}

	var unitID string
	err = querier.QueryRow(ctx,
		`SELECT unit_id
		   FROM node_operations.containment_current
		  WHERE tenant_id = $1
		    AND member_id = $2`,
		tenant.String(),
		member.String(),
	).Scan(&unitID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ConsolidationUnitID{}, false, nil
	}
	if err != nil {
		return domain.ConsolidationUnitID{}, false, fmt.Errorf("current parent: %w", err)
	}
	id, err := domain.NewConsolidationUnitID(unitID)
	if err != nil {
		return domain.ConsolidationUnitID{}, false, fmt.Errorf("current parent: %w", err)
	}
	return id, true, nil
}

func (repository *ConsolidationUnits) syncContainment(
	ctx context.Context,
	executor bentopg.Executor,
	tenant domain.TenantID,
	unit *domain.ConsolidationUnit,
) error {
	if !unit.Closed() {
		if err := repository.insertDesiredContainment(ctx, executor, tenant, unit); err != nil {
			return err
		}
	}
	return repository.dropReleasedContainment(ctx, executor, tenant, unit)
}

func (repository *ConsolidationUnits) insertDesiredContainment(
	ctx context.Context,
	executor bentopg.Executor,
	tenant domain.TenantID,
	unit *domain.ConsolidationUnit,
) error {
	for _, member := range unit.Members() {
		tag, err := executor.Exec(ctx,
			`INSERT INTO node_operations.containment_current (tenant_id, member_id, unit_id)
			 VALUES ($1, $2, $3)
			 ON CONFLICT DO NOTHING`,
			tenant.String(),
			member.String(),
			unit.ID().String(),
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			continue
		}
		var parent string
		err = executor.QueryRow(ctx,
			`SELECT unit_id
			   FROM node_operations.containment_current
			  WHERE tenant_id = $1 AND member_id = $2`,
			tenant.String(),
			member.String(),
		).Scan(&parent)
		if err != nil {
			return err
		}
		if parent != unit.ID().String() {
			return fmt.Errorf("member %s already contained by %s", member.String(), parent)
		}
	}
	return nil
}

func (repository *ConsolidationUnits) dropReleasedContainment(
	ctx context.Context,
	executor bentopg.Executor,
	tenant domain.TenantID,
	unit *domain.ConsolidationUnit,
) error {
	if unit.Closed() {
		_, err := executor.Exec(ctx,
			`DELETE FROM node_operations.containment_current
			  WHERE tenant_id = $1 AND unit_id = $2`,
			tenant.String(),
			unit.ID().String(),
		)
		return err
	}

	kept := make([]string, 0, len(unit.Members()))
	for _, member := range unit.Members() {
		kept = append(kept, member.String())
	}
	_, err := executor.Exec(ctx,
		`DELETE FROM node_operations.containment_current
		  WHERE tenant_id = $1
		    AND unit_id = $2
		    AND NOT (member_id = ANY($3))`,
		tenant.String(),
		unit.ID().String(),
		kept,
	)
	return err
}

func marshalConsolidation(unit *domain.ConsolidationUnit) ([]byte, []byte, error) {
	memberIDs := make([]string, 0, len(unit.Members()))
	for _, member := range unit.Members() {
		memberIDs = append(memberIDs, member.String())
	}
	membersJSON, err := json.Marshal(memberIDs)
	if err != nil {
		return nil, nil, err
	}

	snapshots := unit.Snapshots()
	rows := make([]snapshotRow, 0, len(snapshots))
	for _, snapshot := range snapshots {
		members := make([]string, 0, len(snapshot.Members()))
		for _, member := range snapshot.Members() {
			members = append(members, member.String())
		}
		rows = append(rows, snapshotRow{
			Members:  members,
			Seal:     snapshot.Seal().String(),
			Basis:    snapshot.Basis().String(),
			SealedAt: snapshot.SealedAt().UTC(),
		})
	}
	snapshotsJSON, err := json.Marshal(rows)
	if err != nil {
		return nil, nil, err
	}
	return membersJSON, snapshotsJSON, nil
}

func rebuildConsolidation(
	id domain.ConsolidationUnitID,
	assetRef, phase string,
	membersJSON, snapshotsJSON []byte,
	closedAt *time.Time,
) (*domain.ConsolidationUnit, error) {
	asset, err := domain.NewCarrierAssetReference(assetRef)
	if err != nil {
		return nil, err
	}

	var memberIDs []string
	if err := json.Unmarshal(membersJSON, &memberIDs); err != nil {
		return nil, err
	}
	members := make([]domain.HandlingUnitID, 0, len(memberIDs))
	for _, raw := range memberIDs {
		member, err := domain.NewHandlingUnitID(raw)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}

	var rows []snapshotRow
	if err := json.Unmarshal(snapshotsJSON, &rows); err != nil {
		return nil, err
	}
	snapshots := make([]domain.RehydrateSealedSnapshotSpec, 0, len(rows))
	for _, row := range rows {
		snapMembers := make([]domain.HandlingUnitID, 0, len(row.Members))
		for _, raw := range row.Members {
			member, err := domain.NewHandlingUnitID(raw)
			if err != nil {
				return nil, err
			}
			snapMembers = append(snapMembers, member)
		}
		seal, err := domain.NewSealReference(row.Seal)
		if err != nil {
			return nil, err
		}
		basis, err := domain.NewWorkBasisReference(row.Basis)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, domain.RehydrateSealedSnapshotSpec{
			Members:  snapMembers,
			Seal:     seal,
			Basis:    basis,
			SealedAt: row.SealedAt,
		})
	}

	spec := domain.RehydrateConsolidationUnitSpec{
		ID:        id,
		Asset:     asset,
		Phase:     phase,
		Members:   members,
		Snapshots: snapshots,
	}
	if closedAt != nil {
		spec.ClosedAt = closedAt.UTC()
	}
	return domain.RehydrateConsolidationUnit(spec)
}

func consolidationPhaseOf(unit *domain.ConsolidationUnit) string {
	if unit.Closed() {
		return domain.ConsolidationPhaseClosed
	}
	if unit.Sealed() {
		return domain.ConsolidationPhaseSealed
	}
	return domain.ConsolidationPhaseOpen
}

func closedAtColumn(unit *domain.ConsolidationUnit) *time.Time {
	if !unit.Closed() {
		return nil
	}
	at := unit.ClosedAt().UTC()
	return &at
}
