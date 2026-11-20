package parcelpricing_test

import (
	"context"
	"errors"
	"testing"
	"time"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelpricing"
)

// 本文件钉按判断点选包裹事实来源的三条规则（CONTEXT「预路由使用客户声明快照；首次收寄复核使用当时已经取得的物理事实，到站
// 复核使用当前有效物理实测」）：物理事实一项没取得——退回客户声明并标明；物理事实在而定不下用哪条——不退回声明，各段待
// 判断；物理事实取得了——重量与外廓整份取用，声明一格都不读、不补，取得了的物理事实必须重量与外廓都在。

var (
	intakeAt = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	judgedAt = time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC)
)

// declaredDouble 是客户声明那一路的替身：记下被问了几次。
type declaredDouble struct {
	facts adapter.ParcelFacts
	found bool
	err   error
	asked int
}

func (double *declaredDouble) DeclaredFactsFor(context.Context, nrdomain.InitialRouteJudgmentKey) (adapter.ParcelFacts, bool, error) {
	double.asked++
	return double.facts, double.found, double.err
}

// physicalDouble 是物理事实那一路的替身：记下被问了几次与截至哪一刻。
type physicalDouble struct {
	answer adapter.PhysicalParcelFacts
	err    error
	asked  int
	asOf   time.Time
}

