package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidRehydratedShipmentRequest 是重建入口因快照数据本身而拒绝时给出的理由。它与
// ErrInvalidShipmentRequest 分开：后者说的是「此刻要发生的这件事不合规则」，前者说的是
// 「这份已经发生过的东西不可能是本上下文判出来的」——两者的处置完全不同，前者要人去查
// 库里那一行或写它的适配器，不是让调用方改请求重来。
var ErrInvalidRehydratedShipmentRequest = errors.New("parcel shipment: invalid rehydrated shipment request")

// ErrRehydrationStateNotSupported 是重建入口拒绝一个这扇门还没开到的委托状态时给出的理由。
//
// 它与 ErrInvalidRehydratedShipmentRequest 分开，依据是 ADR-0029 的分格规则：两者的恢复动作
// 不同——一个是等这扇门开到那个状态，一个是去查库里那一行或写它的适配器。压成同一个取值，
// 运维会照着一份完好的数据去找一处不存在的损坏。
var ErrRehydrationStateNotSupported = errors.New("parcel shipment: rehydration does not yet cover this shipment request state")

// RehydrateShipmentRequestSpec 携带重建一份委托所需的全部字段。
//
// 它是本包唯一一处「相信输入」的地方（ADR-0028）：字段一律当**数据**收下，绝不从中重新
// 推导 accepted、state、baseline 或任何派生字段。要重算的那扇门是 Decide，不是这里。
//
// ADR-0030 的准入判据是快照表达能力。今天开`已提交`与`已接受`：后者可达的决定、基线、承诺
// 与客户资料版本都有字段可填；`已拒绝`/`已撤回`仍少撤回或主动拒绝的完整表达，由入口挡住。
type RehydrateShipmentRequestSpec struct {
	// Revision 是这一行在库里的版本。重建的聚合必然已持久化，所以它必须从 1 起。
	Revision          int64
	ShipmentRequestID ShipmentRequestID
	BatchID           SubmissionBatchID
	State             ShipmentRequestState
	SubmittedAt       time.Time
	CurrentVersion    RehydrateSubmissionVersionSpec
	AcceptanceTask    RehydrateAcceptanceTaskSpec
	// PriorVersions/PriorTasks 是受控补充留下的历史（ADR-0045），按形成顺序两两对应。
	// 首次提交的委托两者皆空；有历史而不带回，一份补充过的委托重建后看起来像从未补充过，
	// 而「旧版本及其判断历史继续保留」正是那条转移存在的理由。
	PriorVersions []RehydrateSubmissionVersionSpec
	PriorTasks    []RehydrateAcceptanceTaskSpec
	// PriorLink 是关联出处在库里的样子（`AT-PS-036`/`AT-PS-076`），首次委托两个字段皆零。
	// 有出处而不带回，一份关联新委托重建后看起来像首次委托——「保留原版本或原决定」的
	// 那条线索就断在重建这一步。
	PriorLink RehydratePriorRequestLinkSpec
	// 以下四项在`已提交`下必然缺席；`已接受`下决定、基线、承诺必须在场，资料版本可空。
	Decision           RehydrateAcceptanceDecisionSpec
	DecisionFormed     bool
	Baseline           RehydrateAcceptanceBaselineSpec
	Commitment         RehydrateExpectedCommitmentSpec
	SourceDataVersions []CustomerSourceDataVersion
}

// RehydrateAcceptanceDecisionSpec 是接受决定在库里的样子。决定是产物，没有公开构造器，
// 因此另立重建面（ADR-0030）。Checks 与 Basis 收成品类型：两者都有公开构造器且字段未导出。
type RehydrateAcceptanceDecisionSpec struct {
	DecisionID   AcceptanceDecisionID
	Accepted     bool
	Checks       []AcceptanceCheck
	Basis        CommercialBasisSnapshot
	ManualReview ManualReviewState
	DecidedAt    time.Time
}

func (spec RehydrateAcceptanceDecisionSpec) present() bool {
	return spec.DecisionID.valid()
}

// RehydrateAcceptanceBaselineSpec 是接受基线在库里的样子。基线没有公开构造器。
type RehydrateAcceptanceBaselineSpec struct {
	DeclaredParcelIDs []DeclaredParcelID
	SubmissionVersion SubmissionVersionID
	FixedAt           time.Time
}

