package partycommercial

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// AdoptedStageOwner 从一次委托身份解析出阶段内容要读的拥有规则版本。
//
// 收寄资格与终局规则读接单规则包；取消授权读授权规则。采用哪一版是接受时固定闭包
// 的事（ADR-0058），不在 SourceIdentity 上。found=false = 该身份尚未固定采用哪个
// 规则对象（实例半边），消费方停在未配置，本适配器不造默认包。
type AdoptedStageOwner interface {
	AcceptanceRulePackageFor(
		ctx context.Context,
		identity psdomain.SourceIdentity,
	) (pcdomain.CommercialVersion, bool, error)
	AuthorizationRuleFor(
		ctx context.Context,
		identity psdomain.SourceIdentity,
	) (pcdomain.CommercialVersion, bool, error)
}

// UnconfiguredAdoptedStageOwner 是首发诚实未配置：SourceIdentity 不含采用的规则
// 版本，也没有实例参数能把它补上。不造默认包。
type UnconfiguredAdoptedStageOwner struct{}

func (UnconfiguredAdoptedStageOwner) AcceptanceRulePackageFor(
	context.Context,
	psdomain.SourceIdentity,
) (pcdomain.CommercialVersion, bool, error) {
	return pcdomain.CommercialVersion{}, false, nil
}

func (UnconfiguredAdoptedStageOwner) AuthorizationRuleFor(
	context.Context,
	psdomain.SourceIdentity,
) (pcdomain.CommercialVersion, bool, error) {
	return pcdomain.CommercialVersion{}, false, nil
}

// DeclaredStageContent 把 PC 阶段内容三口接到 PS 翻译适配器的内部协作者上
// （ADR-0025 消费方侧）。装配缝在这里：有 AdoptedStageOwner 才去点读；没有采用版本
// 就停在 found=false。
type DeclaredStageContent struct {
	intake       pcports.IntakeQualificationView
	final        pcports.FinalRuleContentView
	cancellation pcports.CancellationAuthorityContentView
	owners       AdoptedStageOwner
}

func NewDeclaredStageContent(
	intake pcports.IntakeQualificationView,
	final pcports.FinalRuleContentView,
	cancellation pcports.CancellationAuthorityContentView,
	owners AdoptedStageOwner,
) *DeclaredStageContent {
	if owners == nil {
		owners = UnconfiguredAdoptedStageOwner{}
	}
	return &DeclaredStageContent{
		intake:       intake,
		final:        final,
		cancellation: cancellation,
		owners:       owners,
	}
}

var _ IntakeContentSource = (*DeclaredStageContent)(nil)
var _ FinalContentSource = (*DeclaredStageContent)(nil)
var _ CancellationContentSource = (*DeclaredStageContent)(nil)

func (source *DeclaredStageContent) IntakeContentFor(
	ctx context.Context,
	identity psdomain.SourceIdentity,
	_ psdomain.ShipmentRequestID,
) (pcdomain.IntakeQualificationContent, bool, error) {
	none := pcdomain.IntakeQualificationContent{}
	owner, found, err := source.owners.AcceptanceRulePackageFor(ctx, identity)
	if err != nil {
		return none, false, fmt.Errorf("adopted rule package: %w", err)
	}
	if !found {
		return none, false, nil
	}
	tenant, err := pcTenantOf(identity)
	if err != nil {
		return none, false, err
	}
	if source.intake == nil {
		return none, false, fmt.Errorf("intake qualification view is not configured")
	}
	return source.intake.LoadIntakeQualification(ctx, tenant, owner)
}

func (source *DeclaredStageContent) FinalContentFor(
	ctx context.Context,
	identity psdomain.SourceIdentity,
) (pcdomain.FinalRuleContent, bool, error) {
	none := pcdomain.FinalRuleContent{}
	owner, found, err := source.owners.AcceptanceRulePackageFor(ctx, identity)
	if err != nil {
		return none, false, fmt.Errorf("adopted rule package: %w", err)
	}
	if !found {
		return none, false, nil
	}
	tenant, err := pcTenantOf(identity)
	if err != nil {
		return none, false, err
	}
	if source.final == nil {
		return none, false, fmt.Errorf("final rule content view is not configured")
	}
	return source.final.LoadFinalRule(ctx, tenant, owner)
}

func (source *DeclaredStageContent) CancellationContentFor(
	ctx context.Context,
	identity psdomain.SourceIdentity,
) (pcdomain.CancellationAuthorityContent, bool, error) {
	none := pcdomain.CancellationAuthorityContent{}
	owner, found, err := source.owners.AuthorizationRuleFor(ctx, identity)
	if err != nil {
		return none, false, fmt.Errorf("adopted authorization rule: %w", err)
	}
	if !found {
		return none, false, nil
	}
	tenant, err := pcTenantOf(identity)
	if err != nil {
		return none, false, err
	}
	if source.cancellation == nil {
		return none, false, fmt.Errorf("cancellation authority view is not configured")
	}
	return source.cancellation.LoadCancellationAuthority(ctx, tenant, owner)
}

func pcTenantOf(identity psdomain.SourceIdentity) (pcdomain.TenantID, error) {
	tenant, err := pcdomain.NewTenantID(identity.TenantID().String())
	if err != nil {
		return pcdomain.TenantID{}, fmt.Errorf("party-commercial tenant: %w", err)
	}
	return tenant, nil
}
