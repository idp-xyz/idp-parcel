package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidManifestReference   = errors.New("customs compliance: invalid external manifest reference")
	ErrManifestNotUniquelyMatched = errors.New("customs compliance: the manifest cannot be uniquely matched")
)

// ExternalManifestID 是承运商外部监管舱单的外部身份。舱单的形成与提交由承运商在本
// 产品之外拥有——本上下文只接受、关联和解释引用（CONTEXT 硬句 147）。
type ExternalManifestID struct{ requiredValue }

func NewExternalManifestID(value string) (ExternalManifestID, error) {
	required, err := newRequiredValue("external manifest ID", value)
	return ExternalManifestID{required}, err
}

// ManifestSourceVersion 是来源版本。更正、撤销、替代或范围变化形成新来源版本和
// 关系，原引用不覆盖。
type ManifestSourceVersion struct{ requiredValue }

func NewManifestSourceVersion(value string) (ManifestSourceVersion, error) {
	required, err := newRequiredValue("manifest source version", value)
	return ManifestSourceVersion{required}, err
}

// CarrierResponsibilityReference 指名承运商责任。关联只能基于承运商责任、监管程序、
// 方向、适用时间和明确范围形成——同编号、同袋、同总单、同班次不自动证明同一对象
// （CONTEXT 硬句 148）。
type CarrierResponsibilityReference struct{ requiredValue }

func NewCarrierResponsibilityReference(value string) (CarrierResponsibilityReference, error) {
	required, err := newRequiredValue("carrier responsibility reference", value)
	return CarrierResponsibilityReference{required}, err
}

// ManifestDirection 是进出口方向封闭二值。
type ManifestDirection uint8

const (
	ManifestDirectionInvalid ManifestDirection = iota
	ImportManifest
	ExportManifest
)

func (direction ManifestDirection) valid() bool {
	return direction == ImportManifest || direction == ExportManifest
}

func (direction ManifestDirection) String() string {
	switch direction {
	case ImportManifest:
		return "IMPORT"
	case ExportManifest:
		return "EXPORT"
	default:
		return ""
	}
}

// ExternalManifestReferenceSpec 是接受一份外部舱单引用所需的全部输入。
type ExternalManifestReferenceSpec struct {
	Manifest   ExternalManifestID
	Version    ManifestSourceVersion
	Carrier    CarrierResponsibilityReference
	Procedure  CustomsProcedureReference
	Direction  ManifestDirection
	Scope      DecisionScopeReference
	SourceFact string
	AcceptedAt time.Time
}

// ExternalManifestReference 是对承运商外部监管舱单的受控引用。本上下文不形成本地
// 舱单草稿、提交版本或提交尝试——类型上没有那些字段；承运商报告已形成或已提交只是
// 来源事实，不等于监管接收或放行（那些由外部结果分层形成，也不在这里）。
type ExternalManifestReference struct {
	manifest     ExternalManifestID
	version      ManifestSourceVersion
	carrier      CarrierResponsibilityReference
	procedure    CustomsProcedureReference
	direction    ManifestDirection
	scope        DecisionScopeReference
	sourceFact   string
	acceptedAt   time.Time
	priorVersion ManifestSourceVersion
	association  DeclarationUnitID
}

func AcceptManifestReference(spec ExternalManifestReferenceSpec) (ExternalManifestReference, error) {
	if !spec.Manifest.valid() ||
		!spec.Version.valid() ||
		!spec.Carrier.valid() ||
		!spec.Procedure.valid() ||
		!spec.Direction.valid() ||
		!spec.Scope.valid() ||
		spec.SourceFact == "" ||
		spec.AcceptedAt.IsZero() {
		return ExternalManifestReference{}, ErrInvalidManifestReference
	}
	return ExternalManifestReference{
		manifest:   spec.Manifest,
		version:    spec.Version,
		carrier:    spec.Carrier,
		procedure:  spec.Procedure,
		direction:  spec.Direction,
		scope:      spec.Scope,
		sourceFact: spec.SourceFact,
		acceptedAt: spec.AcceptedAt.UTC(),
	}, nil
}

func (reference ExternalManifestReference) Manifest() ExternalManifestID {
	return reference.manifest
}

func (reference ExternalManifestReference) Version() ManifestSourceVersion {
	return reference.version
}

