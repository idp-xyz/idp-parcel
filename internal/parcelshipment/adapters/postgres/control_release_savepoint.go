package postgres

import (
	"context"
	"errors"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// controlReleaseSavepoint 是装饰器在调用方事务里开的 savepoint 名。逐版释放一版一开一收，名字重用无妨；释放侧内层不开同名的
// 点，退回时不会退到别人的点上。
const controlReleaseSavepoint = "parcel_shipment_control_release"

// SavepointedControlRelease 把接受前控制的释放隔在一个 savepoint 里：释放没收口只能留补偿、记进释放未受理册或停在释放
// 那一步，不能连累调用方事务里已经成立的那件事。
//
// 释放跑在调用方那一笔事务里，与调用方已经成立的那件事（一次暂停或一个终局决定）同笔提交。settlement-accounting 的释放
// 编排在库里读写冻结与暴露两本账、取作用域锁，一条 SQL 级错误（语句超时、锁等超时、权限、表被挪走）会把整笔事务打进中止
// 态；释放编排把这类错误折成`未形成`正常交回，PS 侧照样往下走，到提交时整笔才报 25P02 回滚：暂停随之蒸发、投递回滚重投，
// 终局决定写不进去。编排写定的落法——释放失败不撤销暂停、不回滚撤回（UC-PS-005「撤回提交成功后不等待财务释放才生效」）——
// 只有在释放失败不碰事务时才在库里成立。
//
// 内层答`已收口`就 RELEASE。答别的（或答错误）时，只有答的是类型化的`可重试未形成`、且事务此刻没中止，才 RELEASE 保住内层已经
// 落库的那一半；其余一律退回 savepoint，把答复原样交回，各路据此留补偿、记册或停下。释放编排在冻结账本落库之后才读暴露账本，
// 暴露账本读不回而语句没出错（行重建不了账本、读口交了 nil）时，冻结那本的释放不该跟着撤掉：提供方答`可重试未形成`，就是明确
// 承诺重放会补完另一本——两本账各自收口、不必同事务，续办重放时冻结账本按幂等交回原释放答案（ADR-0122 决定三）。SQL 级错误那
// 一格事务已中止，保不住那一半，如实撤掉（ADR-0291）。
//
// 保住只为`可重试未形成`背书。答错误是不知道发生了什么，事务没中止只说明连接可用，不说明提供方的写自洽；答`确定未受理`是
// 提供方说「我没受理」，后面带着已落库的写自相矛盾。这两种按 ADR-0282 都是重放同一答复也不会变、待人处理，保住一半之后没人补完，
// 退回就是什么都没发生。这一层是对 ports.PreAcceptanceControlRelease 的通用护栏，不只服务某一个提供方，所以不替提供方没说的话
// 担保。
//
// 事务状态靠在 savepoint 里执行一条无副作用的语句来探：成功即没中止（中止态的事务对退回以外的语句一律答 25P02），失败
// 一律按已中止处理。误判成已中止的代价是多退回一次、撤掉本可保住的那一半；反方向的误判会在中止的事务上 RELEASE，随后的写
// 全答 25P02，所以只认探测成功。探测自己出错也发生在 savepoint 里，由随后的退回收拾。
//
// 开、退、收 savepoint 本身失败是库此刻答不出，按`可重试未形成`交回，不当作提供方的答复：那一笔事务多半已经中止，
// 提交时整笔回滚，各路照旧留补偿也不会落库。
//
// 落在 PS 侧调用释放的这一层，不落在 settlement-accounting 的编排里：要保护的是 PS 自己的事务里已成立的事，隔离的边界是 PS
// 对外部上下文的这一次调用；释放编排不知道谁在外层、外层还要写什么。做法同网络路由的 SavepointedRouteCandidateCosts。
type SavepointedControlRelease struct {
	db    *bentopg.DB
	inner ports.PreAcceptanceControlRelease
}

var _ ports.PreAcceptanceControlRelease = (*SavepointedControlRelease)(nil)

// reasonControlReleaseSavepointUnavailable 是开、退、收 savepoint 失败时交回的`未形成`原因。本上下文自有，不冠 SA-。
const reasonControlReleaseSavepointUnavailable = "CONTROL_RELEASE_SAVEPOINT_UNAVAILABLE"

// NewSavepointedControlRelease 不在构造时报错，好让装配点把它包在既有释放口外面而不多一段错误处理；少接一个件在第一次释放时
// 答装配缺口（错误），各路把它当作形不成答复，照各自的落法处置。
func NewSavepointedControlRelease(db *bentopg.DB, inner ports.PreAcceptanceControlRelease) *SavepointedControlRelease {
	return &SavepointedControlRelease{db: db, inner: inner}
}

func (release *SavepointedControlRelease) ReleasePreAcceptanceControl(
	ctx context.Context,
	request ports.ControlReleaseRequest,
) (ports.ControlReleaseAnswer, error) {
	if release.db == nil || release.inner == nil {
		return ports.ControlReleaseAnswer{}, errors.New("control release savepoint: not assembled")
	}
	executor, err := release.db.RequireExecutor(ctx)
	if errors.Is(err, bentopg.ErrTransactionRequired) {
		return release.inner.ReleasePreAcceptanceControl(ctx, request)
	}
	if err != nil {
		return savepointUnavailable()
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT "+controlReleaseSavepoint); err != nil {
		return savepointUnavailable()
	}
	answer, releaseErr := release.inner.ReleasePreAcceptanceControl(ctx, request)
	settled := releaseErr == nil && answer.Outcome == ports.ControlReleaseSettled
	// 只有`可重试未形成`才值得探事务状态：别的答复不论事务如何都退回，不多发一次往返。
	keepsPersistedHalf := releaseErr == nil && answer.Outcome == ports.ControlReleaseNotFormed && transactionUsable(ctx, executor)
	if !settled && !keepsPersistedHalf {
		if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+controlReleaseSavepoint); err != nil {
			// 退不回 savepoint，事务就还在中止态，随后的写会自己失败、整笔回滚。
			return savepointUnavailable()
		}
		return answer, releaseErr
	}
	if _, err := executor.Exec(ctx, "RELEASE SAVEPOINT "+controlReleaseSavepoint); err != nil {
		return savepointUnavailable()
	}
	return answer, releaseErr
}

// transactionUsable 在事务里执行一条无副作用的语句：成功就说明事务没中止。任何失败（25P02 本身、探测自己的错误、ctx 已取消、
// 连接断开）都答否，调用方按需要退回处理。
func transactionUsable(ctx context.Context, executor bentopg.Executor) bool {
	_, err := executor.Exec(ctx, "SELECT 1")
	return err == nil
}

func savepointUnavailable() (ports.ControlReleaseAnswer, error) {
	reason, err := domain.NewCheckReason(reasonControlReleaseSavepointUnavailable)
	if err != nil {
		return ports.ControlReleaseAnswer{}, err
	}
	return ports.ControlReleaseAnswer{Outcome: ports.ControlReleaseNotFormed, Reason: reason}, nil
}