func (spec RehydrateAcceptanceBaselineSpec) present() bool {
	return spec.SubmissionVersion.valid() || !spec.FixedAt.IsZero() || len(spec.DeclaredParcelIDs) != 0
}

// RehydrateExpectedCommitmentSpec 是预计承诺在库里的样子。承诺没有公开构造器；Basis 收成品。
type RehydrateExpectedCommitmentSpec struct {
	Basis    CommercialBasisSnapshot
	FormedAt time.Time
}

func (spec RehydrateExpectedCommitmentSpec) present() bool {
	return !spec.FormedAt.IsZero() || spec.Basis.valid()
}

// RehydratePriorRequestLinkSpec 是关联出处的快照表达：要么两个字段都缺席（首次委托），
// 要么都成立。半截的一份由 validForRehydration 拒绝。
type RehydratePriorRequestLinkSpec struct {
	PriorRequestID ShipmentRequestID
	Kind           RequestLinkKind
}

// RehydrateSubmissionVersionSpec 是客户当前请求内容那一份不可覆盖记录在库里的样子。
type RehydrateSubmissionVersionSpec struct {
	VersionID         SubmissionVersionID
	SourceSubmission  SourceSubmissionFingerprint
	DeclaredParcelIDs []DeclaredParcelID
	// Profiles 收成品类型，不另造 Rehydrate*Spec：画像有公开构造函数、字段未导出，
	// 包外造不出半截的一张——理由与 ProcessingAttempts 一字不差。有画像而不带回，
	// 一份申报过测量的版本重建后看起来像没申报过，估价装配从此永远停在缺输入。
	Profiles []DeclaredParcelProfile
	// Elements 同理收成品（pp-seams/05）：子段是值、字段未导出，包外只能经 AddressElementsOf 或零值造它；旧形快照
	// 没有这一段，读回即零值，读口如实答「要素缺席」——不回填、不猜。
	Elements DeclaredAddressElements
	// RequestedProduct 同理收成品（票 psb/17）：包外只能经 RequestedServiceProductOf 或零值造它；旧形快照没有这一格，
	// 读回即未声明——不回填、不猜。
	RequestedProduct DeclaredServiceProduct
	EstablishedAt    time.Time
}

// RehydrateAcceptanceTaskSpec 是接受判断任务在库里的样子。
type RehydrateAcceptanceTaskSpec struct {
	TaskID              AcceptanceDecisionTaskID
	SubmissionVersionID SubmissionVersionID
	EstablishedAt       time.Time
	State               AcceptanceTaskState
	WaitingOn           ResumePath
	// ProcessingAttempts 与 ReviewCompletion 收成品类型，不另造 Rehydrate*Spec：两者是判断的
	// **输入**而非产物，本就有公开构造函数，字段又未导出，包外造不出半截的一份。多一个 Spec
	// 就多一格「相信输入」，而 ADR-0028 要这个面尽可能小。
	//
	// 漏掉它们不只是少两个字段：复核完成度是 Decide 复核那道门的一半，漏了，一次已经做完的
	// 人工复核会被再要求一次，而 CompleteManualReview 的重复闸门也会放行第二次补签；处理记录
	// 漏了，用例要的「未决按原因分类统计」在重建之后从头数起。
	ProcessingAttempts []ProcessingAttempt
	ReviewCompletion   ManualReviewCompletion
	// AuthorizedDisposition 同理收成品（ADR-0132）。漏了它，DisposeUnderAuthority 的一版一次闸门会放行
	// 第二次处置，而`交客户补充`之后的一轮 Decide 会把等待态退回等处置——同一版本被要求处置两次。
	AuthorizedDisposition AuthorizedDisposition
}

