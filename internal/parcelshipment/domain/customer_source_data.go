package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidSourceDataScope           = errors.New("parcel shipment: invalid source data scope")
	ErrInvalidCustomerSourceDataVersion = errors.New("parcel shipment: invalid customer source data version")
	ErrShipmentRequestNotAccepted       = errors.New("parcel shipment: shipment request is not accepted")
	// ErrParcelOutsideAcceptanceBaseline 是业务拒绝而不是输入格式错：范围本身合法，只是它
	// 指的成员不在这份委托接受时固定的集合里。两者分开，接入层才能按 UC 的结果语义各给各的
	// 回执——一个要客户改请求，一个要客户走关联新委托。
	ErrParcelOutsideAcceptanceBaseline = errors.New("parcel shipment: parcel is outside the acceptance baseline")
)

type SourceDataVersionID struct{ requiredValue }

func NewSourceDataVersionID(value string) (SourceDataVersionID, error) {
	required, err := newRequiredValue("source data version ID", value)
	return SourceDataVersionID{required}, err
}

// SourceDataGroupReference 是字段或资料组的引用，不是自由文本描述。UC-PS-002 明禁「以批次、
// 袋、总单或客户名称模糊指代」，而下游要按范围重新判断，指代不明就无从判断影响哪一块。
type SourceDataGroupReference struct{ requiredValue }

func NewSourceDataGroupReference(value string) (SourceDataGroupReference, error) {
	required, err := newRequiredValue("source data group reference", value)
	return SourceDataGroupReference{required}, err
}

// AmendmentReasonReference 是结构化补充/更正原因。与拒绝原因同理：一句自由说明既统计不了，
// 也判断不出它是否落在已登记的允许动作里。
type AmendmentReasonReference struct{ requiredValue }

func NewAmendmentReasonReference(value string) (AmendmentReasonReference, error) {
	required, err := newRequiredValue("amendment reason reference", value)
	return AmendmentReasonReference{required}, err
}

// RequesterReference 是提出请求的一方，与 DeciderReference（实际决定方）分立。UC-PS-002 要求
// 「登录操作人不能替代实际决定方」——合成一格就把这条要求删掉了。
type RequesterReference struct{ requiredValue }

func NewRequesterReference(value string) (RequesterReference, error) {
	required, err := newRequiredValue("requester reference", value)
	return RequesterReference{required}, err
}

// AmendmentAuthoritySnapshot 是本次修订所采用的授权依据快照。授权规则属 party-commercial，
// 本上下文只记所采用的那一份，不判断它够不够格。
type AmendmentAuthoritySnapshot struct{ requiredValue }

func NewAmendmentAuthoritySnapshot(value string) (AmendmentAuthoritySnapshot, error) {
	required, err := newRequiredValue("amendment authority snapshot", value)
	return AmendmentAuthoritySnapshot{required}, err
}

// SourceDataScope 指名这次修订作用在哪里。委托与资料组必填，包裹可空——寄件人一类资料作用
// 于整份委托，逐包裹填反而会把一处更正复制成成员份数。
type SourceDataScope struct {
	shipmentRequestID ShipmentRequestID
	parcelID          DeclaredParcelID
	dataGroup         SourceDataGroupReference
}

// NewShipmentScopedSourceData 形成作用于整份委托的资料范围。
func NewShipmentScopedSourceData(
	requestID ShipmentRequestID,
	dataGroup SourceDataGroupReference,
) (SourceDataScope, error) {
	if !requestID.valid() || !dataGroup.valid() {
		return SourceDataScope{}, ErrInvalidSourceDataScope
	}
	return SourceDataScope{shipmentRequestID: requestID, dataGroup: dataGroup}, nil
}

// NewParcelScopedSourceData 形成只作用于某个声明包裹的资料范围。它与委托级分成两个构造器，
// 而不是留一个可空包裹字段：可空字段分不清「作用于整份委托」与「忘了指名包裹」，而 UC 要求
// 逐包裹、逐字段分别形成结果。
func NewParcelScopedSourceData(
	requestID ShipmentRequestID,
	parcelID DeclaredParcelID,
	dataGroup SourceDataGroupReference,
) (SourceDataScope, error) {
	if !requestID.valid() || !parcelID.valid() || !dataGroup.valid() {
		return SourceDataScope{}, ErrInvalidSourceDataScope
	}
	return SourceDataScope{
		shipmentRequestID: requestID,
		parcelID:          parcelID,
		dataGroup:         dataGroup,
	}, nil
}

