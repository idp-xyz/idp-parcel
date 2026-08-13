package domain

import (
	"errors"
	"time"
)

var ErrInvalidCustomerView = errors.New("visibility exception: invalid customer tracking view")

// CustomerAccountReference 指名货主客户账户（party-commercial 拥有）。客户视图按它
// 隔离——没有账户维度的视图谈不上「只包含当前货主客户账户及其授权对象范围」。
type CustomerAccountReference struct{ requiredValue }

func NewCustomerAccountReference(value string) (CustomerAccountReference, error) {
	required, err := newRequiredValue("customer account reference", value)
	return CustomerAccountReference{required}, err
}

// ViewContentReference 指名一维展示内容的来处（公开里程碑集、获准展示的 ETA、客户
// 服务终局、追踪说明）。
type ViewContentReference struct{ requiredValue }

func NewViewContentReference(value string) (ViewContentReference, error) {
	required, err := newRequiredValue("view content reference", value)
	return ViewContentReference{required}, err
}

// DimensionState 是客户视图单维的封闭三态（CONTEXT 生命周期：「某一维信息待确认或
// 不可披露→只对该维返回待确认、暂不可用或不展示」）。
type DimensionState uint8

const (
	DimensionStateInvalid DimensionState = iota
	DimensionShown
	DimensionPendingConfirmation
	DimensionNotDisclosed
)

func (state DimensionState) String() string {
	switch state {
	case DimensionShown:
		return "SHOWN"
	case DimensionPendingConfirmation:
		return "PENDING_CONFIRMATION"
	case DimensionNotDisclosed:
		return "NOT_DISCLOSED"
	default:
		return ""
	}
}

// ViewDimension 是客户视图的一维：展示时必带内容来处，待确认与不展示必不带内容——
// 两个方向的虚构（无中生有与有中说无）都在构造期拦下。
type ViewDimension struct {
	state   DimensionState
	content ViewContentReference
}

// ShowDimension 形成一个展示维：内容来处必备，不虚构里程碑、ETA、终局或说明。
func ShowDimension(content ViewContentReference) (ViewDimension, error) {
	if !content.valid() {
		return ViewDimension{}, ErrInvalidCustomerView
	}
	return ViewDimension{state: DimensionShown, content: content}, nil
}

// PendDimension 形成一个待确认维——这一维的信息还没到能给客户看的程度，如实说等。
func PendDimension() ViewDimension {
	return ViewDimension{state: DimensionPendingConfirmation}
}

// WithholdDimension 形成一个不展示维——授权或披露规则不允许这一维出现。
func WithholdDimension() ViewDimension {
	return ViewDimension{state: DimensionNotDisclosed}
}

func (dimension ViewDimension) State() DimensionState {
	return dimension.state
}

// Content 只在展示态给出。
func (dimension ViewDimension) Content() (ViewContentReference, bool) {
	return dimension.content, dimension.state == DimensionShown
}

func (dimension ViewDimension) valid() bool {
	switch dimension.state {
	case DimensionShown:
		return dimension.content.valid()
	case DimensionPendingConfirmation, DimensionNotDisclosed:
		return !dimension.content.valid()
	default:
		return false
	}
}

// CustomerViewVersionID 是客户视图的版本标识。更正与替代换版本，原发布历史保留。
type CustomerViewVersionID struct{ requiredValue }

func NewCustomerViewVersionID(value string) (CustomerViewVersionID, error) {
	required, err := newRequiredValue("customer view version ID", value)
	return CustomerViewVersionID{required}, err
}

// CustomerViewDimensions 是视图的四个展示维（CONTEXT 语言：标准追踪里程碑、适用的
// 客户可见 ETA、客户服务终局和更正关系、追踪说明）。
type CustomerViewDimensions struct {
	Milestones ViewDimension
	ETA        ViewDimension
	Final      ViewDimension
	Note       ViewDimension
}

// CustomerTrackingView 是面向已授权货主客户账户的普通查询视图版本。它只基于当前
// 有效投影（引用锚必备——不得从内部案件、原始来源消息或单一交付扫描自行推导）；
// 账户隔离是字段不是约定。查询或展示本身不产生异常披露决定或主动通知——类型上
// 没有那些字段。
type CustomerTrackingView struct {
	version      CustomerViewVersionID
	customer     CustomerAccountReference
	parcel       TrackedParcelReference
	basedOn      ProjectionVersionID
	dimensions   CustomerViewDimensions
	priorVersion CustomerViewVersionID
	publishedAt  time.Time
}

