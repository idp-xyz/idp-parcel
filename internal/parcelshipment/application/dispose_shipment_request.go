package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// AuthorizedDispositionOutcome 是授权处置命令的应用处理结果，分格照 ManualReviewCompletionOutcome
// （ADR-0132 决定二「与 CompleteManualReview 的分格同形」）。
//
// `已处置`与`任务已完结`分开：前者是同一提交版本上的第二次处置（重复提交或换人改主意），读回既有处置
// 即可；后者是决定已越过提交边界（或任务已随撤回停止），去向没有可选的对象——编排交回那一个决定。
// `版本已换代`单列：处置人看的是某一份提交版本上的受限项，新版本到达后那份对象已不存在，把去向记到
// 新任务上就是把 A 版的处置签到 B 版头上。`没停在等处置`是本命令自己的一格：委托停在别的等待态、或
// 根本没停，处置在此刻没有对象。
//
// `未获授权`与`授权规则未配置`是 party-commercial 答复的两格落点，理由同复核那一步：前者是确定的业务
// 答案，后者是租户还没把处置授权规则登记上——首发期 PC 的授权动作词汇里还没有这一格，每一次询问都
// 落在这里，压成前者等于对一个尚未配置的产品说「你无权处置」。两格都什么也不落库。
type AuthorizedDispositionOutcome uint8

const (
	AuthorizedDispositionOutcomeInvalid AuthorizedDispositionOutcome = iota
	AuthorizedDispositionRecorded
	AuthorizedDispositionAlreadyRecorded
	AuthorizedDispositionTaskAlreadyClosed
	AuthorizedDispositionVersionSuperseded
	AuthorizedDispositionNotWaiting
	AuthorizedDispositionConflict
	AuthorizedDispositionNotAuthorized
	AuthorizedDispositionAuthorityRulesNotConfigured
)

func (outcome AuthorizedDispositionOutcome) String() string {
	switch outcome {
	case AuthorizedDispositionRecorded:
		return "RECORDED"
	case AuthorizedDispositionAlreadyRecorded:
		return "ALREADY_DISPOSED"
	case AuthorizedDispositionTaskAlreadyClosed:
		return "TASK_ALREADY_CLOSED"
	case AuthorizedDispositionVersionSuperseded:
		return "VERSION_SUPERSEDED"
	case AuthorizedDispositionNotWaiting:
		return "NOT_WAITING_ON_DISPOSITION"
	case AuthorizedDispositionConflict:
		return "REVISION_CONFLICT"
	case AuthorizedDispositionNotAuthorized:
		return "NOT_AUTHORIZED"
	case AuthorizedDispositionAuthorityRulesNotConfigured:
		return "AUTHORITY_RULES_NOT_CONFIGURED"
	default:
		return ""
	}
}

// DisposeShipmentRequestCommand 说明谁、以什么结构化原因、凭什么证据，为哪一份停在`等待授权处置`的
// 提交版本选定了哪个去向。
//
// 它不带授权引用：那由 party-commercial 签发（所采用的授权规则版本），本编排去问，不由调用方声明——
// 与主动拒绝、复核完成同一条理由，自带一个就等于自己给自己签字。去向、处置人、原因与证据的必填由领域
// 构造函数把守；版本必填是本层的门——处置挂在版本的判断任务上，不指名版本的处置无从判断它签给了谁。
type DisposeShipmentRequestCommand struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	Disposer          domain.DisposerReference
	Choice            domain.AuthorizedDispositionChoice
	Reason            domain.DispositionReasonReference
	Evidence          domain.DispositionEvidenceReference
}

type DisposeShipmentRequestResult struct {
	outcome        AuthorizedDispositionOutcome
	state          domain.ShipmentRequestState
	disposition    domain.AuthorizedDisposition
	hasDisposition bool
	decision       domain.AcceptanceDecision
	hasDecision    bool
	currentVersion domain.SubmissionVersionID
	compensation   domain.OwnershipContinuationReference
}

