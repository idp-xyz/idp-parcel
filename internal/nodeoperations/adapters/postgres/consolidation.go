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

// workFactSourceRow 是一次作业事实的来源在 jsonb 里的样子。四格与 domain.WorkFactSource
// 一一对应；迁移 0003 的在场 CHECK 按这几个键名判，改名要同改那一条。
type workFactSourceRow struct {
	SourceID    string    `json:"sourceId"`
	PerformedBy string    `json:"performedBy"`
	Evidence    string    `json:"evidence"`
	OccurredAt  time.Time `json:"occurredAt"`
}

// snapshotRow 里 SealedAt 与 Source.OccurredAt 同值——前者是既有列面，读回时以 Source
// 为准（重建门按来源自带的业务时间定快照时刻），保留它只为不改动已在读的 SQL 表达式。
type snapshotRow struct {
	Members  []string          `json:"members"`
	Seal     string            `json:"seal"`
	Basis    string            `json:"basis"`
	Source   workFactSourceRow `json:"source"`
	SealedAt time.Time         `json:"sealedAt"`
}

// FindByID 按（租户+实例）取回集运单元，只读、不取锁。否定结果只回 false。读回经重建门复验。
func (repository *ConsolidationUnits) FindByID(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.ConsolidationUnitID,
) (*domain.ConsolidationUnit, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("find consolidation: %w", err)
	}
	unit, found, err := findConsolidation(ctx, querier, consolidationRowQuery, tenant, id)
	if err != nil {
		return nil, false, fmt.Errorf("find consolidation: %w", err)
	}
	return unit, found, nil
}

// FindByIDForUpdate 同 FindByID，另在单元行上取 FOR NO KEY UPDATE 行锁到事务结束（为何要锁见
// ports.ConsolidationStore.FindByIDForUpdate）。取的正是随后那条 UPDATE 本来就要取的那一把，只是提前到读：读来改
// 之间互斥；别的事务插入引用这只单元的行时做的外键核对（FOR KEY SHARE）不陪着等——FOR UPDATE 会把它们也挡住，
// 换不来什么，单元的键从不改。READ COMMITTED 下，等到前一笔提交之后这一读拿到的是提交后的那一版，不是开始等时的
// 那一版。走 RequireExecutor：连接池上取的锁随那条语句自动提交当场放掉。
func (repository *ConsolidationUnits) FindByIDForUpdate(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.ConsolidationUnitID,
) (*domain.ConsolidationUnit, bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("find consolidation for update: %w", err)
	}
	unit, found, err := findConsolidation(ctx, executor, consolidationRowQuery+`
		    FOR NO KEY UPDATE`, tenant, id)
	if err != nil {
		return nil, false, fmt.Errorf("find consolidation for update: %w", err)
	}
	return unit, found, nil
}

const consolidationRowQuery = `SELECT asset_ref, phase, opened_source, members, snapshots, closed_at
		   FROM node_operations.consolidation_unit
		  WHERE tenant_id = $1
		    AND unit_id = $2`

