package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ErrInvalidActualMeasurement 是一次实际测量报不全：重量、长宽高或它们的单位缺了一格。恢复动作是
// 改这次上报。它与作废的拒绝分开：作废指的是另一条已经在册的测量。
var ErrInvalidActualMeasurement = errors.New("node operations: invalid actual measurement")

// ErrInvalidMeasurementInvalidation 是一次作废报不全。恢复动作是补上被作废的那条来源身份、
// 原因和这次作废自己的来源。
var ErrInvalidMeasurementInvalidation = errors.New("node operations: invalid measurement invalidation")

// MeasurementObjectKind 是实际测量所指向的作业对象。作业实物与集运单元是两种身份，
// 裸标识分不出是哪一种，所以种类与标识分两格。
type MeasurementObjectKind uint8

const (
	MeasurementObjectKindInvalid MeasurementObjectKind = iota
	HandlingUnitObject
	ConsolidationUnitObject
)

func (kind MeasurementObjectKind) valid() bool {
	return kind == HandlingUnitObject || kind == ConsolidationUnitObject
}

func (kind MeasurementObjectKind) String() string {
	switch kind {
	case HandlingUnitObject:
		return "HANDLING_UNIT"
	case ConsolidationUnitObject:
		return "CONSOLIDATION_UNIT"
	case MeasurementObjectKindInvalid:
		return ""
	default:
		return ""
	}
}

// MeasurementObjectKindFromName 把册上的词译回种类。词表外的拒：库里出现不认识的种类
// 只可能是写入方绕过了构造器。
func MeasurementObjectKindFromName(name string) (MeasurementObjectKind, error) {
	switch name {
	case HandlingUnitObject.String():
		return HandlingUnitObject, nil
	case ConsolidationUnitObject.String():
		return ConsolidationUnitObject, nil
	default:
		return MeasurementObjectKindInvalid, ErrInvalidActualMeasurement
	}
}

// WorkLocationReference 指名测量发生的位置。节点还没有作业位置登记册，这里只保留
// 现场报上来的引用，不解释它指向哪里。
type WorkLocationReference struct{ requiredValue }

func NewWorkLocationReference(value string) (WorkLocationReference, error) {
	required, err := newRequiredValue("work location reference", value)
	return WorkLocationReference{required}, err
}

// MeasurementOriginalDocument 指名手工录入所凭的原件。它与录入人成对出现：只有原件、
// 说不出是谁照着它录的，这份测量就分不出是现场事实还是事后补记。
type MeasurementOriginalDocument struct{ requiredValue }

func NewMeasurementOriginalDocument(value string) (MeasurementOriginalDocument, error) {
	required, err := newRequiredValue("measurement original document", value)
	return MeasurementOriginalDocument{required}, err
}

// measurementSourceAtRegisterInstant 在测量与作废形成处，把来源上的发生时间折成 registerInstant 的样子（UTC、微秒）。
// 接入口按 RFC 3339 能收到纳秒，而登记册的时间列只存到微秒，读回时又用行上的值重算内容摘要去比存下的：不在形成处截，
// 一条带纳秒的事实落册前后摘要就差几个纳秒，登记成功之后再也读不回来，重放也被判成别的内容。截在这里，来源上的
// 发生时间、摘要、落册、读回，以及据此发出的变化通知，用的是同一个时刻。
//
// 折叠只落在这两类事实上，不改 NewWorkFactSource：它被收寄、集运等别的作业事实共用，那些册上已有的行与它们的
// 重放比对不能跟着动。来源是值，这里改的是副本，调用方手里的那份不变。
func measurementSourceAtRegisterInstant(source WorkFactSource) WorkFactSource {
	source.occurredAt = registerInstant(source.occurredAt)
	return source
}

// ActualMeasurementSpec 是形成一条实际测量所需的全部输入。毛重与外廓尺寸各是一个值对象，数量与单位在它们的构造器里核；
// 对产品内置单位目录的核对在入口做，这里不换算、不改写数字。
type ActualMeasurementSpec struct {
	Source           WorkFactSource
	ObjectKind       MeasurementObjectKind
	ObjectID         string
	Node             NodeReference
	Location         WorkLocationReference
	Basis            WorkBasisReference
	Weight           GrossWeight
	Dimensions       OuterDimensions
	EnteredBy        string
	OriginalDocument MeasurementOriginalDocument
}

// ActualMeasurement 是一条不可覆盖的实际测量：一次称重量方，毛重与外廓尺寸同在。更正与复测都是另登一条完整的，
// 要撤掉旧的就把旧的作废；本类型上没有改结果的方法。
type ActualMeasurement struct {
	source           WorkFactSource
	objectKind       MeasurementObjectKind
	objectID         string
	node             NodeReference
	location         WorkLocationReference
	basis            WorkBasisReference
	weight           GrossWeight
	dimensions       OuterDimensions
	enteredBy        string
	originalDocument MeasurementOriginalDocument
}

