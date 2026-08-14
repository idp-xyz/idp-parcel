// Package outboxintent 拥有「一份结果发一份意图」的入队一步（ADR-0043 的存储面）。
// 它对事件类型与载荷一无所知——各上下文的 outbox 适配器负责组装信封，这里只保证
// 「重发同一份不出第二份，且不撞出中止态」。三个上下文的适配器落出同形后按
// rule-of-three 提炼至此。
package outboxintent

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// bentoOutboxTable 是查重读的框架技术表。表名耦合由各上下文的真库测试守着（表名
// 变化时查询与测试一起红，不会静默漂移）；框架 Store 没有查询口，开了再换。
const bentoOutboxTable = migrate.SchemaBento + ".outbox"

// EnqueueOnce 在调用方事务里把信封入队一次。
//
// 先查后插：同一事务里撞唯一约束（SQLSTATE 23505）会把事务打进中止态，调用方同
// 事务的业务写入会被连带回滚——重发路径（重放）必须先按（来源+事件标识）查已入队，
// 查到即成功返回。并发首发的竞态窗口仍由唯一约束兜底：那一格撞上时本次事务确实
// 该重试，语义无损。
func EnqueueOnce(
	ctx context.Context,
	db *bentopg.DB,
	store *outbox.Store,
	envelope eventing.Envelope,
) error {
	executor, err := db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("outbox intent: %w", err)
	}
	var exists bool
	err = executor.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM `+bentoOutboxTable+` WHERE source = $1 AND event_id = $2)`,
		envelope.Source, string(envelope.ID),
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("outbox intent: check enqueued: %w", err)
	}
	if exists {
		return nil
	}
	if err := store.Enqueue(ctx, envelope); err != nil {
		return fmt.Errorf("outbox intent: enqueue: %w", err)
	}
	return nil
}