func (result DisposeShipmentRequestResult) Outcome() AuthorizedDispositionOutcome {
	return result.outcome
}

// State 是本轮之后委托的生命周期状态：`拒绝`去向记下后是`已拒绝`，`交客户补充`记下后仍是`已提交`。
func (result DisposeShipmentRequestResult) State() domain.ShipmentRequestState {
	return result.state
}

// Disposition 交回任务上的处置留痕：本次记下的，或`已处置`时先到的那一份。缺席即本轮没有处置可读。
func (result DisposeShipmentRequestResult) Disposition() (domain.AuthorizedDisposition, bool) {
	return result.disposition, result.hasDisposition
}

// AcceptanceDecision 在`拒绝`去向记下时是本次形成的拒绝决定；在`任务已完结`且决定确实形成时是那一个
// 既有决定，调用方据以告知处置人结果已定。撤回导致的停止没有决定可交。
func (result DisposeShipmentRequestResult) AcceptanceDecision() (domain.AcceptanceDecision, bool) {
	return result.decision, result.hasDecision
}

// CurrentVersion 是读取时刻的当前提交版本，`版本已换代`时调用方据以重读队列。
func (result DisposeShipmentRequestResult) CurrentVersion() domain.SubmissionVersionID {
	return result.currentVersion
}

// CompensationReference 只在处置已经记下、而随附的资金释放没能确定完成时给出。它与形成决定那一步的
// 同名引用同义：释放按原关联另行续办，处置结果不因它回滚。
func (result DisposeShipmentRequestResult) CompensationReference() domain.OwnershipContinuationReference {
	return result.compensation
}

type DisposeShipmentRequestDeps struct {
	Requests   ports.ShipmentRequestRepository
	Authorizer ports.AuthorizedDispositionAuthorizer
	Judgments  ports.RecordedJudgmentReader
	Release    ports.PreAcceptanceControlRelease
	Identities ports.AcceptanceDecisionIdentity
	Clock      ports.Clock
}

type DisposeShipmentRequestHandler struct {
	deps DisposeShipmentRequestDeps
}

func NewDisposeShipmentRequestHandler(deps DisposeShipmentRequestDeps) *DisposeShipmentRequestHandler {
	return &DisposeShipmentRequestHandler{deps: deps}
}

