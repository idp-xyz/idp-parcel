package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var (
	allocatedAt = time.Date(2026, 8, 31, 20, 0, 0, 0, time.UTC)
	derivedAsOf = time.Date(2026, 8, 31, 23, 59, 0, 0, time.UTC)
)

func allocationSpec(t *testing.T, portions []domain.AllocationPortion) domain.CostAllocationSpec {
	t.Helper()
	return domain.CostAllocationSpec{
		ID:          settlementValue(t, domain.NewAllocationID, "allocation-1"),
		Source:      settlementValue(t, domain.NewAllocationSourceReference, "payable-1"),
		SourceMinor: 10000,
		Currency:    settlementValue(t, domain.NewCurrencyCode, "USD"),
		Rule:        settlementValue(t, domain.NewAllocationRuleVersionReference, "allocation-rule/v1"),
		Portions:    portions,
		Version:     settlementValue(t, domain.NewAllocationVersion, "allocation/v1"),
		AllocatedAt: allocatedAt,
	}
}

func portion(t *testing.T, target string, amountMinor int64) domain.AllocationPortion {
	t.Helper()
	return domain.AllocationPortion{
		Target:      settlementValue(t, domain.NewAllocationTargetReference, target),
		AmountMinor: amountMinor,
	}
}

// Covers: `AT-SA-122`「共享成本有明确分母 → 守恒分摊」、`AT-SA-123`/`AT-SA-124`「无合格
// 对象/分母为零 → 保留全额未分摊，不虚构对象不平均分配」与输入契约「缺规则不使用默认
// 比例」——规则必备专格；份额+未分摊恒等于来源；分摊只引用来源。
func TestAllocationNeedsAVersionedRule(t *testing.T) {
	allocation, err := domain.FormCostAllocation(allocationSpec(t, []domain.AllocationPortion{
		portion(t, "parcel-1", 6000),
		portion(t, "parcel-2", 3999),
	}))
	if err != nil {
		t.Fatalf("form allocation: %v", err)
	}
	// 计数锚定本夹具：来源 10000 = 6000+3999+尾差 1（USD，摊于 2026-08-31T20:00Z）。
	if allocation.UnallocatedMinor() != 1 {
		t.Fatalf("unallocated = %d, want 1（尾差显式保留）", allocation.UnallocatedMinor())
	}

	t.Run("without a rule nothing allocates", func(t *testing.T) {
		spec := allocationSpec(t, []domain.AllocationPortion{portion(t, "parcel-1", 5000)})
		spec.Rule = domain.AllocationRuleVersionReference{}
		if _, err := domain.FormCostAllocation(spec); !errors.Is(err, domain.ErrAllocationNeedsARule) {
			t.Fatalf("error = %v, want ErrAllocationNeedsARule（缺规则不均摊）", err)
		}
	})

	t.Run("no eligible objects keeps the full amount unallocated", func(t *testing.T) {
		empty, err := domain.FormCostAllocation(allocationSpec(t, nil))
		if err != nil {
			t.Fatalf("form empty allocation: %v", err)
		}
		if empty.UnallocatedMinor() != 10000 || len(empty.Portions()) != 0 {
			t.Fatalf("unallocated = %d portions = %d（不虚构对象）", empty.UnallocatedMinor(), len(empty.Portions()))
		}
	})

	t.Run("portions beyond the source are refused", func(t *testing.T) {
		if _, err := domain.FormCostAllocation(allocationSpec(t, []domain.AllocationPortion{
			portion(t, "parcel-1", 6000),
			portion(t, "parcel-2", 5000),
		})); !errors.Is(err, domain.ErrAllocationImbalance) {
			t.Fatalf("error = %v, want ErrAllocationImbalance", err)
		}
	})

	t.Run("a duplicate target is refused", func(t *testing.T) {
		if _, err := domain.FormCostAllocation(allocationSpec(t, []domain.AllocationPortion{
			portion(t, "parcel-1", 3000),
			portion(t, "parcel-1", 2000),
		})); !errors.Is(err, domain.ErrInvalidCostAllocation) {
			t.Fatalf("error = %v; 同一目标被摊了两次", err)
		}
	})
}

// Covers: `AT-SA-126`「分摊规则版本更正 → 原分摊保留，形成新版本和差额」与 `AT-SA-138`
// 「同一来源重复提交 → 返回原分摊，不重复归因」的版本半边——重分摊换版本回指前身，
// 来源金额与身份不变。
func TestReallocationKeepsTheOriginalVersion(t *testing.T) {
	original, err := domain.FormCostAllocation(allocationSpec(t, []domain.AllocationPortion{
		portion(t, "parcel-1", 6000),
		portion(t, "parcel-2", 4000),
	}))
	if err != nil {
		t.Fatalf("form allocation: %v", err)
	}

	reallocated, err := original.Reallocate(
		settlementValue(t, domain.NewAllocationRuleVersionReference, "allocation-rule/v2"),
		[]domain.AllocationPortion{
			portion(t, "parcel-1", 7000),
			portion(t, "parcel-2", 3000),
		},
		settlementValue(t, domain.NewAllocationVersion, "allocation/v2"),
		allocatedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("reallocate: %v", err)
	}
	predecessor, present := reallocated.Corrects()
	if !present || predecessor.String() != "allocation/v1" {
		t.Fatalf("corrects = %q present=%v, want v1", predecessor, present)
	}
	if reallocated.Source() != original.Source() {
		t.Fatal("重分摊换了来源——归因改成了改钱")
	}
	if _, sourceMinor := reallocated.SourceAmount(); sourceMinor != 10000 {
		t.Fatalf("source = %d, want 10000（来源金额不变）", sourceMinor)
	}
	if original.Portions()[0].AmountMinor != 6000 {
		t.Fatal("重分摊改写了原分摊")
	}

	t.Run("reusing the original version is an overwrite and is refused", func(t *testing.T) {
		if _, err := original.Reallocate(
			original.Rule(), original.Portions(), original.Version(), allocatedAt.Add(time.Hour),
		); !errors.Is(err, domain.ErrInvalidCostAllocation) {
			t.Fatalf("error = %v; 沿用原版本号就是覆盖", err)
		}
	})
}

