package postgres

import (
	"encoding/json"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// directionalBasisRow 是 consolidation_fact.directional_basis 这一 jsonb 列的行模型（迁移 0005 的形状 CHECK 照它
// 的键写）。三格同一个形，引用随格：身份成立带所依收寄，服务结果缺少带结果引用。写侧、按键读回与查阅目录共用
// 这一份，不在三处各立一套键名。
type directionalBasisRow struct {
	JudgedAt      time.Time               `json:"judgedAt"`
	Identity      directionalBasisCellRow `json:"identity"`
	ServiceResult directionalBasisCellRow `json:"serviceResult"`
	Routing       directionalBasisCellRow `json:"routing"`
}

type directionalBasisCellRow struct {
	Answer         string   `json:"answer"`
	Reasons        []string `json:"reasons,omitempty"`
	Association    string   `json:"association,omitempty"`
	IntakeSourceID string   `json:"intakeSourceId,omitempty"`
	IntakeVersion  string   `json:"intakeVersion,omitempty"`
	Cancellation   string   `json:"cancellation,omitempty"`
	FinalKind      string   `json:"finalKind,omitempty"`
	FinalVersion   string   `json:"finalVersion,omitempty"`
}

// marshalDirectionalBasis 只在有记录时产出 jsonb；没有记录写 NULL——NULL 读作`未记录`，不能写成一份空对象让形状
// CHECK 去拦。
func marshalDirectionalBasis(record domain.DirectionalBasisRecord) ([]byte, error) {
	if !record.Recorded() {
		return nil, nil
	}
	identity := record.Identity()
	serviceResult := record.ServiceResult()
	row := directionalBasisRow{
		JudgedAt:      record.JudgedAt().UTC(),
		Identity:      cellRowOf(identity.Cell()),
		ServiceResult: cellRowOf(serviceResult.Cell()),
		Routing:       cellRowOf(record.Routing().Cell()),
	}
	if association, established := identity.Association(); established {
		row.Identity.Association = association.String()
		row.Identity.IntakeSourceID = identity.IntakeSourceID()
		row.Identity.IntakeVersion = identity.IntakeVersion().String()
	}
	row.ServiceResult.Cancellation = serviceResult.Cancellation()
	row.ServiceResult.FinalKind, row.ServiceResult.FinalVersion = serviceResult.Final()
	return json.Marshal(row)
}

func cellRowOf(cell domain.DirectionalBasisCell) directionalBasisCellRow {
	row := directionalBasisCellRow{Answer: cell.Answer().String()}
	for _, reason := range cell.Reasons() {
		row.Reasons = append(row.Reasons, reason.String())
	}
	return row
}

// rebuildDirectionalBasis 经领域重建门读回记录：一份坏写入在这里响亮，不变成一份看起来合法的依据。
func rebuildDirectionalBasis(raw []byte) (domain.DirectionalBasisRecord, error) {
	var row directionalBasisRow
	if err := json.Unmarshal(raw, &row); err != nil {
		return domain.DirectionalBasisRecord{}, err
	}
	return domain.RehydrateDirectionalBasis(domain.RehydrateDirectionalBasisSpec{
		Identity:       domain.RehydrateDirectionalBasisCellSpec{Answer: row.Identity.Answer, Reasons: row.Identity.Reasons},
		Association:    row.Identity.Association,
		IntakeSourceID: row.Identity.IntakeSourceID,
		IntakeVersion:  row.Identity.IntakeVersion,
		ServiceResult:  domain.RehydrateDirectionalBasisCellSpec{Answer: row.ServiceResult.Answer, Reasons: row.ServiceResult.Reasons},
		Cancellation:   row.ServiceResult.Cancellation,
		FinalKind:      row.ServiceResult.FinalKind,
		FinalVersion:   row.ServiceResult.FinalVersion,
		Routing:        domain.RehydrateDirectionalBasisCellSpec{Answer: row.Routing.Answer, Reasons: row.Routing.Reasons},
		JudgedAt:       row.JudgedAt,
	})
}