// Handle 让授权处置角色对一份停在`等待授权处置`的委托选定去向（ADR-0132）。
//
// 授权先于一切写动作，也先于版本核对与领域判断（顺序同 RejectShipmentRequestHandler 与
// CompleteManualReviewHandler）：处置权属 party-commercial 的授权规则，编排拿到委托就去问它，未获授权时
// 不签发决定标识、不碰聚合；`版本已换代`那类续办提示是给有处置权的人重读队列用的，不该先于授权答给
// 任何人。
//
// 两个去向都在本命令事务里收口，不发续办信封（ADR-0132 决定二）：`拒绝`当场形成授权角色拒绝决定，形照
// 主动拒绝的保留条款；`交客户补充`把等待态转到`等待受控补充`，之后的触发是既有的「新提交版本已形成」
// 信封。两个去向都对本版本已成立的项按原关联释放占用——`拒绝`即确定未成立，`交客户补充`那一刻本版本也
// 不会再被判，占着客户的资金或额度等一份不会到来的接受没有依据（ADR-0132 决定一末段）。
func (handler *DisposeShipmentRequestHandler) Handle(
	ctx context.Context,
	command DisposeShipmentRequestCommand,
) (DisposeShipmentRequestResult, error) {
	request, found, err := handler.deps.Requests.FindBySourceIdentity(ctx, command.Identity)
	if err != nil {
		return DisposeShipmentRequestResult{}, fmt.Errorf("dispose shipment request: %w", err)
	}
	if !found {
		// 指名一份查不到的委托是调用方的错。接 HTTP 时与其余未形成答案一并 5xx，不细分出「未找到」
		// ——理由同复核端点：拆开就是把统一不可见结果拆开。
		return DisposeShipmentRequestResult{}, fmt.Errorf("dispose shipment request: %w", domain.ErrInvalidShipmentRequest)
	}

	authority, err := handler.deps.Authorizer.AuthorizeDisposition(ctx, ports.AuthorizedDispositionAuthorizationQuery{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		Disposer:          command.Disposer,
		Reason:            command.Reason,
		Evidence:          command.Evidence,
	})
	if err != nil {
		// 权威答不出是未形成，不冒充不允许或未配置（UC-PC-003 结果表）；与本编排其余未形成答案一并
		// 上抛，HTTP 侧 5xx。
		return DisposeShipmentRequestResult{}, fmt.Errorf("dispose shipment request: disposition authority: %w", err)
	}
	switch authority.Outcome {
	case ports.AuthorizationGranted:
	case ports.AuthorizationRefused:
		return DisposeShipmentRequestResult{
			outcome: AuthorizedDispositionNotAuthorized,
			state:   request.State(),
		}, nil
	case ports.AuthorizationRulesNotConfigured:
		return DisposeShipmentRequestResult{
			outcome: AuthorizedDispositionAuthorityRulesNotConfigured,
			state:   request.State(),
		}, nil
	default:
		return DisposeShipmentRequestResult{}, ErrUnexpectedAuthorizationOutcome
	}

	task := request.AcceptanceDecisionTask()
	current := task.SubmissionVersionID()
	if current != command.SubmissionVersion {
		// 处置对象已被新提交版本换代。不记到新任务上，也不报错重试——重试一万次版本也不会换回来；
		// 处置人要做的是按当前版本重读队列。
		return DisposeShipmentRequestResult{
			outcome:        AuthorizedDispositionVersionSuperseded,
			state:          request.State(),
			currentVersion: current,
		}, nil
	}

	disposition, err := domain.NewAuthorizedDisposition(domain.AuthorizedDispositionSpec{
		Choice:     command.Choice,
		Authority:  authority.Authority,
		Disposer:   command.Disposer,
		Reason:     command.Reason,
		Evidence:   command.Evidence,
		DisposedAt: handler.deps.Clock.Now(),
	})
	if err != nil {
		// 去向、处置人、原因与证据的必填由 Intake 在构造领域值时把守，授权引用由端口契约保证`已授权`时
		// 必带；走到这里还缺就是装配或编程错误。
		return DisposeShipmentRequestResult{}, fmt.Errorf("dispose shipment request: %w", err)
	}
	spec := domain.DisposeUnderAuthoritySpec{Disposition: disposition}
	if command.Choice == domain.DisposeByRejection {
		// 决定标识只在`拒绝`去向签发，且在授权之后：一次未获授权的尝试不该消耗一个本上下文签发的
		// 稀缺身份；`交客户补充`不形成决定，不领标识。
		decisionID, err := handler.deps.Identities.NextAcceptanceDecisionID(ctx)
		if err != nil {
			return DisposeShipmentRequestResult{}, fmt.Errorf("dispose shipment request: decision identity: %w", err)
		}
		spec.DecisionID = decisionID
	}

	disposed, err := request.DisposeUnderAuthority(spec)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrAuthorizedDispositionAlreadyRecorded):
			existing, _ := task.AuthorizedDisposition()
			return DisposeShipmentRequestResult{
				outcome:        AuthorizedDispositionAlreadyRecorded,
				state:          request.State(),
				disposition:    existing,
				hasDisposition: true,
				currentVersion: current,
			}, nil
		case errors.Is(err, domain.ErrDecisionAlreadyFormed), errors.Is(err, domain.ErrAcceptanceTaskComplete):
			decision, formed := request.AcceptanceDecision()
			return DisposeShipmentRequestResult{
				outcome:        AuthorizedDispositionTaskAlreadyClosed,
				state:          request.State(),
				decision:       decision,
				hasDecision:    formed,
				currentVersion: current,
			}, nil
		case errors.Is(err, domain.ErrNotWaitingOnAuthorizedDisposition):
			return DisposeShipmentRequestResult{
				outcome:        AuthorizedDispositionNotWaiting,
				state:          request.State(),
				currentVersion: current,
			}, nil
		default:
			return DisposeShipmentRequestResult{}, fmt.Errorf("dispose under authority: %w", err)
		}
	}

	saved, err := handler.deps.Requests.Save(ctx, command.Identity, disposed)
	if err != nil {
		return DisposeShipmentRequestResult{}, fmt.Errorf("dispose shipment request: %w", err)
	}
	if saved != ports.ShipmentRequestSaved {
		// 版本冲突是业务答案（ADR-0031）：抢先那一方可能是另一次处置、一次换代或一个决定。调用方重读
		// 再重放，本层不代猜库里此刻是什么。本方这次处置确定没落库，因此这里不发释放——那笔占用归真正
		// 成立的那条路处置。
		return DisposeShipmentRequestResult{
			outcome:        AuthorizedDispositionConflict,
			state:          request.State(),
			currentVersion: current,
		}, nil
	}

	decision, formed := disposed.AcceptanceDecision()
	return DisposeShipmentRequestResult{
		outcome:        AuthorizedDispositionRecorded,
		state:          disposed.State(),
		disposition:    disposition,
		hasDisposition: true,
		decision:       decision,
		hasDecision:    formed,
		currentVersion: current,
		compensation:   handler.releaseOccupation(ctx, command),
	}, nil
}