func (scope SourceDataScope) ShipmentRequestID() ShipmentRequestID {
	return scope.shipmentRequestID
}

// DeclaredParcelID 的第二个返回值区分「作用于整份委托」与「作用于某个包裹」。
func (scope SourceDataScope) DeclaredParcelID() (DeclaredParcelID, bool) {
	return scope.parcelID, scope.parcelID.valid()
}

func (scope SourceDataScope) DataGroup() SourceDataGroupReference {
	return scope.dataGroup
}

func (scope SourceDataScope) valid() bool {
	return scope.shipmentRequestID.valid() && scope.dataGroup.valid()
}

// SourceDataBasis 是本次修订所依据的基准，两种合法形态：某个既有资料版本，或接受基线——
// 客户首次补充缺失资料时没有前序版本可依据。
//
// 零值不是合法基准。它会让「以基线为准」与「基础版本没填」变成同一个值，而 UC 要求基础版本
// 不同即形成请求冲突，分不开就判不出冲突。
type SourceDataBasis struct {
	priorVersion         SourceDataVersionID
	onAcceptanceBaseline bool
}

func NewAmendmentOfVersion(prior SourceDataVersionID) (SourceDataBasis, error) {
	if !prior.valid() {
		return SourceDataBasis{}, ErrInvalidCustomerSourceDataVersion
	}
	return SourceDataBasis{priorVersion: prior}, nil
}

func NewSupplementOnAcceptanceBaseline() SourceDataBasis {
	return SourceDataBasis{onAcceptanceBaseline: true}
}

// PriorVersion 的第二个返回值为假时表示本次以接受基线为基准，而不是「读不到前序版本」。
func (basis SourceDataBasis) PriorVersion() (SourceDataVersionID, bool) {
	return basis.priorVersion, basis.priorVersion.valid()
}

func (basis SourceDataBasis) OnAcceptanceBaseline() bool {
	return basis.onAcceptanceBaseline
}

func (basis SourceDataBasis) valid() bool {
	return basis.priorVersion.valid() != basis.onAcceptanceBaseline
}

type CustomerSourceDataVersionSpec struct {
	VersionID SourceDataVersionID
	Scope     SourceDataScope
	Basis     SourceDataBasis
	Request   SourceSubmissionFingerprint
	Reason    AmendmentReasonReference
	Requester RequesterReference
	Decider   DeciderReference
	Authority AmendmentAuthoritySnapshot
	// EffectiveAt 是客户声明的资料适用时间（`requestEffectiveAt`），可以缺失：客户没说这份
	// 资料何时起适用是常态。缺失时保持零值，绝不用 occurredAt 或 receivedAt 顶替。
	EffectiveAt time.Time
	FormedAt    time.Time
}

// CustomerSourceDataVersion 是一份不可覆盖的客户来源版本。它保存的是客户声明，不是节点实测、
// 正式申报资料或最终计费资料——那三样各有来源与所有权，本版本触发它们重新判断，不覆盖它们。
type CustomerSourceDataVersion struct {
	versionID   SourceDataVersionID
	scope       SourceDataScope
	basis       SourceDataBasis
	request     SourceSubmissionFingerprint
	reason      AmendmentReasonReference
	requester   RequesterReference
	decider     DeciderReference
	authority   AmendmentAuthoritySnapshot
	effectiveAt time.Time
	formedAt    time.Time
}

// FormCustomerSourceDataVersion 形成一份客户原始资料版本。
//
// 留痕清单整条必填，因为版本不可覆盖：少一项就固定成一份半截版本，事后补不回来，而
// CONTEXT 要求版本「必须明确关联委托、包裹、字段或资料范围、基础版本、原因、请求方、实际
// 决定方、授权快照和业务/适用时间」。适用时间是清单里唯一可缺的一项，缺失本身是有意义的
// 客户事实。
func FormCustomerSourceDataVersion(spec CustomerSourceDataVersionSpec) (CustomerSourceDataVersion, error) {
	version := CustomerSourceDataVersion{
		versionID:   spec.VersionID,
		scope:       spec.Scope,
		basis:       spec.Basis,
		request:     spec.Request,
		reason:      spec.Reason,
		requester:   spec.Requester,
		decider:     spec.Decider,
		authority:   spec.Authority,
		effectiveAt: spec.EffectiveAt,
		formedAt:    spec.FormedAt,
	}
	if !version.valid() {
		return CustomerSourceDataVersion{}, ErrInvalidCustomerSourceDataVersion
	}
	return version, nil
}

