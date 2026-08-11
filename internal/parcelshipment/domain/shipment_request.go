package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidShipmentRequest     = errors.New("parcel shipment: invalid shipment request")
	ErrFutureSubmissionNotAllowed = errors.New("parcel shipment: future submission is not allowed")
)

type SubmissionVersionID struct{ requiredValue }

func NewSubmissionVersionID(value string) (SubmissionVersionID, error) {
	required, err := newRequiredValue("submission version ID", value)
	return SubmissionVersionID{required}, err
}

type AcceptanceDecisionTaskID struct{ requiredValue }

func NewAcceptanceDecisionTaskID(value string) (AcceptanceDecisionTaskID, error) {
	required, err := newRequiredValue("acceptance decision task ID", value)
	return AcceptanceDecisionTaskID{required}, err
}

type ShipmentRequestState uint8

const (
	ShipmentRequestStateInvalid ShipmentRequestState = iota
	ShipmentRequestSubmitted
	ShipmentRequestAccepted
	ShipmentRequestRejected
	ShipmentRequestWithdrawn
)

func (state ShipmentRequestState) String() string {
	switch state {
	case ShipmentRequestSubmitted:
		return "SUBMITTED"
	case ShipmentRequestAccepted:
		return "ACCEPTED"
	case ShipmentRequestRejected:
		return "REJECTED"
	case ShipmentRequestWithdrawn:
		return "WITHDRAWN"
	default:
		return ""
	}
}

// SubmissionVersion 是客户当前请求内容的不可覆盖记录。纠错形成新的提交版本，而不是
// 就地修改这一份。
type SubmissionVersion struct {
	versionID         SubmissionVersionID
	sourceSubmission  SourceSubmissionFingerprint
	declaredParcelIDs []DeclaredParcelID
	establishedAt     time.Time
}

func (version SubmissionVersion) VersionID() SubmissionVersionID {
	return version.versionID
}

func (version SubmissionVersion) SourceSubmission() SourceSubmissionFingerprint {
	return version.sourceSubmission
}

func (version SubmissionVersion) DeclaredParcelIDs() []DeclaredParcelID {
	return append([]DeclaredParcelID(nil), version.declaredParcelIDs...)
}

func (version SubmissionVersion) EstablishedAt() time.Time {
	return version.establishedAt
}

// AcceptanceDecisionTask 记录一个提交版本上可续办的接受判断工作。它不是委托的领域
// 状态：任务未完成时委托仍为`已提交`，任务的建立本身也不构成接受。
type AcceptanceDecisionTask struct {
	taskID              AcceptanceDecisionTaskID
	submissionVersionID SubmissionVersionID
	establishedAt       time.Time
	processingAttempts  []ProcessingAttempt
	reviewCompletion    ManualReviewCompletion
	waitingOn           ResumePath
	state               AcceptanceTaskState
}

// AcceptanceTaskState 区分任务的两个终态。CONTEXT 把它们分开写：`已完成`只在接受或拒绝决定
// 越过提交边界时到达，撤回成立时到达的是`已停止`——保留已执行阶段和结果，但不再形成决定。
// 合成一个布尔会让「这份委托判完了」与「这份委托没人再判了」在事后分不开，而两者的后续动作
// 完全不同：前者有决定可读，后者只有撤回记录和待续的补偿。
type AcceptanceTaskState uint8

// 零值留给`未设`而不是`运行中`，与本包其余枚举同一约定。重建入口明言相信输入，那里
// 「适配器忘了设状态」必须能被检测出来；零值若是一个合法值，忘填会静默过关成`运行中`。
// 代价是构造处要显式写出`运行中`，那本就该显式。
const (
	AcceptanceTaskStateInvalid AcceptanceTaskState = iota
	AcceptanceTaskRunning
	AcceptanceTaskComplete
	AcceptanceTaskStopped
)

func (task AcceptanceDecisionTask) TaskID() AcceptanceDecisionTaskID {
	return task.taskID
}

func (task AcceptanceDecisionTask) SubmissionVersionID() SubmissionVersionID {
	return task.submissionVersionID
}

func (task AcceptanceDecisionTask) EstablishedAt() time.Time {
	return task.establishedAt
}

func (task AcceptanceDecisionTask) IsComplete() bool {
	return task.state == AcceptanceTaskComplete
}

// IsStopped 说的是撤回让这份任务收了工，而不是它判完了。两者都不再接受续办，但只有已完成
// 那一侧有决定可读。
func (task AcceptanceDecisionTask) IsStopped() bool {
	return task.state == AcceptanceTaskStopped
}