func (reference ExternalManifestReference) Carrier() CarrierResponsibilityReference {
	return reference.carrier
}

func (reference ExternalManifestReference) Procedure() CustomsProcedureReference {
	return reference.procedure
}

func (reference ExternalManifestReference) Direction() ManifestDirection {
	return reference.direction
}

func (reference ExternalManifestReference) Scope() DecisionScopeReference {
	return reference.scope
}

func (reference ExternalManifestReference) SourceFact() string {
	return reference.sourceFact
}

func (reference ExternalManifestReference) AcceptedAt() time.Time {
	return reference.acceptedAt
}

// RehydrateManifestReference 从当前行重建引用。库只管当前来源版本，历史由
// priorVersion 指回；关联是当前版上的受控匹配，不随版本自动搬移。
func RehydrateManifestReference(
	spec ExternalManifestReferenceSpec,
	prior ManifestSourceVersion,
	association DeclarationUnitID,
) (ExternalManifestReference, error) {
	reference, err := AcceptManifestReference(spec)
	if err != nil {
		return ExternalManifestReference{}, err
	}
	if prior.valid() {
		if prior == spec.Version {
			return ExternalManifestReference{}, ErrInvalidManifestReference
		}
		reference.priorVersion = prior
	}
	if association.valid() {
		reference.association = association
	}
	return reference, nil
}

// PriorVersion 只在更正/替代后的新引用上给出。
func (reference ExternalManifestReference) PriorVersion() (ManifestSourceVersion, bool) {
	return reference.priorVersion, reference.priorVersion.valid()
}

// Association 报告与申报单元的受控关联及是否已建立。
func (reference ExternalManifestReference) Association() (DeclarationUnitID, bool) {
	return reference.association, reference.association.valid()
}

// AssociationCandidate 是一个候选申报单元及其与引用的匹配维度是否成立。
type AssociationCandidate struct {
	Unit      DeclarationUnitID
	Procedure CustomsProcedureReference
	Direction ManifestDirection
	Scope     DecisionScopeReference
}

// Associate 依据唯一匹配建立与申报单元的受控关联（CONTEXT 生命周期 258：「能够与
// 关务案件、申报单元和运输对象逐范围唯一匹配→形成业务关联和当前采用关系；无法唯一
// 匹配时保持待关联，不创建占位对象或按最近客户、班次猜测」）——程序、方向与范围三维
// 都相符的候选恰一个才关联；零个或多个都保持待关联（独立哨兵）。
func (reference ExternalManifestReference) Associate(
	candidates []AssociationCandidate,
) (ExternalManifestReference, error) {
	if _, associated := reference.Association(); associated {
		return ExternalManifestReference{}, ErrInvalidManifestReference
	}
	matched := make([]DeclarationUnitID, 0, 1)
	for _, candidate := range candidates {
		if !candidate.Unit.valid() {
			return ExternalManifestReference{}, ErrInvalidManifestReference
		}
		if candidate.Procedure == reference.procedure &&
			candidate.Direction == reference.direction &&
			candidate.Scope == reference.scope {
			matched = append(matched, candidate.Unit)
		}
	}
	if len(matched) != 1 {
		return ExternalManifestReference{}, ErrManifestNotUniquelyMatched
	}
	associated := reference
	associated.association = matched[0]
	return associated, nil
}

// Revise 依据承运商明确的更正、撤销、替代或范围变化形成新来源版本：换版本、换范围、
// 指回原版本；原引用、原范围和历史关联保留（CONTEXT 硬句 150），既有申报与监管事实
// 不回退——这里没有它们的字段。关联不随版本自动搬移：新版本重新走唯一匹配。
func (reference ExternalManifestReference) Revise(
	version ManifestSourceVersion,
	scope DecisionScopeReference,
	sourceFact string,
	at time.Time,
) (ExternalManifestReference, error) {
	if !version.valid() || version == reference.version ||
		!scope.valid() || sourceFact == "" ||
		at.IsZero() || at.Before(reference.acceptedAt) {
		return ExternalManifestReference{}, ErrInvalidManifestReference
	}
	revised := reference
	revised.version = version
	revised.scope = scope
	revised.sourceFact = sourceFact
	revised.acceptedAt = at.UTC()
	revised.priorVersion = reference.version
	revised.association = DeclarationUnitID{}
	return revised, nil
}
