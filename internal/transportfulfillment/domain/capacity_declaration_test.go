package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

func capacityDeclaration(t *testing.T, dimension domain.CapacityDimension, unit string, capacity int64) domain.CapacityDeclaration {
	t.Helper()
	return domain.CapacityDeclaration{
		Dimension: dimension,
		Unit:      mustValue(t, domain.NewQuantityUnitReference, unit),
		Capacity:  capacity,
	}
}

// Covers: ADR-0201 补记「容量维度封闭词」——六个维度词各认得回自己；大小写变体、空串与列举之外的词都拒，不折成
// 任何一维。
func TestCapacityDimensionsAreAClosedVocabulary(t *testing.T) {
	for _, dimension := range []domain.CapacityDimension{
		domain.CapacityWeight, domain.CapacityVolume, domain.CapacityPieces,
		domain.CapacityPalletPositions, domain.CapacityBagPositions, domain.CapacityCargoSlots,
	} {
		parsed, err := domain.ParseCapacityDimension(dimension.String())
		if err != nil || parsed != dimension {
			t.Fatalf("%s 认回 = %v %v", dimension, parsed, err)
		}
	}
	for _, word := range []string{"", "weight", "Weight", "TEU", "WEIGHT "} {
		if _, err := domain.ParseCapacityDimension(word); !errors.Is(err, domain.ErrInvalidCapacityDeclaration) {
			t.Fatalf("%q 应拒：%v", word, err)
		}
	}
}

// Covers: `AT-TF-030`「重量和袋位容量同时适用 → 两个维度分别保存，不强制折算为一个数字」——同一班次上两个维度是
// 两个池，各有自己的身份、单位与硬上限，一维的预占不动另一维；池身份由（班次、维度）派生，同一（班次、维度）两次
// 立出同一个身份。
func TestEachDimensionIsItsOwnPoolOnTheSameSchedule(t *testing.T) {
	spec := func(dimension domain.CapacityDimension, unit string, capacity int64) domain.CapacityPoolSpec {
		return domain.CapacityPoolSpec{
			TenantID:  mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
			Schedule:  mustValue(t, domain.NewScheduleReference, "TPL/2026-03-04"),
			Dimension: dimension,
			Unit:      mustValue(t, domain.NewQuantityUnitReference, unit),
			Capacity:  capacity,
		}
	}
	weight, err := domain.EstablishCapacityPool(spec(domain.CapacityWeight, "KG", 5000))
	if err != nil {
		t.Fatal(err)
	}
	bags, err := domain.EstablishCapacityPool(spec(domain.CapacityBagPositions, "BAG", 40))
	if err != nil {
		t.Fatal(err)
	}
	if weight.Pool().String() != "TPL/2026-03-04/WEIGHT" || bags.Pool().String() != "TPL/2026-03-04/BAG_POSITIONS" {
		t.Fatalf("池身份 = %s / %s", weight.Pool(), bags.Pool())
	}
	again, err := domain.EstablishCapacityPool(spec(domain.CapacityWeight, "KG", 5000))
	if err != nil || again.Pool() != weight.Pool() {
		t.Fatalf("同一（班次、维度）再立 = %s %v", again.Pool(), err)
	}

	reserved, err := weight.Reserve(reservationRef(t, "reservation-1"), 4000, reservedUntil, reservedAt)
	if err != nil {
		t.Fatal(err)
	}
	if reserved.AvailableAt(reservedAt) != 1000 || bags.AvailableAt(reservedAt) != 40 {
		t.Fatalf("重量预占动了袋位：重量余 %d，袋位余 %d", reserved.AvailableAt(reservedAt), bags.AvailableAt(reservedAt))
	}

	if _, err := domain.EstablishCapacityPool(spec(domain.CapacityDimensionInvalid, "KG", 5000)); !errors.Is(err, domain.ErrInvalidCapacityPool) {
		t.Fatalf("无维度的池应拒：%v", err)
	}
}

