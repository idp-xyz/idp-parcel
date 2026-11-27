package application

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// packSerials 按打包时的当前商品修订收序列号。
// 跟踪的货：号数等于件数，这一张出库单里不重复。不跟踪的货不收号。
// 商品不在册时不能要求号，带了号也不收。不跟别的出库单比，也不问收货。
func (handler *WaveHandler) packSerials(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, parcels []PackParcelCommand) ([][]string, bool, error) {
	known := map[string]struct {
		tracked bool
		found   bool
	}{}
	return normalizePackSerials(parcels, func(goods domain.GoodsCode) (bool, bool, error) {
		if item, ok := known[goods.String()]; ok {
			return item.tracked, item.found, nil
		}
		tracked, found, err := handler.serialTracked(ctx, tenant, owner, goods)
		if err != nil {
			return false, false, err
		}
		known[goods.String()] = struct {
			tracked bool
			found   bool
		}{tracked, found}
		return tracked, found, nil
	})
}

func normalizePackSerials(parcels []PackParcelCommand, track func(domain.GoodsCode) (bool, bool, error)) ([][]string, bool, error) {
	seen := map[string]struct{}{}
	out := make([][]string, len(parcels))
	for index, parcel := range parcels {
		tracked, found, err := track(parcel.Goods)
		if err != nil {
			return nil, false, err
		}
		serials := make([]string, 0, len(parcel.Serials))
		for _, raw := range parcel.Serials {
			serial, ok := domain.CanonicalSerial(raw)
			if !ok {
				return nil, false, nil
			}
			if _, dup := seen[serial]; dup {
				return nil, false, nil
			}
			seen[serial] = struct{}{}
			serials = append(serials, serial)
		}
		switch {
		case !found || !tracked:
			if len(serials) != 0 {
				return nil, false, nil
			}
		case int64(len(serials)) != parcel.Quantity:
			return nil, false, nil
		}
		out[index] = serials
	}
	return out, true, nil
}

func (handler *WaveHandler) serialTracked(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, goods domain.GoodsCode) (bool, bool, error) {
	revision, found, err := handler.goods.LoadLatestGoods(ctx, tenant, owner, goods)
	if err != nil || !found {
		return false, found, err
	}
	return revision.Content().TrackSerial(), true, nil
}

func storedParcels(parcels []PackParcelCommand, serials [][]string) []ports.PackParcel {
	stored := make([]ports.PackParcel, 0, len(parcels))
	for index, parcel := range parcels {
		stored = append(stored, ports.PackParcel{
			Sequence: parcel.Sequence, Packaging: parcel.Packaging, Weight: parcel.Weight, WeightUnit: parcel.WeightUnit,
			Length: parcel.Length, Width: parcel.Width, Height: parcel.Height, Dimension: parcel.Dimension,
			Goods: parcel.Goods, Unit: parcel.Unit, Quantity: parcel.Quantity, Serials: serials[index],
		})
	}
	return stored
}
