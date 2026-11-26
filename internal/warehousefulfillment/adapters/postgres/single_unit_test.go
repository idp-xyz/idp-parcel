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

// Covers: 单品单件波次不受「拣货必须用周转箱」管——它的货合在一起拿，绑不到哪一张单上；同一个仓里别的波次仍受管。
// 波次读口的每条任务带着货主，合并同一种货时靠它分开。
func TestASingleUnitWaveIsPickedWithoutTotesEvenWhereTotesAreRequired(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	if world.requireTotes(t, handler, "SYN-WH-01", 1, true).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("必须用箱没有登记成")
	}
	one, two := reserve("REQ-SINGLE-NO-TOTE", 1), reserve("REQ-DOUBLE-NEEDS-TOTE", 2)
	single := world.releaseRequests(t, handler, "WAVE-SINGLE-NO-TOTE", one)
	other := world.releaseRequests(t, handler, "WAVE-DOUBLE-NEEDS-TOTE", two)
	if single.Outcome() != application.WaveAllocated || other.Outcome() != application.WaveAllocated {
		t.Fatalf("放出 %s / %s", single.Outcome(), other.Outcome())
	}
	row := world.waveRow(t, "WAVE-SINGLE-NO-TOTE")
	if !row.SingleUnit || len(row.Tasks) != 1 || row.Tasks[0].OwnerAccount != "SYN-OWNER-01" {
		t.Fatalf("单品单件波次 %+v", row)
	}
	if got := world.pick(t, handler, single.Tasks()[0].ID, 1).Outcome(); got != application.WavePicked {
		t.Fatalf("单品单件波次不带箱 = %s", got)
	}
	if got := world.pick(t, handler, other.Tasks()[0].ID, 2).Outcome(); got != application.WaveToteRequired {
		t.Fatalf("别的波次不带箱 = %s", got)
	}
}

// Covers: 复核打包台扫到一件货，配给单品单件波次里已拣完、等复核、要这种货的出库请求里等得最久的那一张；
// 商品编码与这位货主登记过的条码都认。复核走一张，再扫配到下一张；都走完了、码认不出、或没有单要这种货时配不上。
// 不是单品单件波次的单、还没拣的单、从波次撤出的单不配。
func TestAScannedUnitIsMatchedToTheLongestWaitingSingleUnitOrder(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	first, second, unpicked, bigger := reserve("REQ-MATCH-A", 1), reserve("REQ-MATCH-B", 1), reserve("REQ-MATCH-C", 1), reserve("REQ-MATCH-BIG", 2)
	single := world.releaseRequests(t, handler, "WAVE-MATCH", first, second, unpicked)
	other := world.releaseRequests(t, handler, "WAVE-MATCH-BIG", bigger)
	if single.Outcome() != application.WaveAllocated || other.Outcome() != application.WaveAllocated {
		t.Fatalf("放出 %s / %s", single.Outcome(), other.Outcome())
	}
	for _, task := range append(single.Tasks(), other.Tasks()...) {
		if task.Request == unpicked {
			continue
		}
		if world.pick(t, handler, task.ID, task.Quantity).Outcome() != application.WavePicked {
			t.Fatalf("任务 %s 没有拣成", task.ID)
		}
	}
	if world.withdraw(t, handler, "WAVE-MATCH", unpicked).Outcome() != application.WaveWithdrawn {
		t.Fatal("没拣的那张没有撤出")
	}
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := adapter.NewGoodsCatalogue(world.db)
	if err != nil {
		t.Fatal(err)
	}
	matcher := application.NewSingleUnitMatcher(waves, catalogue, world.clock)
	tenant, warehouse := valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	match := func(code string) (application.SingleUnitMatch, bool) {
		t.Helper()
		found, ok, err := matcher.Match(t.Context(), tenant, warehouse, valueOf(t, domain.NewIdentifierCode, code))
		if err != nil {
			t.Fatalf("配单 %s：%v", code, err)
		}
		return found, ok
	}
	oldest := first
	if second.String() < first.String() {
		oldest = second
	}
	got, ok := match("SYN-GOODS-01")
	if !ok || got.Request != oldest || got.Waiting != 2 || got.Goods.String() != "SYN-GOODS-01" || got.Owner.String() != "SYN-OWNER-01" {
		t.Fatalf("按商品编码配到 %+v %v（两张同一刻受理，按请求号定先后，应是 %s）", got, ok, oldest)
	}
	if _, ok := match("SYN-CODE-NOT-THERE"); ok {
		t.Fatal("认不出的码配上了单")
	}
	if world.checkOutbound(t, handler, oldest, "SYN-CHECK-MATCH", 1).Outcome() != application.WaveChecked {
		t.Fatal("复核没有成")
	}
	next, ok := match("SYN-GOODS-01")
	if !ok || next.Request == oldest || next.Waiting != 1 {
		t.Fatalf("复核走一张之后配到 %+v %v", next, ok)
	}
	if world.checkOutbound(t, handler, next.Request, "SYN-CHECK-MATCH-2", 1).Outcome() != application.WaveChecked {
		t.Fatal("第二张复核没有成")
	}
	if left, ok := match("SYN-GOODS-01"); ok {
		t.Fatalf("都复核走了还配到 %+v", left)
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