// RehydrateShipmentRequest 从库里读到的产物重建一份委托。
//
// 它只校验，不重算。校验与导入侧的门禁是互补的：门禁拦业务代码断言结论，校验拦一行坏数据
// 或适配器自己的 bug 变成一个看起来合法的聚合。少一个，另一个都补不上——「反正只有适配器
// 能调，这几条校验可以省」正是 ADR-0028 预先否掉的那条推论。
func RehydrateShipmentRequest(snapshot RehydrateShipmentRequestSpec) (ShipmentRequest, error) {
	if err := admitRehydratedState(snapshot.State); err != nil {
		return ShipmentRequest{}, err
	}
	priorVersions := make([]SubmissionVersion, 0, len(snapshot.PriorVersions))
	for _, prior := range snapshot.PriorVersions {
		priorVersions = append(priorVersions, SubmissionVersion{
			versionID:         prior.VersionID,
			sourceSubmission:  prior.SourceSubmission,
			declaredParcelIDs: append([]DeclaredParcelID(nil), prior.DeclaredParcelIDs...),
			profiles:          append([]DeclaredParcelProfile(nil), prior.Profiles...),
			elements:          prior.Elements,
			requestedProduct:  prior.RequestedProduct,
			establishedAt:     prior.EstablishedAt,
		})
	}
	priorTasks := make([]AcceptanceDecisionTask, 0, len(snapshot.PriorTasks))
	for _, prior := range snapshot.PriorTasks {
		priorTasks = append(priorTasks, AcceptanceDecisionTask{
			taskID:                prior.TaskID,
			submissionVersionID:   prior.SubmissionVersionID,
			establishedAt:         prior.EstablishedAt,
			state:                 prior.State,
			waitingOn:             prior.WaitingOn,
			processingAttempts:    append([]ProcessingAttempt(nil), prior.ProcessingAttempts...),
			reviewCompletion:      prior.ReviewCompletion,
			authorizedDisposition: prior.AuthorizedDisposition,
		})
	}
	request := ShipmentRequest{
		revision:          snapshot.Revision,
		shipmentRequestID: snapshot.ShipmentRequestID,
		batchID:           snapshot.BatchID,
		state:             snapshot.State,
		submittedAt:       snapshot.SubmittedAt,
		currentVersion: SubmissionVersion{
			versionID:         snapshot.CurrentVersion.VersionID,
			sourceSubmission:  snapshot.CurrentVersion.SourceSubmission,
			declaredParcelIDs: append([]DeclaredParcelID(nil), snapshot.CurrentVersion.DeclaredParcelIDs...),
			profiles:          append([]DeclaredParcelProfile(nil), snapshot.CurrentVersion.Profiles...),
			elements:          snapshot.CurrentVersion.Elements,
			requestedProduct:  snapshot.CurrentVersion.RequestedProduct,
			establishedAt:     snapshot.CurrentVersion.EstablishedAt,
		},
		acceptanceTask: AcceptanceDecisionTask{
			taskID:              snapshot.AcceptanceTask.TaskID,
			submissionVersionID: snapshot.AcceptanceTask.SubmissionVersionID,
			establishedAt:       snapshot.AcceptanceTask.EstablishedAt,
			state:               snapshot.AcceptanceTask.State,
			waitingOn:           snapshot.AcceptanceTask.WaitingOn,
			processingAttempts: append(
				[]ProcessingAttempt(nil), snapshot.AcceptanceTask.ProcessingAttempts...,
			),
			reviewCompletion:      snapshot.AcceptanceTask.ReviewCompletion,
			authorizedDisposition: snapshot.AcceptanceTask.AuthorizedDisposition,
		},
		priorVersions: priorVersions,
		priorTasks:    priorTasks,
		priorLink: PriorRequestLink{
			prior: snapshot.PriorLink.PriorRequestID,
			kind:  snapshot.PriorLink.Kind,
		},
		decision: AcceptanceDecision{
			decisionID:   snapshot.Decision.DecisionID,
			accepted:     snapshot.Decision.Accepted,
			checks:       append([]AcceptanceCheck(nil), snapshot.Decision.Checks...),
			basis:        snapshot.Decision.Basis,
			manualReview: snapshot.Decision.ManualReview,
			decidedAt:    snapshot.Decision.DecidedAt,
		},
		decisionFormed: snapshot.DecisionFormed,
		baseline: AcceptanceBaseline{
			declaredParcelIDs: append([]DeclaredParcelID(nil), snapshot.Baseline.DeclaredParcelIDs...),
			submissionVersion: snapshot.Baseline.SubmissionVersion,
			fixedAt:           snapshot.Baseline.FixedAt,
		},
		commitment: ExpectedCommitment{
			basis:    snapshot.Commitment.Basis,
			formedAt: snapshot.Commitment.FormedAt,
		},
		sourceDataVersions: append([]CustomerSourceDataVersion(nil), snapshot.SourceDataVersions...),
	}
	if err := request.validForRehydration(); err != nil {
		return ShipmentRequest{}, err
	}
	return request, nil
}