// releaseOccupation 在处置落库后按原关联解除本版本的资金控制，形照 RejectShipmentRequestHandler 的同名
// 段：读不回已记录的判断时不发释放（不知道关联就发，settlement-accounting 无从认领哪一笔），交回补偿
// 续办引用；按「有没有成立的项」发而不按结论（ADR-0125 决定四）。
//
// 释放失败不回滚处置：处置已经越过提交边界，回滚它等于让一次补偿失败改写业务结果；补偿按原关联另行
// 续办，续办方是系统——责任引用答的是谁承担失败或补偿责任，不改变谁来重试（ADR-0132 决定四）。
func (handler *DisposeShipmentRequestHandler) releaseOccupation(
	ctx context.Context,
	command DisposeShipmentRequestCommand,
) domain.OwnershipContinuationReference {
	recorded, err := handler.deps.Judgments.LoadRecordedJudgments(
		ctx, command.Identity.TenantID(), command.ShipmentRequestID, command.SubmissionVersion)
	if err != nil {
		return handler.compensationReference(command, RecordedJudgmentsUnavailable)
	}
	if !recorded.FinancialControl.OccupationFormed() {
		return domain.OwnershipContinuationReference{}
	}

	if err := handler.deps.Release.ReleasePreAcceptanceControl(ctx, ports.ControlReleaseRequest{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		ControlResultID:   recorded.FinancialControl.ResultID(),
	}); err != nil {
		return handler.compensationReference(
			command,
			ControlReleasePending,
			recorded.FinancialControl.ResultID().String(),
		)
	}
	return domain.OwnershipContinuationReference{}
}

// compensationReference 派生一次补偿的续办引用。释放失败那一条带上控制关联，与主动拒绝和自动拒绝派生的
// 引用逐字一致：同一笔占用因同一原因停下，三条路径必须给出同一个引用，续办方才不必先知道这次是谁停的。
func (handler *DisposeShipmentRequestHandler) compensationReference(
	command DisposeShipmentRequestCommand,
	reason JudgmentPendingReason,
	scope ...string,
) domain.OwnershipContinuationReference {
	return judgmentContinuation(
		reason,
		append([]string{
			command.Identity.TenantID().String(),
			command.Identity.CustomerAccountID().String(),
			command.ShipmentRequestID.String(),
			command.SubmissionVersion.String(),
		}, scope...)...,
	)
}
