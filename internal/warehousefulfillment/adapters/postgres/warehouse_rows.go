package postgres

import (
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// warehouseColumns 是仓修订表（0001 迁移）被读出的列，读回领域修订与读面的行共用这一份，列序与 storedWarehouse 的扫描一一对应。
const warehouseColumns = `tenant_id, warehouse_code, revision, warehouse_name, kind, country_code, address_text,
	business_time_zone, colocated_node, deactivated, registrant, registered_at`

type rowScanner interface {
	Scan(dest ...any) error
}

// storedWarehouse 是库里一行的原样：可缺的两格是 NULL 的指针，文本一个字节都没动。
type storedWarehouse struct {
	tenant       string
	code         string
	revision     int
	name         string
	kind         string
	country      string
	address      string
	zone         *string
	node         *string
	deactivated  bool
	registrant   string
	registeredAt time.Time
}

func scanStoredWarehouse(scanner rowScanner) (storedWarehouse, error) {
	var stored storedWarehouse
	err := scanner.Scan(
		&stored.tenant, &stored.code, &stored.revision, &stored.name, &stored.kind, &stored.country, &stored.address,
		&stored.zone, &stored.node, &stored.deactivated, &stored.registrant, &stored.registeredAt,
	)
	return stored, err
}

// row 把一行转成读面的行。时刻统一成 UTC，与领域修订里的登记时刻同形。
func (stored storedWarehouse) row() ports.WarehouseRow {
	row := ports.WarehouseRow{
		WarehouseCode: stored.code,
		Revision:      stored.revision,
		Name:          stored.name,
		Kind:          stored.kind,
		CountryCode:   stored.country,
		AddressText:   stored.address,
		Deactivated:   stored.deactivated,
		Registrant:    stored.registrant,
		RegisteredAt:  stored.registeredAt.UTC(),
	}
	if stored.zone != nil {
		row.BusinessTimeZone, row.HasBusinessTimeZone = *stored.zone, true
	}
	if stored.node != nil {
		row.ColocatedNode, row.HasColocatedNode = *stored.node, true
	}
	return row
}

// revisionOfStored 经领域的真构造器重建一笔修订：任何一格过不了门即坏数据，上抛不吞。错误里只带仓编码与修订号，
// 不带地址正文（日志与指标不带地址正文，UC-WF-001「权限、隔离与审计」）。
func revisionOfStored(stored storedWarehouse) (domain.WarehouseRevision, error) {
	fail := func(cell string, err error) (domain.WarehouseRevision, error) {
		return domain.WarehouseRevision{}, fmt.Errorf("在册的仓 %q 修订 %d 的%s读不回：%w", stored.code, stored.revision, cell, err)
	}
	tenant, err := domain.NewTenantID(stored.tenant)
	if err != nil {
		return fail("租户", err)
	}
	code, err := domain.NewWarehouseCode(stored.code)
	if err != nil {
		return fail("仓编码", err)
	}
	name, err := domain.NewWarehouseName(stored.name)
	if err != nil {
		return fail("名称", err)
	}
	kind, known := domain.WarehouseKindNamed(stored.kind)
	if !known {
		return fail("种类", fmt.Errorf("%q 不在封闭集内", stored.kind))
	}
	country, err := domain.NewCountryCode(stored.country)
	if err != nil {
		return fail("国家码", err)
	}
	addressText, err := domain.NewAddressText(stored.address)
	if err != nil {
		return fail("地址正文", err)
	}
	spec := domain.WarehouseContentSpec{
		Name:        name,
		Kind:        kind,
		Address:     domain.NewWarehouseAddress(country, addressText),
		Deactivated: stored.deactivated,
	}
	if stored.zone != nil {
		if spec.BusinessTimeZone, err = domain.NewBusinessTimeZone(*stored.zone); err != nil {
			return fail("业务时区", err)
		}
	}
	if stored.node != nil {
		if spec.ColocatedNode, err = domain.NewColocatedNode(*stored.node); err != nil {
			return fail("同址物流节点", err)
		}
	}
	content, err := domain.NewWarehouseContent(spec)
	if err != nil {
		return fail("内容", err)
	}
	registrant, err := domain.NewRegistrant(stored.registrant)
	if err != nil {
		return fail("登记责任方", err)
	}
	revision, err := domain.NewWarehouseRevision(tenant, code, stored.revision, content, registrant, stored.registeredAt)
	if err != nil {
		return fail("修订", err)
	}
	return revision, nil
}