func (version CustomerSourceDataVersion) VersionID() SourceDataVersionID {
	return version.versionID
}

func (version CustomerSourceDataVersion) Scope() SourceDataScope {
	return version.scope
}

func (version CustomerSourceDataVersion) Basis() SourceDataBasis {
	return version.basis
}

func (version CustomerSourceDataVersion) Request() SourceSubmissionFingerprint {
	return version.request
}

func (version CustomerSourceDataVersion) Reason() AmendmentReasonReference {
	return version.reason
}

func (version CustomerSourceDataVersion) Requester() RequesterReference {
	return version.requester
}

func (version CustomerSourceDataVersion) Decider() DeciderReference {
	return version.decider
}

func (version CustomerSourceDataVersion) Authority() AmendmentAuthoritySnapshot {
	return version.authority
}

func (version CustomerSourceDataVersion) EffectiveAt() time.Time {
	return version.effectiveAt
}

// HasEffectiveAt 让「客户没声明适用时间」可读。它与零值时间分开暴露，是因为下游按适用时间
// 重新派生判断，而「没有适用时间」要走的是另一条路，不是「适用时间为零点」。
func (version CustomerSourceDataVersion) HasEffectiveAt() bool {
	return !version.effectiveAt.IsZero()
}

func (version CustomerSourceDataVersion) FormedAt() time.Time {
	return version.formedAt
}

// AmendCustomerSourceData 把一份客户原始资料版本追加到已接受委托上。
//
// 只对`已接受`开放：决定前的纠错按 CONTEXT 形成新的提交版本并重新判断，已拒绝或已撤回后的
// 新需求形成关联新委托。三条路各有各的重新判断，用资料版本抄近道就把那一步跳过了。
//
// 它不改接受基线，也不动既有版本——版本不可覆盖，一份被顶掉的旧版本连同它的原因、授权与
// 适用时间一起消失，而下游可能正按它办事。
func (request ShipmentRequest) AmendCustomerSourceData(
	version CustomerSourceDataVersion,
) (ShipmentRequest, error) {
	if request.state != ShipmentRequestAccepted {
		return ShipmentRequest{}, ErrShipmentRequestNotAccepted
	}
	if !version.valid() {
		return ShipmentRequest{}, ErrInvalidCustomerSourceDataVersion
	}
	if version.scope.shipmentRequestID != request.shipmentRequestID {
		return ShipmentRequest{}, ErrInvalidSourceDataScope
	}
	if request.SourceDataScopeOutsideAcceptanceBaseline(version.scope) {
		return ShipmentRequest{}, ErrParcelOutsideAcceptanceBaseline
	}

	// 复制而不是就地 append：ShipmentRequest 按值传递，共用底层数组会让两条从同一份委托
	// 分出去的修订互相覆盖对方追加的版本。
	appended := make([]CustomerSourceDataVersion, 0, len(request.sourceDataVersions)+1)
	appended = append(appended, request.sourceDataVersions...)
	request.sourceDataVersions = append(appended, version)
	return request, nil
}

// SourceDataScopeOutsideAcceptanceBaseline 回答某处资料范围是否指向接受基线之外的成员。
//
// 指名成员的范围才受此限。不指名成员的委托级范围一律在内——寄件人一类资料本就作用于整份
// 委托，拿成员去卡它会把一份合法更正拒掉。
//
// 它单独暴露，是为了让编排在形成版本之前就能问：`AT-PS-023` 的拒绝不需要任何已登记目录，
// 基线自己就是成员集合的权威。把这一问压到 AmendCustomerSourceData 里才发生，它就落在了
// 规则矩阵查询的下游，一次矩阵读不回会把确定的业务拒绝变成未决。两处共用这一段判断，因此
// 权威仍然只有一处。
//
// 尚未接受的委托交回 false：那时没有基线可比，「越过基线」这个问题谈不上，真正的答案是
// `还没接受`，由 AmendCustomerSourceData 的状态闸门给出。
func (request ShipmentRequest) SourceDataScopeOutsideAcceptanceBaseline(scope SourceDataScope) bool {
	if request.state != ShipmentRequestAccepted {
		return false
	}
	parcelID, named := scope.DeclaredParcelID()
	return named && !request.baseline.covers(parcelID)
}