// NewActualMeasurement 收下一条测量。毛重与外廓尺寸都必填：只称了重、没量方的那一条不收，反过来也不收——计价重量取实重
// 与体积重的较大者，缺一样这条测量就算不出计价重量；两个值对象的零值就是「缺了」。录入人与原件要么都在、要么都不在——
// 设备上报没有录入人，手工录入缺一格就说不清这份数字是谁照哪张原件写的。
func NewActualMeasurement(spec ActualMeasurementSpec) (ActualMeasurement, error) {
	source := measurementSourceAtRegisterInstant(spec.Source)
	if !source.valid() || !spec.ObjectKind.valid() || strings.TrimSpace(spec.ObjectID) == "" ||
		!spec.Node.valid() || !spec.Location.valid() || !spec.Basis.valid() ||
		!spec.Weight.valid() || !spec.Dimensions.valid() {
		return ActualMeasurement{}, ErrInvalidActualMeasurement
	}
	enteredBy := strings.TrimSpace(spec.EnteredBy)
	hasDocument := spec.OriginalDocument.valid()
	if (enteredBy == "") != !hasDocument {
		return ActualMeasurement{}, ErrInvalidActualMeasurement
	}
	return ActualMeasurement{
		source:           source,
		objectKind:       spec.ObjectKind,
		objectID:         spec.ObjectID,
		node:             spec.Node,
		location:         spec.Location,
		basis:            spec.Basis,
		weight:           spec.Weight,
		dimensions:       spec.Dimensions,
		enteredBy:        enteredBy,
		originalDocument: spec.OriginalDocument,
	}, nil
}

func (measurement ActualMeasurement) Source() WorkFactSource { return measurement.source }

func (measurement ActualMeasurement) ObjectKind() MeasurementObjectKind {
	return measurement.objectKind
}

func (measurement ActualMeasurement) ObjectID() string { return measurement.objectID }

func (measurement ActualMeasurement) Node() NodeReference { return measurement.node }

func (measurement ActualMeasurement) Location() WorkLocationReference { return measurement.location }

func (measurement ActualMeasurement) Basis() WorkBasisReference { return measurement.basis }

// Weight 交回毛重。单位是现场报的原词，没有换算。
func (measurement ActualMeasurement) Weight() GrossWeight { return measurement.weight }

// Dimensions 交回外廓尺寸。三边共用现场报的那一个单位。
func (measurement ActualMeasurement) Dimensions() OuterDimensions { return measurement.dimensions }

// ManualEntry 给出手工录入的人与原件。设备上报的测量没有这两格。
func (measurement ActualMeasurement) ManualEntry() (string, MeasurementOriginalDocument, bool) {
	if measurement.enteredBy == "" {
		return "", MeasurementOriginalDocument{}, false
	}
	return measurement.enteredBy, measurement.originalDocument, true
}

// ContentDigest 是这条测量的内容摘要。来源身份不在里面：它是幂等键，同一键下摘要
// 不同才是冲突。记录时刻也不在里面，它由服务端另记。
func (measurement ActualMeasurement) ContentDigest() string {
	return measurementDigest(measurementDocument{
		Face:             "ACTUAL_MEASUREMENT",
		ObjectKind:       measurement.objectKind.String(),
		ObjectID:         measurement.objectID,
		Node:             measurement.node.String(),
		Location:         measurement.location.String(),
		Basis:            measurement.basis.String(),
		WeightValue:      measurement.weight.Value(),
		WeightUnit:       measurement.weight.Unit(),
		Length:           measurement.dimensions.Length(),
		Width:            measurement.dimensions.Width(),
		Height:           measurement.dimensions.Height(),
		DimensionUnit:    measurement.dimensions.Unit(),
		PerformedBy:      measurement.source.PerformedBy().String(),
		Evidence:         measurement.source.Evidence().String(),
		OccurredAt:       measurement.source.OccurredAt().UTC().Format(time.RFC3339Nano),
		EnteredBy:        measurement.enteredBy,
		OriginalDocument: measurement.originalDocument.String(),
	})
}

// MeasurementInvalidationSpec 是作废一条已登记测量所需的输入。TargetSourceID 指回被作废
// 那条的来源身份，不复制它的数字。
type MeasurementInvalidationSpec struct {
	Source           WorkFactSource
	TargetSourceID   string
	Reason           string
	EnteredBy        string
	OriginalDocument MeasurementOriginalDocument
}

// MeasurementInvalidation 是一条作废事实。它不改被作废的那条测量，只让「仍有效」的
// 读口不再把那条交出来。
type MeasurementInvalidation struct {
	source           WorkFactSource
	targetSourceID   string
	reason           string
	enteredBy        string
	originalDocument MeasurementOriginalDocument
}

