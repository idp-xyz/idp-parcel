package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// IntakeIdentities 实现 ports.IntakeIdentityView：按作业实物读收寄册上带收寄的行，供方向依据记录判身份（票
// no-directional-basis/01）。它与 Receptions 读同一张表却分立：Receptions 按来源身份存取一次收寄判断，这里按实物
// 汇总它在册上的各次收寄，两种问法各有各的索引（迁移 0005 的 reception_intake_unit_idx）。
type IntakeIdentities struct {
	db *bentopg.DB
}

func NewIntakeIdentities(db *bentopg.DB) (*IntakeIdentities, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &IntakeIdentities{db: db}, nil
}

var _ ports.IntakeIdentityView = (*IntakeIdentities)(nil)

// FindIntakeIdentities 交回这件实物在册上每一行带收寄的记录（收寄已形成与待识别两格；另两格不带收寄，也就没有
// 作业实物可按）。查无此物交回空切片。
//
// 走 ReadExecutor：调用方不绑事务时读落在池上（为什么要在写事务之前读，见 application.DirectionalBasisJudge）。
// 行上的值逐个过领域构造器，坏行作为错误交回，编排把它记成`读不动`。
func (view *IntakeIdentities) FindIntakeIdentities(
	ctx context.Context,
	tenant domain.TenantID,
	unit domain.HandlingUnitID,
) ([]domain.IntakeIdentityObservation, error) {
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find intake identities: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT source_id, intake, identity_conflict
		   FROM node_operations.reception
		  WHERE tenant_id = $1
		    AND intake IS NOT NULL
		    AND intake ->> 'unit' = $2
		  ORDER BY source_id`,
		tenant.String(),
		unit.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("find intake identities: %w", err)
	}
	defer rows.Close()

	observations := []domain.IntakeIdentityObservation{}
	for rows.Next() {
		var (
			sourceID string
			raw      []byte
			conflict bool
		)
		if err := rows.Scan(&sourceID, &raw, &conflict); err != nil {
			return nil, fmt.Errorf("find intake identities: %w", err)
		}
		observation, err := intakeIdentityOf(sourceID, raw, conflict)
		if err != nil {
			return nil, fmt.Errorf("find intake identities: source %q: %w", sourceID, err)
		}
		observations = append(observations, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find intake identities: %w", err)
	}
	return observations, nil
}

// intakeIdentityOf 只取身份要用的几样：版本、关联（待识别行缺席）、业务时间。行模型与收寄册写侧同一份 intakeRow。
func intakeIdentityOf(sourceID string, raw []byte, conflict bool) (domain.IntakeIdentityObservation, error) {
	var row intakeRow
	if err := json.Unmarshal(raw, &row); err != nil {
		return domain.IntakeIdentityObservation{}, err
	}
	version, err := domain.NewIntakeResultVersion(row.Version)
	if err != nil {
		return domain.IntakeIdentityObservation{}, err
	}
	observation := domain.IntakeIdentityObservation{
		SourceID:         sourceID,
		Version:          version,
		IdentityConflict: conflict,
		ReceivedAt:       row.ReceivedAt.UTC(),
	}
	if row.Association != "" {
		association, err := domain.NewParcelAssociationReference(row.Association)
		if err != nil {
			return domain.IntakeIdentityObservation{}, err
		}
		observation.Association = association
	}
	return observation, nil
}
