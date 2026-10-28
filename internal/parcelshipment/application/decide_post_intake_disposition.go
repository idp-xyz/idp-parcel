package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ErrPostIntakeDispositionNotAssembled 说明决定编排少接了一个必需件。少接一个件等多久都不会长出来，所以响亮报错，不当作未决，也不悄悄跳过信封。
var ErrPostIntakeDispositionNotAssembled = errors.New("parcel shipment: post-intake disposition decision is not assembled")

// ErrPostIntakeDispositionRequestNotVisible 是`统一不可见结果`：命令指名的请求在册上不是一份待回应的停止请求——查无、属于另一个租户、不属于这件包裹、或者
// 是一份已取消/已拒绝的取消记录，同一个答案。可区分即可枚举别人的取消请求。
var ErrPostIntakeDispositionRequestNotVisible = errors.New("parcel shipment: post-intake disposition request is not visible")

// PostIntakeDispositionOutcome 是决定命令的封闭结果。除`已决定`之外都不落任何东西。
//
//   - `已决定`：这一次赢了，决定追加到包裹的决定链上，信封同笔交出。
//   - `已决定过`：同一个决定方带着同样内容重发了同一条命令，读回先前形成的那一份；不追加、不再发信封。
//   - `前一份已变`：命令里的前一版与决定链此刻的链尾对不上（别人已经决定过），读回链尾，决定人看过再决定。
//   - `未越过边界`：这件包裹此刻没有责任起点——收寄还没被采用，或者是面单渠道服务（承运商收寄就是它的终局边界，没有收寄后处置这条路）。确定的拒绝，不是未决：
//     取消请求走取消编排。
//   - `终局成立`：包裹已有当前有效终局或有效取消；新需求建立关联委托，不重开原结果。
//   - `请求已被回应`：命令指名的那份请求先被另一份决定回应了；要改，是在决定链上追加一份运营发起的更正。
type PostIntakeDispositionOutcome uint8

const (
	PostIntakeDispositionOutcomeInvalid PostIntakeDispositionOutcome = iota
	PostIntakeDispositionDecided
	PostIntakeDispositionAlreadyDecided
	PostIntakeDispositionPredecessorMoved
	PostIntakeDispositionBoundaryNotCrossed
	PostIntakeDispositionFinalOutcomeStands
	PostIntakeDispositionRequestAlreadyAnswered
)

func (outcome PostIntakeDispositionOutcome) String() string {
	switch outcome {
	case PostIntakeDispositionDecided:
		return "DECIDED"
	case PostIntakeDispositionAlreadyDecided:
		return "ALREADY_DECIDED"
	case PostIntakeDispositionPredecessorMoved:
		return "PREDECESSOR_MOVED"
	case PostIntakeDispositionBoundaryNotCrossed:
		return "BOUNDARY_NOT_CROSSED"
	case PostIntakeDispositionFinalOutcomeStands:
		return "FINAL_OUTCOME_STANDS"
	case PostIntakeDispositionRequestAlreadyAnswered:
		return "REQUEST_ALREADY_ANSWERED"
	default:
		return ""
	}
}

// DecidePostIntakeDispositionCommand 指名对哪件包裹形成什么决定，凭什么。租户与决定人由接入面按认证结果给，不从请求体收。
//
// Supersedes 是决定人看到的决定链链尾版本（没有决定过为 0）：它让「我是看过这一版才改的」成为命令的一部分，并发里两个决定人不会互相覆盖。RequestKey 非零时
// 决定来源是客户请求，指名它回应的那份请求；为零是运营发起。
type DecidePostIntakeDispositionCommand struct {
	Tenant      domain.TenantID
	Parcel      domain.DeclaredParcelID
	Supersedes  int
	Direction   domain.PostIntakeDispositionDirection
	RequestKey  domain.SourceRequestKey
	Destination domain.PostIntakeDispositionDestination
	Decider     domain.PostIntakeDispositionDeciderReference
	Basis       domain.PostIntakeDispositionBasis
}

type DecidePostIntakeDispositionResult struct {
	outcome     PostIntakeDispositionOutcome
	disposition domain.PostIntakeDisposition
	hasDecision bool
	basis       domain.CheckReason
}

