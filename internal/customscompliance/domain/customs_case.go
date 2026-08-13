package domain

import (
	"errors"
	"time"
)

var ErrInvalidCustomsCase = errors.New("customs compliance: invalid customs case")

// CustomsCaseID 是关务案件的标识。
type CustomsCaseID struct{ requiredValue }

func NewCustomsCaseID(value string) (CustomsCaseID, error) {
	required, err := newRequiredValue("customs case ID", value)
	return CustomsCaseID{required}, err
}

// RegulatoryJurisdictionReference 指名监管辖区。
type RegulatoryJurisdictionReference struct{ requiredValue }

func NewRegulatoryJurisdictionReference(value string) (RegulatoryJurisdictionReference, error) {
	required, err := newRequiredValue("regulatory jurisdiction reference", value)
	return RegulatoryJurisdictionReference{required}, err
}

// ObligationScopeReference 指名法定义务范围。
type ObligationScopeReference struct{ requiredValue }

func NewObligationScopeReference(value string) (ObligationScopeReference, error) {
	required, err := newRequiredValue("obligation scope reference", value)
	return ObligationScopeReference{required}, err
}

// CaseParcelAssociation 是案件与一个包裹的关联：包裹身份、客户归属与来源资料引用
// ——只引用，不复制或覆盖源事实（UC-CC-001：源事实的家在 parcel-shipment）。
type CaseParcelAssociation struct {
	Parcel    string
	Customer  string
	SourceRef string
}

func (association CaseParcelAssociation) complete() bool {
	return association.Parcel != "" && association.Customer != "" && association.SourceRef != ""
}

// CaseRoleSnapshot 是一项初始关务参与方角色资格快照：角色、参与方与授权依据各自
// 确认——不从企业类型或代理关系自动推导（UC-CC-001 硬句），推导的入口在这里不存在：
// 三件都要显式给出。
type CaseRoleSnapshot struct {
	Role      string
	Party     string
	Authority string
}

func (snapshot CaseRoleSnapshot) complete() bool {
	return snapshot.Role != "" && snapshot.Party != "" && snapshot.Authority != ""
}

// CustomsCaseSpec 是建立一个关务案件所需的全部输入。
type CustomsCaseSpec struct {
	ID            CustomsCaseID
	Jurisdiction  RegulatoryJurisdictionReference
	Direction     ManifestDirection
	Procedure     CustomsProcedureReference
	Obligation    ObligationScopeReference
	Parcels       []CaseParcelAssociation
	Roles         []CaseRoleSnapshot
	EstablishedAt time.Time
}

// CustomsCase 是在一个固定监管辖区、进出口方向、监管程序和法定义务范围内组织后续
// 监管履责的责任容器。它不是「清关中」状态也不是可提交的申报——类型上没有状态推进
// 与申报字段；申报单元、就绪、授权、提交由后续用例各自形成。
type CustomsCase struct {
	id            CustomsCaseID
	jurisdiction  RegulatoryJurisdictionReference
	direction     ManifestDirection
	procedure     CustomsProcedureReference
	obligation    ObligationScopeReference
	parcels       []CaseParcelAssociation
	roles         []CaseRoleSnapshot
	establishedAt time.Time
}

// EstablishCustomsCase 建立案件。四维监管范围与包裹关联集缺一不可（包裹不重——同一
// 包裹在一个案件里只关联一次，但一个包裹可以关联多个彼此独立的案件——那是跨案件的
// 事，这里管不着也不该管）；角色快照可为空清单（初始角色未确认如实空白，后续确认
// 再补），但给出的每项必须完整。
func EstablishCustomsCase(spec CustomsCaseSpec) (CustomsCase, error) {
	if !spec.ID.valid() ||
		!spec.Jurisdiction.valid() ||
		!spec.Direction.valid() ||
		!spec.Procedure.valid() ||
		!spec.Obligation.valid() ||
		len(spec.Parcels) == 0 ||
		spec.EstablishedAt.IsZero() {
		return CustomsCase{}, ErrInvalidCustomsCase
	}
	seen := make(map[string]bool, len(spec.Parcels))
	for _, association := range spec.Parcels {
		if !association.complete() || seen[association.Parcel] {
			return CustomsCase{}, ErrInvalidCustomsCase
		}
		seen[association.Parcel] = true
	}
	for _, snapshot := range spec.Roles {
		if !snapshot.complete() {
			return CustomsCase{}, ErrInvalidCustomsCase
		}
	}
	return CustomsCase{
		id:            spec.ID,
		jurisdiction:  spec.Jurisdiction,
		direction:     spec.Direction,
		procedure:     spec.Procedure,
		obligation:    spec.Obligation,
		parcels:       append([]CaseParcelAssociation(nil), spec.Parcels...),
		roles:         append([]CaseRoleSnapshot(nil), spec.Roles...),
		establishedAt: spec.EstablishedAt.UTC(),
	}, nil
}

func (customsCase CustomsCase) ID() CustomsCaseID { return customsCase.id }

func (customsCase CustomsCase) Jurisdiction() RegulatoryJurisdictionReference {
	return customsCase.jurisdiction
}

func (customsCase CustomsCase) Direction() ManifestDirection { return customsCase.direction }

func (customsCase CustomsCase) Procedure() CustomsProcedureReference {
	return customsCase.procedure
}

func (customsCase CustomsCase) Obligation() ObligationScopeReference {
	return customsCase.obligation
}

func (customsCase CustomsCase) Parcels() []CaseParcelAssociation {
	return append([]CaseParcelAssociation(nil), customsCase.parcels...)
}

func (customsCase CustomsCase) Roles() []CaseRoleSnapshot {
	return append([]CaseRoleSnapshot(nil), customsCase.roles...)
}

func (customsCase CustomsCase) EstablishedAt() time.Time { return customsCase.establishedAt }