// admitRehydratedState 判这扇门今天开到哪个状态。
//
// 见 ADR-0030：一个状态只有在快照类型能表达它可达的全部字段时才准进来。今天满足这条的是
// `已提交`与`已接受`。`已拒绝`/`已撤回`的判断产物在规格里仍没有完整字段可填，放进来重建出
// 的聚合会缺撤回或主动拒绝留痕，而它此后看起来与真的一样。
func admitRehydratedState(state ShipmentRequestState) error {
	switch state {
	case ShipmentRequestSubmitted, ShipmentRequestAccepted:
		return nil
	case ShipmentRequestStateInvalid:
		return rehydrationRefusal("委托状态未设")
	case ShipmentRequestRejected, ShipmentRequestWithdrawn:
		return fmt.Errorf("%w：%s", ErrRehydrationStateNotSupported, state)
	default:
		// 越界值印数字而不是名字：`String()` 对它交回空串，而一个说不出是哪个值的拒绝，
		// 适配器作者只能靠猜。
		return rehydrationRefusal(fmt.Sprintf("委托状态不是本上下文的取值：%d", uint8(state)))
	}
}

// validForRehydration 是「哪些状态组合是合法的」头一回被写下来。
//
// 今天没有任何函数在检查这些命题，一致性是 Decide 构造式保证的；重建入口一开，它们全都
// 造得出来。写漏的方向是放一个业务上不可能的聚合过关，而它此后看起来与真的一样。
// 三个时间戳（submittedAt、当前版本的 establishedAt、任务的 establishedAt）之间刻意不设
// 关系。`SubmitShipmentRequest` today 把三者写成同一个值，但那是首次提交这一条路径的巧合，
// 不是不变式：`UC-PS-002` 的新提交版本一落地，版本的建立时刻就不再等于委托的提交时刻。
// 在这里按今天的巧合收紧，等于把一条尚未确认的规则写成生产默认值。
//
// 「这个状态准不准进门」不在这里判，那是 admitRehydratedState 的事，它在本函数之前跑过；
// 本函数只回答「进来的这一份成不成立」。两者都要，缺一个另一个补不上（ADR-0030）。
func (request ShipmentRequest) validForRehydration() error {
	if request.revision < 1 {
		return rehydrationRefusal("聚合版本未从持久化读出")
	}
	if !request.shipmentRequestID.valid() || !request.batchID.valid() || request.submittedAt.IsZero() {
		return rehydrationRefusal("委托身份或提交时刻缺失")
	}
	if err := request.currentVersion.validForRehydration(); err != nil {
		return err
	}
	if err := request.acceptanceTask.validForRehydration(); err != nil {
		return err
	}
	// 任务是「某一个提交版本上」的工作。两半各自指向不同的提交版本，说明这份聚合是拼出来的。
	if request.acceptanceTask.submissionVersionID != request.currentVersion.versionID {
		return rehydrationRefusal("接受判断任务挂在另一个提交版本上")
	}
	// `已提交`与任务未收工只能同真同假：任务`已完成`只在决定越过提交边界时到达，`已停止`
	// 只在撤回或版本换代时到达，而撤回会把委托带离`已提交`、换代会立起新的运行中任务。
	if request.state == ShipmentRequestSubmitted && !request.acceptanceTask.running() {
		return rehydrationRefusal("委托仍为已提交，接受判断任务却已收工")
	}
	if err := request.acceptedProductsValidForRehydration(); err != nil {
		return err
	}
	if err := request.linkValidForRehydration(); err != nil {
		return err
	}
	return request.historyValidForRehydration()
}

