package application

import (
	"context"
	"errors"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// ErrControlScopeLockNotAssembled 说明这份委托有要排队的占用作用域，而取锁口没接上。它与取锁失败落同一格（业务事实尚未
// 形成），但单列一个哨兵：少接一个件等多久都不会长出来，与一次偶发的依赖故障不是一回事。
//
// 哨兵只在授权处置的错误链里露出：撤回、主动拒绝和接受链形成决定的两处（自动拒绝、停进`等待受控补充`）都把取锁错误折成
// 未决`DECISION_NOT_RECORDED`，哨兵在它们的结果里读不到，要分辨装配缺口只能回到装配本身。
var ErrControlScopeLockNotAssembled = errors.New("parcel shipment: control scope lock is not assembled")

// lockScopesBeforeSave 在保存委托之前取齐这份委托各版已成立占用所在作用域的锁（ports.PreAcceptanceControlScopeLock 头注
// 是锁序全文）。
//
// 保存委托（对委托行 UPDATE，R）与取作用域锁（S）在同一事务里，必须先 S 后 R：接受链先走财务控制取 S、再保存委托取 R，
// 本路径若先保存、再在释放里取 S，同一份委托并发时两笔互等。所以取锁在这里、保存之前，而不是留给保存之后的释放。
//
// 取哪几把由已记录的判断决定：每一版读回控制结果，成立了占用且存下了作用域的才取，与释放那一侧认领的是同一份。读不回某一版
// 的判断时不取那一版，也不停下——保存之后的释放读同一份判断，同样读不回，会把那一版留成补偿续办；撤回不等财务释放才生效
// （UC-PS-005）。次序与取锁的去重见 lockControlScopes。
func (scope occupationRelease) lockScopesBeforeSave(ctx context.Context, request domain.ShipmentRequest) error {
	controls := make([]domain.FinancialControlResult, 0, len(request.PriorSubmissionVersions())+1)
	for _, version := range versionsInLockOrder(request) {
		recorded, err := scope.judgments.LoadRecordedJudgments(ctx, scope.identity.TenantID(), scope.request, version)
		if err != nil {
			continue
		}
		controls = append(controls, recorded.FinancialControl)
	}
	return scope.lockControlScopes(ctx, controls)
}

// lockControlScopes 按给定次序为各控制结果成立占用的作用域取锁，同一作用域只取一次。次序是调用方的责任：同一份委托的各版
// 按 versionsInLockOrder。
func (scope occupationRelease) lockControlScopes(ctx context.Context, controls []domain.FinancialControlResult) error {
	taken := make(map[string]struct{}, len(controls))
	for _, control := range controls {
		if !control.OccupationFormed() || !control.Scope().Recorded() {
			continue
		}
		occupation := control.Scope()
		key := occupation.LegalEntity() + "\x1f" + occupation.Account() + "\x1f" + occupation.Currency()
		if _, done := taken[key]; done {
			continue
		}
		if scope.scopeLock == nil {
			return ErrControlScopeLockNotAssembled
		}
		if err := scope.scopeLock.LockControlScope(ctx, scope.identity.TenantID(), occupation); err != nil {
			return err
		}
		taken[key] = struct{}{}
	}
	return nil
}

// versionsInLockOrder 交回一份委托各版取作用域锁的次序：被取代各版按形成顺序在前，当前版本在后。接受链开头先放被取代各版
// （releaseSuperseded）、再施加当前版本的控制，取的就是这个次序；撤回类路径照它取，同一份委托上两边的锁之间才不成环。
func versionsInLockOrder(request domain.ShipmentRequest) []domain.SubmissionVersionID {
	priors := request.PriorSubmissionVersions()
	versions := make([]domain.SubmissionVersionID, 0, len(priors)+1)
	for _, prior := range priors {
		versions = append(versions, prior.VersionID())
	}
	return append(versions, request.CurrentSubmissionVersion().VersionID())
}