func findConsolidation(
	ctx context.Context,
	querier bentopg.Querier,
	query string,
	tenant domain.TenantID,
	id domain.ConsolidationUnitID,
) (*domain.ConsolidationUnit, bool, error) {
	var assetRef, phase string
	var openedSourceJSON, membersJSON, snapshotsJSON []byte
	var closedAt *time.Time
	err := querier.QueryRow(ctx, query, tenant.String(), id.String()).
		Scan(&assetRef, &phase, &openedSourceJSON, &membersJSON, &snapshotsJSON, &closedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	unit, err := rebuildConsolidation(id, assetRef, phase, openedSourceJSON, membersJSON, snapshotsJSON, closedAt)
	if err != nil {
		return nil, false, err
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
	openedSourceJSON, err := json.Marshal(sourceRowOf(unit.OpenedBy()))
	if err != nil {
		return ports.ConsolidationSaveOutcomeInvalid, fmt.Errorf("save consolidation: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.consolidation_unit
			(tenant_id, unit_id, asset_ref, phase, opened_source, members, snapshots, closed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT DO NOTHING`,
		tenant.String(),
		unit.ID().String(),
		unit.Asset().String(),
		consolidationPhaseOf(unit),
		openedSourceJSON,
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

// Update 落加入/移出/封装/开封/关闭这一步状态推进：身份、载具与开启来源在开启时固定，
// 重写等值也不给入口——每一步各自的来源事实归 ConsolidationFacts 登记，不回头改这一行。
// 行不存在如实报错；租户条件进语句（ADR-0003）。
//
// 交进来的单元按整行写，它的每个成员都重新插进容纳索引（已在的不动）、不在它身上的本单元容纳行删掉；这里不比
// 版本，分不出这份单元是什么时候读的。所以它只该收 FindByIDForUpdate 读出来的单元：旧读数会盖掉别的事务提交的
// 推进，并把别的事务刚删掉的边写回（ports.ConsolidationStore.FindByIDForUpdate）。
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

// LineageForUpdate 沿当前容纳索引从目标单元逐级上溯到根，自身在先；先取（租户）的事务级咨询锁，再读。
//
// 锁按租户而不按链上的行：要锁的那几行在走完链之前不知道是哪几行，边走边锁的次序随链而异、两笔反向走会互等。
// 只把移入串起来就够，前提是新边只由移入加：移出、封装、开封、关闭经 Update 把单元的成员整份写回容纳索引，它们
// 读单元走 FindByIDForUpdate、持着单元行锁，写回的只是当时已提交的那几条边，删边只会让链路变短。哪一口改用
// 不持锁的旧读数，就能把别的事务刚删掉的边写回，而写回不判环——这把锁挡不住那一种。CHECK 引用不了别的行，
// 这一半库面守不住。走 RequireExecutor：连接池上取到的锁随那条语句自动提交当场放掉，读链与落成员之间不再串行。
// 键照本仓其余事务级锁的双 int 形式，撞上只让两个租户多等一会儿。同一事务里再取是重入。
//
// 上溯用 path 数组带着走过的各级，再次踏上已走过的一级即停并交回 ErrContainmentLineageCyclic：册已成环时不带 path
// 的递归不会终止，而成环的册不是这一次移入能判的东西。每件对象至多一个直接父级（容纳主键），所以链路是一条线，
// 按 path 长度排序就是从目标到根的次序。
func (repository *ConsolidationUnits) LineageForUpdate(
	ctx context.Context,
	tenant domain.TenantID,
	unit domain.ConsolidationUnitID,
) (domain.ContainmentLineage, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("containment lineage: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('node_operations.containment_current.tenant'), hashtext($1))`,
		tenant.String(),
	); err != nil {
		return nil, fmt.Errorf("containment lineage: 取租户成员关系锁：%w", err)
	}

	rows, err := executor.Query(ctx,
		`WITH RECURSIVE lineage AS (
		     SELECT $2::text AS unit_id, ARRAY[$2::text] AS path, false AS revisited
		      UNION ALL
		     SELECT parent.unit_id, step.path || parent.unit_id, parent.unit_id = ANY (step.path)
		       FROM lineage AS step
		       JOIN node_operations.containment_current AS parent
		         ON parent.tenant_id = $1 AND parent.member_id = step.unit_id
		      WHERE NOT step.revisited
		 )
		 SELECT unit_id, revisited
		   FROM lineage
		  ORDER BY cardinality(path)`,
		tenant.String(),
		unit.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("containment lineage: %w", err)
	}
	defer rows.Close()

	var lineage domain.ContainmentLineage
	for rows.Next() {
		var raw string
		var revisited bool
		if err := rows.Scan(&raw, &revisited); err != nil {
			return nil, fmt.Errorf("containment lineage: %w", err)
		}
		if revisited {
			return nil, fmt.Errorf("containment lineage: %w: %s reached again above %s", ports.ErrContainmentLineageCyclic, raw, unit)
		}
		ancestor, err := domain.NewConsolidationUnitID(raw)
		if err != nil {
			return nil, fmt.Errorf("containment lineage: %w", err)
		}
		lineage = append(lineage, ancestor)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("containment lineage: %w", err)
	}
	return lineage, nil
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
			Source:   sourceRowOf(snapshot.Source()),
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
	openedSourceJSON, membersJSON, snapshotsJSON []byte,
	closedAt *time.Time,
) (*domain.ConsolidationUnit, error) {
	asset, err := domain.NewCarrierAssetReference(assetRef)
	if err != nil {
		return nil, err
	}

	var openedSourceRow workFactSourceRow
	if err := json.Unmarshal(openedSourceJSON, &openedSourceRow); err != nil {
		return nil, err
	}
	openedBy, err := rebuildWorkFactSource(openedSourceRow)
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
		source, err := rebuildWorkFactSource(row.Source)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, domain.RehydrateSealedSnapshotSpec{
			Members: snapMembers,
			Seal:    seal,
			Basis:   basis,
			Source:  source,
		})
	}

	spec := domain.RehydrateConsolidationUnitSpec{
		ID:        id,
		Asset:     asset,
		OpenedBy:  openedBy,
		Phase:     phase,
		Members:   members,
		Snapshots: snapshots,
	}
	if closedAt != nil {
		spec.ClosedAt = closedAt.UTC()
	}
	return domain.RehydrateConsolidationUnit(spec)
}

func sourceRowOf(source domain.WorkFactSource) workFactSourceRow {
	return workFactSourceRow{
		SourceID:    source.SourceID(),
		PerformedBy: source.PerformedBy().String(),
		Evidence:    source.Evidence().String(),
		OccurredAt:  source.OccurredAt().UTC(),
	}
}

// rebuildWorkFactSource 经领域构造器回到来源值对象——行数据不直接当合法来源用，
// 与重建门同一条纪律：缺格或空白的行在读回时就被拦下，不留到业务代码里。
func rebuildWorkFactSource(row workFactSourceRow) (domain.WorkFactSource, error) {
	performedBy, err := domain.NewPerformingPartyReference(row.PerformedBy)
	if err != nil {
		return domain.WorkFactSource{}, err
	}
	evidence, err := domain.NewExecutionEvidenceReference(row.Evidence)
	if err != nil {
		return domain.WorkFactSource{}, err
	}
	return domain.NewWorkFactSource(row.SourceID, performedBy, evidence, row.OccurredAt)
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
