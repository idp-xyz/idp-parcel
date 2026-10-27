package nodeoperations

import (
	"context"
	"fmt"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// UnitsHoldingMemberReader 是节点作业集运成员关系的反向那一问；真实装配交给 nodeoperations/adapters/postgres.ConsolidationUnitsHoldingMember。
type UnitsHoldingMemberReader interface {
	ReadUnitsEverHoldingMember(
		ctx context.Context,
		tenant nodomain.TenantID,
		member string,
	) ([]nodomain.ConsolidationUnitID, error)
}

// IdentifiedMemberView 实现 veports.IdentifiedMemberView：全程追踪在一件作业实物识别成功时，问节点作业「这封信指的收寄认出了哪件
// 包裹」与「这件实物曾随过哪几件单元」。只翻译——收寄身份怎么认、成员口径怎么算，都由节点作业答。
type IdentifiedMemberView struct {
	receptions ReceptionFinder
	holders    UnitsHoldingMemberReader
}

// NewIdentifiedMemberView 两口都必须在：缺了哪一口，每一封收寄形成信封上的补派生都只能停在未决。
func NewIdentifiedMemberView(receptions ReceptionFinder, holders UnitsHoldingMemberReader) (*IdentifiedMemberView, error) {
	if receptions == nil || holders == nil {
		return nil, fmt.Errorf("visibility exception nodeoperations adapter: the reception finder and the holder reader are both required")
	}
	return &IdentifiedMemberView{receptions: receptions, holders: holders}, nil
}

var _ veports.IdentifiedMemberView = (*IdentifiedMemberView)(nil)

// FormedIntakeIdentity 按信封键回读收寄。只有「节点收寄已形成」且带着版本化包裹关联的那一格认出了包裹；待识别、未形成、形成了却
// 没有关联都交回 Identified 为假——没有可补的对象，这一路入账收工，不像投影那一路等着识别（投影要这件包裹的收寄事实才有东西可投，
// 补派生没有关联就无从说「补给谁」）。记录与键不符、租户对不上、关联立不成追踪包裹引用，是册上的确定性问题。
func (view *IdentifiedMemberView) FormedIntakeIdentity(
	ctx context.Context,
	tenant vedomain.TenantID,
	source string,
) (veports.FormedIntakeIdentity, error) {
	none := veports.FormedIntakeIdentity{}
	noTenant, err := nodomain.NewTenantID(tenant.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", veports.ErrFormedIntakeUntranslatable, err)
	}
	record, found, err := view.receptions.FindByKey(ctx, noports.ReceptionKey{TenantID: noTenant, SourceID: source})
	if err != nil {
		return none, fmt.Errorf("read node intake reception: node operations: %w", err)
	}
	if !found {
		return none, fmt.Errorf("node intake reception %q is not yet visible", source)
	}
	switch record.Kind {
	case noports.RecordIntakeNotFormed, noports.RecordPendingIdentification:
		return none, nil
	case noports.RecordIntakeFormed:
	default:
		return none, fmt.Errorf("%w: source %q kind %d", veports.ErrFormedIntakeUntranslatable, source, record.Kind)
	}
	if record.Key.TenantID != noTenant || record.Key.SourceID != source || record.Intake.TenantID() != noTenant {
		return none, fmt.Errorf("%w: source %q disagrees with its key", veports.ErrFormedIntakeUntranslatable, source)
	}
	association, associated := record.Intake.Association()
	if !associated {
		return none, nil
	}
	parcel, err := vedomain.NewTrackedParcelReference(association.String())
	if err != nil {
		return none, fmt.Errorf("%w: source %q parcel association: %v", veports.ErrFormedIntakeUntranslatable, source, err)
	}
	return veports.FormedIntakeIdentity{Identified: true, Handling: record.Intake.Unit().String(), Parcel: parcel}, nil
}

// UnitsEverHolding 问节点作业这个成员曾被直接装入哪几件单元。单元标识原串交回；空白的单元标识是册上的确定性问题。
func (view *IdentifiedMemberView) UnitsEverHolding(
	ctx context.Context,
	tenant vedomain.TenantID,
	member string,
) ([]string, error) {
	noTenant, err := nodomain.NewTenantID(tenant.String())
	if err != nil {
		return nil, fmt.Errorf("%w: tenant: %v", veports.ErrConsolidationMembershipUntranslatable, err)
	}
	units, err := view.holders.ReadUnitsEverHoldingMember(ctx, noTenant, member)
	if err != nil {
		return nil, fmt.Errorf("read units holding member %q: node operations: %w", member, err)
	}
	holders := make([]string, 0, len(units))
	for _, unit := range units {
		if unit.String() == "" {
			return nil, fmt.Errorf("%w: blank unit holding member %q", veports.ErrConsolidationMembershipUntranslatable, member)
		}
		holders = append(holders, unit.String())
	}
	return holders, nil
}
