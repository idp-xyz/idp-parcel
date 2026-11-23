package application

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

type storageCell struct {
	location  string
	goods     string
	unit      string
	state     string
	batch     string
	form      string
	carton    string
	quantity  int64
	inboundAt time.Time
}

func (handler *ChargeHandler) storageLines(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	dayStart, dayEnd time.Time,
	location *time.Location,
	volumeUnit string,
) ([]ports.ChargeLine, error) {
	deltas, err := handler.charges.ListStorageDeltas(ctx, tenant, owner, warehouse, dayEnd)
	if err != nil {
		return nil, fmt.Errorf("close storage day: %w", err)
	}
	freezes, err := handler.charges.ListStorageFreezes(ctx, tenant, owner, warehouse, dayEnd)
	if err != nil {
		return nil, fmt.Errorf("close storage day: %w", err)
	}
	cells := balancesAt(deltas, dayEnd)
	places := map[string]storagePlace{}
	lines := make([]ports.ChargeLine, 0, len(cells))
	type batchKey struct {
		batch, state, form, usage, frozen, goods, unit string
	}
	type batchGroup struct {
		key       batchKey
		quantity  int64
		inboundAt time.Time
		carton    string
	}
	groups := map[batchKey]*batchGroup{}
	var order []batchKey
	for _, cell := range cells {
		place, err := handler.placeAt(ctx, tenant, warehouse, cell.location, dayEnd, places)
		if err != nil {
			return nil, err
		}
		frozenQty := frozenQuantity(freezes, cell)
		parts := []struct {
			quantity int64
			frozen   string
		}{{quantity: cell.quantity, frozen: chargeNone}}
		if frozenQty > 0 && frozenQty < cell.quantity {
			parts = []struct {
				quantity int64
				frozen   string
			}{{quantity: frozenQty, frozen: "有"}, {quantity: cell.quantity - frozenQty, frozen: chargeNone}}
		} else if frozenQty >= cell.quantity {
			parts = []struct {
				quantity int64
				frozen   string
			}{{quantity: cell.quantity, frozen: "有"}}
		}
		for _, part := range parts {
			key := batchKey{
				batch: cell.batch, state: attributeOrNone(cell.state), form: cell.form,
				usage: place.usage, frozen: part.frozen, goods: cell.goods, unit: cell.unit,
			}
			group := groups[key]
			if group == nil {
				group = &batchGroup{key: key, inboundAt: cell.inboundAt, carton: cell.carton}
				groups[key] = group
				order = append(order, key)
			}
			group.quantity += part.quantity
		}
	}
	for _, key := range order {
		group := groups[key]
		pieces := group.quantity
		if handler.goods != nil && group.key.goods != "" && group.key.unit != "" {
			goods, err := domain.NewGoodsCode(group.key.goods)
			if err != nil {
				return nil, err
			}
			unit, err := domain.NewGoodsUnitName(group.key.unit)
			if err != nil {
				return nil, err
			}
			revision, found, err := handler.goods.LoadGoodsAt(ctx, tenant, owner, goods, dayEnd)
			if err != nil {
				return nil, fmt.Errorf("close storage day: %w", err)
			}
			if found {
				pieces = baseQuantity(revision, unit, group.quantity)
			}
		}
		volume, err := handler.cellVolume(ctx, tenant, owner, warehouse, group.key.goods, group.key.unit, group.carton, group.quantity, volumeUnit, dayEnd)
		if err != nil {
			return nil, err
		}
		lines = append(lines, ports.ChargeLine{
			Kind: "INBOUND_BATCH",
			Measures: []ports.ChargeMeasure{
				countMeasure("PIECE", pieces),
				volume,
				countMeasure("DAY", int64(inclusiveDays(group.inboundAt, dayStart, location))),
			},
			Attributes: []ports.ChargeAttribute{
				attribute("STOCK_STATE", group.key.state),
				attribute("STOCK_FORM", group.key.form),
				attribute("ZONE_USAGE", group.key.usage),
				attribute("FROZEN", group.key.frozen),
			},
		})
	}
	occupied := locationsAt(deltas, dayEnd)
	locations := make([]string, 0, len(occupied))
	for code := range occupied {
		locations = append(locations, code)
	}
	sort.Strings(locations)
	for _, code := range locations {
		place, err := handler.placeAt(ctx, tenant, warehouse, code, dayEnd, places)
		if err != nil {
			return nil, err
		}
		days := 1
		earlier := dayEnd.Add(-24 * time.Hour)
		for locationsAt(deltas, earlier)[code] {
			days++
			earlier = earlier.Add(-24 * time.Hour)
			if days > 10000 {
				break
			}
		}
		lines = append(lines, ports.ChargeLine{
			Kind: "LOCATION",
			Measures: []ports.ChargeMeasure{
				countMeasure("LOCATION_DAY", int64(days)),
			},
			Attributes: []ports.ChargeAttribute{
				attribute("LOCATION_TYPE", place.locationType),
			},
		})
	}
	return lines, nil
}