// acceptedProductsValidForRehydration 把`已提交`与`已接受`各自该不该带判断产物写下来。
//
// 决定、基线、承诺、资料版本在`已提交`下必然缺席；`已接受`必须带齐决定、完成任务、基线与
// 承诺，且版本、成员、依据与决定时刻互相一致。半截快照是坏数据，不得折成「本期不支持」。
func (request ShipmentRequest) acceptedProductsValidForRehydration() error {
	hasDecision := request.decision.decisionID.valid()
	hasBaseline := len(request.baseline.declaredParcelIDs) != 0 ||
		request.baseline.submissionVersion.valid() || !request.baseline.fixedAt.IsZero()
	hasCommitment := !request.commitment.formedAt.IsZero() || request.commitment.basis.valid()
	hasSourceData := len(request.sourceDataVersions) != 0
	hasWithdrawal := request.withdrawal.formed()

	if request.state == ShipmentRequestSubmitted {
		if request.decisionFormed || hasDecision {
			return rehydrationRefusal("已提交委托携带了接受决定")
		}
		if hasBaseline {
			return rehydrationRefusal("已提交委托携带了接受基线")
		}
		if hasCommitment {
			return rehydrationRefusal("已提交委托携带了预计承诺")
		}
		if hasSourceData {
			return rehydrationRefusal("已提交委托携带了客户资料版本")
		}
		if hasWithdrawal {
			return rehydrationRefusal("已提交委托携带了撤回")
		}
		return nil
	}

	if request.state != ShipmentRequestAccepted {
		return nil
	}

	if hasWithdrawal {
		return rehydrationRefusal("已接受委托携带了撤回")
	}
	if !request.decisionFormed {
		return rehydrationRefusal("已接受委托没有越过决定边界")
	}
	if !hasDecision {
		return rehydrationRefusal("已接受委托没有接受决定")
	}
	if !request.decision.accepted {
		return rehydrationRefusal("已接受委托的决定不是接受")
	}
	if !request.acceptanceTask.IsComplete() {
		return rehydrationRefusal("已接受委托的判断任务尚未完成")
	}
	if _, waiting := request.acceptanceTask.WaitingOn(); waiting {
		return rehydrationRefusal("已接受委托的判断任务仍在等待")
	}
	if !hasBaseline {
		return rehydrationRefusal("已接受委托没有接受基线")
	}
	if !hasCommitment {
		return rehydrationRefusal("已接受委托没有预计承诺")
	}
	if request.baseline.submissionVersion != request.currentVersion.versionID {
		return rehydrationRefusal("接受基线固定在另一个提交版本上")
	}
	if request.baseline.fixedAt.IsZero() {
		return rehydrationRefusal("接受基线没有固定时刻")
	}
	if !sameDeclaredParcels(request.baseline.declaredParcelIDs, request.currentVersion.declaredParcelIDs) {
		return rehydrationRefusal("接受基线成员与当前提交版本不一致")
	}
	if !request.decision.basis.valid() {
		return rehydrationRefusal("接受决定没有可用的商业依据")
	}
	if !request.commitment.basis.sameAs(request.decision.basis) {
		return rehydrationRefusal("预计承诺的商业依据与接受决定不一致")
	}
	if err := request.acceptedDecisionHolds(); err != nil {
		return err
	}
	if !request.commitment.formedAt.Equal(request.decision.decidedAt) ||
		!request.baseline.fixedAt.Equal(request.decision.decidedAt) {
		return rehydrationRefusal("接受基线或预计承诺的时刻与决定时刻不一致")
	}
	if request.decision.decidedAt.IsZero() {
		return rehydrationRefusal("接受决定没有决定时刻")
	}
	for _, version := range request.sourceDataVersions {
		if !version.valid() {
			return rehydrationRefusal("客户资料版本立不起来")
		}
		if version.scope.shipmentRequestID != request.shipmentRequestID {
			return rehydrationRefusal("客户资料版本指着另一份委托")
		}
		if parcelID, named := version.scope.DeclaredParcelID(); named && !request.baseline.covers(parcelID) {
			return rehydrationRefusal("客户资料版本指名了接受基线外的成员")
		}
	}
	return nil
}

// acceptedDecisionHolds 校验已接受决定是否可能由 Decide 形成：校验组无失败/未决且适用组与
// 成员都到场，复核状态等于规则包声明与任务完成事实的合成，且合成结果是接受允许的两格之一。
// 它不重算 state、baseline、commitment。
func (request ShipmentRequest) acceptedDecisionHolds() error {
	classified, err := classifyAcceptanceChecks(request.decision.checks)
	if err != nil {
		return rehydrationRefusal("接受决定含非法校验")
	}
	if classified.failed > 0 || classified.undetermined > 0 {
		return rehydrationRefusal("已接受委托的校验含失败或未决")
	}
	if !everyApplicableGroupJudged(request.decision.basis.applicable, classified.judgedGroups) {
		return rehydrationRefusal("已接受委托漏了适用校验组")
	}
	if !request.everyMemberJudged(classified.judgedMembers) {
		return rehydrationRefusal("已接受委托漏了成员判断")
	}

	synthesized := request.manualReviewState(request.decision.basis.manualReview)
	if request.decision.manualReview != synthesized {
		return rehydrationRefusal("接受决定的复核状态与规则包声明和任务完成事实对不上")
	}
	if synthesized != ManualReviewNotRequired && synthesized != ManualReviewCompleted {
		return rehydrationRefusal("已接受委托的复核不是不要求或已完成")
	}
	return nil
}