// CustomerSourceDataVersions 按形成顺序交回全部版本。
func (request ShipmentRequest) CustomerSourceDataVersions() []CustomerSourceDataVersion {
	return append([]CustomerSourceDataVersion(nil), request.sourceDataVersions...)
}

// SourceDataAdoptionOutcome 是某个资料范围上当前采用判断的结论。
//
// CONTEXT 声明的封闭集合有五项：已采用、待下游判断、待补充、冲突、未决。这里只放已经有产生
// 规则的两项，按本仓「枚举取值与产生它的规则同时出现」的处置办。`待复核`是 UC-PS-002 对
// CONTEXT`待补充`那一格的点名叫法，用在重叠范围上——客户补不出东西来，要人来并。
//
// 缺产生规则的三项：`待下游判断`取决于 BD-PS-010 的字段与阶段矩阵；`冲突`与`待复核`的分界
// 取决于 BD-PS-012 的合并规则，未登记前 UC 要求一律停在`待复核`，因此`冲突`现在无从产生；
// `未决`要等依赖读不回来这类情形，那属应用编排，不在本层。
type SourceDataAdoptionOutcome uint8

const (
	SourceDataAdoptionOutcomeInvalid SourceDataAdoptionOutcome = iota
	SourceDataAdopted
	SourceDataAwaitingReview
)

func (outcome SourceDataAdoptionOutcome) String() string {
	switch outcome {
	case SourceDataAdopted:
		return "ADOPTED"
	case SourceDataAwaitingReview:
		return "AWAITING_REVIEW"
	default:
		return ""
	}
}

// SourceDataAdoptionJudgment 是某个资料范围上当前可供下游消费的引用。它是派生结果，不是可
// 手工覆盖的全局资料状态——改它只能靠再形成一份版本。
type SourceDataAdoptionJudgment struct {
	outcome SourceDataAdoptionOutcome
	adopted SourceDataVersionID
}

func (judgment SourceDataAdoptionJudgment) Outcome() SourceDataAdoptionOutcome {
	return judgment.outcome
}

// AdoptedVersion 指名当前该被下游消费的那一版。
func (judgment SourceDataAdoptionJudgment) AdoptedVersion() (SourceDataVersionID, bool) {
	return judgment.adopted, judgment.adopted.valid()
}

// CurrentSourceDataAdoption 按版本关系派生某个资料范围上的当前采用判断。
//
// 判断按范围分别派生：CONTEXT 要求它依据「对象范围」形成，而 UC 明禁形成批量级的总状态。
// 该范围上没有版本时不存在判断，与「有判断但还没定」是两回事。
//
// 裁决靠基准关系而不是到达顺序。一份基准已经过期的版本照样追加保存，但它不推进采用判断——
// 让它推进就等于「最后到达者获胜」，而那正是 UC 点名禁止的：两位客户代表各自基于同一版改同
// 一块资料时，后写的那份会静默吃掉先写的那份。停在`待复核`要人来并，合并规则登记之前没有
// 别的正确答案。
func (request ShipmentRequest) CurrentSourceDataAdoption(
	scope SourceDataScope,
) (SourceDataAdoptionJudgment, bool) {
	judgment := SourceDataAdoptionJudgment{}
	forked := false
	for _, version := range request.sourceDataVersions {
		if version.scope != scope {
			continue
		}
		prior, chained := version.basis.PriorVersion()
		if !chained || prior == judgment.adopted {
			judgment.outcome = SourceDataAdopted
			judgment.adopted = version.versionID
			continue
		}
		// 分叉一旦出现就一直留着，直到有人并掉它。此后基准正确的修订仍推进 adopted——它们
		// 确实接在当前采用的那一版后面——但不能顺手把判断推回`已采用`：一次无关的修订不构成
		// 对分叉的裁决，而判断回到`已采用`就没人再去看那条没并的支线了。
		forked = true
	}
	if forked {
		judgment.outcome = SourceDataAwaitingReview
	}
	return judgment, judgment.outcome != SourceDataAdoptionOutcomeInvalid
}

func (version CustomerSourceDataVersion) valid() bool {
	return version.versionID.valid() &&
		version.scope.valid() &&
		version.basis.valid() &&
		version.request.valid() &&
		version.reason.valid() &&
		version.requester.valid() &&
		version.decider.valid() &&
		version.authority.valid() &&
		!version.formedAt.IsZero()
}
