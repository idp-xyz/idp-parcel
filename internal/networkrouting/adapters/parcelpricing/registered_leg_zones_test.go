package parcelpricing_test

import (
	"context"
	"errors"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelpricing"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

type legZoneRegister struct {
	zones map[string][]nrports.LegPricingZone
	err   error
	asked []string
}

func (register *legZoneRegister) LoadLegPricingZones(
	_ context.Context, tenant nrdomain.TenantID, lineCode string, version int32,
) ([]nrports.LegPricingZone, error) {
	register.asked = append(register.asked, tenant.String()+"|"+lineCode)
	return register.zones[lineCode], register.err
}

// Covers: ADR-0189 决定二——逐段区域按候选所在的线路版本与段号（从 1 起，册上按段链下标）读租户登的区，原样交出；
// 没登的段答未配置，不编区；候选不带线路引用与登记册读不动都响亮。
func TestRegisteredLegZonesHandTheTenantsZoneThroughVerbatim(t *testing.T) {
	register := &legZoneRegister{zones: map[string][]nrports.LegPricingZone{
		"SYN-LINE-1": {{SegmentIndex: 0, Zone: "Z1", Basis: "b"}, {SegmentIndex: 1, Zone: "Z-LANE-7", Basis: "b"}},
	}}
	source, err := adapter.NewRegisteredLegZones(register)
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	candidate, err := nrdomain.NewLineCandidateID("SYN-LINE-1", 1)
	if err != nil {
		t.Fatalf("candidate: %v", err)
	}
	query := adapter.LegZoneQuery{Key: judgmentKey(t), Candidate: candidate, Ordinal: 2}
	zone, configured, err := source.ZoneFor(t.Context(), query)
	if err != nil || !configured || zone.Zone != "Z-LANE-7" || zone.HasPostal {
		t.Fatalf("段 2 = %+v configured=%v err=%v", zone, configured, err)
	}
	if register.asked[0] != "tenant-1|SYN-LINE-1" {
		t.Fatalf("asked %v", register.asked)
	}

	query.Ordinal = 3
	if zone, configured, err := source.ZoneFor(t.Context(), query); err != nil || configured || zone.Zone != "" {
		t.Fatalf("没登的段 = %+v configured=%v err=%v", zone, configured, err)
	}

	bare, err := nrdomain.NewCandidateID("not-a-line-candidate")
	if err != nil {
		t.Fatalf("candidate: %v", err)
	}
	if _, _, err := source.ZoneFor(t.Context(), adapter.LegZoneQuery{Key: judgmentKey(t), Candidate: bare, Ordinal: 1}); err == nil {
		t.Fatal("不带线路引用的候选该响亮")
	}

	boom := errors.New("register is down")
	broken, _ := adapter.NewRegisteredLegZones(&legZoneRegister{err: boom})
	if _, _, err := broken.ZoneFor(t.Context(), adapter.LegZoneQuery{Key: judgmentKey(t), Candidate: candidate, Ordinal: 1}); !errors.Is(err, boom) {
		t.Fatalf("读不动该原样上抛：%v", err)
	}
	if _, err := adapter.NewRegisteredLegZones(nil); err == nil {
		t.Fatal("缺登记册该在构造时拒")
	}
}
