package postgres

import (
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// packagingMaterialColumns 是包材修订表（0004 迁移）被读出的列，读回领域修订与读面的行共用这一份，列序与 storedPackagingMaterial 的扫描
// 一一对应。四个数值列按文本读出（numeric 的文本是精确的十进制，如 30.000000），再经领域的数值构造门读回规范写法，不经浮点。
const packagingMaterialColumns = `tenant_id, packaging_material_code, revision, packaging_material_name,
	length_value::text, width_value::text, height_value::text, dimensions_unit,
	weight_value::text, weight_unit, deactivated, registrant, registered_at`

// storedPackagingMaterial 是库里一行的原样：文本与数值的文本一个字节都没动。
type storedPackagingMaterial struct {
	tenant         string
	code           string
	revision       int
	name           string
	length         string
	width          string
	height         string
	dimensionsUnit string
	weight         string
	weightUnit     string
	deactivated    bool
	registrant     string
	registeredAt   time.Time
}

func scanStoredPackagingMaterial(scanner rowScanner) (storedPackagingMaterial, error) {
	var stored storedPackagingMaterial
	err := scanner.Scan(
		&stored.tenant, &stored.code, &stored.revision, &stored.name,
		&stored.length, &stored.width, &stored.height, &stored.dimensionsUnit,
		&stored.weight, &stored.weightUnit, &stored.deactivated, &stored.registrant, &stored.registeredAt,
	)
	return stored, err
}

// row 把一行转成读面的行：数值经领域的数值构造门读回规范写法（去掉多余的前导零与尾零），与登记口回显的同一份。时刻统一成 UTC，与领域修订里的
// 登记时刻同形。任何一个数值读不回都是坏数据，上抛不吞。
func (stored storedPackagingMaterial) row() (ports.PackagingMaterialRow, error) {
	revision, err := packagingMaterialRevisionOfStored(stored)
	if err != nil {
		return ports.PackagingMaterialRow{}, err
	}
	content := revision.Content()
	return ports.PackagingMaterialRow{
		PackagingMaterialCode: stored.code,
		Revision:              stored.revision,
		Name:                  stored.name,
		Length:                content.Length().String(),
		Width:                 content.Width().String(),
		Height:                content.Height().String(),
		DimensionsUnit:        stored.dimensionsUnit,
		Weight:                content.Weight().String(),
		WeightUnit:            stored.weightUnit,
		Deactivated:           stored.deactivated,
		Registrant:            stored.registrant,
		RegisteredAt:          stored.registeredAt.UTC(),
	}, nil
}

// packagingMaterialRevisionOfStored 经领域的真构造器重建一笔修订：任何一格过不了门即坏数据，上抛不吞。错误里只带包材编码与修订号。
func packagingMaterialRevisionOfStored(stored storedPackagingMaterial) (domain.PackagingMaterialRevision, error) {
	fail := func(cell string, err error) (domain.PackagingMaterialRevision, error) {
		return domain.PackagingMaterialRevision{}, fmt.Errorf(
			"在册的包材 %q 修订 %d 的%s读不回：%w", stored.code, stored.revision, cell, err)
	}
	tenant, err := domain.NewTenantID(stored.tenant)
	if err != nil {
		return fail("租户", err)
	}
	code, err := domain.NewPackagingMaterialCode(stored.code)
	if err != nil {
		return fail("包材编码", err)
	}
	name, err := domain.NewPackagingMaterialName(stored.name)
	if err != nil {
		return fail("名称", err)
	}
	length, err := domain.NewMeasure(stored.length)
	if err != nil {
		return fail("长", err)
	}
	width, err := domain.NewMeasure(stored.width)
	if err != nil {
		return fail("宽", err)
	}
	height, err := domain.NewMeasure(stored.height)
	if err != nil {
		return fail("高", err)
	}
	dimensionsUnit, err := domain.NewUnitCode(stored.dimensionsUnit)
	if err != nil {
		return fail("尺寸单位", err)
	}
	weight, err := domain.NewMeasure(stored.weight)
	if err != nil {
		return fail("重量", err)
	}
	weightUnit, err := domain.NewUnitCode(stored.weightUnit)
	if err != nil {
		return fail("重量单位", err)
	}
	content, err := domain.NewPackagingMaterialContent(domain.PackagingMaterialContentSpec{
		Name:           name,
		Length:         length,
		Width:          width,
		Height:         height,
		DimensionsUnit: dimensionsUnit,
		Weight:         weight,
		WeightUnit:     weightUnit,
		Deactivated:    stored.deactivated,
	})
	if err != nil {
		return fail("内容", err)
	}
	registrant, err := domain.NewRegistrant(stored.registrant)
	if err != nil {
		return fail("登记责任方", err)
	}
	revision, err := domain.NewPackagingMaterialRevision(tenant, code, stored.revision, content, registrant, stored.registeredAt)
	if err != nil {
		return fail("修订", err)
	}
	return revision, nil
}