func sameDeclaredParcels(left, right []DeclaredParcelID) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[DeclaredParcelID]int, len(left))
	for _, parcel := range left {
		counts[parcel]++
	}
	for _, parcel := range right {
		counts[parcel]--
		if counts[parcel] < 0 {
			return false
		}
	}
	return true
}

// linkValidForRehydration 校验关联出处：要么整个缺席，要么方向与指向都成立且不指自己。
// 半截的一份（有方向没指向、或反过来）读不出它是首次委托还是关联新委托，必须拒。
// 方向与**原委托**终态的互证不在这里重做：那要读另一份聚合，重建入口只看这一行。
func (request ShipmentRequest) linkValidForRehydration() error {
	link := request.priorLink
	if link == (PriorRequestLink{}) {
		return nil
	}
	if !link.kind.valid() {
		return rehydrationRefusal(fmt.Sprintf("关联方向不是本上下文的取值：%d", uint8(link.kind)))
	}
	if !link.prior.valid() {
		return rehydrationRefusal("关联出处没有指向原委托")
	}
	if link.prior == request.shipmentRequestID {
		return rehydrationRefusal("关联出处指向委托自己")
	}
	return nil
}

// historyValidForRehydration 校验受控补充留下的历史（ADR-0045）。历史版本与历史任务按
// 形成顺序两两对应；历史任务不得仍在运行——「同一时刻只有一个待判断的当前提交版本」，
// 一份带着两个运行中任务的聚合是拼出来的。
func (request ShipmentRequest) historyValidForRehydration() error {
	if len(request.priorVersions) != len(request.priorTasks) {
		return rehydrationRefusal("历史提交版本与历史判断任务数量对不上")
	}
	seen := map[SubmissionVersionID]struct{}{request.currentVersion.versionID: {}}
	for index, prior := range request.priorVersions {
		if err := prior.validForRehydration(); err != nil {
			return err
		}
		if _, duplicated := seen[prior.versionID]; duplicated {
			return rehydrationRefusal("历史提交版本与另一代版本重号")
		}
		seen[prior.versionID] = struct{}{}

		task := request.priorTasks[index]
		if err := task.validForRehydration(); err != nil {
			return err
		}
		if task.running() {
			return rehydrationRefusal("历史判断任务仍在运行")
		}
		if task.submissionVersionID != prior.versionID {
			return rehydrationRefusal("历史判断任务挂在另一个提交版本上")
		}
	}
	return nil
}

func (version SubmissionVersion) validForRehydration() error {
	if !version.versionID.valid() || version.establishedAt.IsZero() {
		return rehydrationRefusal("提交版本身份或建立时刻缺失")
	}
	// 来源指纹是本上下文来源保全的锚点，构造路径（SubmissionCandidate）第一件事就是校验它。
	// 重建路径漏掉它，等于让一份没有来源的提交版本从旁门进来。
	if !version.sourceSubmission.valid() {
		return rehydrationRefusal("提交版本没有可用的来源指纹")
	}
	// 一份没有声明成员的提交版本判不出接受基线，也不该存在：成员集合是接受语言的起点。
	if len(version.declaredParcelIDs) == 0 {
		return rehydrationRefusal("提交版本没有声明成员")
	}
	// 逐个校 + 去重，与构造路径 `NewSubmissionCandidate` 那三条对齐。只校「非空」时，一个
	// 空成员或一份重复成员会原样进接受基线，而 `Decide` 的接受支直接拿这个切片造基线，
	// `acceptance_decision.go` 又写明基线「恒覆盖该提交版本的完整声明成员」——一份错的基线
	// 此后与真的无从分辨，而 `SourceDataScopeOutsideAcceptanceBaseline` 会照它拒掉更正。
	//
	// 「半截的一条包外造不出来，但一批零值造得出」这句本文件已经为 processingAttempts 写过，
	// 它对声明成员一字不差地成立：`make([]DeclaredParcelID, 3)` 忘了填就是。
	seen := make(map[DeclaredParcelID]struct{}, len(version.declaredParcelIDs))
	for _, parcelID := range version.declaredParcelIDs {
		if !parcelID.valid() {
			return rehydrationRefusal("提交版本上有一个立不起来的声明成员")
		}
		if _, duplicated := seen[parcelID]; duplicated {
			return rehydrationRefusal("提交版本上有重复的声明成员")
		}
		seen[parcelID] = struct{}{}
	}
	// 画像与构造路径同一套贴合校验（成员集合内、每成员至多一张、半截拒收）。批量零值
	// 造得出（make 忘填），指着集合外成员的一张画像则会让估价装配拿另一个包裹的测量。
	if _, err := declaredProfilesFor(version.profiles, version.declaredParcelIDs); err != nil {
		return rehydrationRefusal("提交版本上的成员声明画像立不起来")
	}
	return nil
}

