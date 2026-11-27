package application

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 预取用的包材。没选、没登记或停用，这一份就不预取，放波次本身仍然成立。
type prefetchPackaging interface {
	LoadLatestPackagingMaterial(context.Context, domain.TenantID, domain.PackagingMaterialCode) (domain.PackagingMaterialRevision, bool, error)
}

type wavePrefetch struct {
	commissioner ports.OutboundCommissioner
	measurements ports.WarehouseMeasurementRead
	packaging    prefetchPackaging
}

// WithPrefetch 接上放波次时的估算委托。没接、没选包材、仓测不齐或代办没登记，都不预取，打包后再交。
func (handler *WaveHandler) WithPrefetch(
	commissioner ports.OutboundCommissioner,
	measurements ports.WarehouseMeasurementRead,
	packaging prefetchPackaging,
) *WaveHandler {
	if commissioner == nil || measurements == nil || packaging == nil {
		return handler
	}
	handler.prefetch = &wavePrefetch{commissioner: commissioner, measurements: measurements, packaging: packaging}
	return handler
}

func (handler *WaveHandler) prefetchRequests(ctx context.Context, command ReleaseWaveCommand, tasks []ports.PickTask) error {
	if handler.prefetch == nil || !command.Prefetch || command.Packaging.String() == "" {
		return nil
	}
	material, found, err := handler.prefetch.packaging.LoadLatestPackagingMaterial(ctx, command.Tenant, command.Packaging)
	if err != nil {
		return fmt.Errorf("release wave: %w", err)
	}
	if !found || material.Content().Deactivated() {
		return nil
	}
	byRequest := map[string][]ports.PickTask{}
	for _, task := range tasks {
		byRequest[task.Request.String()] = append(byRequest[task.Request.String()], task)
	}
	for _, id := range command.Requests {
		request, found, err := handler.requests.LoadByID(ctx, command.Tenant, id)
		if err != nil {
			return fmt.Errorf("release wave: %w", err)
		}
		if !found || request.Dispatch() != domain.DispatchProductNetwork || request.PlatformStocking() {
			continue
		}
		// 撤单或少拣之后再放进波次的请求，上一次放出时已经预取过：估算委托只交一份，记下的那一份照旧用。
		if _, had, err := handler.waves.LoadPrefetch(ctx, command.Tenant, id); err != nil {
			return fmt.Errorf("release wave: %w", err)
		} else if had {
			continue
		}
		parcel, ok, err := prefetchParcel(ctx, handler.prefetch.measurements, command.Tenant, request, material.Content(), byRequest[id.String()])
		if err != nil {
			return fmt.Errorf("release wave: %w", err)
		}
		if !ok {
			continue
		}
		answer, err := handler.prefetch.commissioner.Submit(ctx, ports.CommissionSubmission{
			Tenant: command.Tenant.String(), Owner: request.Owner().String(), Request: id.String(), Sequence: 1,
			Service: request.Service(), Recipient: request.Recipient(), Address: request.Address(), Parcel: parcel,
		})
		if err != nil {
			return fmt.Errorf("release wave: %w", err)
		}
		if answer.Outcome != ports.CommissionSubmittedWord && answer.Outcome != ports.CommissionExistingWord {
			continue
		}
		if answer.ShipmentRequestID == "" {
			return fmt.Errorf("release wave: prefetch named no shipment request")
		}
		if err := handler.waves.SavePrefetch(ctx, command.Tenant, id, ports.PrefetchRecord{
			ShipmentID: answer.ShipmentRequestID, DeclaredParcelID: answer.DeclaredParcelID,
			Weight: parcel.Weight, WeightUnit: parcel.WeightUnit,
			Length: parcel.Length, Width: parcel.Width, Height: parcel.Height, Dimension: parcel.Dimension,
			Quantity: parcel.Quantity,
		}); err != nil {
			return fmt.Errorf("release wave: %w", err)
		}
	}
	return nil
}

