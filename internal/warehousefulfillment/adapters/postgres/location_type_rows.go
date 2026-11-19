package postgres

import (
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// locationTypeColumns 是库位类型修订表（0003 迁移）被读出的列，读回领域修订与读面的行共用这一份，列序与 storedLocationType 的扫描
// 一一对应。
const locationTypeColumns = `tenant_id, location_type_code, revision, location_type_name, deactivated, registrant, registered_at`

// storedLocationType 是库里一行的原样：文本一个字节都没动。
type storedLocationType struct {
	tenant       string
	code         string
	revision     int
	name         string
	deactivated  bool
	registrant   string
	registeredAt time.Time
}

func scanStoredLocationType(scanner rowScanner) (storedLocationType, error) {
	var stored storedLocationType
	err := scanner.Scan(
		&stored.tenant, &stored.code, &stored.revision, &stored.name, &stored.deactivated, &stored.registrant, &stored.registeredAt,
	)
	return stored, err
}

// row 把一行转成读面的行。时刻统一成 UTC，与领域修订里的登记时刻同形。
func (stored storedLocationType) row() ports.LocationTypeRow {
	return ports.LocationTypeRow{
		LocationTypeCode: stored.code,
		Revision:         stored.revision,
		Name:             stored.name,
		Deactivated:      stored.deactivated,
		Registrant:       stored.registrant,
		RegisteredAt:     stored.registeredAt.UTC(),
	}
}

// locationTypeRevisionOfStored 经领域的真构造器重建一笔修订：任何一格过不了门即坏数据，上抛不吞。错误里只带库位类型编码与修订号。
func locationTypeRevisionOfStored(stored storedLocationType) (domain.LocationTypeRevision, error) {
	fail := func(cell string, err error) (domain.LocationTypeRevision, error) {
		return domain.LocationTypeRevision{}, fmt.Errorf(
			"在册的库位类型 %q 修订 %d 的%s读不回：%w", stored.code, stored.revision, cell, err)
	}
	tenant, err := domain.NewTenantID(stored.tenant)
	if err != nil {
		return fail("租户", err)
	}
	code, err := domain.NewLocationTypeCode(stored.code)
	if err != nil {
		return fail("库位类型编码", err)
	}
	name, err := domain.NewLocationTypeName(stored.name)
	if err != nil {
		return fail("名称", err)
	}
	content, err := domain.NewLocationTypeContent(domain.LocationTypeContentSpec{Name: name, Deactivated: stored.deactivated})
	if err != nil {
		return fail("内容", err)
	}
	registrant, err := domain.NewRegistrant(stored.registrant)
	if err != nil {
		return fail("登记责任方", err)
	}
	revision, err := domain.NewLocationTypeRevision(tenant, code, stored.revision, content, registrant, stored.registeredAt)
	if err != nil {
		return fail("修订", err)
	}
	return revision, nil
}
