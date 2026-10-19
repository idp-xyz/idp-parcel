package registrationjson

import (
	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// legPricingZoneDocument 是一份段计价区域批文（ADR-0189 决定二）：一条已在册的线路版本，加若干段的区域。今天只有受控
// 批量口登它，租户取批文里的 tenant_id；在线登记口随管理台那一面再开，届时照其余各族加 ForTenant 那一路。
type legPricingZoneDocument struct {
	TenantID string                  `json:"tenant_id"`
	LineCode string                  `json:"line_code"`
	Version  int32                   `json:"version"`
	Zones    []legPricingZonePayload `json:"zones"`
}

type legPricingZonePayload struct {
	SegmentIndex int    `json:"segment_index"`
	Zone         string `json:"zone"`
	Basis        string `json:"basis"`
}

// LegPricingZonesFromJSON 是受控批量口那一路。形状缺失（缺区域、段号重复之类）不在这里判，交登记用例逐格答拒因。
// 依据格与其余各族同走 basisFrom：段计价区域是租户取值，参考配置里没有，直接行写引用串就是冒称一次没发生的采用。
func LegPricingZonesFromJSON(raw []byte) (application.RegisterLegPricingZonesCommand, error) {
	var document legPricingZoneDocument
	if err := decodeStrict(raw, &document); err != nil {
		return application.RegisterLegPricingZonesCommand{}, err
	}
	tenant, err := tenantFromDocument(document.TenantID)
	if err != nil {
		return application.RegisterLegPricingZonesCommand{}, err
	}
	zones := make([]ports.LegPricingZone, 0, len(document.Zones))
	for _, zone := range document.Zones {
		basis, err := basisFrom(&zone.Basis)
		if err != nil {
			return application.RegisterLegPricingZonesCommand{}, err
		}
		zones = append(zones, ports.LegPricingZone{SegmentIndex: zone.SegmentIndex, Zone: zone.Zone, Basis: basis.String()})
	}
	return application.RegisterLegPricingZonesCommand{
		TenantID:    tenant,
		LineCode:    document.LineCode,
		LineVersion: document.Version,
		Zones:       zones,
	}, nil
}
