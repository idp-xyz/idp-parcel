package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// DeclarationUnits 实现 ports.DeclarationUnitStore（ADR-0073 决定一/二）。只建立、
// 无 UPDATE 语句：「同一单元的案件维不得变更」靠没有更新路径承载，换案件即建立替代
// 单元。同键已在册答`已有记录`（ON CONFLICT DO NOTHING 保事务可用），内容一致与否由
// 编排读回自己比——与本上下文其余写口同一套代数（ADR-0031）。
//
// 成员清单按字典序落 jsonb：形成顺序不构成不同的组成（与提交内容指纹同一口径），
// 排序后读回比对才不会把同一集合的两种排列比成两份内容。
type DeclarationUnits struct {
	db *bentopg.DB
}

func NewDeclarationUnits(db *bentopg.DB) (*DeclarationUnits, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &DeclarationUnits{db: db}, nil
}

var _ ports.DeclarationUnitStore = (*DeclarationUnits)(nil)

func (repository *DeclarationUnits) Save(
	ctx context.Context,
	tenant domain.TenantID,
	unit domain.DeclarationUnit,
	formedAt time.Time,
) (ports.DeclarationUnitSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationUnitSaveOutcomeInvalid, fmt.Errorf("save declaration unit: %w", err)
	}
	if formedAt.IsZero() {
		return ports.DeclarationUnitSaveOutcomeInvalid,
			fmt.Errorf("save declaration unit: the formation instant is required")
	}

	members := make([]string, 0, len(unit.Members()))
	for _, member := range unit.Members() {
		members = append(members, member.String())
	}
	sort.Strings(members)
	membersRaw, err := json.Marshal(members)
	if err != nil {
		return ports.DeclarationUnitSaveOutcomeInvalid, fmt.Errorf("save declaration unit: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.declaration_unit
			(tenant_id, unit_id, case_id, procedure_ref, members, formed_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (tenant_id, unit_id) DO NOTHING`,
		tenant.String(),
		unit.ID().String(),
		unit.Case().String(),
		unit.Procedure().String(),
		membersRaw,
		formedAt.UTC(),
	)
	if err != nil {
		return ports.DeclarationUnitSaveOutcomeInvalid, fmt.Errorf("save declaration unit: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.DeclarationUnitAlreadyRecorded, nil
	}
	return ports.DeclarationUnitSaved, nil
}

// FindByID 取回单元身份与案件维。读回经 FormDeclarationUnit 整门重验——身份、案件、
// 程序与组成任一坏行在这里炸成错误，不进编排。
func (repository *DeclarationUnits) FindByID(
	ctx context.Context,
	tenant domain.TenantID,
	unitID domain.DeclarationUnitID,
) (domain.DeclarationUnit, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.DeclarationUnit{}, false, fmt.Errorf("find declaration unit: %w", err)
	}

	var (
		caseID, procedure string
		membersRaw        []byte
	)
	err = querier.QueryRow(ctx,
		`SELECT case_id, procedure_ref, members
		   FROM customs_compliance.declaration_unit
		  WHERE tenant_id = $1 AND unit_id = $2`,
		tenant.String(), unitID.String(),
	).Scan(&caseID, &procedure, &membersRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DeclarationUnit{}, false, nil
	}
	if err != nil {
		return domain.DeclarationUnit{}, false, fmt.Errorf("find declaration unit: %w", err)
	}

	customsCase, err := domain.NewCustomsCaseID(caseID)
	if err != nil {
		return domain.DeclarationUnit{}, false, fmt.Errorf("rebuild declaration unit: %w", err)
	}
	procedureRef, err := domain.NewCustomsProcedureReference(procedure)
	if err != nil {
		return domain.DeclarationUnit{}, false, fmt.Errorf("rebuild declaration unit: %w", err)
	}
	var members []string
	if err := json.Unmarshal(membersRaw, &members); err != nil {
		return domain.DeclarationUnit{}, false, fmt.Errorf("rebuild declaration unit: members: %w", err)
	}
	references := make([]domain.DeclaredParcelReference, 0, len(members))
	for _, member := range members {
		reference, err := domain.NewDeclaredParcelReference(member)
		if err != nil {
			return domain.DeclarationUnit{}, false, fmt.Errorf("rebuild declaration unit: %w", err)
		}
		references = append(references, reference)
	}
	unit, err := domain.FormDeclarationUnit(unitID, customsCase, procedureRef, references)
	if err != nil {
		return domain.DeclarationUnit{}, false, fmt.Errorf("rebuild declaration unit: %w", err)
	}
	return unit, true, nil
}
