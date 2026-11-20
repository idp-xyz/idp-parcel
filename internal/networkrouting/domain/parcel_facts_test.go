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

// Covers: 实测变化重判的判断点是此刻的物理实测，不按抄来的控制依据答。锚点是节点收寄时尤其不能落到首次收寄：
// 那一格把物理事实截到收寄的业务时间，锚点之后才登记的实测就读不到。既有控制触发的判断点不因此改变。
func TestAMeasurementChangeRejudgeReadsPhysicalFactsAsOfNowEvenWhenTheAnchorIsIntake(t *testing.T) {
	spec := triggerSpec(t, domain.NodeIntakeControl)
	rejudge, err := domain.NewMeasurementChangeRejudge(spec)
	if err != nil {
		t.Fatal(err)
	}
	point := rejudge.ParcelFactsPoint()
	now := spec.OccurredAt.Add(48 * time.Hour)
	asOf, ok := point.PhysicalFactsAsOf(now)
	if point.Kind() != domain.MeasurementChangeFactsPoint || !point.Valid() || !point.WantsPhysicalFacts() || !ok || !asOf.Equal(now) {
		t.Fatalf("重判判断点 = %v、截至 %s（ok %v），want 实测变化重判、截至判断此刻 %s", point.Kind(), asOf, ok, now)
	}
	if asOf.Equal(spec.OccurredAt) {
		t.Fatal("节点收寄锚点上的重判把物理事实截到了收寄的业务时间")
	}

	plain, err := domain.NewReassessmentTrigger(spec)
	if err != nil {
		t.Fatal(err)
	}
	if plain.ParcelFactsPoint().Kind() != domain.FirstIntakeFactsPoint {
		t.Fatalf("节点收寄触发的判断点 = %v，want 首次收寄", plain.ParcelFactsPoint().Kind())
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
// 响亮报错，不兜底成某一格——物理事实不与声明并用，「并用」那个字面不在封闭集里，读回与任何未知字面同样报错。
func TestOnlyPricedFactsBasesCanBeCitedAndTheClosedSetRoundTrips(t *testing.T) {
	priced := map[domain.ParcelFactsBasis]bool{
		domain.ParcelFactsDeclaration:         true,
		domain.ParcelFactsPhysical:            true,
		domain.ParcelFactsDeclarationFallback: true,
		domain.ParcelFactsPhysicalUnresolved:  false,
		domain.ParcelFactsNone:                false,
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
	for _, unknown := range []string{"LAST_WEIGHED", "PHYSICAL_WITH_DECLARATION", ""} {
		if basis, err := domain.ParseParcelFactsBasis(unknown); err == nil {
			t.Fatalf("认不出的依据 %q 被读成 %s", unknown, basis)
		}
	}
	for basis := domain.ParcelFactsDeclaration; basis.Valid(); basis++ {
		if _, listed := priced[basis]; !listed {
			t.Fatalf("封闭集里的 %s 没有列在本用例的出价表上", basis)
		}
	}
	if _, err := domain.NewPlannedLegCostCitation(domain.PlannedLegCostCitationSpec{
		Ordinal: 1, EvaluationID: "EST-1", PlanReference: "plan/v1",
		ComparisonAmount: "21", ComparisonCurrency: "CNY",
	}); !errors.Is(err, domain.ErrInvalidRoutePlan) {
		t.Fatalf("没标依据的出处被收下：err = %v", err)
	}
}
