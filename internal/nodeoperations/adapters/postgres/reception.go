// Package postgres 是 node-operations 自有语义端口的 PostgreSQL 适配器。
//
// 显式 SQL、行模型与写入代数翻译都留在这里，不进领域对象。所有语句显式携带租户
// 条件：按 ADR-0003 运营集团租户是最高数据隔离边界，缺了它另一个租户的同名来源
// 标识就会被当成同一次回传。
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// Receptions 实现 ports.ReceptionStore（写入代数同 ADR-0031）。
type Receptions struct {
	db *bentopg.DB
}

func NewReceptions(db *bentopg.DB) (*Receptions, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &Receptions{db: db}, nil
}

// intakeRow 与 controlRow 是 jsonb 列的行模型，只在本包存在；领域对象经构造函数
// 重建，行模型不外泄。
type intakeRow struct {
	Unit        string    `json:"unit"`
	Node        string    `json:"node"`
	DeliveredBy string    `json:"deliveredBy"`
	Evidence    string    `json:"evidence"`
	Version     string    `json:"version"`
	Association string    `json:"association,omitempty"`
	ReceivedAt  time.Time `json:"receivedAt"`
}

type controlRow struct {
	Unit          string     `json:"unit"`
	Node          string     `json:"node"`
	Kind          string     `json:"kind"`
	Basis         string     `json:"basis"`
	EstablishedAt time.Time  `json:"establishedAt"`
	ReleasedBy    string     `json:"releasedBy,omitempty"`
	ReleasedAt    *time.Time `json:"releasedAt,omitempty"`
}

// FindByKey 按（租户+来源标识）取回已保存的收寄判断。
//
// 走 ReadExecutor：事务内读得到本事务刚写的行，事务外用显式注入的连接池。否定结果
// 只回 false，不区分「不存在」与「属于另一个租户」。
func (repository *Receptions) FindByKey(
	ctx context.Context,
	key ports.ReceptionKey,
) (ports.ReceptionRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ReceptionRecord{}, false, fmt.Errorf("find reception: %w", err)
	}

	row := querier.QueryRow(ctx,
		`SELECT `+receptionColumns+`
		   FROM node_operations.reception
		  WHERE tenant_id = $1
		    AND source_id = $2`,
		key.TenantID.String(),
		key.SourceID,
	)
	record, err := scanReception(row, key.TenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ReceptionRecord{}, false, nil
	}
	if err != nil {
		return ports.ReceptionRecord{}, false, fmt.Errorf("find reception: %w", err)
	}
	return record, true, nil
}

