// Package domain 承载节点作业的领域模型：作业实物、节点收寄、实物控制与节点侧作业
// 事实。它是面向小包网络流转的场站作业模型，不是 WMS——不建库存、可用量或库存价值；
// 也不拥有正式包裹身份，那属 parcel-shipment。
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrBlankValue        = errors.New("node operations: blank value")
	ErrInvalidNodeIntake = errors.New("node operations: invalid node intake")
)

type requiredValue struct {
	value string
}

func newRequiredValue(name, value string) (requiredValue, error) {
	if strings.TrimSpace(value) == "" {
		return requiredValue{}, fmt.Errorf("%w: %s", ErrBlankValue, name)
	}
	return requiredValue{value: value}, nil
}

func (value requiredValue) String() string {
	return value.value
}

func (value requiredValue) valid() bool {
	return strings.TrimSpace(value.value) != ""
}

type TenantID struct{ requiredValue }

func NewTenantID(value string) (TenantID, error) {
	required, err := newRequiredValue("tenant ID", value)
	return TenantID{required}, err
}

// HandlingUnitID 是节点现场能区分的作业实物标识。它是本上下文的作业身份，不是客户
// 服务身份——正式包裹身份属 parcel-shipment，作业实物记录不能替代它。
type HandlingUnitID struct{ requiredValue }

func NewHandlingUnitID(value string) (HandlingUnitID, error) {
	required, err := newRequiredValue("handling unit ID", value)
	return HandlingUnitID{required}, err
}

// NodeReference 指名收寄发生的物流节点。节点网络身份属 network-routing 的网络定义，
// 这里引用作业发生地。
type NodeReference struct{ requiredValue }

func NewNodeReference(value string) (NodeReference, error) {
	required, err := newRequiredValue("node reference", value)
	return NodeReference{required}, err
}

// DeliveringPartyReference 指名实际交付实物的客户或其授权交付方。
type DeliveringPartyReference struct{ requiredValue }

func NewDeliveringPartyReference(value string) (DeliveringPartyReference, error) {
	required, err := newRequiredValue("delivering party reference", value)
	return DeliveringPartyReference{required}, err
}

// ReceptionEvidenceReference 指名明确接收的证据（点验、签收、受控接收记录）。它是
// 节点收寄与「到站扫描/卸载/发现实物」的分界——后者没有明确接收证据，构造期就进不来。
type ReceptionEvidenceReference struct{ requiredValue }

func NewReceptionEvidenceReference(value string) (ReceptionEvidenceReference, error) {
	required, err := newRequiredValue("reception evidence reference", value)
	return ReceptionEvidenceReference{required}, err
}

// ParcelAssociationReference 指名一次版本化的正式包裹关联。它可以缺席：待识别实物
// 同样被接收并取得控制，识别成功后通过版本化关联补上，不删原记录。
type ParcelAssociationReference struct{ requiredValue }

func NewParcelAssociationReference(value string) (ParcelAssociationReference, error) {
	required, err := newRequiredValue("parcel association reference", value)
	return ParcelAssociationReference{required}, err
}

// IntakeResultVersion 是收寄结果的版本标识。parcel-shipment 的采用判断按它幂等；更正
// 形成新版本不覆盖本版。
type IntakeResultVersion struct{ requiredValue }

func NewIntakeResultVersion(value string) (IntakeResultVersion, error) {
	required, err := newRequiredValue("intake result version", value)
	return IntakeResultVersion{required}, err
}

// NodeIntakeSpec 是形成一次节点收寄结果所需的全部输入。
type NodeIntakeSpec struct {
	TenantID    TenantID
	Unit        HandlingUnitID
	Node        NodeReference
	DeliveredBy DeliveringPartyReference
	Evidence    ReceptionEvidenceReference
	Version     IntakeResultVersion
	Association ParcelAssociationReference
	ReceivedAt  time.Time
}

// NodeIntake 是客户或其授权交付方在节点直接交付、节点完成明确接收并取得控制的业务
// 结果（CONTEXT 语言）。接收证据必备——客户提交、委托接受、到站扫描、卸载或发现实物
// 都不等于节点收寄，分界就在「明确接收」这份证据上。包裹关联可缺席：待识别实物同样
// 真实地被收寄与控制。
type NodeIntake struct {
	tenantID    TenantID
	unit        HandlingUnitID
	node        NodeReference
	deliveredBy DeliveringPartyReference
	evidence    ReceptionEvidenceReference
	version     IntakeResultVersion
	association ParcelAssociationReference
	receivedAt  time.Time
}

func FormNodeIntake(spec NodeIntakeSpec) (NodeIntake, error) {
	if !spec.TenantID.valid() ||
		!spec.Unit.valid() ||
		!spec.Node.valid() ||
		!spec.DeliveredBy.valid() ||
		!spec.Evidence.valid() ||
		!spec.Version.valid() ||
		spec.ReceivedAt.IsZero() {
		return NodeIntake{}, ErrInvalidNodeIntake
	}
	return NodeIntake{
		tenantID:    spec.TenantID,
		unit:        spec.Unit,
		node:        spec.Node,
		deliveredBy: spec.DeliveredBy,
		evidence:    spec.Evidence,
		version:     spec.Version,
		association: spec.Association,
		receivedAt:  spec.ReceivedAt.UTC(),
	}, nil
}

func (intake NodeIntake) TenantID() TenantID {
	return intake.tenantID
}

func (intake NodeIntake) Unit() HandlingUnitID {
	return intake.unit
}

func (intake NodeIntake) Node() NodeReference {
	return intake.node
}

func (intake NodeIntake) DeliveredBy() DeliveringPartyReference {
	return intake.deliveredBy
}

func (intake NodeIntake) Evidence() ReceptionEvidenceReference {
	return intake.evidence
}

func (intake NodeIntake) Version() IntakeResultVersion {
	return intake.version
}

// Association 报告版本化的正式包裹关联及其是否在场。缺席是真话：这是一件待识别实物，
// 识别成功后由新的关联版本补上。
func (intake NodeIntake) Association() (ParcelAssociationReference, bool) {
	return intake.association, intake.association.valid()
}

// ReceivedAt 是实际接收发生时间——parcel-shipment 的责任起点与正式承诺生效时间最终
// 锚在它上。
func (intake NodeIntake) ReceivedAt() time.Time {
	return intake.receivedAt
}

// Identify 为待识别实物补上版本化包裹关联，交回新版本的收寄结果：原记录不删、已发生
// 事实不复制（CONTEXT：识别成功后通过版本化关联连接正式对象）。已有关联的收寄不得
// 换绑——那是身份冲突的处置，不是识别。
func (intake NodeIntake) Identify(
	association ParcelAssociationReference,
	version IntakeResultVersion,
) (NodeIntake, error) {
	if intake.association.valid() {
		return NodeIntake{}, ErrInvalidNodeIntake
	}
	if !association.valid() || !version.valid() || version == intake.version {
		return NodeIntake{}, ErrInvalidNodeIntake
	}
	identified := intake
	identified.association = association
	identified.version = version
	return identified, nil
}