// prefetchParcel 用所分配商品的仓测重量合计，加上包材重量，尺寸取包材外廓。单位对不上、没测过或不止一种商品，就不预取。
func prefetchParcel(
	ctx context.Context,
	measurements ports.WarehouseMeasurementRead,
	tenant domain.TenantID,
	request domain.OutboundRequest,
	packaging domain.PackagingMaterialContent,
	tasks []ports.PickTask,
) (ports.CommissionParcel, bool, error) {
	var none ports.CommissionParcel
	if len(tasks) == 0 {
		return none, false, nil
	}
	goods := tasks[0].Goods
	unit := tasks[0].Unit
	var quantity int64
	for _, task := range tasks {
		if task.Goods != goods || task.Unit != unit || task.Quantity < 1 {
			return none, false, nil
		}
		quantity += task.Quantity
	}
	versions, err := measurements.ListForGoodsLevel(ctx, tenant, request.Owner(), goods, unit)
	if err != nil {
		return none, false, err
	}
	if len(versions) == 0 {
		return none, false, nil
	}
	latest := versions[0]
	for _, version := range versions[1:] {
		if domain.CompareWarehouseMeasurementOrder(version, latest) > 0 {
			latest = version
		}
	}
	if latest.Weight().Unit().String() != packaging.WeightUnit().String() {
		return none, false, nil
	}
	goodsWeight, ok := scaleMeasure(latest.Weight().Value(), quantity)
	if !ok {
		return none, false, nil
	}
	total, ok := addMeasures(goodsWeight, packaging.Weight())
	if !ok {
		return none, false, nil
	}
	return ports.CommissionParcel{
		Sequence: 1, Weight: total.String(), WeightUnit: packaging.WeightUnit().String(),
		Length: packaging.Length().String(), Width: packaging.Width().String(), Height: packaging.Height().String(),
		Dimension: packaging.DimensionsUnit().String(), Goods: goods.String(), Unit: unit.String(), Quantity: quantity,
	}, true, nil
}

func scaleMeasure(measure domain.Measure, factor int64) (domain.Measure, bool) {
	if measure.String() == "" || factor < 1 {
		return domain.Measure{}, false
	}
	digits, scale, ok := measureDigits(measure)
	if !ok {
		return domain.Measure{}, false
	}
	coefficient := new(big.Int)
	if _, ok := coefficient.SetString(digits, 10); !ok {
		return domain.Measure{}, false
	}
	coefficient.Mul(coefficient, big.NewInt(factor))
	return measureFromDigits(coefficient, scale)
}

func addMeasures(left, right domain.Measure) (domain.Measure, bool) {
	if left.String() == "" || right.String() == "" {
		return domain.Measure{}, false
	}
	leftDigits, leftScale, ok := measureDigits(left)
	if !ok {
		return domain.Measure{}, false
	}
	rightDigits, rightScale, ok := measureDigits(right)
	if !ok {
		return domain.Measure{}, false
	}
	scale := leftScale
	if rightScale > scale {
		scale = rightScale
	}
	sum := new(big.Int).Add(scaledDigits(leftDigits, leftScale, scale), scaledDigits(rightDigits, rightScale, scale))
	return measureFromDigits(sum, scale)
}

func measureDigits(measure domain.Measure) (string, int, bool) {
	text := measure.String()
	if text == "" {
		return "", 0, false
	}
	whole, fraction, dotted := strings.Cut(text, ".")
	if !dotted {
		return whole, 0, true
	}
	return strings.TrimLeft(whole+fraction, "0"), len(fraction), true
}

func scaledDigits(digits string, from, to int) *big.Int {
	if digits == "" {
		digits = "0"
	}
	coefficient := new(big.Int)
	coefficient.SetString(digits, 10)
	for index := from; index < to; index++ {
		coefficient.Mul(coefficient, big.NewInt(10))
	}
	return coefficient
}

func measureFromDigits(coefficient *big.Int, scale int) (domain.Measure, bool) {
	text := coefficient.String()
	if scale > 0 {
		for len(text) <= scale {
			text = "0" + text
		}
		text = text[:len(text)-scale] + "." + text[len(text)-scale:]
	}
	measure, err := domain.NewMeasure(text)
	if err != nil {
		return domain.Measure{}, false
	}
	return measure, true
}