func (handler *ChargeHandler) cellVolume(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	goodsCode, unitName, cartonNumber string,
	quantity int64,
	volumeUnit string,
	at time.Time,
) (ports.ChargeMeasure, error) {
	goods, _ := domain.NewGoodsCode(goodsCode)
	level, _ := domain.NewGoodsUnitName(unitName)
	carton, _ := domain.NewCartonNumber(cartonNumber)
	if goods.String() == "" && carton.String() == "" {
		return massMeasure(chargeClassVolume, volumeUnit, "", "", "", "", 1), nil
	}
	return handler.volumeAt(ctx, tenant, owner, warehouse, goods, level, carton, quantity, at, volumeUnit)
}

type storagePlace struct {
	usage        string
	locationType string
}

func (handler *ChargeHandler) placeAt(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	location string,
	at time.Time,
	cache map[string]storagePlace,
) (storagePlace, error) {
	if place, found := cache[location]; found {
		return place, nil
	}
	usage, locationType, _, err := handler.charges.PlaceAt(ctx, tenant, warehouse, location, at)
	if err != nil {
		return storagePlace{}, fmt.Errorf("close storage day: %w", err)
	}
	place := storagePlace{usage: attributeOrNone(usage), locationType: attributeOrNone(locationType)}
	cache[location] = place
	return place, nil
}

func balancesAt(deltas []ports.StorageDelta, at time.Time) []storageCell {
	type key struct{ location, goods, unit, state, batch, form, carton string }
	totals := map[key]*storageCell{}
	var order []key
	for _, delta := range deltas {
		if delta.Posted.After(at) {
			continue
		}
		id := key{
			location: delta.Location, goods: delta.Goods, unit: delta.Unit, state: delta.State,
			batch: delta.Batch, form: delta.Form, carton: delta.Carton,
		}
		cell := totals[id]
		if cell == nil {
			cell = &storageCell{
				location: delta.Location, goods: delta.Goods, unit: delta.Unit, state: delta.State,
				batch: delta.Batch, form: delta.Form, carton: delta.Carton, inboundAt: delta.InboundAt,
			}
			totals[id] = cell
			order = append(order, id)
		}
		cell.quantity += delta.Delta
		if !delta.InboundAt.IsZero() {
			cell.inboundAt = delta.InboundAt
		}
	}
	listed := make([]storageCell, 0, len(order))
	for _, id := range order {
		cell := totals[id]
		if cell.quantity > 0 {
			listed = append(listed, *cell)
		}
	}
	return listed
}

func locationsAt(deltas []ports.StorageDelta, at time.Time) map[string]bool {
	occupied := map[string]bool{}
	for _, cell := range balancesAt(deltas, at) {
		occupied[cell.location] = true
	}
	return occupied
}

func frozenQuantity(freezes []ports.StorageFreeze, cell storageCell) int64 {
	var total int64
	for _, freeze := range freezes {
		if cell.form == "CARTON" {
			if freeze.Target == "CARTON" && freeze.Carton == cell.carton {
				total += freeze.Quantity
			}
			continue
		}
		if freeze.Target == "GOODS" && freeze.Location == cell.location && freeze.Goods == cell.goods &&
			freeze.Unit == cell.unit && freeze.State == cell.state && freeze.Batch == cell.batch {
			total += freeze.Quantity
		}
	}
	if total < 0 {
		return 0
	}
	return total
}

func inclusiveDays(inbound, billingDay time.Time, location *time.Location) int {
	from := inbound.In(location)
	to := billingDay.In(location)
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	days := int(end.Sub(start).Hours()/24) + 1
	if days < 1 {
		return 1
	}
	return days
}
