package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// VersionClosureOutcome 是按后继收尾旧版两口（截止与同起点退役，ADR-0233）共用的结果代数，判据同本包其余登记族：
// `已登记`是写下了一条新的截止或一次退役，`已登记（重放）`是同一次收尾再来一遍、一行不写，`未受理`是这一次收尾不成立、
// 一行不写——原因随 Cause 交回。
//
// 没有「内容冲突」格。截止那一侧，同一旧版上内容不同的更正在区间更正册上本是合法的下一条（ADR-0056 决定一），入口拒掉
// 的是截止本身不成立；退役那一侧，旧版已凭别的决定退役，登记方要做的是改请求，与其余不成立的收尾同一种续办。
type VersionClosureOutcome uint8

const (
	VersionClosureOutcomeInvalid VersionClosureOutcome = iota
	VersionClosureRegistered
	VersionClosureAlreadyRegistered
	VersionClosureNotAccepted
)

func (outcome VersionClosureOutcome) String() string {
	switch outcome {
	case VersionClosureRegistered:
		return "REGISTERED"
	case VersionClosureAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case VersionClosureNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// VersionClosureResult 携带落点、被拒的原因，以及落下（或重放认出）的那一次收尾：截止交回更正，退役交回退役——登记方
// 据它看到旧版此后选到哪一刻、或从哪一刻起不再参选。
type VersionClosureResult struct {
	outcome       VersionClosureOutcome
	cause         error
	correction    domain.ValidityCorrection
	hasCorrection bool
	retirement    domain.VersionRetirement
	hasRetirement bool
}

func (result VersionClosureResult) Outcome() VersionClosureOutcome { return result.outcome }

func (result VersionClosureResult) Cause() error { return result.cause }

func (result VersionClosureResult) Correction() (domain.ValidityCorrection, bool) {
	return result.correction, result.hasCorrection
}

func (result VersionClosureResult) Retirement() (domain.VersionRetirement, bool) {
	return result.retirement, result.hasRetirement
}

func closureNotAccepted(cause error) VersionClosureResult {
	return VersionClosureResult{outcome: VersionClosureNotAccepted, cause: cause}
}

// VersionClosureTarget 是两口共用的那一半：哪一范围、哪一对象的哪一版，让位给同一对象的哪一版。对象只写一次，命令的
// 形状里装不下「后继是另一个对象」。
type VersionClosureTarget struct {
	Tenant    domain.TenantID
	Scope     domain.CommercialScopeReference
	Kind      domain.CommercialObjectKind
	ObjectID  domain.CommercialObjectID
	Version   domain.CommercialVersionLabel
	Successor domain.CommercialVersionLabel
}

// CutCommercialVersionAtSuccessorCommand 把旧版截止在后继的生效时点。截止点不由调用方给，取后继此刻的选用起点——给了
// 就多一处会与后继对不上的格。更正依据与更正时刻由调用方给，理由在 CutAtSuccessorStart。
type CutCommercialVersionAtSuccessorCommand struct {
	VersionClosureTarget
	Reference   domain.ValidityCorrectionReference
	CorrectedAt time.Time
}

// RetireCommercialVersionAtSameStartCommand 让旧版按决定引用让位给同起点的后继。没有退役时刻一格：退役时刻取登记时刻，
// 理由在 RetireAtSameStartSuccessor。
type RetireCommercialVersionAtSameStartCommand struct {
	VersionClosureTarget
	Reference domain.RetirementReference
}

// CloseCommercialVersionHandler 是按后继收尾旧版的写侧编排：锁旧版行 → 读回整册 → 领域判收尾 → 写更正或退役。调用方
// 逐命令各起事务；登记写口无事务即拒。
type CloseCommercialVersionHandler struct {
	registry ports.VersionClosureRegistry
	clock    ports.Clock
}

func NewCloseCommercialVersionHandler(registry ports.VersionClosureRegistry, clock ports.Clock) *CloseCommercialVersionHandler {
	return &CloseCommercialVersionHandler{registry: registry, clock: clock}
}

// Cut 落一次截止。领域交回的每一种拒绝都答`未受理`：它们都是输入或册面状态不允许这一次收尾，重试同一份请求不会变好，
// 要登记方改请求或先去发布后继；读写失败才是 error，端点按「没形成答案」答。
func (handler *CloseCommercialVersionHandler) Cut(
	ctx context.Context,
	command CutCommercialVersionAtSuccessorCommand,
) (VersionClosureResult, error) {
	const operation = "cut commercial version at successor"
	registry, found, err := handler.lockAndLoad(ctx, command.VersionClosureTarget)
	if err != nil {
		return VersionClosureResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return closureNotAccepted(predecessorNotRegistered(command.VersionClosureTarget)), nil
	}
	target := command.VersionClosureTarget
	correction, replay, err := registry.CutAtSuccessorStart(
		target.Tenant, target.Scope, target.Kind, target.ObjectID, target.Version, target.Successor,
		command.Reference, command.CorrectedAt,
	)
	if err != nil {
		return closureNotAccepted(err), nil
	}
	cut := VersionClosureResult{correction: correction, hasCorrection: true}
	if replay {
		cut.outcome = VersionClosureAlreadyRegistered
		return cut, nil
	}
	saved, err := handler.registry.SaveValidityCorrection(ctx, correction)
	if err != nil {
		return VersionClosureResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	switch saved {
	case ports.ValidityCorrectionSaved:
		cut.outcome = VersionClosureRegistered
	case ports.ValidityCorrectionAlreadyRegistered:
		// 整册是在锁之后读的，同内容的一条在册就会在上面被认成重放；走到这里说明锁与读之间的约定被打破了，仍按库答。
		cut.outcome = VersionClosureAlreadyRegistered
	default:
		return VersionClosureResult{}, fmt.Errorf("%s: unexpected save outcome %d", operation, saved)
	}
	return cut, nil
}

// RetireAtSameStart 落一次同起点退役。拒绝的分格同 Cut。
func (handler *CloseCommercialVersionHandler) RetireAtSameStart(
	ctx context.Context,
	command RetireCommercialVersionAtSameStartCommand,
) (VersionClosureResult, error) {
	const operation = "retire commercial version at same start"
	registry, found, err := handler.lockAndLoad(ctx, command.VersionClosureTarget)
	if err != nil {
		return VersionClosureResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return closureNotAccepted(predecessorNotRegistered(command.VersionClosureTarget)), nil
	}
	target := command.VersionClosureTarget
	retirement, replay, err := registry.RetireAtSameStartSuccessor(
		target.Tenant, target.Scope, target.Kind, target.ObjectID, target.Version, target.Successor,
		command.Reference, handler.clock.Now(),
	)
	if err != nil {
		return closureNotAccepted(err), nil
	}
	retired := VersionClosureResult{retirement: retirement, hasRetirement: true}
	if replay {
		retired.outcome = VersionClosureAlreadyRegistered
		return retired, nil
	}
	saved, err := handler.registry.SaveVersionRetirement(ctx, retirement)
	if err != nil {
		return VersionClosureResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	switch saved {
	case ports.VersionRetirementSaved:
		retired.outcome = VersionClosureRegistered
	case ports.VersionRetirementAlreadyRegistered:
		// 判据同 Cut 那一格：锁之后读的整册里该已认出重放，按库答，时刻却是这一次的——不交回一个册上没有的时刻。
		return VersionClosureResult{outcome: VersionClosureAlreadyRegistered}, nil
	case ports.VersionRetirementContentConflict:
		return closureNotAccepted(domain.ErrVersionRetiredOtherwise), nil
	default:
		return VersionClosureResult{}, fmt.Errorf("%s: unexpected save outcome %d", operation, saved)
	}
	return retired, nil
}

// lockAndLoad 是两口共用的前半段：先锁旧版那一行，再读回整册（顺序的理由在 ports.VersionClosureRegistry）。found=false
// 是旧版不在本租户的册上，答`未受理`，不是故障。
func (handler *CloseCommercialVersionHandler) lockAndLoad(
	ctx context.Context,
	target VersionClosureTarget,
) (*domain.CommercialRegistry, bool, error) {
	found, err := handler.registry.LockVersionForClosure(ctx, target.Tenant, target.Kind, target.ObjectID, target.Version)
	if err != nil || !found {
		return nil, found, err
	}
	registry, err := handler.registry.LoadForScope(ctx, target.Tenant, target.Scope)
	if err != nil {
		return nil, false, err
	}
	return registry, true, nil
}

func predecessorNotRegistered(target VersionClosureTarget) error {
	return fmt.Errorf("%w: predecessor %s/%s", domain.ErrClosureVersionNotRegistered, target.ObjectID, target.Version)
}
