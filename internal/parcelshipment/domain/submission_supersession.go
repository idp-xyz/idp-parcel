package domain

import (
	"errors"
	"time"
)

var (
	// ErrSubmissionBoundaryChanged 是业务拒绝而不是输入格式错：新版本自身合法，只是成员
	// 集合与当前版本不一致——增删拆并是「成员重组」，CONTEXT 把它判给关联新委托，这条
	// 转移不许吞下（ADR-0045）。与 ErrParcelOutsideAcceptanceBaseline 分开：那条说的是
	// 已接受后的资料范围越界，这条说的是提交版本本身想换成员。
	ErrSubmissionBoundaryChanged     = errors.New("parcel shipment: member set changed; a linked new request is required")
	ErrInvalidSubmissionSupersession = errors.New("parcel shipment: invalid new submission version")
)

// NewSubmissionVersionSpec 是受控补充/纠错在同一委托下形成新提交版本所需的全部输入。
//
// 版本号与任务号由本上下文身份工厂签发（ADR-0045），来源指纹必须来自补充请求自己的
// 来源身份——同一来源身份携带不同内容是接入冲突，不是新版本。
type NewSubmissionVersionSpec struct {
	VersionID         SubmissionVersionID
	TaskID            AcceptanceDecisionTaskID
	SourceSubmission  SourceSubmissionFingerprint
	DeclaredParcelIDs []DeclaredParcelID
	// Profiles 是随新版本申报的成员声明画像（ADR-0048）：普通资料纠错可以更正测量，
	// 画像随本版本重报，不从旧版本静默继承——继承会把「客户改了话」与「客户没说」混掉。
	Profiles      []DeclaredParcelProfile
	EstablishedAt time.Time
}

// FormNewSubmissionVersion 在`已提交`态用受控补充或纠错形成同一委托的新提交版本
// （`AT-PS-036` 第一支；形状裁于 ADR-0045）。
//
// 新版本成为唯一待判断版本：旧版本进历史切片、旧任务转`已停止`——它没有办完，判成
// `已完成`是撒谎，删掉是抹历史。新任务随新版本建立，重建门的 task↔currentVersion
// 不变量原样成立。
//
// 决定边界不变：接受、拒绝或撤回越过边界后，同一信号分别走资料修订（已接受）与关联
// 新委托（已拒绝/已撤回），不得用新版本改写已决定的委托。
func (request ShipmentRequest) FormNewSubmissionVersion(spec NewSubmissionVersionSpec) (ShipmentRequest, error) {
	if request.decisionFormed {
		return ShipmentRequest{}, ErrDecisionAlreadyFormed
	}
	if request.state != ShipmentRequestSubmitted {
		return ShipmentRequest{}, ErrInvalidShipmentRequest
	}
	if !spec.VersionID.valid() || !spec.TaskID.valid() ||
		!spec.SourceSubmission.valid() || spec.EstablishedAt.IsZero() {
		return ShipmentRequest{}, ErrInvalidSubmissionSupersession
	}

	// 版本号不得与任何一代重号：历史是按号回指的，重号会让两代版本此后无从分辨。
	if spec.VersionID == request.currentVersion.versionID {
		return ShipmentRequest{}, ErrInvalidSubmissionSupersession
	}
	for _, prior := range request.priorVersions {
		if spec.VersionID == prior.versionID {
			return ShipmentRequest{}, ErrInvalidSubmissionSupersession
		}
	}
	// 新版本必须以自己的来源身份到达。同一身份携带不同内容在来源保全层是接入冲突，同一
	// 身份同内容是重放——两条路都在编排短路，能走到这里还带着旧身份，只可能是装配错误。
	if spec.SourceSubmission.Identity() == request.currentVersion.sourceSubmission.Identity() {
		return ShipmentRequest{}, ErrInvalidSubmissionSupersession
	}
	for _, prior := range request.priorVersions {
		if spec.SourceSubmission.Identity() == prior.sourceSubmission.Identity() {
			return ShipmentRequest{}, ErrInvalidSubmissionSupersession
		}
	}

	members, err := supersessionMembers(spec.DeclaredParcelIDs, request.currentVersion.declaredParcelIDs)
	if err != nil {
		return ShipmentRequest{}, err
	}
	profiles, err := declaredProfilesFor(spec.Profiles, members)
	if err != nil {
		return ShipmentRequest{}, err
	}

	// 复制而不是就地 append，理由同 AmendCustomerSourceData：聚合按值传递，共用底层数组
	// 会让两条从同一份委托分出去的转移互相覆盖对方追加的历史。
	priorVersions := make([]SubmissionVersion, 0, len(request.priorVersions)+1)
	priorVersions = append(priorVersions, request.priorVersions...)
	request.priorVersions = append(priorVersions, request.currentVersion)

	stoppedTask := request.acceptanceTask
	stoppedTask.state = AcceptanceTaskStopped
	stoppedTask.waitingOn = ResumePathInvalid
	priorTasks := make([]AcceptanceDecisionTask, 0, len(request.priorTasks)+1)
	priorTasks = append(priorTasks, request.priorTasks...)
	request.priorTasks = append(priorTasks, stoppedTask)

	request.currentVersion = SubmissionVersion{
		versionID:         spec.VersionID,
		sourceSubmission:  spec.SourceSubmission,
		declaredParcelIDs: members,
		profiles:          profiles,
		establishedAt:     spec.EstablishedAt,
	}
	request.acceptanceTask = AcceptanceDecisionTask{
		taskID:              spec.TaskID,
		submissionVersionID: spec.VersionID,
		establishedAt:       spec.EstablishedAt,
		state:               AcceptanceTaskRunning,
	}
	return request, nil
}

// supersessionMembers 校验新版本的成员集合与当前版本等值（不看顺序），并交回一份去重后的
// 拷贝。集合必须一致：`同一委托同一时刻只有一个待判断的当前提交版本`说的是内容换代，成员
// 换代是另一份委托。
func supersessionMembers(declared, current []DeclaredParcelID) ([]DeclaredParcelID, error) {
	if len(declared) == 0 {
		return nil, ErrInvalidSubmissionSupersession
	}
	seen := make(map[DeclaredParcelID]struct{}, len(declared))
	members := make([]DeclaredParcelID, 0, len(declared))
	for _, parcelID := range declared {
		if !parcelID.valid() {
			return nil, ErrInvalidSubmissionSupersession
		}
		if _, duplicated := seen[parcelID]; duplicated {
			return nil, ErrInvalidSubmissionSupersession
		}
		seen[parcelID] = struct{}{}
		members = append(members, parcelID)
	}

	if len(seen) != len(current) {
		return nil, ErrSubmissionBoundaryChanged
	}
	for _, parcelID := range current {
		if _, present := seen[parcelID]; !present {
			return nil, ErrSubmissionBoundaryChanged
		}
	}
	return members, nil
}

// PriorSubmissionVersions 交回历史提交版本的拷贝，按形成顺序排列。旧版本及其判断历史
// 必须保留（CONTEXT），读口只暴露、不解释。
func (request ShipmentRequest) PriorSubmissionVersions() []SubmissionVersion {
	return append([]SubmissionVersion(nil), request.priorVersions...)
}

// PriorAcceptanceTasks 交回历史判断任务的拷贝，与历史版本按位对应。
func (request ShipmentRequest) PriorAcceptanceTasks() []AcceptanceDecisionTask {
	return append([]AcceptanceDecisionTask(nil), request.priorTasks...)
}