func (result DecidePostIntakeDispositionResult) Outcome() PostIntakeDispositionOutcome {
	return result.outcome
}

// Disposition 在`已决定`时是本次追加的决定；在`已决定过`时是先前形成的那一份；在`前一份已变`时是链尾；在`请求已被回应`时是回应了它的那份。
// 其余结果没有决定可给。
func (result DecidePostIntakeDispositionResult) Disposition() (domain.PostIntakeDisposition, bool) {
	return result.disposition, result.hasDecision
}

// FinalBasis 在`终局成立`时指名那份终局或有效取消。
func (result DecidePostIntakeDispositionResult) FinalBasis() domain.CheckReason {
	return result.basis
}

type DecidePostIntakeDispositionDeps struct {
	Dispositions ports.PostIntakeDispositions
	Requests     ports.PostIntakeDispositionRequests
	// Adoptions 只读责任起点：包裹有没有越过取消边界、越过的是哪一版，都以采用链为准，不信请求记录里当时记下的版本。
	Adoptions     ports.ResponsibilityStartView
	Finals        ports.CurrentFinalView
	Cancellations ports.ParcelCancellationView
	Handoff       ports.PostIntakeDispositionHandoff
	// BoundaryLock 让决定与同一包裹上的取消、收寄采用、终局采用排队：决定的前提（已越界、尚无终局）正是它们在锁里写的东西。
	BoundaryLock ports.CancellationBoundaryLock
	Clock        ports.Clock
}

type DecidePostIntakeDispositionHandler struct {
	deps DecidePostIntakeDispositionDeps
}

func NewDecidePostIntakeDispositionHandler(deps DecidePostIntakeDispositionDeps) *DecidePostIntakeDispositionHandler {
	return &DecidePostIntakeDispositionHandler{deps: deps}
}

