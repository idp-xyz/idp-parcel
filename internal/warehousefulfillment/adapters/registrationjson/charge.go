package registrationjson

import (
	"io"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
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

type chargeCorrectionDocument struct {
	carriedIdentity
	Occurrence *string                 `json:"occurrenceId"`
	Version    *int                    `json:"version"`
	Measures   []chargeMeasureDocument `json:"measures"`
}

type chargeMeasureDocument struct {
	Unit       *string `json:"unit"`
	Quantity   *string `json:"quantity"`
	Unmeasured *bool   `json:"unmeasured"`
}

// ChargeVoid 点名要作废的那一版。作废不收计量。
func ChargeVoid(reader io.Reader, tenant domain.TenantID, registrant domain.Registrant) (application.ChargeCorrectionCommand, error) {
	command, err := chargeCorrection(reader, tenant, registrant, "charge void")
	if err != nil {
		return application.ChargeCorrectionCommand{}, err
	}
	if len(command.Draft.Measures) > 0 {
		return application.ChargeCorrectionCommand{}, malformed("void does not carry measures")
	}
	return command, nil
}

// ChargeRestate 点名要重述的那一版，并带上整份计量。
func ChargeRestate(reader io.Reader, tenant domain.TenantID, registrant domain.Registrant) (application.ChargeCorrectionCommand, error) {
	return chargeCorrection(reader, tenant, registrant, "charge restate")
}

func chargeCorrection(reader io.Reader, _ domain.TenantID, _ domain.Registrant, name string) (application.ChargeCorrectionCommand, error) {
	var none application.ChargeCorrectionCommand
	document, err := decodeDocument[chargeCorrectionDocument](reader, name)
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	occurrence, err := requiredText("occurrenceId", document.Occurrence, nonBlank("occurrenceId"))
	if err != nil {
		return none, err
	}
	if document.Version == nil || *document.Version < 1 {
		return none, malformed("version is required")
	}
	measures := make([]ports.ChargeMeasure, 0, len(document.Measures))
	for _, measure := range document.Measures {
		unit, err := requiredText("unit", measure.Unit, nonBlank("unit"))
		if err != nil {
			return none, err
		}
		item := ports.ChargeMeasure{Unit: unit}
		if measure.Unmeasured != nil && *measure.Unmeasured {
			item.Unmeasured = true
		} else if measure.Quantity == nil || *measure.Quantity == "" {
			return none, malformed("quantity is required")
		} else {
			item.Quantity = *measure.Quantity
		}
		measures = append(measures, item)
	}
	return application.ChargeCorrectionCommand{Occurrence: occurrence, Version: *document.Version, Draft: application.ChargeDraft{Measures: measures}}, nil
}
