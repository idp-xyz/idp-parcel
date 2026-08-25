package outboxintent_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// PBC-08 行为面负向证据（bento-gate-reeval 票 02）：入队一步是写，无事务上下文必须被
// RequireExecutor 拒绝。此前它的证据只经各上下文 Outbox 适配器间接取道——按票 01 的
// 口径问题裁定：共享写步自带第一手证明，不留豁免清单。守卫先于任何入参解读，store
// 与信封因此可以为零值——若有人把顺序改掉，本断言会以别的失败形状如实变红。
func TestEnqueueOnceRefusesToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	err = outboxintent.EnqueueOnce(t.Context(), db, nil, eventing.Envelope{})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}