func (task AcceptanceDecisionTask) running() bool {
	return task.state == AcceptanceTaskRunning
}

type SubmitShipmentRequestSpec struct {
	Candidate   SubmissionCandidate
	Gate        FutureSubmissionGate
	VersionID   SubmissionVersionID
	TaskID      AcceptanceDecisionTaskID
	SubmittedAt time.Time
}

type ShipmentRequest struct {
	// revision 是这份聚合被读出时所在的持久化版本，未持久化为零。它由仓储在写入成功后
	// 推进，**状态转移不动它**：一次保存对应一次版本推进，而两次转移之间只保存一次，
	// 由转移各自加一会让版本跳号，框架合同要的却是严格递增一。
	//
	// 转移都以值接收者复制整份聚合再返回，所以它天然被带下去。真正的风险是日后某个转移
	// 改成重新构造一个 ShipmentRequest{...}，或改成指针接收者就地改——那会把版本悄悄刷回零，
	// 随后一次按预期版本的写入会当作并发冲突失败，或者更糟，覆盖掉别人的写入。守它的是
	// `TestNoStateTransitionMovesTheAggregateRevision`：反射枚举值接收者转移，并把指针接收者
	// 与非 `(ShipmentRequest, error)` 签名直接判违规（扫不到却声称守住是假阴性）。包级函数
	// 形状由 `TestNoPackageLevelFunctionActsAsAShipmentRequestTransition` 另守。
	revision          int64
	shipmentRequestID ShipmentRequestID
	batchID           SubmissionBatchID
	state             ShipmentRequestState
	currentVersion    SubmissionVersion
	acceptanceTask    AcceptanceDecisionTask
	withdrawal        Withdrawal
	submittedAt       time.Time
	decision          AcceptanceDecision
	decisionFormed    bool
	baseline          AcceptanceBaseline
	commitment        ExpectedCommitment
	// sourceDataVersions 只追加。它与 baseline 并列而不是改写 baseline：接受基线固定的是
	// 客户声明的服务范围，资料版本记的是此后的补充与更正，两者都要留。
	sourceDataVersions []CustomerSourceDataVersion
}

// SubmitShipmentRequest 在放行的建单门禁之后建立一份`已提交`委托。它不形成接受或
// 拒绝。
func SubmitShipmentRequest(spec SubmitShipmentRequestSpec) (ShipmentRequest, error) {
	if !spec.Candidate.valid() ||
		!spec.VersionID.valid() ||
		!spec.TaskID.valid() ||
		spec.SubmittedAt.IsZero() {
		return ShipmentRequest{}, ErrInvalidShipmentRequest
	}
	if !spec.Gate.valid() {
		return ShipmentRequest{}, ErrInvalidFutureSubmissionGate
	}
	if !spec.Gate.IsAllowed() {
		return ShipmentRequest{}, ErrFutureSubmissionNotAllowed
	}

	return ShipmentRequest{
		shipmentRequestID: spec.Candidate.ShipmentRequestID(),
		batchID:           spec.Candidate.BatchID(),
		state:             ShipmentRequestSubmitted,
		currentVersion: SubmissionVersion{
			versionID:         spec.VersionID,
			sourceSubmission:  spec.Candidate.SourceSubmission(),
			declaredParcelIDs: spec.Candidate.DeclaredParcelIDs(),
			establishedAt:     spec.SubmittedAt,
		},
		acceptanceTask: AcceptanceDecisionTask{
			taskID:              spec.TaskID,
			submissionVersionID: spec.VersionID,
			establishedAt:       spec.SubmittedAt,
			state:               AcceptanceTaskRunning,
		},
		submittedAt: spec.SubmittedAt,
	}, nil
}

// Revision 交回这份聚合被读出时所在的持久化版本，未持久化为零。仓储据它按预期版本写入，
// 从而认出并发覆盖。
func (request ShipmentRequest) Revision() int64 {
	return request.revision
}

func (request ShipmentRequest) ShipmentRequestID() ShipmentRequestID {
	return request.shipmentRequestID
}

func (request ShipmentRequest) BatchID() SubmissionBatchID {
	return request.batchID
}

func (request ShipmentRequest) State() ShipmentRequestState {
	return request.state
}

func (request ShipmentRequest) CurrentSubmissionVersion() SubmissionVersion {
	return request.currentVersion
}

func (request ShipmentRequest) AcceptanceDecisionTask() AcceptanceDecisionTask {
	return request.acceptanceTask
}

func (request ShipmentRequest) SubmittedAt() time.Time {
	return request.submittedAt
}