// Covers: UC-SA-006「经营毛利/损失已派生……不直接编辑或只存净额」与 `AT-SA-137`「迟到
// 成本在截点后到达 → 原指标快照不变，新版本关联原截点」——毛利只由组成算出（规格里
// 没有毛利字段），类型上没有任何改数方法；重派生换版本回指前身。
func TestOperatingResultIsDerivedNotEdited(t *testing.T) {
	resultType := reflect.TypeOf(domain.OperatingResult{})
	for index := 0; index < resultType.NumMethod(); index++ {
		name := strings.ToLower(resultType.Method(index).Name)
		for _, banned := range []string{"set", "edit", "override", "adjustmargin"} {
			if strings.HasPrefix(name, banned) {
				t.Fatalf("OperatingResult 带方法 %q——派生指标就有了直接编辑的入口", name)
			}
		}
	}

	components := []domain.ResultComponent{
		{
			Source:      settlementValue(t, domain.NewComponentSourceReference, "customer-charge-1"),
			Effect:      domain.IncreasesResult,
			AmountMinor: 15000,
		},
		{
			Source:      settlementValue(t, domain.NewComponentSourceReference, "payable-1"),
			Effect:      domain.DecreasesResult,
			AmountMinor: 10000,
		},
		{
			Source:      settlementValue(t, domain.NewComponentSourceReference, "credit-note-1"),
			Effect:      domain.IncreasesResult,
			AmountMinor: 2000,
		},
	}
	result, err := domain.DeriveOperatingResult(
		settlementValue(t, domain.NewOperatingScopeReference, "customer-1"),
		settlementValue(t, domain.NewBillingPeriodReference, "period-2026-08"),
		domain.ConfirmedBasis,
		settlementValue(t, domain.NewCurrencyCode, "USD"),
		components,
		settlementValue(t, domain.NewOperatingResultVersion, "result/v1"),
		derivedAsOf,
	)
	if err != nil {
		t.Fatalf("derive result: %v", err)
	}
	// 计数锚定本夹具：15000−10000+2000 = 7000（已确认口径，截至 2026-08-31T23:59Z；
	// 应付与贷项按各自方向分别计入，不静默净额）。
	if _, margin := result.Margin(); margin != 7000 {
		t.Fatalf("margin = %d, want 7000", margin)
	}
	if len(result.Components()) != 3 {
		t.Fatalf("components = %d, want 3（组成保留，不只存净额）", len(result.Components()))
	}

	t.Run("a late cost rederives a new version keeping the snapshot", func(t *testing.T) {
		late := append(append([]domain.ResultComponent(nil), components...), domain.ResultComponent{
			Source:      settlementValue(t, domain.NewComponentSourceReference, "late-cost-1"),
			Effect:      domain.DecreasesResult,
			AmountMinor: 1000,
		})
		rederived, err := result.Rederive(late,
			settlementValue(t, domain.NewOperatingResultVersion, "result/v2"),
			derivedAsOf.Add(24*time.Hour))
		if err != nil {
			t.Fatalf("rederive: %v", err)
		}
		predecessor, present := rederived.Corrects()
		if !present || predecessor.String() != "result/v1" {
			t.Fatalf("corrects = %q present=%v", predecessor, present)
		}
		if _, margin := rederived.Margin(); margin != 6000 {
			t.Fatalf("rederived margin = %d, want 6000", margin)
		}
		if _, margin := result.Margin(); margin != 7000 {
			t.Fatal("重派生改写了原快照")
		}
		if _, err := result.Rederive(components, result.Version(), derivedAsOf); !errors.Is(err, domain.ErrInvalidOperatingResult) {
			t.Fatalf("error = %v; 沿用原版本号就是覆盖", err)
		}
	})

	t.Run("empty components derive nothing", func(t *testing.T) {
		if _, err := domain.DeriveOperatingResult(
			settlementValue(t, domain.NewOperatingScopeReference, "customer-1"),
			settlementValue(t, domain.NewBillingPeriodReference, "period-2026-08"),
			domain.ConfirmedBasis,
			settlementValue(t, domain.NewCurrencyCode, "USD"),
			nil,
			settlementValue(t, domain.NewOperatingResultVersion, "result/vx"),
			derivedAsOf,
		); !errors.Is(err, domain.ErrInvalidOperatingResult) {
			t.Fatalf("error = %v; 没有组成的指标是编出来的数", err)
		}
	})

	t.Run("the basis set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, basis := range []domain.OperatingBasis{
			domain.EstimatedBasis, domain.ConfirmedBasis, domain.SettledBasis,
		} {
			label := basis.String()
			if label == "" {
				t.Fatalf("basis %d has no label", basis)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 3 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if domain.OperatingBasis(len(labels)+1).String() != "" {
			t.Fatal("第四个口径带了标签——封闭集合被悄悄放开")
		}
	})
}