func (double *physicalDouble) PhysicalParcelFactsFor(
	_ context.Context, _ nrdomain.InitialRouteJudgmentKey, asOf time.Time,
) (adapter.PhysicalParcelFacts, error) {
	double.asked++
	double.asOf = asOf
	return double.answer, double.err
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

func declaredFacts(t *testing.T, weight string, withDimensions bool) *declaredDouble {
	t.Helper()
	facts := adapter.ParcelFacts{
		Weight: weightOf(t, weight),
		Facts:  []ppdomain.VersionedFactReference{factReference(t, ppdomain.ArtifactDeclaredMeasurement, "parcel-1", "declaration@baseline")},
		Basis:  nrdomain.ParcelFactsDeclaration,
	}
	if withDimensions {
		dimensions := dimensionsOf(t, "30", "20", "10")
		facts.Dimensions = &dimensions
	}
	return &declaredDouble{facts: facts, found: true}
}

func factReference(t *testing.T, kind ppdomain.ArtifactKind, id, version string) ppdomain.VersionedFactReference {
	t.Helper()
	fact, err := ppdomain.NewVersionedFactReference(mustReference(t, kind, id, version))
	if err != nil {
		t.Fatalf("事实引用：%v", err)
	}
	return fact
}

func dimensionsOf(t *testing.T, length, width, height string) ppdomain.Dimensions {
	t.Helper()
	dimensions, err := ppdomain.NewDimensions(decimalOf(t, length), decimalOf(t, width), decimalOf(t, height), ppdomain.LengthUnitCentimeter)
	if err != nil {
		t.Fatalf("外廓：%v", err)
	}
	return dimensions
}

// obtained 造一份取得了的物理事实：一条实测同时带毛重与外廓，事实引用标明出自实际测量。
func obtained(t *testing.T, weight string, dimensions ppdomain.Dimensions) adapter.PhysicalParcelFacts {
	t.Helper()
	facts, err := adapter.ObtainedPhysicalFacts(weightOf(t, weight), dimensions,
		[]ppdomain.VersionedFactReference{factReference(t, ppdomain.ArtifactActualMeasurement, "parcel-1", "measurement-1")})
	if err != nil {
		t.Fatalf("物理事实：%v", err)
	}
	return facts
}

func pointedFacts(t *testing.T, declared *declaredDouble, physical *physicalDouble) *adapter.PointedParcelFacts {
	t.Helper()
	facts, err := adapter.NewPointedParcelFacts(declared, physical, fixedClock{at: judgedAt})
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	return facts
}

func firstIntake(t *testing.T) nrdomain.ParcelFactsPoint {
	t.Helper()
	point, err := nrdomain.NewFirstIntakeFactsPoint(intakeAt)
	if err != nil {
		t.Fatal(err)
	}
	return point
}

func factKinds(facts adapter.ParcelFacts) []ppdomain.ArtifactKind {
	var kinds []ppdomain.ArtifactKind
	for _, fact := range facts.Facts {
		kinds = append(kinds, fact.Reference().Kind())
	}
	return kinds
}

// Covers: 预路由只取客户声明，一次物理事实也不问；依据标客户声明。
func TestThePreRoutePointReadsOnlyTheDeclaration(t *testing.T) {
	declared, physical := declaredFacts(t, "2.5", true), &physicalDouble{answer: obtained(t, "9", dimensionsOf(t, "50", "40", "30"))}

	got, found, err := pointedFacts(t, declared, physical).ParcelFactsFor(context.Background(), judgmentKey(t), nrdomain.NewPreRouteFactsPoint())
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if physical.asked != 0 {
		t.Fatalf("预路由问了 %d 次物理事实，want 0", physical.asked)
	}
	if got.Basis != nrdomain.ParcelFactsDeclaration || got.Weight.Value().String() != "2.5" {
		t.Fatalf("依据 = %s、实重 = %s，want 客户声明 2.5", got.Basis, got.Weight.Value())
	}
}

// Covers: 物理事实的时点由判断点定——首次收寄复核截到收寄的业务时间（不是判断此刻），到站复核与授权改路决定取判断此刻。
func TestPhysicalFactsAreReadAsOfTheMomentTheJudgmentPointNames(t *testing.T) {
	for name, test := range map[string]struct {
		point nrdomain.ParcelFactsPoint
		asOf  time.Time
	}{
		"首次收寄":   {firstIntake(t), intakeAt},
		"到站":     {nrdomain.NewArrivalFactsPoint(), judgedAt},
		"授权决定":   {nrdomain.NewDecisionFactsPoint(), judgedAt},
		"实测变化重判": {nrdomain.NewMeasurementChangeFactsPoint(), judgedAt},
	} {
		t.Run(name, func(t *testing.T) {
			physical := &physicalDouble{answer: obtained(t, "9", dimensionsOf(t, "50", "40", "30"))}
			if _, _, err := pointedFacts(t, declaredFacts(t, "2.5", false), physical).ParcelFactsFor(context.Background(), judgmentKey(t), test.point); err != nil {
				t.Fatal(err)
			}
			if physical.asked != 1 || !physical.asOf.Equal(test.asOf) {
				t.Fatalf("问了 %d 次、截至 %s，want 1 次、截至 %s", physical.asked, physical.asOf, test.asOf)
			}
		})
	}
}

// Covers: 实测与声明不符时按实测——重量与外廓都取得了：取物理事实，声明不读、不进事实引用；依据标物理事实。
func TestObtainedPhysicalFactsPriceInsteadOfTheDeclaration(t *testing.T) {
	measured := dimensionsOf(t, "50", "40", "30")
	declared, physical := declaredFacts(t, "2.5", true), &physicalDouble{answer: obtained(t, "12", measured)}

	got, found, err := pointedFacts(t, declared, physical).ParcelFactsFor(context.Background(), judgmentKey(t), nrdomain.NewArrivalFactsPoint())
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if got.Weight.Value().String() != "12" || got.Dimensions == nil || got.Dimensions.LongestSide().Value().String() != "50" {
		t.Fatalf("实重 = %s、外廓 = %+v，want 实测的 12 与 50×40×30", got.Weight.Value(), got.Dimensions)
	}
	if got.Basis != nrdomain.ParcelFactsPhysical || declared.asked != 0 {
		t.Fatalf("依据 = %s、读声明 %d 次，want 物理事实、不读声明", got.Basis, declared.asked)
	}
	if kinds := factKinds(got); len(kinds) != 1 || kinds[0] != ppdomain.ArtifactActualMeasurement {
		t.Fatalf("事实引用 = %v，want 只有实际测量", kinds)
	}
}

// Covers: 物理事实取得了就整份取用——声明里重量与外廓都与实测不同时，声明源一次都没被读（替身记次数），交出的重量与外廓都是
// 物理事实的，依据标物理事实，事实引用里没有声明的引用；声明源读不回也不牵连这一格，因为根本不读。三个取物理事实的判断点同此。
func TestObtainedPhysicalFactsAreTakenWholeAndTheDeclarationIsNeverRead(t *testing.T) {
	for name, point := range map[string]nrdomain.ParcelFactsPoint{
		"首次收寄": firstIntake(t), "到站": nrdomain.NewArrivalFactsPoint(), "授权决定": nrdomain.NewDecisionFactsPoint(),
		"实测变化重判": nrdomain.NewMeasurementChangeFactsPoint(),
	} {
		t.Run(name, func(t *testing.T) {
			measured := dimensionsOf(t, "50", "40", "30")
			declared := declaredFacts(t, "2.5", true)
			declared.err = errors.New("声明读不回")

			got, found, err := pointedFacts(t, declared, &physicalDouble{answer: obtained(t, "12", measured)}).
				ParcelFactsFor(context.Background(), judgmentKey(t), point)
			if err != nil || !found {
				t.Fatalf("found=%v err=%v", found, err)
			}
			if declared.asked != 0 {
				t.Fatalf("取得了物理事实却读了 %d 次声明，want 0", declared.asked)
			}
			if got.Weight.Value().String() != "12" || got.Dimensions == nil ||
				got.Dimensions.LongestSide().Value().String() != "50" || got.Dimensions.ShortestSide().Value().String() != "30" {
				t.Fatalf("实重 = %s、外廓 = %+v，want 实测的 12 与 50×40×30，没有一格是声明的", got.Weight.Value(), got.Dimensions)
			}
			if kinds := factKinds(got); got.Basis != nrdomain.ParcelFactsPhysical || len(kinds) != 1 || kinds[0] != ppdomain.ArtifactActualMeasurement {
				t.Fatalf("依据 = %s、事实引用 = %v，want 物理事实、只有实际测量", got.Basis, kinds)
			}
		})
	}
}

// Covers: 取得了的物理事实重量与外廓都在——只带重量或只带外廓的物理事实构造时报错，不收下。重量与外廓取值，没有「只带一样」
// 的签名，只剩没经构造门的零值一条路，同样拒。不然「只有一部分」会重新有了来源，复核就得拿声明去补，造出重量是实测、外廓
// 是声明的混合快照。
func TestPhysicalFactsWithOnlyOneOfWeightAndDimensionsCannotBeObtained(t *testing.T) {
	reference := []ppdomain.VersionedFactReference{factReference(t, ppdomain.ArtifactActualMeasurement, "parcel-1", "measurement-1")}
	weight, dimensions := weightOf(t, "12"), dimensionsOf(t, "50", "40", "30")

	if _, err := adapter.ObtainedPhysicalFacts(weight, ppdomain.Dimensions{}, reference); err == nil {
		t.Fatal("只有重量的物理事实被收下")
	}
	if _, err := adapter.ObtainedPhysicalFacts(ppdomain.Weight{}, dimensions, reference); err == nil {
		t.Fatal("只有外廓的物理事实被收下")
	}
	if _, err := adapter.ObtainedPhysicalFacts(weight, dimensions, reference); err != nil {
		t.Fatalf("重量与外廓都在的物理事实被拒：%v", err)
	}
}

// Covers: 物理事实一项都没取得时退回客户声明，依据标退回，排序照常；声明也没有则一项事实都没有。
func TestWhenNoPhysicalFactWasObtainedTheDeclarationStandsInAndSaysSo(t *testing.T) {
	for _, point := range []nrdomain.ParcelFactsPoint{firstIntake(t), nrdomain.NewArrivalFactsPoint(), nrdomain.NewDecisionFactsPoint()} {
		got, found, err := pointedFacts(t, declaredFacts(t, "2.5", true), &physicalDouble{answer: adapter.NoPhysicalFactsObtained()}).
			ParcelFactsFor(context.Background(), judgmentKey(t), point)
		if err != nil || !found {
			t.Fatalf("found=%v err=%v", found, err)
		}
		if got.Basis != nrdomain.ParcelFactsDeclarationFallback || got.Weight.Value().String() != "2.5" {
			t.Fatalf("依据 = %s、实重 = %s，want 退回声明的 2.5", got.Basis, got.Weight.Value())
		}
		if kinds := factKinds(got); len(kinds) != 1 || kinds[0] != ppdomain.ArtifactDeclaredMeasurement {
			t.Fatalf("事实引用 = %v，want 只有声明", kinds)
		}
	}

	got, found, err := pointedFacts(t, &declaredDouble{}, &physicalDouble{answer: adapter.NoPhysicalFactsObtained()}).
		ParcelFactsFor(context.Background(), judgmentKey(t), nrdomain.NewArrivalFactsPoint())
	if err != nil || found || got.Basis != nrdomain.ParcelFactsNone {
		t.Fatalf("found=%v err=%v 依据 = %s，want 没有事实、依据 NONE", found, err, got.Basis)
	}
}

// Covers: 物理事实在而定不下用哪条时不退回声明——客户声明不得顶替实测；不问声明，各段待判断，依据说明是定不下。
func TestUnresolvedPhysicalFactsAreNotReplacedByTheDeclaration(t *testing.T) {
	declared := declaredFacts(t, "2.5", true)
	got, found, err := pointedFacts(t, declared, &physicalDouble{answer: adapter.UnresolvedPhysicalFacts()}).
		ParcelFactsFor(context.Background(), judgmentKey(t), nrdomain.NewArrivalFactsPoint())
	if err != nil || found {
		t.Fatalf("found=%v err=%v，want 没有可用的事实", found, err)
	}
	if got.Basis != nrdomain.ParcelFactsPhysicalUnresolved || declared.asked != 0 {
		t.Fatalf("依据 = %s、读声明 %d 次，want 定不下、不读声明", got.Basis, declared.asked)
	}
}

// Covers: 读口读不回是依赖故障，原样上抛，不折成「没取得」而退回声明——那会在依赖故障时悄悄按声明出价；声明读不回、而要退回
// 声明出价时同样上抛（ADR-0029：读不动是未决）。取得了物理事实时不读声明，声明读不回与它无关（见整份取用那一条）。
func TestAReadFailureIsNeverTurnedIntoAFallback(t *testing.T) {
	broken := errors.New("读库失败")
	if _, _, err := pointedFacts(t, declaredFacts(t, "2.5", false), &physicalDouble{err: broken}).
		ParcelFactsFor(context.Background(), judgmentKey(t), nrdomain.NewArrivalFactsPoint()); !errors.Is(err, broken) {
		t.Fatalf("物理事实读不回：err = %v，want 上抛", err)
	}
	if _, _, err := pointedFacts(t, &declaredDouble{err: broken}, &physicalDouble{answer: adapter.NoPhysicalFactsObtained()}).
		ParcelFactsFor(context.Background(), judgmentKey(t), nrdomain.NewArrivalFactsPoint()); !errors.Is(err, broken) {
		t.Fatalf("声明读不回：err = %v，want 上抛", err)
	}
}

// Covers: 认不出的判断点与认不出的物理事实答复是装配写坏了，响亮报错，不落进某一格；挂在 ErrRouteCostAnswerUnusable 名下。
func TestAnUnknownPointOrPhysicalAnswerFailsLoudly(t *testing.T) {
	facts := pointedFacts(t, declaredFacts(t, "2.5", false), &physicalDouble{})
	if _, _, err := facts.ParcelFactsFor(context.Background(), judgmentKey(t), nrdomain.ParcelFactsPoint{}); !errors.Is(err, nrports.ErrRouteCostAnswerUnusable) {
		t.Fatalf("零值判断点：err = %v，want ErrRouteCostAnswerUnusable", err)
	}
	if _, _, err := facts.ParcelFactsFor(context.Background(), judgmentKey(t), nrdomain.NewArrivalFactsPoint()); !errors.Is(err, nrports.ErrRouteCostAnswerUnusable) {
		t.Fatalf("零值物理事实答复：err = %v，want ErrRouteCostAnswerUnusable", err)
	}
}

// Covers: 取得了却什么都没带、没带事实引用的物理事实立不起来；合成缺声明来源、物理来源或时钟也立不起来。
func TestPhysicalFactsCannotBeObtainedWithoutContentOrProvenance(t *testing.T) {
	reference := []ppdomain.VersionedFactReference{factReference(t, ppdomain.ArtifactActualMeasurement, "parcel-1", "measurement-1")}
	weight, dimensions := weightOf(t, "1"), dimensionsOf(t, "30", "20", "10")
	if _, err := adapter.ObtainedPhysicalFacts(ppdomain.Weight{}, ppdomain.Dimensions{}, reference); err == nil {
		t.Fatal("重量与外廓都没有的物理事实被收下")
	}
	if _, err := adapter.ObtainedPhysicalFacts(weight, dimensions, nil); err == nil {
		t.Fatal("没有事实引用的物理事实被收下")
	}
	if _, err := adapter.NewPointedParcelFacts(nil, &physicalDouble{}, fixedClock{}); err == nil {
		t.Fatal("没有声明来源的合成被收下")
	}
	if _, err := adapter.NewPointedParcelFacts(&declaredDouble{}, nil, fixedClock{}); err == nil {
		t.Fatal("没有物理来源的合成被收下")
	}
	if _, err := adapter.NewPointedParcelFacts(&declaredDouble{}, &physicalDouble{}, nil); err == nil {
		t.Fatal("没有时钟的合成被收下")
	}
}
