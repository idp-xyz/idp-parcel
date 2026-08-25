package postgres_test

import (
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
)

// PBC-08 行为面负向证据（bento-gate-reeval 票 02）：版本签发推进序列状态、是写不是读
// （result_identity.go 的裁定），无事务上下文必须被 RequireExecutor 拒绝。守卫住在
// 未导出的 next 里，证据落在两个导出签发口上——那是守卫真实的调用路径。
func TestResultVersionIssuanceRefusesToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()

	versions, err := adapter.NewResultVersions(db)
	if err != nil {
		t.Fatalf("构造版本签发器：%v", err)
	}
	if _, err := versions.NextPickupResultVersion(ctx); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务签发揽收结果版本应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := versions.NextDeliveryResultVersion(ctx); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务签发交付结果版本应返回 ErrTransactionRequired，实得：%v", err)
	}
}