// PublishCustomerView 形成首个客户视图版本。四维各自三态，任一维立不住（展示无内容
// 或待确认带内容）整个视图立不起来。
func PublishCustomerView(
	version CustomerViewVersionID,
	customer CustomerAccountReference,
	parcel TrackedParcelReference,
	basedOn ProjectionVersionID,
	dimensions CustomerViewDimensions,
	publishedAt time.Time,
) (CustomerTrackingView, error) {
	if !version.valid() || !customer.valid() || !parcel.valid() || !basedOn.valid() ||
		publishedAt.IsZero() {
		return CustomerTrackingView{}, ErrInvalidCustomerView
	}
	if !dimensions.Milestones.valid() || !dimensions.ETA.valid() ||
		!dimensions.Final.valid() || !dimensions.Note.valid() {
		return CustomerTrackingView{}, ErrInvalidCustomerView
	}
	return CustomerTrackingView{
		version:     version,
		customer:    customer,
		parcel:      parcel,
		basedOn:     basedOn,
		dimensions:  dimensions,
		publishedAt: publishedAt.UTC(),
	}, nil
}

func (view CustomerTrackingView) Version() CustomerViewVersionID {
	return view.version
}

func (view CustomerTrackingView) Customer() CustomerAccountReference {
	return view.customer
}

func (view CustomerTrackingView) Parcel() TrackedParcelReference {
	return view.parcel
}

// BasedOn 是本视图锚定的投影版本——视图只基于当前有效投影形成。
func (view CustomerTrackingView) BasedOn() ProjectionVersionID {
	return view.basedOn
}

func (view CustomerTrackingView) Dimensions() CustomerViewDimensions {
	return view.dimensions
}

func (view CustomerTrackingView) PublishedAt() time.Time {
	return view.publishedAt
}

// PriorVersion 只在更正/替代版本上给出，指回被替代的那一版。
func (view CustomerTrackingView) PriorVersion() (CustomerViewVersionID, bool) {
	return view.priorVersion, view.priorVersion.valid()
}

// CustomerViewSnapshot 是持久化层重建视图所需的全量状态。字段经 PublishCustomerView
// 的同一套不变量验证；前版指回随快照携带——替代关系是已发生的历史，重建不重演
// Supersede（重演需要原视图在手，而库里只有当前版本）。
type CustomerViewSnapshot struct {
	Version      CustomerViewVersionID
	Customer     CustomerAccountReference
	Parcel       TrackedParcelReference
	BasedOn      ProjectionVersionID
	Dimensions   CustomerViewDimensions
	PriorVersion CustomerViewVersionID
	PublishedAt  time.Time
}

// RehydrateCustomerView 从快照重建视图。读回的东西同样要过一遍不变量，否则一次坏
// 写入会在这里变成一个看起来合法的视图。
func RehydrateCustomerView(snapshot CustomerViewSnapshot) (CustomerTrackingView, error) {
	view, err := PublishCustomerView(
		snapshot.Version,
		snapshot.Customer,
		snapshot.Parcel,
		snapshot.BasedOn,
		snapshot.Dimensions,
		snapshot.PublishedAt,
	)
	if err != nil {
		return CustomerTrackingView{}, err
	}
	if snapshot.PriorVersion.valid() {
		if snapshot.PriorVersion == snapshot.Version {
			return CustomerTrackingView{}, ErrInvalidCustomerView
		}
		view.priorVersion = snapshot.PriorVersion
	}
	return view, nil
}

// Supersede 依据来源更正、有效性变化、ETA 新版本或终局更正形成新的客户视图版本
// （CONTEXT 生命周期）：换版本、换投影锚、换维度、指回原版；原发布历史保留，但不再
// 有效的内容不得继续显示为当前事实——「当前」由最新版本承担，原版本只是历史。
func (view CustomerTrackingView) Supersede(
	version CustomerViewVersionID,
	basedOn ProjectionVersionID,
	dimensions CustomerViewDimensions,
	publishedAt time.Time,
) (CustomerTrackingView, error) {
	if version == view.version {
		return CustomerTrackingView{}, ErrInvalidCustomerView
	}
	superseded, err := PublishCustomerView(
		version, view.customer, view.parcel, basedOn, dimensions, publishedAt)
	if err != nil {
		return CustomerTrackingView{}, err
	}
	superseded.priorVersion = view.version
	return superseded, nil
}
