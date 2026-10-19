package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 夹具全为合成值（S 级）：段 RPV-1#1 走连接 CONN 自 A 至 B，窗口 [10:00, 16:00]（UTC，2026-10-21）。

var legWindowStart = time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)

func preparationLegFor(t *testing.T) domain.PreparationLeg {
	t.Helper()
	leg, err := domain.NewPreparationLeg(
		mustValue(t, domain.NewPlannedSegmentReference, "RPV-1#1"),
		directionFor(t),
		legWindowStart,
		legWindowStart.Add(6*time.Hour),
	)
	if err != nil {
		t.Fatalf("段：%v", err)
	}
	return leg
}

func adHocSchedule(t *testing.T, id, connection, from, to string, departs time.Time, duration time.Duration) domain.TransportSchedule {
	t.Helper()
	direction, err := domain.NewScheduleDirection(connection, from, to)
	if err != nil {
		t.Fatalf("方向：%v", err)
	}
	schedule, err := domain.FormTransportSchedule(domain.TransportScheduleSpec{
		TenantID:     mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Schedule:     mustValue(t, domain.NewScheduleReference, id),
		Direction:    direction,
		DepartsAt:    departs,
		ArrivesAt:    departs.Add(duration),
		ServiceScope: mustValue(t, domain.NewScheduleServiceScopeReference, "SCOPE"),
	})
	if err != nil {
		t.Fatalf("班次 %s：%v", id, err)
	}
	return schedule
}

func poolOn(t *testing.T, schedule domain.TransportSchedule, dimension domain.CapacityDimension, unit string, capacity int64) domain.CapacityPool {
	t.Helper()
	pool, err := domain.EstablishCapacityPool(domain.CapacityPoolSpec{
		TenantID:  schedule.TenantID(),
		Schedule:  schedule.Schedule(),
		Dimension: dimension,
		Unit:      mustValue(t, domain.NewQuantityUnitReference, unit),
		Capacity:  capacity,
	})
	if err != nil {
		t.Fatalf("池：%v", err)
	}
	return pool
}

// declaredParcel 是一件报了 1.50 KG、20×15×10 CM 的包裹的需求量快照。
func declaredParcel(t *testing.T) domain.DemandSnapshot {
	t.Helper()
	demand, err := domain.DeclaredDemand(domain.DeclaredDemandSpec{
		Anchor:      "baseline",
		WeightValue: "1.50", WeightUnit: "KG",
		Length: "20", Width: "15", Height: "10", DimensionUnit: "CM",
	})
	if err != nil {
		t.Fatalf("需求量：%v", err)
	}
	return demand
}