// Covers: ADR-0201 决定三「可选的逐维容量声明」——声明按维度次序归一（列举顺序不同的同一组声明是同一份正文），
// 同一维度声明两次、硬上限不为正、缺单位都拒；交出去的正文是拷贝，改它改不到周期安排本体。
func TestTemplateCapacityDeclarationsAreGated(t *testing.T) {
	content := templateContentFor(t, everyDay(), "")
	content.Capacities = []domain.CapacityDeclaration{
		capacityDeclaration(t, domain.CapacityPieces, "PCS", 800),
		capacityDeclaration(t, domain.CapacityWeight, "KG", 5000),
	}
	template := registeredTemplate(t, content)
	declared := template.Content().Capacities
	if len(declared) != 2 || declared[0].Dimension != domain.CapacityWeight || declared[1].Dimension != domain.CapacityPieces {
		t.Fatalf("声明未按维度次序归一：%+v", declared)
	}
	declared[0].Capacity = 1
	if template.Content().Capacities[0].Capacity != 5000 {
		t.Fatal("改交出去的正文改到了周期安排本体")
	}

	for name, capacities := range map[string][]domain.CapacityDeclaration{
		"同一维度两次": {
			capacityDeclaration(t, domain.CapacityWeight, "KG", 5000),
			capacityDeclaration(t, domain.CapacityWeight, "LB", 11000),
		},
		"硬上限为零": {capacityDeclaration(t, domain.CapacityWeight, "KG", 0)},
		"缺单位":   {{Dimension: domain.CapacityWeight, Capacity: 5000}},
		"缺维度":   {{Unit: mustValue(t, domain.NewQuantityUnitReference, "KG"), Capacity: 5000}},
	} {
		broken := templateContentFor(t, everyDay(), "")
		broken.Capacities = capacities
		if _, err := domain.RegisterScheduleTemplate(domain.ScheduleTemplateSpec{
			TenantID: mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
			Code:     mustValue(t, domain.NewScheduleTemplateCode, "TPL"),
			Content:  broken,
		}); !errors.Is(err, domain.ErrInvalidCapacityDeclaration) {
			t.Fatalf("%s：应拒，实得 %v", name, err)
		}
	}

	_, ordered, err := domain.CanonicalizeScheduleTemplatePayload(content)
	if err != nil {
		t.Fatal(err)
	}
	reversed := content
	reversed.Capacities = []domain.CapacityDeclaration{content.Capacities[1], content.Capacities[0]}
	if _, digest, err := domain.CanonicalizeScheduleTemplatePayload(reversed); err != nil || digest != ordered {
		t.Fatalf("列举顺序不同的同一组声明摘要不同：%s / %s %v", ordered, digest, err)
	}
	_, bare, err := domain.CanonicalizeScheduleTemplatePayload(templateContentFor(t, everyDay(), ""))
	if err != nil || bare == ordered {
		t.Fatalf("声不声明容量摘要应不同：%s %v", bare, err)
	}
}

// Covers: ADR-0201 决定三「物化时随班次建池」——周期安排在它物化出的那一班上逐维各立一个空池，身份由（班次、维度）
// 派生，同一（周期安排、日期）两次物化得同一组池；不声明容量的周期安排不立池；别的周期安排或临时班次上的池不归
// 这份声明定。
func TestMaterializingCapacityPoolsFollowsTheDeclarations(t *testing.T) {
	content := templateContentFor(t, everyDay(), "")
	content.Capacities = []domain.CapacityDeclaration{
		capacityDeclaration(t, domain.CapacityWeight, "KG", 5000),
		capacityDeclaration(t, domain.CapacityPieces, "PCS", 800),
	}
	template := registeredTemplate(t, content)
	schedule, err := domain.MaterializeSchedule(template, mustRunningDate(t, "2026-03-04"))
	if err != nil {
		t.Fatal(err)
	}
	pools, err := template.MaterializeCapacityPools(schedule)
	if err != nil || len(pools) != 2 {
		t.Fatalf("物化池 = %d %v", len(pools), err)
	}
	if pools[0].Pool().String() != "TPL/2026-03-04/WEIGHT" || pools[0].Capacity() != 5000 || pools[0].Unit().String() != "KG" ||
		pools[1].Pool().String() != "TPL/2026-03-04/PIECES" || pools[1].Capacity() != 800 {
		t.Fatalf("物化池 = %s %d / %s %d", pools[0].Pool(), pools[0].Capacity(), pools[1].Pool(), pools[1].Capacity())
	}
	for _, pool := range pools {
		if pool.Schedule() != schedule.Schedule() || len(pool.ReservationSnapshots()) != 0 {
			t.Fatalf("池 %s 不在那一班上或不是空池", pool.Pool())
		}
	}
	again, err := template.MaterializeCapacityPools(schedule)
	if err != nil || len(again) != 2 || again[0].Pool() != pools[0].Pool() || again[1].Pool() != pools[1].Pool() {
		t.Fatalf("同一班再物化池 = %v %v", again, err)
	}

	bare := registeredTemplate(t, templateContentFor(t, everyDay(), ""))
	bareSchedule, err := domain.MaterializeSchedule(bare, mustRunningDate(t, "2026-03-04"))
	if err != nil {
		t.Fatal(err)
	}
	if none, err := bare.MaterializeCapacityPools(bareSchedule); err != nil || len(none) != 0 {
		t.Fatalf("不声明容量的周期安排立了池：%d %v", len(none), err)
	}

	adHoc := adHocScheduleFor(t, "SCH-ADHOC", reservedAt, reservedAt.Add(2*time.Hour))
	if _, err := template.MaterializeCapacityPools(adHoc); !errors.Is(err, domain.ErrInvalidCapacityPool) {
		t.Fatalf("临时班次上不该按周期安排立池：%v", err)
	}
	other, err := domain.RegisterScheduleTemplate(domain.ScheduleTemplateSpec{
		TenantID: mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Code:     mustValue(t, domain.NewScheduleTemplateCode, "TPL-OTHER"),
		Content:  content,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.MaterializeCapacityPools(schedule); !errors.Is(err, domain.ErrInvalidCapacityPool) {
		t.Fatalf("别的周期安排物化出的班次不归这份声明：%v", err)
	}
}