// Handle 对一件越过取消边界的包裹形成一份收寄后服务处置决定（ADR-0332 决定一至四）。
//
// 锁序：包裹取消边界锁（ports.CancellationBoundaryLock，P）→ 读决定链、终局、责任起点、请求 → 追加决定 → 交信封。本编排不保存委托，不取接受链的作用域锁（S）与委托行锁
// （R），一笔事务至多持一个 P 键，所以与取消、采用、终局判断、冲突解开各路之间、与持 S、R 的各路之间都不成环。调用方把整个 Handle 放进一笔事务：决定与信封同生共死，
// 返回错误即整笔回滚。
//
// 读不动不是未决：这是运营人员下的命令，读口出错就整笔失败（答案未形成），让人稍后重来；已形成的业务答案（上面各格）是正常返回。
func (handler *DecidePostIntakeDispositionHandler) Handle(
	ctx context.Context,
	command DecidePostIntakeDispositionCommand,
) (DecidePostIntakeDispositionResult, error) {
	if err := handler.assembled(); err != nil {
		return DecidePostIntakeDispositionResult{}, err
	}
	if command.Supersedes < 0 {
		return DecidePostIntakeDispositionResult{}, fmt.Errorf("%w: predecessor version %d", domain.ErrInvalidPostIntakeDisposition, command.Supersedes)
	}
	if err := domain.PostIntakeDispositionShapeError(command.Direction, command.Destination.String() != ""); err != nil {
		return DecidePostIntakeDispositionResult{}, err
	}
	if !lockCancellationBoundary(ctx, handler.deps.BoundaryLock, command.Tenant, command.Parcel) {
		return DecidePostIntakeDispositionResult{}, ErrCancellationBoundaryNotLocked
	}

	// 重发先于一切判断：决定形成之后包裹可能又有了终局，同一个人原样重发要读回自己那一份，不是被告知终局成立。
	next, found, err := handler.deps.Dispositions.FindPostIntakeDispositionAt(ctx, command.Tenant, command.Parcel, command.Supersedes+1)
	if err != nil {
		return DecidePostIntakeDispositionResult{}, fmt.Errorf("decide post-intake disposition: %w", err)
	}
	if found && sameCommand(next, command) {
		return decidedBefore(next, PostIntakeDispositionAlreadyDecided), nil
	}

	if stands, basis, err := handler.standingFinal(ctx, command); err != nil {
		return DecidePostIntakeDispositionResult{}, err
	} else if stands {
		return DecidePostIntakeDispositionResult{outcome: PostIntakeDispositionFinalOutcomeStands, basis: basis}, nil
	}

	start, crossed, err := handler.deps.Adoptions.FindResponsibilityStart(ctx, command.Tenant, command.Parcel)
	if err != nil {
		return DecidePostIntakeDispositionResult{}, fmt.Errorf("decide post-intake disposition: responsibility start: %w", err)
	}
	if !crossed {
		return DecidePostIntakeDispositionResult{outcome: PostIntakeDispositionBoundaryNotCrossed}, nil
	}

	if command.RequestKey.String() != "" {
		if answered, err := handler.requestAnswer(ctx, command); err != nil {
			return DecidePostIntakeDispositionResult{}, err
		} else if answered != nil {
			return *answered, nil
		}
	}

	head, hasHead, err := handler.deps.Dispositions.FindCurrentPostIntakeDisposition(ctx, command.Tenant, command.Parcel)
	if err != nil {
		return DecidePostIntakeDispositionResult{}, fmt.Errorf("decide post-intake disposition: %w", err)
	}
	if headVersion(head, hasHead) != command.Supersedes {
		return predecessorMoved(head, hasHead), nil
	}

	source := domain.PostIntakeOperatorInitiated
	if command.RequestKey.String() != "" {
		source = domain.PostIntakeCustomerRequest
	}
	// 决定时刻截到库的精度（微秒）：首次决定的答复带的是这里的值，之后读回的是库里存的值，两者要是同一个值。
	disposition, err := domain.DecidePostIntakeDisposition(domain.PostIntakeDispositionSpec{
		Tenant:              command.Tenant,
		Parcel:              command.Parcel,
		Version:             command.Supersedes + 1,
		Supersedes:          command.Supersedes,
		Direction:           command.Direction,
		Source:              source,
		RequestKey:          command.RequestKey,
		Destination:         command.Destination,
		ResponsibilityStart: start.Key.Version,
		Decider:             command.Decider,
		Basis:               command.Basis,
		DecidedAt:           handler.deps.Clock.Now().UTC().Truncate(time.Microsecond),
	})
	if err != nil {
		return DecidePostIntakeDispositionResult{}, fmt.Errorf("decide post-intake disposition: %w", err)
	}
	appended, err := handler.deps.Dispositions.AppendPostIntakeDisposition(ctx, disposition)
	if err != nil {
		return DecidePostIntakeDispositionResult{}, fmt.Errorf("decide post-intake disposition: %w", err)
	}
	switch appended {
	case ports.PostIntakeDispositionAppended:
	case ports.PostIntakeDispositionVersionTaken:
		return handler.lostRace(ctx, command)
	default:
		return DecidePostIntakeDispositionResult{}, fmt.Errorf("decide post-intake disposition: append outcome %d is not a known outcome", appended)
	}
	if err := handler.deps.Handoff.HandOffPostIntakeDisposition(ctx, ports.PostIntakeDispositionHandoffIntent{Disposition: disposition}); err != nil {
		return DecidePostIntakeDispositionResult{}, fmt.Errorf("decide post-intake disposition: hand off: %w", err)
	}
	return decidedBefore(disposition, PostIntakeDispositionDecided), nil
}

func (handler *DecidePostIntakeDispositionHandler) assembled() error {
	deps := handler.deps
	if deps.Dispositions == nil || deps.Requests == nil || deps.Adoptions == nil || deps.Finals == nil ||
		deps.Cancellations == nil || deps.Handoff == nil || deps.BoundaryLock == nil || deps.Clock == nil {
		return ErrPostIntakeDispositionNotAssembled
	}
	return nil
}