func prepare(t *testing.T, demand domain.DemandSnapshot, configured bool, candidates ...domain.PreparationCandidateInput) domain.TransportPreparation {
	t.Helper()
	preparation, err := domain.PrepareTransportOpportunity(domain.PrepareTransportOpportunitySpec{
		TenantID:   mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Leg:        preparationLegFor(t),
		Object:     mustValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-01"),
		Rule:       domain.BuiltInPreparationRuleVersion(),
		Demand:     demand,
		Configured: configured,
		Candidates: candidates,
		JudgedAt:   legWindowStart.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("准备：%v", err)
	}
	return preparation
}

// Covers: AT-TF-132——申报译成需求量：重量照抄，体积取三边乘积（单位由长度单位派生），件数恒为一件；外廓缺席就没有体积一维，
// 袋位一类维度从来没有。
func TestDeclaredDemandHasWeightVolumeAndOnePiece(t *testing.T) {
	quantities := map[string]string{}
	for _, quantity := range declaredParcel(t).Quantities() {
		quantities[quantity.Dimension.String()] = quantity.Amount + " " + quantity.Unit.String()
	}
	want := map[string]string{"WEIGHT": "1.50 KG", "VOLUME": "3000 CM3", "PIECES": "1 PCS"}
	if len(quantities) != len(want) {
		t.Fatalf("各维 = %v, want %v", quantities, want)
	}
	for dimension, amount := range want {
		if quantities[dimension] != amount {
			t.Fatalf("%s = %q, want %q", dimension, quantities[dimension], amount)
		}
	}

	fractional, err := domain.DeclaredDemand(domain.DeclaredDemandSpec{
		WeightValue: "2", WeightUnit: "LB", Length: "0.5", Width: "0.25", Height: "1.2", DimensionUnit: "M",
	})
	if err != nil {
		t.Fatalf("小数外廓：%v", err)
	}
	if volume := fractional.Quantities()[1]; volume.Amount != "0.15" || volume.Unit.String() != "M3" {
		t.Fatalf("体积 = %s %s, want 0.15 M3", volume.Amount, volume.Unit)
	}
	if _, anchored := fractional.Anchor(); anchored {
		t.Fatal("没给锚却带了锚")
	}

	weightOnly, err := domain.DeclaredDemand(domain.DeclaredDemandSpec{WeightValue: "3", WeightUnit: "KG"})
	if err != nil {
		t.Fatalf("只报重量：%v", err)
	}
	if got := len(weightOnly.Quantities()); got != 2 {
		t.Fatalf("只报重量的维数 = %d, want 2（重量与件数）", got)
	}
	nothing, err := domain.DeclaredDemand(domain.DeclaredDemandSpec{})
	if err != nil {
		t.Fatalf("什么都没报：%v", err)
	}
	if quantities := nothing.Quantities(); len(quantities) != 1 || quantities[0].Dimension != domain.CapacityPieces {
		t.Fatalf("什么都没报 = %v, want 只有件数", quantities)
	}

	for name, spec := range map[string]domain.DeclaredDemandSpec{
		"重量不是十进制": {WeightValue: "1e3", WeightUnit: "KG"},
		"重量为零":    {WeightValue: "0.00", WeightUnit: "KG"},
		"重量缺单位":   {WeightValue: "1"},
		"外廓缺一边":   {Length: "1", Width: "2", DimensionUnit: "CM"},
		"外廓缺单位":   {Length: "1", Width: "2", Height: "3"},
	} {
		if _, err := domain.DeclaredDemand(spec); !errors.Is(err, domain.ErrInvalidDemandSnapshot) {
			t.Fatalf("%s：err = %v, want ErrInvalidDemandSnapshot", name, err)
		}
	}
}

// Covers: AT-TF-103（准备侧）——该连接空册答`未配置`，不带候选、不选班次。
func TestAnEmptyRegisterPreparesNotConfigured(t *testing.T) {
	preparation := prepare(t, declaredParcel(t), false)
	if preparation.Outcome() != domain.PreparationNotConfigured || len(preparation.Candidates()) != 0 {
		t.Fatalf("空册 = %s，候选 %d", preparation.Outcome(), len(preparation.Candidates()))
	}
	if _, selected := preparation.Selected(); selected {
		t.Fatal("空册选了班次")
	}
}

// Covers: AT-TF-028、AT-TF-128——登过但窗口内没有合格候选：答`无当前运输机会`，候选连同淘汰原因一起留下，不建占位班次。
func TestCandidatesOutsideTheLegAreEliminatedWithReasons(t *testing.T) {
	inputs := []domain.PreparationCandidateInput{
		{Schedule: adHocSchedule(t, "S-LATE", "CONN", "A", "B", legWindowStart.Add(5*time.Hour), 2*time.Hour)},
		{Schedule: adHocSchedule(t, "S-EARLY", "CONN", "A", "B", legWindowStart.Add(-time.Hour), time.Hour)},
		{Schedule: adHocSchedule(t, "S-REVERSED", "CONN", "B", "A", legWindowStart.Add(time.Hour), time.Hour)},
		{Schedule: adHocSchedule(t, "S-OTHER", "CONN-2", "A", "B", legWindowStart.Add(2*time.Hour), time.Hour)},
	}
	preparation := prepare(t, declaredParcel(t), true, inputs...)
	if preparation.Outcome() != domain.PreparationNoCurrentOpportunity {
		t.Fatalf("结论 = %s", preparation.Outcome())
	}
	got := map[string]domain.CandidateVerdict{}
	for _, candidate := range preparation.Candidates() {
		got[candidate.Schedule.String()] = candidate.Verdict
		if len(candidate.Pools) != 0 {
			t.Fatalf("出局的 %s 带了池依据", candidate.Schedule)
		}
	}
	want := map[string]domain.CandidateVerdict{
		"S-LATE":     domain.CandidateArrivesAfterWindow,
		"S-EARLY":    domain.CandidateDepartsBeforeWindow,
		"S-REVERSED": domain.CandidateNodesMismatch,
		"S-OTHER":    domain.CandidateConnectionMismatch,
	}
	for schedule, verdict := range want {
		if got[schedule] != verdict {
			t.Fatalf("%s = %s, want %s", schedule, got[schedule], verdict)
		}
	}
	if first := preparation.Candidates()[0].Schedule.String(); first != "S-EARLY" {
		t.Fatalf("候选没按出发先后排：首个 %s", first)
	}
}

// Covers: AT-TF-129——择优：出发最早优先、同刻按班次标识；跳过容量说不准或不够的班，取第一个已登池每一维都够的；可用量扣掉
// 判断时点仍有效的预占；不预占（池原样不动）。
func TestTheEarliestSufficientCandidateIsPrepared(t *testing.T) {
	unconfirmed := adHocSchedule(t, "S-1", "CONN", "A", "B", legWindowStart, time.Hour)
	insufficient := adHocSchedule(t, "S-2", "CONN", "A", "B", legWindowStart.Add(time.Hour), time.Hour)
	tieLater := adHocSchedule(t, "S-4", "CONN", "A", "B", legWindowStart.Add(2*time.Hour), time.Hour)
	tieEarlier := adHocSchedule(t, "S-3", "CONN", "A", "B", legWindowStart.Add(2*time.Hour), time.Hour)

	reserved, err := poolOn(t, insufficient, domain.CapacityWeight, "KG", 10).
		Reserve(mustValue(t, domain.NewCapacityReservationReference, "R-1"), demandIn(t, 9, "KG"), legWindowStart.Add(24*time.Hour), legWindowStart.Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("预占：%v", err)
	}
	preparation := prepare(t, declaredParcel(t), true,
		domain.PreparationCandidateInput{Schedule: tieLater, Registered: true,
			Pools: []domain.CapacityPool{poolOn(t, tieLater, domain.CapacityWeight, "KG", 100)}},
		domain.PreparationCandidateInput{Schedule: unconfirmed, Registered: true,
			Pools: []domain.CapacityPool{poolOn(t, unconfirmed, domain.CapacityBagPositions, "BAG", 5)}},
		domain.PreparationCandidateInput{Schedule: insufficient, Registered: true, Pools: []domain.CapacityPool{reserved}},
		domain.PreparationCandidateInput{Schedule: tieEarlier,
			Pools: []domain.CapacityPool{
				poolOn(t, tieEarlier, domain.CapacityWeight, "G", 1500),
				poolOn(t, tieEarlier, domain.CapacityPieces, "PCS", 1),
				poolOn(t, tieEarlier, domain.CapacityVolume, "M3", 1),
			}},
	)
	if preparation.Outcome() != domain.PreparationOpportunityPrepared {
		t.Fatalf("结论 = %s", preparation.Outcome())
	}
	if selected, _ := preparation.Selected(); selected.String() != "S-3" {
		t.Fatalf("选中 %s, want S-3（同刻按标识）", selected)
	}
	verdicts := map[string]domain.CandidateVerdict{}
	for _, candidate := range preparation.Candidates() {
		verdicts[candidate.Schedule.String()] = candidate.Verdict
		if candidate.Schedule.String() == "S-3" {
			if candidate.Registered() {
				t.Fatal("只在评估时物化的那一班被记成已登记")
			}
			for _, pool := range candidate.Pools {
				if pool.Dimension == domain.CapacityWeight && (pool.Requested != "1500" || pool.Available != 1500) {
					t.Fatalf("1.50 KG 换成克 = %q / 可用 %d, want 1500 / 1500", pool.Requested, pool.Available)
				}
				if pool.Dimension == domain.CapacityVolume && pool.Requested != "0.003" {
					t.Fatalf("3000 CM3 换成立方米 = %q, want 0.003", pool.Requested)
				}
			}
		}
	}
	if verdicts["S-1"] != domain.CandidateCapacityUnconfirmed || verdicts["S-2"] != domain.CandidateCapacityInsufficient ||
		verdicts["S-4"] != domain.CandidateCapacitySufficient {
		t.Fatalf("各班判词 = %v", verdicts)
	}
	if available := reserved.AvailableAt(legWindowStart.Add(-time.Hour)); available != 1 {
		t.Fatalf("准备改动了池：可用 %d, want 1", available)
	}
}

// Covers: AT-TF-130——有合格候选但没有一班说得准：没有池、池的维度在需求量里没有数量、单位认不出（含大小写不同的拼写），
// 都答`容量待确认`，不以默认容量继续。
func TestCapacityThatCannotBeJudgedIsUnconfirmed(t *testing.T) {
	noPool := adHocSchedule(t, "S-1", "CONN", "A", "B", legWindowStart, time.Hour)
	pallet := adHocSchedule(t, "S-2", "CONN", "A", "B", legWindowStart.Add(time.Hour), time.Hour)
	lowercase := adHocSchedule(t, "S-3", "CONN", "A", "B", legWindowStart.Add(2*time.Hour), time.Hour)
	crossDimension := adHocSchedule(t, "S-4", "CONN", "A", "B", legWindowStart.Add(3*time.Hour), time.Hour)
	preparation := prepare(t, declaredParcel(t), true,
		domain.PreparationCandidateInput{Schedule: noPool},
		domain.PreparationCandidateInput{Schedule: pallet,
			Pools: []domain.CapacityPool{poolOn(t, pallet, domain.CapacityPalletPositions, "PLT", 4)}},
		domain.PreparationCandidateInput{Schedule: lowercase,
			Pools: []domain.CapacityPool{poolOn(t, lowercase, domain.CapacityWeight, "kg", 1000)}},
		domain.PreparationCandidateInput{Schedule: crossDimension,
			Pools: []domain.CapacityPool{poolOn(t, crossDimension, domain.CapacityVolume, "KG", 1000)}},
	)
	if preparation.Outcome() != domain.PreparationCapacityUnconfirmed {
		t.Fatalf("结论 = %s", preparation.Outcome())
	}
	want := map[string]domain.PoolVerdict{
		"S-2": domain.PoolDemandPending,
		"S-3": domain.PoolUnitNotConvertible,
		"S-4": domain.PoolUnitNotConvertible,
	}
	for _, candidate := range preparation.Candidates() {
		if candidate.Verdict != domain.CandidateCapacityUnconfirmed {
			t.Fatalf("%s = %s", candidate.Schedule, candidate.Verdict)
		}
		if verdict, judged := want[candidate.Schedule.String()]; judged &&
			(len(candidate.Pools) != 1 || candidate.Pools[0].Verdict != verdict || candidate.Pools[0].Requested != "") {
			t.Fatalf("%s 的池依据 = %+v, want %s 且不写需求量", candidate.Schedule, candidate.Pools, verdict)
		}
	}
}

// Covers: AT-TF-131——每一班都有一维硬容量不够：答`当前机会不可用`；一维不够时别的维说不说得准都不救这一班。
func TestEveryCandidateShortOfHardCapacityIsUnavailable(t *testing.T) {
	first := adHocSchedule(t, "S-1", "CONN", "A", "B", legWindowStart, time.Hour)
	second := adHocSchedule(t, "S-2", "CONN", "A", "B", legWindowStart.Add(time.Hour), time.Hour)
	preparation := prepare(t, declaredParcel(t), true,
		domain.PreparationCandidateInput{Schedule: first, Pools: []domain.CapacityPool{
			poolOn(t, first, domain.CapacityWeight, "KG", 1),
			poolOn(t, first, domain.CapacityBagPositions, "BAG", 5),
		}},
		domain.PreparationCandidateInput{Schedule: second, Pools: []domain.CapacityPool{
			poolOn(t, second, domain.CapacityWeight, "LB", 3),
		}},
	)
	if preparation.Outcome() != domain.PreparationOpportunityUnavailable {
		t.Fatalf("结论 = %s", preparation.Outcome())
	}
	for _, candidate := range preparation.Candidates() {
		if candidate.Verdict != domain.CandidateCapacityInsufficient {
			t.Fatalf("%s = %s", candidate.Schedule, candidate.Verdict)
		}
	}
	if requested := preparation.Candidates()[1].Pools[0].Requested; requested != "3.306934" {
		t.Fatalf("1.50 KG 换成磅 = %q, want 3.306934（六位小数）", requested)
	}
}

// Covers: AT-TF-133（重建侧）——读回的结论与候选依据推不到一处就拒：选中的不是第一个够的、未配置却带候选、候选乱序、判词与池
// 依据不符。照写下的读回则原样成立。
func TestRehydrationRefusesAnOutcomeTheCandidatesDoNotSupport(t *testing.T) {
	sufficient := adHocSchedule(t, "S-1", "CONN", "A", "B", legWindowStart, time.Hour)
	later := adHocSchedule(t, "S-2", "CONN", "A", "B", legWindowStart.Add(time.Hour), time.Hour)
	prepared := prepare(t, declaredParcel(t), true,
		domain.PreparationCandidateInput{Schedule: sufficient, Pools: []domain.CapacityPool{poolOn(t, sufficient, domain.CapacityPieces, "PCS", 2)}},
		domain.PreparationCandidateInput{Schedule: later, Pools: []domain.CapacityPool{poolOn(t, later, domain.CapacityPieces, "PCS", 2)}},
	)
	spec := domain.RehydrateTransportPreparationSpec{
		TenantID:   prepared.TenantID(),
		Leg:        prepared.Leg(),
		Object:     prepared.Object(),
		Rule:       prepared.Rule(),
		Demand:     prepared.Demand(),
		Outcome:    prepared.Outcome(),
		Candidates: prepared.Candidates(),
		JudgedAt:   prepared.JudgedAt(),
	}
	spec.Selected, _ = prepared.Selected()
	if _, err := domain.RehydrateTransportPreparation(spec); err != nil {
		t.Fatalf("照写下的读回被拒：%v", err)
	}

	wrongPick := spec
	wrongPick.Selected = mustValue(t, domain.NewScheduleReference, "S-2")
	notConfigured := spec
	notConfigured.Outcome, notConfigured.Selected = domain.PreparationNotConfigured, domain.ScheduleReference{}
	reordered := spec
	reordered.Candidates = []domain.PreparationCandidate{spec.Candidates[1], spec.Candidates[0]}
	tampered := spec
	tampered.Candidates = prepared.Candidates()
	tampered.Candidates[0].Verdict = domain.CandidateCapacityInsufficient
	for name, broken := range map[string]domain.RehydrateTransportPreparationSpec{
		"选中的不是第一个够的": wrongPick, "未配置却带候选": notConfigured, "候选乱序": reordered, "判词与池依据不符": tampered,
	} {
		if _, err := domain.RehydrateTransportPreparation(broken); !errors.Is(err, domain.ErrInvalidTransportPreparation) {
			t.Fatalf("%s：err = %v", name, err)
		}
	}
}

// Covers: AT-TF-133（摘要侧）——依据摘要只随段的方向、窗口与需求量变：同一依据同串，申报一变就是另一串。
func TestThePreparationInputDigestFollowsTheBasisOnly(t *testing.T) {
	leg := preparationLegFor(t)
	first, err := domain.CanonicalizePreparationInput(leg, declaredParcel(t))
	if err != nil {
		t.Fatalf("摘要：%v", err)
	}
	again, _ := domain.CanonicalizePreparationInput(leg, declaredParcel(t))
	corrected, err := domain.DeclaredDemand(domain.DeclaredDemandSpec{Anchor: "SDV-2", WeightValue: "1.60", WeightUnit: "KG"})
	if err != nil {
		t.Fatalf("更正后的申报：%v", err)
	}
	changed, _ := domain.CanonicalizePreparationInput(leg, corrected)
	if first != again || first == changed {
		t.Fatalf("摘要：首 %s 再 %s 改 %s", first, again, changed)
	}
}
