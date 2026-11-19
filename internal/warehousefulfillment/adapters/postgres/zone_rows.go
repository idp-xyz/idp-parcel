package postgres

import (
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// zoneColumns 是库区修订表（0002 迁移）被读出的列，读回领域修订与读面的行共用这一份，列序与 storedZone 的扫描一一对应。
const zoneColumns = `tenant_id, warehouse_code, zone_code, revision, zone_name, zone_usage, deactivated, registrant, registered_at`

// storedZone 是库里一行的原样：文本一个字节都没动。
type storedZone struct {
	tenant       string
	warehouse    string
	code         string
	revision     int
	name         string
	usage        string
	deactivated  bool
	registrant   string
	registeredAt time.Time
}

func scanStoredZone(scanner rowScanner) (storedZone, error) {
	var stored storedZone
	err := scanner.Scan(
		&stored.tenant, &stored.warehouse, &stored.code, &stored.revision, &stored.name, &stored.usage,
		&stored.deactivated, &stored.registrant, &stored.registeredAt,
	)
	return stored, err
}

// row 把一行转成读面的行。时刻统一成 UTC，与领域修订里的登记时刻同形。
func (stored storedZone) row() ports.ZoneRow {
	return ports.ZoneRow{
		WarehouseCode: stored.warehouse,
		ZoneCode:      stored.code,
		Revision:      stored.revision,
		Name:          stored.name,
		Usage:         stored.usage,
		Deactivated:   stored.deactivated,
		Registrant:    stored.registrant,
		RegisteredAt:  stored.registeredAt.UTC(),
	}
}

// zoneRevisionOfStored 经领域的真构造器重建一笔修订：任何一格过不了门即坏数据，上抛不吞。错误里只带仓编码、库区编码与修订号。
func zoneRevisionOfStored(stored storedZone) (domain.ZoneRevision, error) {
	fail := func(cell string, err error) (domain.ZoneRevision, error) {
		return domain.ZoneRevision{}, fmt.Errorf(
			"在册的库区 %q/%q 修订 %d 的%s读不回：%w", stored.warehouse, stored.code, stored.revision, cell, err)
	}
	tenant, err := domain.NewTenantID(stored.tenant)
	if err != nil {
		return fail("租户", err)
	}
	warehouse, err := domain.NewWarehouseCode(stored.warehouse)
	if err != nil {
		return fail("仓编码", err)
	}
	code, err := domain.NewZoneCode(stored.code)
	if err != nil {
		return fail("库区编码", err)
	}
	name, err := domain.NewZoneName(stored.name)
	if err != nil {
		return fail("名称", err)
	}
	usage, known := domain.ZoneUsageNamed(stored.usage)
	if !known {
		return fail("用途", fmt.Errorf("%q 不在封闭集内", stored.usage))
	}
	content, err := domain.NewZoneContent(domain.ZoneContentSpec{Name: name, Usage: usage, Deactivated: stored.deactivated})
	if err != nil {
		return fail("内容", err)
	}
	registrant, err := domain.NewRegistrant(stored.registrant)
	if err != nil {
		return fail("登记责任方", err)
	}
	revision, err := domain.NewZoneRevision(tenant, warehouse, code, stored.revision, content, registrant, stored.registeredAt)
	if err != nil {
		return fail("修订", err)
	}
	return revision, nil
}
