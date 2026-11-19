package postgres

import (
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// locationColumns 是库位修订表（0005 迁移）被读出的列，读回领域修订与读面的行共用这一份，列序与 storedLocation 的扫描一一对应。
const locationColumns = `tenant_id, warehouse_code, location_code, revision, zone_code, location_type_code, priority, deactivated, registrant, registered_at`

// storedLocation 是库里一行的原样：文本一个字节都没动。库位类型编码与优先级是可空列，nil 就是库里的 NULL——「没有」。
type storedLocation struct {
	tenant       string
	warehouse    string
	code         string
	revision     int
	zone         string
	locationType *string
	priority     *int32
	deactivated  bool
	registrant   string
	registeredAt time.Time
}

func scanStoredLocation(scanner rowScanner) (storedLocation, error) {
	var stored storedLocation
	err := scanner.Scan(
		&stored.tenant, &stored.warehouse, &stored.code, &stored.revision, &stored.zone, &stored.locationType,
		&stored.priority, &stored.deactivated, &stored.registrant, &stored.registeredAt,
	)
	return stored, err
}

// row 把一行转成读面的行。时刻统一成 UTC，与领域修订里的登记时刻同形。可缺的两格原样交出 nil，不折成空串或 0。
func (stored storedLocation) row() ports.LocationRow {
	return ports.LocationRow{
		WarehouseCode:    stored.warehouse,
		LocationCode:     stored.code,
		Revision:         stored.revision,
		ZoneCode:         stored.zone,
		LocationTypeCode: stored.locationType,
		Priority:         stored.priority,
		Deactivated:      stored.deactivated,
		Registrant:       stored.registrant,
		RegisteredAt:     stored.registeredAt.UTC(),
	}
}

// locationRevisionOfStored 经领域的真构造器重建一笔修订：任何一格过不了门即坏数据，上抛不吞。错误里只带仓编码、库位编码与修订号。
func locationRevisionOfStored(stored storedLocation) (domain.LocationRevision, error) {
	fail := func(cell string, err error) (domain.LocationRevision, error) {
		return domain.LocationRevision{}, fmt.Errorf(
			"在册的库位 %q/%q 修订 %d 的%s读不回：%w", stored.warehouse, stored.code, stored.revision, cell, err)
	}
	tenant, err := domain.NewTenantID(stored.tenant)
	if err != nil {
		return fail("租户", err)
	}
	warehouse, err := domain.NewWarehouseCode(stored.warehouse)
	if err != nil {
		return fail("仓编码", err)
	}
	code, err := domain.NewLocationCode(stored.code)
	if err != nil {
		return fail("库位编码", err)
	}
	zone, err := domain.NewZoneCode(stored.zone)
	if err != nil {
		return fail("所属库区", err)
	}
	spec := domain.LocationContentSpec{ZoneCode: zone, Deactivated: stored.deactivated}
	if stored.locationType != nil {
		if spec.LocationType, err = domain.NewLocationTypeCode(*stored.locationType); err != nil {
			return fail("库位类型", err)
		}
	}
	if stored.priority != nil {
		spec.Priority = domain.NewLocationPriority(*stored.priority)
	}
	content, err := domain.NewLocationContent(spec)
	if err != nil {
		return fail("内容", err)
	}
	registrant, err := domain.NewRegistrant(stored.registrant)
	if err != nil {
		return fail("登记责任方", err)
	}
	revision, err := domain.NewLocationRevision(tenant, warehouse, code, stored.revision, content, registrant, stored.registeredAt)
	if err != nil {
		return fail("修订", err)
	}
	return revision, nil
}
