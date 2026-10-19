package parcelpricing

import (
	"context"
	"errors"
	"fmt"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// LegPricingZoneRead 读一条线路版本已登的段计价区域（本上下文自己的登记册，ADR-0189 决定二）。
type LegPricingZoneRead interface {
	LoadLegPricingZones(
		ctx context.Context,
		tenant nrdomain.TenantID,
		lineCode string,
		version int32,
	) ([]nrports.LegPricingZone, error)
}

// RegisteredLegZones 是 LegZoneSource 的段计价区域那一路：按候选所在的线路版本与段号读租户登的区，原样作为调用方给
// 的分区交给 parcel-pricing，本上下文不解释也不校它在不在卡上。没登即没有这一格：没绑分区目录的卡这一段待判断。
//
// 绑了分区目录的卡不靠它：那一路的区由 parcel-pricing 用包裹的邮编路线经目录读出（ADR-0189 决定一），登了区也不读。
type RegisteredLegZones struct {
	zones LegPricingZoneRead
}

var _ LegZoneSource = (*RegisteredLegZones)(nil)

func NewRegisteredLegZones(zones LegPricingZoneRead) (*RegisteredLegZones, error) {
	if zones == nil {
		return nil, errors.New("network routing parcel pricing adapter: registered leg zones need the leg pricing zone register")
	}
	return &RegisteredLegZones{zones: zones}, nil
}

func (source *RegisteredLegZones) ZoneFor(ctx context.Context, query LegZoneQuery) (LegZone, bool, error) {
	code, version, ok := query.Candidate.LineVersion()
	if !ok {
		return LegZone{}, false, fmt.Errorf("%w: candidate %q carries no line reference", errUntranslatableEvaluation, query.Candidate)
	}
	zones, err := source.zones.LoadLegPricingZones(ctx, query.Key.TenantID, code, version)
	if err != nil {
		return LegZone{}, false, fmt.Errorf("load leg pricing zones for %s: %w", code, err)
	}
	// 段号从 1 起，登记册按段链下标存。
	for _, zone := range zones {
		if zone.SegmentIndex == query.Ordinal-1 {
			return LegZone{Zone: zone.Zone}, true, nil
		}
	}
	return LegZone{}, false, nil
}
