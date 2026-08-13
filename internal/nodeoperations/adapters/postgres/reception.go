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
// 只回 false，不区分「不存在」与「属于另一个租户」。读回的一切经领域构造函数重建：
// 一次坏写入在这里暴露，而不是变成一个看起来合法的收寄结果。
func (repository *Receptions) FindByKey(
	ctx context.Context,
	key ports.ReceptionKey,
) (ports.ReceptionRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ReceptionRecord{}, false, fmt.Errorf("find reception: %w", err)
	}

	var (
		digest, kindName, refusal   string
		intakeJSON, controlJSON     []byte
		candidatesJSON, markersJSON []byte
		identityConflict            bool
		recordedAt                  time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT content_digest, kind, intake, control, candidates,
		        identity_conflict, refusal_reason, service_markers, recorded_at
		   FROM node_operations.reception
		  WHERE tenant_id = $1
		    AND source_id = $2`,
		key.TenantID.String(),
		key.SourceID,
	).Scan(&digest, &kindName, &intakeJSON, &controlJSON, &candidatesJSON,
		&identityConflict, &refusal, &markersJSON, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ReceptionRecord{}, false, nil
	}
	if err != nil {
		return ports.ReceptionRecord{}, false, fmt.Errorf("find reception: %w", err)
	}

	kind, err := receptionKindFrom(kindName)
	if err != nil {
		return ports.ReceptionRecord{}, false, fmt.Errorf("find reception: %w", err)
	}
	record := ports.ReceptionRecord{
		Key:              key,
		ContentDigest:    digest,
		Kind:             kind,
		IdentityConflict: identityConflict,
		RefusalReason:    refusal,
		RecordedAt:       recordedAt.UTC(),
	}

	if len(intakeJSON) > 0 {
		intake, err := rebuildIntake(key.TenantID, intakeJSON)
		if err != nil {
			return ports.ReceptionRecord{}, false, fmt.Errorf("find reception: %w", err)
		}
		record.Intake = intake
	}
	if len(controlJSON) > 0 {
		control, err := rebuildControl(key.TenantID, controlJSON)
		if err != nil {
			return ports.ReceptionRecord{}, false, fmt.Errorf("find reception: %w", err)
		}
		record.Control = control
	}

	var candidateRefs []string
	if err := json.Unmarshal(candidatesJSON, &candidateRefs); err != nil {
		return ports.ReceptionRecord{}, false, fmt.Errorf("find reception: %w", err)
	}
	for _, raw := range candidateRefs {
		candidate, err := domain.NewParcelAssociationReference(raw)
		if err != nil {
			return ports.ReceptionRecord{}, false, fmt.Errorf("find reception: %w", err)
		}
		record.Candidates = append(record.Candidates, candidate)
	}
	if err := json.Unmarshal(markersJSON, &record.ServiceMarkers); err != nil {
		return ports.ReceptionRecord{}, false, fmt.Errorf("find reception: %w", err)
	}
	return record, true, nil
}

// Save 写下一次收寄判断。同（租户+来源标识）已有记录时答`已有记录`——业务答案不是
// 错误（ADR-0031）；用 ON CONFLICT DO NOTHING 而不是捕 23505 译码：撞键的 INSERT 会
// 把整个事务打进中止态，而编排拿到`已有记录`还要在同一个事务里读回原结果作答。
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

	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.reception
			(tenant_id, source_id, content_digest, kind, intake, control,
			 candidates, identity_conflict, refusal_reason, service_markers, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
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
		record.RecordedAt.UTC(),
	)
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
	default:
		return 0, fmt.Errorf("unknown reception record kind %q", raw)
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
