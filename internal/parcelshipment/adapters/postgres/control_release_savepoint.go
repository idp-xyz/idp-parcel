package postgres

import (
	"context"
	"errors"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// controlReleaseSavepoint 是装饰器在调用方事务里开的 savepoint 名。逐版释放一版一开一收，名字重用无妨；释放侧内层不开同名的
// 点，退回时不会退到别人的点上。
const controlReleaseSavepoint = "parcel_shipment_control_release"

// SavepointedControlRelease 把接受前控制的释放隔在一个 savepoint 里：释放放不掉只能留补偿或停在释放那一步，不能连累调用方
// 事务里已经成立的那件事。
//
// 释放跑在调用方那一笔事务里，与调用方已经成立的那件事（一次暂停或一个终局决定）同笔提交。settlement-accounting 的释放
// 编排在库里读写冻结与暴露两本账、取作用域锁，一条 SQL 级错误（语句超时、锁等超时、权限、表被挪走）会把整笔事务打进中止
// 态；释放编排把这类错误折成`未形成`正常交回，PS 侧照样往下走，到提交时整笔才报 25P02 回滚：暂停随之蒸发、投递回滚重投，
// 终局决定写不进去。编排写定的落法——释放失败不撤销暂停、不回滚撤回（UC-PS-005「撤回提交成功后不等待财务释放才生效」）——
// 只有在释放失败不碰事务时才在库里成立。
//
// 内层答错误就退回 savepoint、把错误原样交回，各路据此留补偿或停下；成功就 RELEASE。ctx 不在事务里时直通：没有可保护的写，
// 事务块外也开不了 savepoint。
//
// 落在 PS 侧调用释放的这一层，不落在 settlement-accounting 的编排里：要保护的是 PS 自己的事务里已成立的事，隔离的边界是 PS
// 对外部上下文的这一次调用；释放编排不知道谁在外层、外层还要写什么。做法同网络路由的 SavepointedRouteCandidateCosts。
type SavepointedControlRelease struct {
	db    *bentopg.DB
	inner ports.PreAcceptanceControlRelease
}

var _ ports.PreAcceptanceControlRelease = (*SavepointedControlRelease)(nil)

// NewSavepointedControlRelease 不在构造时报错，好让装配点把它包在既有释放口外面而不多一段错误处理；少接一个件在第一次释放时
// 答装配缺口，各路把它当作一次放不掉，照旧留补偿或停下。
func NewSavepointedControlRelease(db *bentopg.DB, inner ports.PreAcceptanceControlRelease) *SavepointedControlRelease {
	return &SavepointedControlRelease{db: db, inner: inner}
}

func (release *SavepointedControlRelease) ReleasePreAcceptanceControl(
	ctx context.Context,
	request ports.ControlReleaseRequest,
) error {
	if release.db == nil || release.inner == nil {
		return errors.New("control release savepoint: not assembled")
	}
	executor, err := release.db.RequireExecutor(ctx)
	if errors.Is(err, bentopg.ErrTransactionRequired) {
		return release.inner.ReleasePreAcceptanceControl(ctx, request)
	}
	if err != nil {
		return fmt.Errorf("control release savepoint: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT "+controlReleaseSavepoint); err != nil {
		return fmt.Errorf("control release savepoint: %w", err)
	}
	if releaseErr := release.inner.ReleasePreAcceptanceControl(ctx, request); releaseErr != nil {
		if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+controlReleaseSavepoint); err != nil {
			// 退不回 savepoint，事务就还在中止态，随后的写会自己失败、整笔回滚；两条错误都交回，免得只剩一半原因。
			return errors.Join(releaseErr, fmt.Errorf("control release savepoint rollback: %w", err))
		}
		return releaseErr
	}
	if _, err := executor.Exec(ctx, "RELEASE SAVEPOINT "+controlReleaseSavepoint); err != nil {
		return fmt.Errorf("control release savepoint release: %w", err)
	}
	return nil
}
