package postgres_test

import (
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// PBC-08 行为面负向证据（bento-gate-reeval 票 02）：写口在无事务上下文必须被
// RequireExecutor 拒绝。拒绝先于任何入参解读，所以传零值就够；若有人把入参校验挪到
// 守卫之前，断言会以「错误不是 ErrTransactionRequired」如实变红——守卫先行本身是
// 被守的性质。本包其余写口的同款证据在各自聚合的测试文件里，这里只补漏网的。
func TestResolutionKeyWritesRefuseToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()

	keys, err := adapter.NewCommercialResolutionKeyStore(db)
	if err != nil {
		t.Fatalf("构造解析键登记库：%v", err)
	}
	if _, err := keys.RegisterResolutionKey(ctx, pspartycommercial.ResolutionKeyRow{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记解析键应返回 ErrTransactionRequired，实得：%v", err)
	}
}
