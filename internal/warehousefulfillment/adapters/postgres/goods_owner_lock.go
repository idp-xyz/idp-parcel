package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
)

// 商品登记与记数共用这一把货主锁，键必须逐字相同。键若在两处各写一遍，改了一处另一处不会报错，
// 只是两边不再互斥：记数可以在登记提交之前读到旧修订，登记也可以在记数提交之前答没有在库库存
// （UC-WF-002「一致性、幂等与并发」）。

func lockGoodsOwnerExclusive(ctx context.Context, executor bentopg.Executor, tenant, owner string) error {
	return lockGoodsOwner(ctx, executor, false, tenant, owner)
}

func lockGoodsOwnerShared(ctx context.Context, executor bentopg.Executor, tenant, owner string) error {
	return lockGoodsOwner(ctx, executor, true, tenant, owner)
}

func lockGoodsOwner(ctx context.Context, executor bentopg.Executor, shared bool, tenant, owner string) error {
	statement := `SELECT pg_advisory_xact_lock(hashtext('warehouse_fulfillment.goods.owner'), hashtext($1 || chr(31) || $2))`
	if shared {
		statement = `SELECT pg_advisory_xact_lock_shared(hashtext('warehouse_fulfillment.goods.owner'), hashtext($1 || chr(31) || $2))`
	}
	if _, err := executor.Exec(ctx, statement, tenant, owner); err != nil {
		return fmt.Errorf("lock goods owner: %w", err)
	}
	return nil
}