// NewMeasurementInvalidation 收下一条作废。原因去掉首尾空白后必须还在；作废自己的来源
// 身份不得与被作废那条相同，否则一次上报会同时是测量又是作废。
func NewMeasurementInvalidation(spec MeasurementInvalidationSpec) (MeasurementInvalidation, error) {
	source := measurementSourceAtRegisterInstant(spec.Source)
	reason := strings.TrimSpace(spec.Reason)
	if !source.valid() || strings.TrimSpace(spec.TargetSourceID) == "" || reason == "" ||
		source.SourceID() == spec.TargetSourceID {
		return MeasurementInvalidation{}, ErrInvalidMeasurementInvalidation
	}
	enteredBy := strings.TrimSpace(spec.EnteredBy)
	if (enteredBy == "") != !spec.OriginalDocument.valid() {
		return MeasurementInvalidation{}, ErrInvalidMeasurementInvalidation
	}
	return MeasurementInvalidation{
		source:           source,
		targetSourceID:   spec.TargetSourceID,
		reason:           reason,
		enteredBy:        enteredBy,
		originalDocument: spec.OriginalDocument,
	}, nil
}

func (invalidation MeasurementInvalidation) Source() WorkFactSource { return invalidation.source }

func (invalidation MeasurementInvalidation) TargetSourceID() string {
	return invalidation.targetSourceID
}

func (invalidation MeasurementInvalidation) Reason() string { return invalidation.reason }

func (invalidation MeasurementInvalidation) ManualEntry() (string, MeasurementOriginalDocument, bool) {
	if invalidation.enteredBy == "" {
		return "", MeasurementOriginalDocument{}, false
	}
	return invalidation.enteredBy, invalidation.originalDocument, true
}

// ContentDigest 是这条作废的内容摘要。被作废的来源身份在里面：同一条作废身份改指
// 另一条测量，是冲突，不是重放。
func (invalidation MeasurementInvalidation) ContentDigest() string {
	return invalidationDigest(invalidationDocument{
		Face:             "MEASUREMENT_INVALIDATION",
		TargetSourceID:   invalidation.targetSourceID,
		Reason:           invalidation.reason,
		PerformedBy:      invalidation.source.PerformedBy().String(),
		Evidence:         invalidation.source.Evidence().String(),
		OccurredAt:       invalidation.source.OccurredAt().UTC().Format(time.RFC3339Nano),
		EnteredBy:        invalidation.enteredBy,
		OriginalDocument: invalidation.originalDocument.String(),
	})
}

type measurementDocument struct {
	Canonicalization string `json:"canonicalization"`
	Face             string `json:"face"`
	ObjectKind       string `json:"objectKind,omitempty"`
	ObjectID         string `json:"objectId,omitempty"`
	Node             string `json:"node,omitempty"`
	Location         string `json:"location,omitempty"`
	Basis            string `json:"basis,omitempty"`
	WeightValue      string `json:"weightValue,omitempty"`
	WeightUnit       string `json:"weightUnit,omitempty"`
	Length           string `json:"length,omitempty"`
	Width            string `json:"width,omitempty"`
	Height           string `json:"height,omitempty"`
	DimensionUnit    string `json:"dimensionUnit,omitempty"`
	PerformedBy      string `json:"performedBy"`
	Evidence         string `json:"evidence"`
	OccurredAt       string `json:"occurredAt"`
	EnteredBy        string `json:"enteredBy,omitempty"`
	OriginalDocument string `json:"originalDocument,omitempty"`
}

// invalidationDocument 是作废内容摘要的规范化形状。被作废的来源身份与原因各用自己的 Go 字段名，JSON 键名与字段顺序
// 却必须与已落册的摘要字节一致：被作废的来源身份仍写在 objectId，原因仍写在 basis，改了键名或顺序，册上存下的作废
// 就被判成别的内容。
type invalidationDocument struct {
	Canonicalization string `json:"canonicalization"`
	Face             string `json:"face"`
	TargetSourceID   string `json:"objectId,omitempty"`
	Reason           string `json:"basis,omitempty"`
	PerformedBy      string `json:"performedBy"`
	Evidence         string `json:"evidence"`
	OccurredAt       string `json:"occurredAt"`
	EnteredBy        string `json:"enteredBy,omitempty"`
	OriginalDocument string `json:"originalDocument,omitempty"`
}

func measurementDigest(document measurementDocument) string {
	document.Canonicalization = payloadCanonicalizationVersion
	return canonicalDigest(document)
}

func invalidationDigest(document invalidationDocument) string {
	document.Canonicalization = payloadCanonicalizationVersion
	return canonicalDigest(document)
}

func canonicalDigest(document any) string {
	encoded, err := json.Marshal(document)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return payloadCanonicalizationVersion + ":" + hex.EncodeToString(sum[:])
}

func newMeasurementQuantity(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if !measurementDecimalShape(trimmed) || !measurementPositive(trimmed) {
		return "", ErrInvalidActualMeasurement
	}
	return trimmed, nil
}

func measurementDecimalShape(raw string) bool {
	if raw == "" {
		return false
	}
	whole, fraction, dotted := strings.Cut(raw, ".")
	if !measurementDigits(whole) {
		return false
	}
	return !dotted || measurementDigits(fraction)
}

func measurementDigits(raw string) bool {
	if raw == "" {
		return false
	}
	for _, character := range raw {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func measurementPositive(raw string) bool {
	for _, character := range raw {
		if character >= '1' && character <= '9' {
			return true
		}
	}
	return false
}