// standingFinal 问包裹此刻有没有当前有效终局或有效取消：有则不再有收寄后服务处置可做。取消编排的边界核验读同样两处，这里是它的镜像。
func (handler *DecidePostIntakeDispositionHandler) standingFinal(
	ctx context.Context,
	command DecidePostIntakeDispositionCommand,
) (bool, domain.CheckReason, error) {
	final, found, err := handler.deps.Finals.FindCurrentFinal(ctx, command.Tenant, command.Parcel)
	if err != nil {
		return false, domain.CheckReason{}, fmt.Errorf("decide post-intake disposition: current final: %w", err)
	}
	if found && final.Finalized {
		basis, err := domain.NewCheckReason("FINAL_OUTCOME/" + final.Final.Version().String())
		return err == nil, basis, wrapBasis(err)
	}
	cancellation, cancelled, err := handler.deps.Cancellations.FindCancellation(ctx, command.Tenant, command.Parcel)
	if err != nil {
		return false, domain.CheckReason{}, fmt.Errorf("decide post-intake disposition: effective cancellation: %w", err)
	}
	if cancelled {
		basis, err := domain.NewCheckReason("PARCEL_CANCELLATION/" + cancellation.ID().String())
		return err == nil, basis, wrapBasis(err)
	}
	return false, domain.CheckReason{}, nil
}

func wrapBasis(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("decide post-intake disposition: name the standing final: %w", err)
}

// requestAnswer 核对命令指名的请求：它须是这件包裹上一份待回应的停止请求（越界待处置，或被认定不成立的取消），不是则统一不可见；已被另一份决定回应则答
// `请求已被回应`。返回非空指针说明已有答案，调用方直接交回。
func (handler *DecidePostIntakeDispositionHandler) requestAnswer(
	ctx context.Context,
	command DecidePostIntakeDispositionCommand,
) (*DecidePostIntakeDispositionResult, error) {
	key := ports.CancellationRequestKey{TenantID: command.Tenant, RequestKey: command.RequestKey, Parcel: command.Parcel}
	if _, found, err := handler.deps.Requests.FindPostIntakeDispositionRequest(ctx, key); err != nil {
		return nil, fmt.Errorf("decide post-intake disposition: request: %w", err)
	} else if !found {
		return nil, fmt.Errorf("decide post-intake disposition: %w", ErrPostIntakeDispositionRequestNotVisible)
	}
	answering, answered, err := handler.deps.Dispositions.FindPostIntakeDispositionAnswering(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("decide post-intake disposition: answering decision: %w", err)
	}
	if answered {
		result := decidedBefore(answering, PostIntakeDispositionRequestAlreadyAnswered)
		return &result, nil
	}
	return nil, nil
}

// lostRace 处理追加撞上唯一约束：锁内本不该发生，有锁之外的写入时重读一遍再作答——请求被回应了，或链尾已经动了。
func (handler *DecidePostIntakeDispositionHandler) lostRace(
	ctx context.Context,
	command DecidePostIntakeDispositionCommand,
) (DecidePostIntakeDispositionResult, error) {
	if command.RequestKey.String() != "" {
		answered, err := handler.requestAnswer(ctx, command)
		if err != nil {
			return DecidePostIntakeDispositionResult{}, err
		}
		if answered != nil {
			return *answered, nil
		}
	}
	head, hasHead, err := handler.deps.Dispositions.FindCurrentPostIntakeDisposition(ctx, command.Tenant, command.Parcel)
	if err != nil {
		return DecidePostIntakeDispositionResult{}, fmt.Errorf("decide post-intake disposition: %w", err)
	}
	return predecessorMoved(head, hasHead), nil
}

func headVersion(head domain.PostIntakeDisposition, present bool) int {
	if !present {
		return 0
	}
	return head.Version()
}

func predecessorMoved(head domain.PostIntakeDisposition, present bool) DecidePostIntakeDispositionResult {
	return DecidePostIntakeDispositionResult{
		outcome:     PostIntakeDispositionPredecessorMoved,
		disposition: head,
		hasDecision: present,
	}
}

func decidedBefore(disposition domain.PostIntakeDisposition, outcome PostIntakeDispositionOutcome) DecidePostIntakeDispositionResult {
	return DecidePostIntakeDispositionResult{outcome: outcome, disposition: disposition, hasDecision: true}
}

// sameCommand 问已有的那一版是不是这条命令的重发：去向、回应的请求、目的地、依据和决定方都一样。
func sameCommand(existing domain.PostIntakeDisposition, command DecidePostIntakeDispositionCommand) bool {
	return existing.Direction() == command.Direction &&
		existing.RequestKey() == command.RequestKey &&
		existing.Destination() == command.Destination &&
		existing.Basis() == command.Basis &&
		existing.Decider() == command.Decider
}
