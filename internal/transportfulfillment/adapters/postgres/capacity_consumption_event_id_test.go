package postgres

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// TestCapacityConsumptionEventIDKeepsKeysThatConcatenateAlikeApart 证池身份与预占里带「/」时，两组不同的键不会算出同一个
// ID：池身份写作「班次身份/维度词」，班次身份本身也可能带「/」，按「/」拼起来的话
// （池「甲/乙」，预占「丙」）与（池「甲」，预占「乙/丙」）是同一个串，EnqueueOnce 会把第二份静默吞掉。
func TestCapacityConsumptionEventIDKeepsKeysThatConcatenateAlikeApart(t *testing.T) {
	tenant, err := domain.NewTenantID("tenant-a")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	poolKey := func(pool string) ports.CapacityPoolKey {
		ref, err := domain.NewCapacityPoolReference(pool)
		if err != nil {
			t.Fatalf("池：%v", err)
		}
		return ports.CapacityPoolKey{TenantID: tenant, Pool: ref}
	}

	first := capacityConsumptionEventID(poolKey("SCHED-1/2026-10-27/WEIGHT"), "res-1")
	second := capacityConsumptionEventID(poolKey("SCHED-1"), "2026-10-27/WEIGHT/res-1")
	if first == second {
		t.Fatalf("两组不同的（池，预占）算出同一个 ID：%s", first)
	}
	if again := capacityConsumptionEventID(poolKey("SCHED-1/2026-10-27/WEIGHT"), "res-1"); again != first {
		t.Fatalf("同一组键两次算出不同 ID：%s / %s", first, again)
	}
}