func (task AcceptanceDecisionTask) validForRehydration() error {
	if !task.taskID.valid() || task.establishedAt.IsZero() {
		return rehydrationRefusal("接受判断任务身份或建立时刻缺失")
	}
	// 逐取值分派，不留兜底。只拒零值不够：一个越界值（那一列存了 99）此外只会被
	// `已提交 ⇒ 任务运行中` 那条撞上，而那条的前件是`已提交`——门开到别的状态时它不适用。
	// 值域这件事要由它自己的检查回答，不能挂在另一条命题的前件上。
	switch task.state {
	case AcceptanceTaskRunning, AcceptanceTaskComplete, AcceptanceTaskStopped:
	case AcceptanceTaskStateInvalid:
		return rehydrationRefusal("接受判断任务状态未设")
	default:
		return rehydrationRefusal(fmt.Sprintf("接受判断任务状态不是本上下文的取值：%d", uint8(task.state)))
	}
	// 等待态要么缺席（零值），要么落在三个等待态之内。完全不校的话，库里一个越界值会被
	// `WaitingOn()` 报成**缺席**——即「这任务不等任何人」，而不是被拒成一行坏数据；产出是一份
	// 不等人、也永远不会有人来续办的运行中任务。
	//
	// 同一个 `ResumePath` 作为 `ProcessingAttempt.resumePath` 时经 `attempt.valid()` 查的是
	// 同一个值域，两处口径因此一致。
	if task.waitingOn != ResumePathInvalid && !task.waitingOn.valid() {
		return rehydrationRefusal(fmt.Sprintf("接受判断任务的等待态不是本上下文的取值：%d", uint8(task.waitingOn)))
	}
	// 处理记录逐条校验。半截的一条包外造不出来——字段未导出，构造器全校验——但一批零值造得
	// 出：`make([]ProcessingAttempt, n)` 忘了填就是，而那会让「这份委托卡过几轮」多数出几轮
	// 不存在的。复核完成度不必同样逐项查：它是单个值，零值即缺席，非零只可能来自那个全校验
	// 的构造器，凑不出批量那条路。
	for _, attempt := range task.processingAttempts {
		if !attempt.valid() {
			return rehydrationRefusal("接受判断任务上有一条不完整的处理记录")
		}
	}
	// 处置记录同复核完成度只判在场，不逐项查。但它与等待态有一条命题：处置转移把等待态从`等待授权处置`
	// 带走（`拒绝`清零、`交客户补充`转到`等待受控补充`），所以「已处置却仍停在等处置」不可能由本上下文
	// 写出——放它进门，队列会把一份已处置的委托再列给处置角色。`拒绝`去向另有一条：它越过决定边界收了
	// 任务的工，一条处置为`拒绝`的运行中任务同样是拼出来的。
	if task.authorizedDisposition.recorded() {
		if task.waitingOn == ResumeByAuthorizedDisposition {
			return rehydrationRefusal("接受判断任务已处置却仍停在等待授权处置")
		}
		if task.authorizedDisposition.choice == DisposeByRejection && task.running() {
			return rehydrationRefusal("接受判断任务已处置为拒绝却仍在运行")
		}
	}
	return nil
}

// rehydrationRefusal 让哨兵值对 errors.Is 仍然成立，同时把拒绝的那一条说出来。压平成一句
// 「这行不合法」，拿到它的适配器作者只能靠猜——本包对同类问题的既定态度见 ResolutionReason。
func rehydrationRefusal(reason string) error {
	return fmt.Errorf("%w：%s", ErrInvalidRehydratedShipmentRequest, reason)
}
