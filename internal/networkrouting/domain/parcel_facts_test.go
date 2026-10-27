package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// 本文件钉候选出价的判断点与包裹事实依据（CONTEXT「预路由使用客户声明快照；首次收寄复核使用当时已经取得的物理事实，到站
// 复核使用当前有效物理实测」）：判断点由触发所携的控制依据定，依据只在出了价的几格上进成本出处。

// Covers: 判断点由控制依据定——节点收寄与场外揽收是有效网络收寄的两种来源，同属首次收寄复核，且截到收寄的业务时间；
// 权威运输交接进接收节点是到站复核，不带时点（取判断此刻）。
func TestAReassessmentTriggerNamesItsParcelFactsPoint(t *testing.T) {
	for control, want := range map[domain.ControlEvidenceKind]domain.ParcelFactsPointKind{
		domain.NodeIntakeControl:        domain.FirstIntakeFactsPoint,
		domain.OffsitePickupControl:     domain.FirstIntakeFactsPoint,
		domain.TransportHandoverControl: domain.ArrivalFactsPoint,
	} {
		spec := triggerSpec(t, control)
		trigger, err := domain.NewReassessmentTrigger(spec)
		if err != nil {
			t.Fatalf("new trigger with %s: %v", control, err)
		}
		point := trigger.ParcelFactsPoint()
		if point.Kind() != want || !point.Valid() || !point.WantsPhysicalFacts() {
			t.Fatalf("%s 的判断点 = %v，want %v 且要物理事实", control, point.Kind(), want)
		}
		now := spec.OccurredAt.Add(48 * time.Hour)
		asOf, ok := point.PhysicalFactsAsOf(now)
		if !ok {
			t.Fatalf("%s 的判断点答不出物理事实时点", control)
		}
		wantAsOf := now
		if want == domain.FirstIntakeFactsPoint {
			wantAsOf = spec.OccurredAt
		}
		if !asOf.Equal(wantAsOf) {
			t.Fatalf("%s 的物理事实截至 %s，want %s", control, asOf, wantAsOf)
		}
	}
}

// Covers: 预路由不读物理事实；零值判断点不合法，每个取成本的调用点都得自己指名；首次收寄没有收寄业务时间就无从截。
func TestParcelFactsPointsRefuseWhatNamesNoJudgmentPoint(t *testing.T) {
	pre := domain.NewPreRouteFactsPoint()
	if !pre.Valid() || pre.WantsPhysicalFacts() {
		t.Fatalf("预路由判断点 valid=%v wantsPhysical=%v，want 合法且不要物理事实", pre.Valid(), pre.WantsPhysicalFacts())
	}
	if _, ok := pre.PhysicalFactsAsOf(time.Now()); ok {
		t.Fatal("预路由答出了物理事实时点")
	}
	var zero domain.ParcelFactsPoint
	if zero.Valid() || zero.WantsPhysicalFacts() {
		t.Fatal("零值判断点被当成合法")
	}
	if _, err := domain.NewFirstIntakeFactsPoint(time.Time{}); !errors.Is(err, domain.ErrInvalidParcelFactsPoint) {
		t.Fatalf("err = %v, want ErrInvalidParcelFactsPoint", err)
	}
	for name, point := range map[string]domain.ParcelFactsPoint{
		"到站":   domain.NewArrivalFactsPoint(),
		"授权决定": domain.NewDecisionFactsPoint(),
	} {
		if !point.Valid() || !point.WantsPhysicalFacts() {
			t.Fatalf("%s 判断点 valid=%v wantsPhysical=%v", name, point.Valid(), point.WantsPhysicalFacts())
		}
	}
}

// Covers: 依据只有出了价的几格能进成本出处；定不下用哪条实测与一项事实也没有都没有出价，进不了出处。库里读回认不出的文本
// 响亮报错，不兜底成某一格。
func TestOnlyPricedFactsBasesCanBeCitedAndTheClosedSetRoundTrips(t *testing.T) {
	priced := map[domain.ParcelFactsBasis]bool{
		domain.ParcelFactsDeclaration:             true,
		domain.ParcelFactsPhysical:                true,
		domain.ParcelFactsPhysicalWithDeclaration: true,
		domain.ParcelFactsDeclarationFallback:     true,
		domain.ParcelFactsPhysicalUnresolved:      false,
		domain.ParcelFactsNone:                    false,
	}
	for basis, citable := range priced {
		parsed, err := domain.ParseParcelFactsBasis(basis.String())
		if err != nil || parsed != basis {
			t.Fatalf("%s 读回 = %v、%v", basis, parsed, err)
		}
		_, err = domain.NewPlannedLegCostCitation(domain.PlannedLegCostCitationSpec{
			Ordinal: 1, EvaluationID: "EST-1", PlanReference: "plan/v1",
			ComparisonAmount: "21", ComparisonCurrency: "CNY", FactsBasis: basis,
		})
		if citable && err != nil {
			t.Fatalf("%s 进不了出处：%v", basis, err)
		}
		if !citable && !errors.Is(err, domain.ErrInvalidRoutePlan) {
			t.Fatalf("%s 进了出处：err = %v，want ErrInvalidRoutePlan", basis, err)
		}
	}
	if _, err := domain.ParseParcelFactsBasis("LAST_WEIGHED"); err == nil {
		t.Fatal("认不出的依据被兜底成某一格")
	}
	if _, err := domain.NewPlannedLegCostCitation(domain.PlannedLegCostCitationSpec{
		Ordinal: 1, EvaluationID: "EST-1", PlanReference: "plan/v1",
		ComparisonAmount: "21", ComparisonCurrency: "CNY",
	}); !errors.Is(err, domain.ErrInvalidRoutePlan) {
		t.Fatalf("没标依据的出处被收下：err = %v", err)
	}
}
