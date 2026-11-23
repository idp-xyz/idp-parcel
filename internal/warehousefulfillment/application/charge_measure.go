package application

import (
	"context"
	"math/big"
	"strconv"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

const (
	chargeClassWeight = "WEIGHT"
	chargeClassVolume = "VOLUME"
	chargeNone        = "无"
)

// 长度单位的立方就是目录里那个体积码，数量是长乘宽乘高，不换到别的体积单位。
var lengthCube = map[string]string{
	"CM": "CM3",
	"M":  "M3",
	"IN": "IN3",
	"FT": "FT3",
}

func countMeasure(unit string, quantity int64) ports.ChargeMeasure {
	return ports.ChargeMeasure{Unit: unit, Quantity: strconv.FormatInt(quantity, 10)}
}

func attributeOrNone(value string) string {
	if value == "" {
		return chargeNone
	}
	return value
}

func attribute(name, value string) ports.ChargeAttribute {
	return ports.ChargeAttribute{Name: name, Value: attributeOrNone(value)}
}

func withAttribute(attributes []ports.ChargeAttribute, name, value string) []ports.ChargeAttribute {
	for index := range attributes {
		if attributes[index].Name == name {
			return attributes
		}
	}
	return append(attributes, ports.ChargeAttribute{Name: name, Value: value})
}

func scaleInt(quantity, factor int64) int64 {
	if factor <= 1 || quantity <= 0 {
		return quantity
	}
	if quantity > (1<<63-1)/factor {
		return quantity
	}
	return quantity * factor
}

func multiplyDecimal(left, right string) (string, bool) {
	a, ok := new(big.Rat).SetString(left)
	if !ok || a.Sign() < 0 {
		return "", false
	}
	b, ok := new(big.Rat).SetString(right)
	if !ok || b.Sign() < 0 {
		return "", false
	}
	return formatRat(new(big.Rat).Mul(a, b)), true
}

func scaleDecimal(value string, quantity int64) (string, bool) {
	if value == "" || quantity <= 0 {
		return "", false
	}
	return multiplyDecimal(value, strconv.FormatInt(quantity, 10))
}

func formatRat(value *big.Rat) string {
	if value.Sign() == 0 {
		return "0"
	}
	if value.IsInt() {
		return value.Num().String()
	}
	numerator := new(big.Int).Set(value.Num())
	denominator := new(big.Int).Set(value.Denom())
	whole := new(big.Int)
	remainder := new(big.Int)
	whole.QuoRem(numerator, denominator, remainder)
	fraction := ""
	for index := 0; index < 12 && remainder.Sign() != 0; index++ {
		remainder.Mul(remainder, big.NewInt(10))
		digit := new(big.Int).Quo(remainder, denominator)
		remainder.Rem(remainder, denominator)
		fraction += digit.String()
	}
	fraction = strings.TrimRight(fraction, "0")
	if fraction == "" {
		return whole.String()
	}
	return whole.String() + "." + fraction
}

func cubeOf(length, width, height, lengthUnit string) (string, string, bool) {
	unit, known := lengthCube[lengthUnit]
	if !known || length == "" || width == "" || height == "" {
		return "", "", false
	}
	area, ok := multiplyDecimal(length, width)
	if !ok {
		return "", "", false
	}
	volume, ok := multiplyDecimal(area, height)
	if !ok {
		return "", "", false
	}
	return volume, unit, true
}

// massMeasure 把一次测量收成重量或体积计量。单位永远是该仓登记的那个词。
// 测到的单位不同就不折算：这一格未测量，原值与原单位另记。没有测量时，有货主声明就附上。
func massMeasure(class, registered, measured, measuredUnit, declared, declaredUnit string, quantity int64) ports.ChargeMeasure {
	measure := ports.ChargeMeasure{Unit: registered, Class: class}
	if measured != "" && measuredUnit != "" {
		scaled, ok := scaleDecimal(measured, quantity)
		if !ok {
			scaled = measured
		}
		if measuredUnit == registered {
			measure.Quantity = scaled
			return measure
		}
		measure.Unmeasured = true
		measure.Original = scaled
		measure.OriginalUnit = measuredUnit
	} else {
		measure.Unmeasured = true
	}
	if declared != "" && declaredUnit != "" {
		if scaled, ok := scaleDecimal(declared, quantity); ok {
			declared = scaled
		}
		measure.Declared = declared
		measure.DeclaredUnit = declaredUnit
	}
	return measure
}

func observedWeight(registered, measured, measuredUnit string) ports.ChargeMeasure {
	return massMeasure(chargeClassWeight, registered, measured, measuredUnit, "", "", 1)
}

func observedVolume(registered, length, width, height, lengthUnit string) ports.ChargeMeasure {
	value, unit, ok := cubeOf(length, width, height, lengthUnit)
	if !ok {
		return massMeasure(chargeClassVolume, registered, "", "", "", "", 1)
	}
	return massMeasure(chargeClassVolume, registered, value, unit, "", "", 1)
}

func (handler *ChargeHandler) CurrentUnits(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode) (string, string, bool, error) {
	return handler.charges.LoadUnits(ctx, tenant, warehouse)
}

func (handler *ChargeHandler) LineMass(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	goods domain.GoodsCode,
	level domain.GoodsUnitName,
	carton domain.CartonNumber,
	quantity int64,
	at time.Time,
) (ports.ChargeMeasure, ports.ChargeMeasure, bool, error) {
	weightUnit, volumeUnit, found, err := handler.charges.LoadUnits(ctx, tenant, warehouse)
	if err != nil || !found {
		return ports.ChargeMeasure{}, ports.ChargeMeasure{}, false, err
	}
	if quantity <= 0 {
		quantity = 1
	}
	weight, volume, err := handler.massAt(ctx, tenant, owner, warehouse, goods, level, carton, quantity, at, weightUnit, volumeUnit)
	if err != nil {
		return ports.ChargeMeasure{}, ports.ChargeMeasure{}, false, err
	}
	return weight, volume, true, nil
}

func (handler *ChargeHandler) volumeAt(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	goods domain.GoodsCode,
	level domain.GoodsUnitName,
	carton domain.CartonNumber,
	quantity int64,
	at time.Time,
	volumeUnit string,
) (ports.ChargeMeasure, error) {
	_, volume, err := handler.massAt(ctx, tenant, owner, warehouse, goods, level, carton, quantity, at, "", volumeUnit)
	return volume, err
}

func (handler *ChargeHandler) massAt(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	goods domain.GoodsCode,
	level domain.GoodsUnitName,
	carton domain.CartonNumber,
	quantity int64,
	at time.Time,
	weightUnit, volumeUnit string,
) (ports.ChargeMeasure, ports.ChargeMeasure, error) {
	var measuredWeight, measuredWeightUnit, length, width, height, dimension string
	var declaredWeight, declaredWeightUnit, declaredLength, declaredWidth, declaredHeight, declaredDimension string
	if handler.measurements != nil {
		versions, err := handler.measurementsOf(ctx, tenant, owner, warehouse, goods, level, carton)
		if err != nil {
			return ports.ChargeMeasure{}, ports.ChargeMeasure{}, err
		}
		if current, found := measurementAsOf(versions, at); found {
			measuredWeight = current.Weight().Value().String()
			measuredWeightUnit = current.Weight().Unit().String()
			dims := current.Dimensions()
			length = dims.Length().String()
			width = dims.Width().String()
			height = dims.Height().String()
			dimension = dims.Unit().String()
		}
	}
	if handler.goods != nil && goods.String() != "" && level.String() != "" {
		when := at
		if when.IsZero() {
			when = handler.clock.Now()
		}
		revision, found, err := handler.goods.LoadGoodsAt(ctx, tenant, owner, goods, when)
		if err != nil {
			return ports.ChargeMeasure{}, ports.ChargeMeasure{}, err
		}
		if found {
			declaredWeight, declaredWeightUnit, declaredLength, declaredWidth, declaredHeight, declaredDimension = declaredMeasurement(revision, level)
		}
	}
	weight := massMeasure(chargeClassWeight, weightUnit, measuredWeight, measuredWeightUnit, declaredWeight, declaredWeightUnit, quantity)
	volumeValue, volumeUnitMeasured, measured := cubeOf(length, width, height, dimension)
	declaredValue, declaredUnit, declared := cubeOf(declaredLength, declaredWidth, declaredHeight, declaredDimension)
	if !measured {
		volumeValue, volumeUnitMeasured = "", ""
	}
	if !declared {
		declaredValue, declaredUnit = "", ""
	}
	volume := massMeasure(chargeClassVolume, volumeUnit, volumeValue, volumeUnitMeasured, declaredValue, declaredUnit, quantity)
	return weight, volume, nil
}

func (handler *ChargeHandler) measurementsOf(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	goods domain.GoodsCode,
	level domain.GoodsUnitName,
	carton domain.CartonNumber,
) ([]domain.WarehouseMeasurement, error) {
	if carton.String() != "" {
		return handler.measurements.ListForCarton(ctx, tenant, owner, warehouse, carton)
	}
	if goods.String() == "" || level.String() == "" {
		return nil, nil
	}
	return handler.measurements.ListForGoodsLevel(ctx, tenant, owner, goods, level)
}

func measurementAsOf(versions []domain.WarehouseMeasurement, at time.Time) (domain.WarehouseMeasurement, bool) {
	var chosen domain.WarehouseMeasurement
	found := false
	for _, version := range versions {
		if !at.IsZero() && version.OccurredAt().After(at) {
			continue
		}
		if !found || domain.CompareWarehouseMeasurementOrder(version, chosen) > 0 {
			chosen = version
			found = true
		}
	}
	return chosen, found
}

func declaredMeasurement(revision domain.GoodsRevision, level domain.GoodsUnitName) (weight, weightUnit, length, width, height, dimension string) {
	for _, measurement := range revision.Content().DeclaredMeasurements() {
		if measurement.Level() != level {
			continue
		}
		if value, ok := measurement.Weight(); ok {
			weight = value.Value().String()
			weightUnit = value.Unit().String()
		}
		if dims, ok := measurement.Dimensions(); ok {
			length = dims.Length().String()
			width = dims.Width().String()
			height = dims.Height().String()
			dimension = dims.Unit().String()
		}
	}
	return weight, weightUnit, length, width, height, dimension
}

func baseQuantity(revision domain.GoodsRevision, unit domain.GoodsUnitName, quantity int64) int64 {
	factor, ok := unitFactor(revision, unit)
	if !ok {
		return quantity
	}
	return scaleInt(quantity, factor)
}
