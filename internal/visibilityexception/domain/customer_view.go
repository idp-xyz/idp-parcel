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

// FinalDimensionOver 把披露策略对终局一维的答复落到一份投影上。策略回答「准不准给这位
// 客户看终局」，回答不了「终局形成了没有」——那只有投影里 PS 已采用的终局能答。
//
// 准许展示而投影里还没有终局时，这一维答待确认：如实说等，不虚构终局（CONTEXT「不得
// 从……单一交付扫描自行推导终局」、`AT-VE-160`）。不答不展示——那会把一次没人作过的
// 披露决定说成有人作过，而终局一形成，同一份策略就要展示它。待确认与不展示原样放行：
// 策略不准看的，终局形成了也不给看。
func FinalDimensionOver(permitted ViewDimension, projection TrackingProjection) ViewDimension {
	if permitted.state != DimensionShown || projection.CarriesCustomerServiceFinal() {
		return permitted
	}
	return PendDimension()
}

// MilestonesDimensionOver 把披露策略对里程碑一维的答复落到一份投影上。策略回答「准不准
// 给这位客户看公开里程碑」，回答不了「投影里有没有一条标准追踪里程碑」——条目未归类
// （映射目录没登这一类事实，或这类事实本就不是里程碑，如初始路由、终局）时，客户能看的
// 里程碑一条也没有。
//
// 准许展示而投影里没有已归类条目时这一维答待确认，不答展示：答展示等于告诉客户「里程碑
// 可看」，背后却什么都没有。理由与 FinalDimensionOver 同：不答不展示，待确认与不展示原样
// 放行，只收窄不放宽。
func MilestonesDimensionOver(permitted ViewDimension, projection TrackingProjection) ViewDimension {
	if permitted.state != DimensionShown || projection.CarriesClassifiedMilestone() {
		return permitted
	}
	return PendDimension()
}

// ETADimensionWithoutVisibilityJudgement 把披露策略对 ETA 一维的答复落到「没有 ETA 客户
// 可见性判断」上。策略回答「准不准给这位客户看 ETA」；某一份 ETA 能不能对客，要
// `UC-VE-003` 的 ETA 客户可见性判断答（质量门槛、合同与客户服务规则，`AT-VE-187`），而本
// 上下文今天还没有这一对象，派生编排因而读不到任何获准对客的 ETA。
//
// 准许展示时这一维答待确认（CONTEXT「客户全程追踪视图只基于……获准对客展示的 ETA……
// 形成」）：答展示，客户读到的是「ETA：展示」，背后没有任何 ETA。不答不展示：`AT-VE-158`
// 预期的「不展示 ETA」对的是判断已经形成、答未达门槛那一格；这里连判断都没有，答不展示是把
// 没人作过的判断说成作过，理由与 FinalDimensionOver 同。待确认与不展示原样放行。ETA 客户可见性
// 判断落地时，本函数改为收那份判断、按它的结论答，而不是另起一道。
func ETADimensionWithoutVisibilityJudgement(permitted ViewDimension) ViewDimension {
	if permitted.state != DimensionShown {
		return permitted
	}
	return PendDimension()
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
	confirmedOn  ProjectionVersionID
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
		confirmedOn: basedOn,
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

// BasedOn 是本版形成时所依据的投影版本——视图只基于当前有效投影形成。它随版本一起定下，
// 此后不变：Reaffirm 不动它，按版本读回的已发布版本答的也是它。
func (view CustomerTrackingView) BasedOn() ProjectionVersionID {
	return view.basedOn
}

// ConfirmedOn 是本版最近一次按之确认四维未变的投影版本。形成即确认一次，所以未经 Reaffirm
// 时等于 BasedOn；当前版跟上较新投影时只有这一格前移。派生编排的幂等判断读它：同一投影
// 版本已确认过，就是重放。它是内部派生的账，不是对客发布内容，客户追踪响应体不带它——
// 客户的采用版本由版本号承担（CONTEXT「留存投影版本的按版本读回属运营追踪查阅，不属客户
// 发布物」）。被替代之后不再有当前确认，按版本读回的历史版这一格退回 BasedOn。
func (view CustomerTrackingView) ConfirmedOn() ProjectionVersionID {
	return view.confirmedOn
}

func (view CustomerTrackingView) Dimensions() CustomerViewDimensions {
	return view.dimensions
}

// Equal 回答两组四维对客户是否是同一份答复：逐维三态与内容来处都相同。视图换不换代
// 只看这一问（CONTEXT「客户全程追踪视图的版本由披露更正与替代关系驱动，不与内部投影
// 版本一一对应」）。
func (dimensions CustomerViewDimensions) Equal(other CustomerViewDimensions) bool {
	return dimensions == other
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
// Supersede：前一版虽按版本读得回，重建一个版本不该先去读另一个版本。
//
// ConfirmedOn 可缺：缺席即未经重新确认，取 BasedOn。
type CustomerViewSnapshot struct {
	Version      CustomerViewVersionID
	Customer     CustomerAccountReference
	Parcel       TrackedParcelReference
	BasedOn      ProjectionVersionID
	ConfirmedOn  ProjectionVersionID
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
	if snapshot.ConfirmedOn.valid() {
		view.confirmedOn = snapshot.ConfirmedOn
	}
	return view, nil
}

// Reaffirm 让当前版跟上一份较新的投影：按那份投影重新得出的四维与本版相同时不形成新版本
// （CONTEXT「投影重新派生本身不要求形成新的客户视图版本」），只前移 ConfirmedOn。版本、
// 四维、发布时间、形成时依据与前版指回原样保留——客户已经读到的那一版不因内部投影换
// 版本而被改写。四维变了就必须 Supersede，这里拒绝：不换代却换内容，正是静默改写历史
// 披露。投影已确认过也拒绝：那是重放，由调用方按已有结果答。
func (view CustomerTrackingView) Reaffirm(
	confirmedOn ProjectionVersionID,
	dimensions CustomerViewDimensions,
) (CustomerTrackingView, error) {
	if !confirmedOn.valid() || confirmedOn == view.confirmedOn || !dimensions.Equal(view.dimensions) {
		return CustomerTrackingView{}, ErrInvalidCustomerView
	}
	reaffirmed := view
	reaffirmed.confirmedOn = confirmedOn
	return reaffirmed, nil
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
