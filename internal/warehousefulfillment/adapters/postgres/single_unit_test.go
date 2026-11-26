package postgres_test

import (
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func (world receiptWorld) listedOutbound(t *testing.T, raw string) map[string]ports.OutboundRequestRow {
	t.Helper()
	requests, err := adapter.NewOutboundRequests(world.db)
	if err != nil {
		t.Fatal(err)
	}
	query, err := ports.OutboundRequestPages.DecodeRaw(raw)
	if err != nil {
		t.Fatalf("出库列表查询 %q：%v", raw, err)
	}
	page, err := requests.List(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), 50, query)
	if err != nil {
		t.Fatalf("列出库请求：%v", err)
	}
	rows := map[string]ports.OutboundRequestRow{}
	for _, row := range page.Rows {
		rows[row.RequestID] = row
	}
	return rows
}

func (world receiptWorld) listedWaves(t *testing.T, raw string) map[string]ports.WaveRow {
	t.Helper()
	store, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	query, err := ports.WavePages.DecodeRaw(raw)
	if err != nil {
		t.Fatalf("波次列表查询 %q：%v", raw, err)
	}
	page, err := store.ListWaves(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), 50, query)
	if err != nil {
		t.Fatalf("列波次：%v", err)
	}
	rows := map[string]ports.WaveRow{}
	for _, row := range page.Rows {
		rows[row.WaveID] = row
	}
	return rows
}

// Covers: 「单品单件」是要拣的只有一件的出库请求：预占需求合计是 1 个基本单位。一行两件的不是；
// 一套按组件展开成两件的套装也不是——按展开之后的件数算，不按下单的行数算。
// 波次里每一张都是单品单件，这个波次才是单品单件波次；混进一张别的就不是。两个列表都能按它筛。
func TestSingleUnitRequestsAndWavesAreToldApartByWhatHasToBePicked(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-KIT-01", 1, "REQ-KIT-SINGLE", func(spec *domain.GoodsContentSpec) {
		spec.Name = valueOf(t, domain.NewGoodsName, "合成套装")
		spec.Aliases = nil
		spec.Composite = true
		spec.OutboundMode = domain.GoodsOutboundByComponent
		spec.Components = []domain.GoodsComponent{domain.NewGoodsComponent(valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), 2)}
	}))
	one, another, two := reserve("REQ-SINGLE-A", 1), reserve("REQ-SINGLE-B", 1), reserve("REQ-DOUBLE", 2)
	command := outboundCommand(t, "REQ-ONE-KIT", 1)
	command.Lines = []domain.OutboundLine{mustOutboundLine(t, "SYN-KIT-01", 1)}
	accepted := world.acceptOutbound(t, world.outbound(t), command)
	kit, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok {
		t.Fatalf("套装受理 %s", accepted.Outcome())
	}

	single := world.listedOutbound(t, "singleUnit=true")
	if len(single) != 2 || single[one.String()].WantedQuantity != 1 || single[another.String()].RequestID == "" {
		t.Fatalf("单品单件的请求 %+v", single)
	}
	others := world.listedOutbound(t, "singleUnit=false")
	if len(others) != 2 || others[two.String()].WantedQuantity != 2 || others[kit.ID().String()].WantedQuantity != 2 {
		t.Fatalf("不是单品单件的请求 %+v", others)
	}

	if world.releaseRequests(t, handler, "WAVE-SINGLE", one, another).Outcome() != application.WaveAllocated ||
		world.releaseRequests(t, handler, "WAVE-MIXED", two, kit.ID()).Outcome() != application.WaveAllocated {
		t.Fatal("两个波次没有放出")
	}
	if !world.waveRow(t, "WAVE-SINGLE").SingleUnit || world.waveRow(t, "WAVE-MIXED").SingleUnit {
		t.Fatalf("单品单件波次 %v，混合波次 %v", world.waveRow(t, "WAVE-SINGLE").SingleUnit, world.waveRow(t, "WAVE-MIXED").SingleUnit)
	}
	if waves := world.listedWaves(t, "singleUnit=true"); len(waves) != 1 || !waves["WAVE-SINGLE"].SingleUnit {
		t.Fatalf("按单品单件筛波次 %+v", waves)
	}
	if waves := world.listedWaves(t, "singleUnit=false"); len(waves) != 1 || waves["WAVE-MIXED"].WaveID == "" {
		t.Fatalf("筛不是单品单件的波次 %+v", waves)
	}
}

// Covers: 一个单品单件的请求与一个两件的请求放进同一个波次，这个波次不是单品单件波次。
func TestAWaveWithOneLargerRequestIsNotASingleUnitWave(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	one, two := reserve("REQ-SINGLE-MIX", 1), reserve("REQ-DOUBLE-MIX", 2)
	if world.releaseRequests(t, handler, "WAVE-ONE-BIGGER", one, two).Outcome() != application.WaveAllocated {
		t.Fatal("波次没有放出")
	}
	if world.waveRow(t, "WAVE-ONE-BIGGER").SingleUnit {
		t.Fatal("混着一张两件的请求，仍被当成单品单件波次")
	}
}
