package registrationjson

import (
	"io"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

type chargeUnitsDocument struct {
	carriedIdentity
	Warehouse *string `json:"warehouse"`
	Weight    *string `json:"weightUnit"`
	Volume    *string `json:"volumeUnit"`
}

// ChargeUnits 把一个仓的重量单位与体积单位译出来。不填默认单位。
func ChargeUnits(reader io.Reader, _ domain.TenantID, _ domain.Registrant) (application.RegisterChargeUnitsCommand, error) {
	var none application.RegisterChargeUnitsCommand
	document, err := decodeDocument[chargeUnitsDocument](reader, "charge units")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	weight, err := requiredText("weightUnit", document.Weight, nonBlank("weightUnit"))
	if err != nil {
		return none, err
	}
	volume, err := requiredText("volumeUnit", document.Volume, nonBlank("volumeUnit"))
	if err != nil {
		return none, err
	}
	return application.RegisterChargeUnitsCommand{Warehouse: warehouse, Weight: weight, Volume: volume}, nil
}

type chargeAgingDocument struct {
	carriedIdentity
	Warehouse *string `json:"warehouse"`
	Basis     *string `json:"basis"`
}

// ChargeAging 把库龄口径译出来。只认实批与理论先进先出。
func ChargeAging(reader io.Reader, _ domain.TenantID, _ domain.Registrant) (application.RegisterChargeAgingCommand, error) {
	var none application.RegisterChargeAgingCommand
	document, err := decodeDocument[chargeAgingDocument](reader, "charge aging")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	basis, err := requiredText("basis", document.Basis, nonBlank("basis"))
	if err != nil {
		return none, err
	}
	if basis != "ACTUAL_BATCH" && basis != "THEORETICAL_FIFO" {
		return none, malformed("basis is not a known aging basis")
	}
	return application.RegisterChargeAgingCommand{Warehouse: warehouse, Basis: basis}, nil
}

type chargeProjectDocument struct {
	carriedIdentity
	Warehouse *string `json:"warehouse"`
	Name      *string `json:"name"`
}

// ChargeProject 把增值服务项目名译出来。
func ChargeProject(reader io.Reader, _ domain.TenantID, _ domain.Registrant) (application.RegisterChargeProjectCommand, error) {
	var none application.RegisterChargeProjectCommand
	document, err := decodeDocument[chargeProjectDocument](reader, "charge project")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	name, err := requiredText("name", document.Name, nonBlank("name"))
	if err != nil {
		return none, err
	}
	return application.RegisterChargeProjectCommand{Warehouse: warehouse, Name: name}, nil
}

type storageDayDocument struct {
	carriedIdentity
	Owner     *string `json:"ownerAccount"`
	Warehouse *string `json:"warehouse"`
	Day       *string `json:"day"`
}

// StorageDay 把一次仓租日结译成货主、仓与计费日。不收计量。
func StorageDay(reader io.Reader, _ domain.TenantID, _ domain.Registrant) (application.CloseStorageDayCommand, error) {
	var none application.CloseStorageDayCommand
	document, err := decodeDocument[storageDayDocument](reader, "storage day")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	owner, err := requiredText("ownerAccount", document.Owner, domain.NewOwnerAccountID)
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	raw, err := requiredText("day", document.Day, nonBlank("day"))
	if err != nil {
		return none, err
	}
	day, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return none, malformed("day must be a calendar date")
	}
	return application.CloseStorageDayCommand{Owner: owner, Warehouse: warehouse, Day: day}, nil
}