// FindOtherUnitIntakes 按（租户+正式包裹关联）取回别的作业实物的收寄记录（ADR-0196），走索引 reception_intake_association_idx。
// 只认收寄本身带的关联——待识别行的候选不是关联，不在此列。
//
// 查询之前按（租户、包裹关联）取事务级咨询锁，把同一件包裹上「读别的实物关联 → 落收寄」串起来（票
// no-reception-identity/03）。没有它，两件贴同一个号的实物在两笔并发事务里送站，READ COMMITTED 下各自读不到对方尚未
// 提交的关联，两件都关联成立。后到的一方等在锁上，先到的提交后才查；READ COMMITTED 下这条查询另取快照，读得到刚提交的
// 关联。锁随事务释放，所以走 RequireExecutor：没有事务时锁在取锁那条语句结束就放掉，读完到落册之间不再串行，静默放过
// 比报错更糟。锁键各维以 chr(31) 分隔后取 hashtext，口径同 access_identity 的 lockMappingKey；哈希撞上只多等一会儿，不错判。
//
// 被身份冲突处置版接续过的行不交回（ADR-0210 决定八），谓词见 succeededByDisposition；截断在取锁之后的同一条查询里判，处置版与
// 它截断的那几行在同一把锁下落册，读到的不会是半截处置。
func (repository *Receptions) FindOtherUnitIntakes(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelAssociationReference,
	unit domain.HandlingUnitID,
) ([]ports.ReceptionRecord, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find other unit intakes: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('node_operations.reception.parcel_association'), hashtext($1 || chr(31) || $2))`,
		tenant.String(), parcel.String(),
	); err != nil {
		return nil, fmt.Errorf("find other unit intakes: 取包裹锁：%w", err)
	}
	rows, err := executor.Query(ctx,
		`SELECT `+receptionColumns+`
		   FROM node_operations.reception
		  WHERE tenant_id = $1
		    AND (intake ->> 'association') = $2
		    AND (intake ->> 'unit') <> $3
		    AND NOT `+succeededByDisposition("reception")+`
		  ORDER BY source_id`,
		tenant.String(),
		parcel.String(),
		unit.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("find other unit intakes: %w", err)
	}
	defer rows.Close()

	records := []ports.ReceptionRecord{}
	for rows.Next() {
		record, err := scanReception(rows, tenant)
		if err != nil {
			return nil, fmt.Errorf("find other unit intakes: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find other unit intakes: %w", err)
	}
	return records, nil
}

// maxJudgmentChain 给沿后继走的那条递归查询封顶。（租户，前版）唯一保证链不分叉，却拦不住绕过编排写出的环；封顶让环报错，
// 而不是让查询不收敛。一条链上的版本是重核、冲突与续办逐次追加的，远到不了这个数。
const maxJudgmentChain = 1024

// CurrentVersion 沿后继走到头（ADR-0199 决定二），一条递归查询走完。每一步按（租户，前版）找唯一的那一行，走索引
// reception_predecessor_unique。走 ReadExecutor：事务内读得到本事务刚追加的那一版。
func (repository *Receptions) CurrentVersion(
	ctx context.Context,
	tenant domain.TenantID,
	version domain.IntakeResultVersion,
) (domain.IntakeResultVersion, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.IntakeResultVersion{}, fmt.Errorf("current judgment version: %w", err)
	}
	var current string
	var depth int
	if err := querier.QueryRow(ctx,
		`WITH RECURSIVE chain (version, depth) AS (
		     SELECT $2::text, 0
		   UNION ALL
		     SELECT r.judgment_version, chain.depth + 1
		       FROM chain
		       JOIN node_operations.reception r
		         ON r.tenant_id = $1
		        AND r.predecessor_version = chain.version
		      WHERE chain.depth < $3
		 )
		 SELECT version, depth FROM chain ORDER BY depth DESC LIMIT 1`,
		tenant.String(), version.String(), maxJudgmentChain,
	).Scan(&current, &depth); err != nil {
		return domain.IntakeResultVersion{}, fmt.Errorf("current judgment version: %w", err)
	}
	if depth >= maxJudgmentChain {
		return domain.IntakeResultVersion{}, fmt.Errorf("current judgment version: chain from %q does not end within %d versions", version, maxJudgmentChain)
	}
	return domain.NewIntakeResultVersion(current)
}

// receptionColumns 与 scanReception 的扫描顺序是同一份约定，两处一起改。
const receptionColumns = `source_id, content_digest, kind, intake, control, candidates,
		        identity_conflict, refusal_reason, service_markers, service_result_check, recorded_at,
		        judgment_version, predecessor_version, source_document, merged_version, disposition_document`

// scanReception 把一行收寄册读回成记录：读回的一切经领域构造函数重建，一次坏写入在这里暴露，而不是变成一个看起来
// 合法的收寄结果。
func scanReception(row pgx.Row, tenant domain.TenantID) (ports.ReceptionRecord, error) {
	var (
		sourceID, digest, kindName  string
		refusal                     string
		intakeJSON, controlJSON     []byte
		candidatesJSON, markersJSON []byte
		identityConflict            bool
		checkName                   *string
		recordedAt                  time.Time
		judgment, predecessor       *string
		sourceJSON                  []byte
		merged                      *string
		dispositionJSON             []byte
	)
	if err := row.Scan(&sourceID, &digest, &kindName, &intakeJSON, &controlJSON, &candidatesJSON,
		&identityConflict, &refusal, &markersJSON, &checkName, &recordedAt,
		&judgment, &predecessor, &sourceJSON, &merged, &dispositionJSON); err != nil {
		return ports.ReceptionRecord{}, err
	}

	kind, err := receptionKindFrom(kindName)
	if err != nil {
		return ports.ReceptionRecord{}, err
	}
	check, err := serviceResultCheckFrom(checkName)
	if err != nil {
		return ports.ReceptionRecord{}, err
	}
	record := ports.ReceptionRecord{
		Key:                ports.ReceptionKey{TenantID: tenant, SourceID: sourceID},
		ContentDigest:      digest,
		Kind:               kind,
		IdentityConflict:   identityConflict,
		RefusalReason:      refusal,
		ServiceResultCheck: check,
		RecordedAt:         recordedAt.UTC(),
	}

	if record.JudgmentVersion, err = versionFrom(judgment); err != nil {
		return ports.ReceptionRecord{}, err
	}
	if record.Predecessor, err = versionFrom(predecessor); err != nil {
		return ports.ReceptionRecord{}, err
	}
	if record.MergedInto, err = versionFrom(merged); err != nil {
		return ports.ReceptionRecord{}, err
	}
	// 来源文档为空是来源保全之前的行，读回零值即「来源未保全」，不是坏行。
	if len(sourceJSON) > 0 {
		if record.Source, err = domain.ParseDeliverySource(sourceJSON); err != nil {
			return ports.ReceptionRecord{}, err
		}
	}

	if len(dispositionJSON) > 0 {
		if record.Disposition, err = domain.ParseIdentityDisposition(dispositionJSON); err != nil {
			return ports.ReceptionRecord{}, err
		}
	}

	if len(intakeJSON) > 0 {
		intake, err := rebuildIntake(tenant, intakeJSON)
		if err != nil {
			return ports.ReceptionRecord{}, err
		}
		record.Intake = intake
	}
	if len(controlJSON) > 0 {
		control, err := rebuildControl(tenant, controlJSON)
		if err != nil {
			return ports.ReceptionRecord{}, err
		}
		record.Control = control
	}

	var candidateRefs []string
	if err := json.Unmarshal(candidatesJSON, &candidateRefs); err != nil {
		return ports.ReceptionRecord{}, err
	}
	for _, raw := range candidateRefs {
		candidate, err := domain.NewParcelAssociationReference(raw)
		if err != nil {
			return ports.ReceptionRecord{}, err
		}
		record.Candidates = append(record.Candidates, candidate)
	}
	if err := json.Unmarshal(markersJSON, &record.ServiceMarkers); err != nil {
		return ports.ReceptionRecord{}, err
	}
	return record, nil
}

// Save 写下一次收寄判断。同（租户+来源标识）已有记录时答`已有记录`——业务答案不是
// 错误（ADR-0031）；用 ON CONFLICT DO NOTHING 而不是捕 23505 译码：撞键的 INSERT 会
// 把整个事务打进中止态，而编排拿到`已有记录`还要在同一个事务里读回原结果作答。
//
// ON CONFLICT 只认主键。（租户，前版）唯一撞上时不该吞成`已有记录`：来源不同，读回的那一行不是这一版的答案；照常报唯一
// 违例，译成 ports.ErrPredecessorAlreadySucceeded，事务随之中止、不形成答案。
//
// 走 RequireExecutor：收寄判断落库与将来同一步的意图发布必须同生共死。
func (repository *Receptions) Save(
	ctx context.Context,
	record ports.ReceptionRecord,
) (ports.ReceptionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ReceptionSaveOutcomeInvalid, fmt.Errorf("save reception: %w", err)
	}

	intakeJSON, controlJSON, err := marshalPresence(record)
	if err != nil {
		return ports.ReceptionSaveOutcomeInvalid, fmt.Errorf("save reception: %w", err)
	}
	candidateRefs := make([]string, 0, len(record.Candidates))
	for _, candidate := range record.Candidates {
		candidateRefs = append(candidateRefs, candidate.String())
	}
	candidatesJSON, err := json.Marshal(candidateRefs)
	if err != nil {
		return ports.ReceptionSaveOutcomeInvalid, fmt.Errorf("save reception: %w", err)
	}
	markers := record.ServiceMarkers
	if markers == nil {
		markers = []string{}
	}
	markersJSON, err := json.Marshal(markers)
	if err != nil {
		return ports.ReceptionSaveOutcomeInvalid, fmt.Errorf("save reception: %w", err)
	}
	// 零值来源写 NULL：那是来源未保全的形状，不是一份空文档。
	var sourceJSON []byte
	if record.Source.Preserved() {
		if sourceJSON, err = domain.CanonicalizeDeliverySource(record.Source); err != nil {
			return ports.ReceptionSaveOutcomeInvalid, fmt.Errorf("save reception: %w", err)
		}
	}

	var dispositionJSON []byte
	if record.Disposition.Recorded() {
		if dispositionJSON, err = domain.CanonicalizeIdentityDisposition(record.Disposition); err != nil {
			return ports.ReceptionSaveOutcomeInvalid, fmt.Errorf("save reception: %w", err)
		}
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.reception
			(tenant_id, source_id, content_digest, kind, intake, control,
			 candidates, identity_conflict, refusal_reason, service_markers, service_result_check, recorded_at,
			 judgment_version, predecessor_version, source_document, merged_version, disposition_document)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		 ON CONFLICT (tenant_id, source_id) DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.SourceID,
		record.ContentDigest,
		record.Kind.String(),
		intakeJSON,
		controlJSON,
		candidatesJSON,
		record.IdentityConflict,
		record.RefusalReason,
		markersJSON,
		serviceResultCheckColumn(record.ServiceResultCheck),
		record.RecordedAt.UTC(),
		versionColumn(record.JudgmentVersion),
		versionColumn(record.Predecessor),
		sourceJSON,
		versionColumn(record.MergedInto),
		dispositionJSON,
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "reception_predecessor_unique" {
		return ports.ReceptionSaveOutcomeInvalid, fmt.Errorf("save reception %q: %w", record.Key.SourceID, ports.ErrPredecessorAlreadySucceeded)
	}
	if err != nil {
		return ports.ReceptionSaveOutcomeInvalid, fmt.Errorf("save reception: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ReceptionAlreadyRecorded, nil
	}
	return ports.ReceptionSaved, nil
}

// marshalPresence 只在收寄/控制在场时产出 jsonb；缺席写 NULL——两格在场规则由迁移
// 的 CHECK 在库里再守一遍。
func marshalPresence(record ports.ReceptionRecord) ([]byte, []byte, error) {
	var intakeJSON, controlJSON []byte
	if record.Intake.ReceivedAt().IsZero() {
		return nil, nil, nil
	}

	row := intakeRow{
		Unit:        record.Intake.Unit().String(),
		Node:        record.Intake.Node().String(),
		DeliveredBy: record.Intake.DeliveredBy().String(),
		Evidence:    record.Intake.Evidence().String(),
		Version:     record.Intake.Version().String(),
		ReceivedAt:  record.Intake.ReceivedAt().UTC(),
	}
	if association, present := record.Intake.Association(); present {
		row.Association = association.String()
	}
	intakeJSON, err := json.Marshal(row)
	if err != nil {
		return nil, nil, err
	}

	control := controlRow{
		Unit:          record.Control.Unit().String(),
		Node:          record.Control.Node().String(),
		Kind:          record.Control.EstablishmentKind().String(),
		Basis:         record.Control.Basis().String(),
		EstablishedAt: record.Control.EstablishedAt().UTC(),
	}
	if releasedBy, releasedAt, released := record.Control.Release(); released {
		control.ReleasedBy = releasedBy.String()
		releasedUTC := releasedAt.UTC()
		control.ReleasedAt = &releasedUTC
	}
	controlJSON, err = json.Marshal(control)
	if err != nil {
		return nil, nil, err
	}
	return intakeJSON, controlJSON, nil
}

func rebuildIntake(tenant domain.TenantID, raw []byte) (domain.NodeIntake, error) {
	var row intakeRow
	if err := json.Unmarshal(raw, &row); err != nil {
		return domain.NodeIntake{}, err
	}
	unit, err := domain.NewHandlingUnitID(row.Unit)
	if err != nil {
		return domain.NodeIntake{}, err
	}
	node, err := domain.NewNodeReference(row.Node)
	if err != nil {
		return domain.NodeIntake{}, err
	}
	deliveredBy, err := domain.NewDeliveringPartyReference(row.DeliveredBy)
	if err != nil {
		return domain.NodeIntake{}, err
	}
	evidence, err := domain.NewReceptionEvidenceReference(row.Evidence)
	if err != nil {
		return domain.NodeIntake{}, err
	}
	version, err := domain.NewIntakeResultVersion(row.Version)
	if err != nil {
		return domain.NodeIntake{}, err
	}
	spec := domain.NodeIntakeSpec{
		TenantID:    tenant,
		Unit:        unit,
		Node:        node,
		DeliveredBy: deliveredBy,
		Evidence:    evidence,
		Version:     version,
		ReceivedAt:  row.ReceivedAt,
	}
	if row.Association != "" {
		association, err := domain.NewParcelAssociationReference(row.Association)
		if err != nil {
			return domain.NodeIntake{}, err
		}
		spec.Association = association
	}
	return domain.FormNodeIntake(spec)
}

func rebuildControl(tenant domain.TenantID, raw []byte) (domain.PhysicalControl, error) {
	var row controlRow
	if err := json.Unmarshal(raw, &row); err != nil {
		return domain.PhysicalControl{}, err
	}
	unit, err := domain.NewHandlingUnitID(row.Unit)
	if err != nil {
		return domain.PhysicalControl{}, err
	}
	node, err := domain.NewNodeReference(row.Node)
	if err != nil {
		return domain.PhysicalControl{}, err
	}
	kind, err := controlKindFrom(row.Kind)
	if err != nil {
		return domain.PhysicalControl{}, err
	}
	basis, err := domain.NewControlBasisReference(row.Basis)
	if err != nil {
		return domain.PhysicalControl{}, err
	}
	control, err := domain.EstablishPhysicalControl(domain.PhysicalControlSpec{
		TenantID:      tenant,
		Unit:          unit,
		Node:          node,
		Kind:          kind,
		Basis:         basis,
		EstablishedAt: row.EstablishedAt,
	})
	if err != nil {
		return domain.PhysicalControl{}, err
	}
	if row.ReleasedAt != nil {
		releasedBy, err := domain.NewTransferOutReference(row.ReleasedBy)
		if err != nil {
			return domain.PhysicalControl{}, err
		}
		control, err = control.TransferOut(releasedBy, *row.ReleasedAt)
		if err != nil {
			return domain.PhysicalControl{}, err
		}
	}
	return control, nil
}

func receptionKindFrom(raw string) (ports.ReceptionRecordKind, error) {
	switch raw {
	case ports.RecordIntakeFormed.String():
		return ports.RecordIntakeFormed, nil
	case ports.RecordPendingIdentification.String():
		return ports.RecordPendingIdentification, nil
	case ports.RecordIntakeNotFormed.String():
		return ports.RecordIntakeNotFormed, nil
	case ports.RecordReceptionUndecided.String():
		return ports.RecordReceptionUndecided, nil
	case ports.RecordMergedIntoExistingIntake.String():
		return ports.RecordMergedIntoExistingIntake, nil
	default:
		return 0, fmt.Errorf("unknown reception record kind %q", raw)
	}
}

// versionColumn 把缺席的判断版本或前版写成 NULL：首判没有前版，来源保全之前的未收寄与待确认没有判断版本。
func versionColumn(version domain.IntakeResultVersion) *string {
	name := version.String()
	if name == "" {
		return nil
	}
	return &name
}

func versionFrom(raw *string) (domain.IntakeResultVersion, error) {
	if raw == nil {
		return domain.IntakeResultVersion{}, nil
	}
	return domain.NewIntakeResultVersion(*raw)
}

// serviceResultCheckColumn 把不带写成 NULL：只有形成格带核对态，这条形状规则由迁移的 CHECK 在库里再守一遍。
func serviceResultCheckColumn(check ports.ServiceResultCheck) *string {
	name := check.String()
	if name == "" {
		return nil
	}
	return &name
}

func serviceResultCheckFrom(raw *string) (ports.ServiceResultCheck, error) {
	if raw == nil {
		return ports.ServiceResultCheckAbsent, nil
	}
	switch *raw {
	case ports.ServiceResultsChecked.String():
		return ports.ServiceResultsChecked, nil
	case ports.ServiceResultsUnreadable.String():
		return ports.ServiceResultsUnreadable, nil
	default:
		return 0, fmt.Errorf("unknown service result check %q", *raw)
	}
}

func controlKindFrom(raw string) (domain.ControlEstablishmentKind, error) {
	switch raw {
	case domain.EstablishedByNodeIntake.String():
		return domain.EstablishedByNodeIntake, nil
	case domain.EstablishedByHandoverIn.String():
		return domain.EstablishedByHandoverIn, nil
	default:
		return 0, fmt.Errorf("unknown control establishment kind %q", raw)
	}
}
