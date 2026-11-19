package postgres_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// Covers: AT-WF-009（库位类型的部分、包材的部分）持久化面——「本租户有没有登记过任何一个仓」：一个仓都没登记答没有；别的租户登记的仓
// 不算；本租户登记了就答有，停用的仓也算（登记册只追加，登记过就留着）；入参没经构造器是装配错了，答 error，不折成「没有」。
func TestHasAnyWarehouseAnswersPerTenantAndCountsDeactivatedWarehouses(t *testing.T) {
	warehouses, _, transactor, _ := newWarehouseStores(t)
	ctx := t.Context()
	own := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	other := valueOf(t, domain.NewTenantID, "SYN-TENANT-02")

	if registered, err := warehouses.HasAnyWarehouse(ctx, own); err != nil || registered {
		t.Fatalf("一个仓都没登记 = (%v, %v)，want (false, nil)", registered, err)
	}

	mustSave(t, transactor, warehouses, revisionFixture(t, "SYN-TENANT-02", "SYN-WH-01", 1, nil))
	if registered, err := warehouses.HasAnyWarehouse(ctx, own); err != nil || registered {
		t.Fatalf("只有别的租户登记了仓 = (%v, %v)，want 本租户仍是 (false, nil)", registered, err)
	}
	if registered, err := warehouses.HasAnyWarehouse(ctx, other); err != nil || !registered {
		t.Fatalf("别的租户自己 = (%v, %v)，want (true, nil)", registered, err)
	}

	// 只登记过一笔、而且那一笔就是停用的修订：停用的仓也算登记过。
	mustSave(t, transactor, warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-09", 1, func(spec *domain.WarehouseContentSpec) {
		spec.Deactivated = true
	}))
	if registered, err := warehouses.HasAnyWarehouse(ctx, own); err != nil || !registered {
		t.Fatalf("登记了一个停用的仓 = (%v, %v)，want (true, nil)", registered, err)
	}

	if _, err := warehouses.HasAnyWarehouse(ctx, domain.TenantID{}); err == nil {
		t.Errorf("缺租户应报错，不折成「没有」")
	}
}
