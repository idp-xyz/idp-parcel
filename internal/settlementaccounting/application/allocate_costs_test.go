package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	allocationAt   = time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)
	operatingAsOf  = time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC)
	operatingNowAt = time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
)

type allocationStoreDouble struct {
	records map[string]ports.AllocationRecord
	findErr error
}

func newAllocationStore() *allocationStoreDouble {
	return &allocationStoreDouble{records: map[string]ports.AllocationRecord{}}
}

func allocationStoreKey(key ports.AllocationKey) string {
	return key.TenantID.String() + "|" + key.Allocation.String()
}

func (double *allocationStoreDouble) FindByKey(
	_ context.Context,
	key ports.AllocationKey,
) (ports.AllocationRecord, bool, error) {
	if double.findErr != nil {
		return ports.AllocationRecord{}, false, double.findErr
	}
	record, found := double.records[allocationStoreKey(key)]
	return record, found, nil
}

func (double *allocationStoreDouble) Save(
	_ context.Context,
	record ports.AllocationRecord,
) (ports.AllocationSaveOutcome, error) {
	if _, exists := double.records[allocationStoreKey(record.Key)]; exists {
		return ports.AllocationAlreadyFormed, nil
	}
	double.records[allocationStoreKey(record.Key)] = record
	return ports.AllocationSaved, nil
}

func (double *allocationStoreDouble) Replace(
	_ context.Context,
	record ports.AllocationRecord,
) (bool, error) {
	if _, exists := double.records[allocationStoreKey(record.Key)]; !exists {
		return false, nil
	}
	double.records[allocationStoreKey(record.Key)] = record
	return true, nil
}

type resultStoreDouble struct {
	records map[string]ports.OperatingResultRecord
	findErr error
	saves   int
}

func newResultStore() *resultStoreDouble {
	return &resultStoreDouble{records: map[string]ports.OperatingResultRecord{}}
}

func resultStoreKey(key ports.OperatingResultKey) string {
	return key.TenantID.String() + "|" + key.Scope.String() + "|" + key.Period.String() + "|" + key.Basis.String()
}

func (double *resultStoreDouble) FindByKey(
	_ context.Context,
	key ports.OperatingResultKey,
) (ports.OperatingResultRecord, bool, error) {
	if double.findErr != nil {
		return ports.OperatingResultRecord{}, false, double.findErr
	}
	record, found := double.records[resultStoreKey(key)]
	return record, found, nil
}

func (double *resultStoreDouble) Save(
	_ context.Context,
	record ports.OperatingResultRecord,
) (ports.OperatingResultSaveOutcome, error) {
	double.saves++
	if _, exists := double.records[resultStoreKey(record.Key)]; exists {
		return ports.OperatingResultAlreadyDerived, nil
	}
	double.records[resultStoreKey(record.Key)] = record
	return ports.OperatingResultSaved, nil
}

func (double *resultStoreDouble) Replace(
	_ context.Context,
	record ports.OperatingResultRecord,
) (bool, error) {
	if _, exists := double.records[resultStoreKey(record.Key)]; !exists {
		return false, nil
	}
	double.records[resultStoreKey(record.Key)] = record
	return true, nil
}

type ruleViewDouble struct {
	configured bool
	err        error
}

func (double *ruleViewDouble) LoadAllocationRule(
	_ context.Context,
	_ domain.TenantID,
	_ domain.AllocationSourceReference,
) (domain.AllocationRuleVersionReference, bool, error) {
	if double.err != nil {
		return domain.AllocationRuleVersionReference{}, false, double.err
	}
	if !double.configured {
		return domain.AllocationRuleVersionReference{}, false, nil
	}
	rule, err := domain.NewAllocationRuleVersionReference("allocation-rule/v3")
	return rule, true, err
}

type operatingHandoffDouble struct {
	intents []ports.OperatingIntent
	err     error
}

func (double *operatingHandoffDouble) HandOffOperating(
	_ context.Context,
	intent ports.OperatingIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type operatingClock struct{ at time.Time }

func (clock operatingClock) Now() time.Time { return clock.at }

type operatingFixture struct {
	allocations *allocationStoreDouble
	results     *resultStoreDouble
	rules       *ruleViewDouble
	handoff     *operatingHandoffDouble
	handler     *application.AllocateCostsHandler
}

func newOperatingFixture(t *testing.T) *operatingFixture {
	t.Helper()
	fixture := &operatingFixture{
		allocations: newAllocationStore(),
		results:     newResultStore(),
		rules:       &ruleViewDouble{configured: true},
		handoff:     &operatingHandoffDouble{},
	}
	fixture.handler = application.NewAllocateCostsHandler(application.AllocateCostsDeps{
		Allocations: fixture.allocations,
		Results:     fixture.results,
		Rules:       fixture.rules,
		Downstream:  fixture.handoff,
		Clock:       operatingClock{at: operatingNowAt},
	})
	return fixture
}

func allocateCommand(t *testing.T) application.AllocateCostCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.AllocateCostCommand{
		TenantID:    tenant,
		Allocation:  "allocation-1",
		Source:      "linehaul-cost-1",
		SourceMinor: 10000,
		Currency:    "USD",
		Portions: []application.PortionDirective{
			{Target: "customer-1", AmountMinor: 6000},
			{Target: "customer-2", AmountMinor: 3000},
		},
		Version:     "allocation-1/v1",
		AllocatedAt: allocationAt,
	}
}

func deriveCommand(t *testing.T) application.DeriveResultCommand {
	t.Helper()
	tenant, _ := domain.NewTenantID("tenant-1")
	return application.DeriveResultCommand{
		TenantID: tenant,
		Scope:    "customer-1",
		Period:   "2026-08",
		Basis:    domain.ConfirmedBasis,
		Currency: "USD",
		Components: []application.ComponentDirective{
			{Source: "confirmed-revenue-1", Effect: domain.IncreasesResult, AmountMinor: 20000},
			{Source: "allocated-cost-1", Effect: domain.DecreasesResult, AmountMinor: 12000},
		},
		Version: "result/v1",
		AsOf:    operatingAsOf,
	}
}

// Covers: `AT-SA-123/124`「无规则不分摊、不默认均摊；无合格对象全额保留为未分摊余额」
// 的编排面——规则版本由视图核对（未配置→未决）；守恒领域把门（份额+余额恒等来源，
// 超额未受理）；幂等分重放/冲突。
func TestAllocationNeedsAConfiguredRule(t *testing.T) {
	fixture := newOperatingFixture(t)
	command := allocateCommand(t)

	first, err := fixture.handler.Allocate(context.Background(), command)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if first.Outcome() != application.CostAllocated {
		t.Fatalf("outcome = %q, want COST_ALLOCATED", first.Outcome())
	}
	record, _ := first.Allocation()
	// 余额锚定本夹具：10000-6000-3000=1000 保留为未分摊。
	if record.Allocation.UnallocatedMinor() != 1000 {
		t.Fatalf("unallocated = %d, want 1000", record.Allocation.UnallocatedMinor())
	}
	if record.Allocation.Rule().String() != "allocation-rule/v3" {
		t.Fatal("规则版本没有来自视图核对")
	}

	t.Run("an unconfigured rule catalogue is undecided", func(t *testing.T) {
		unconfigured := newOperatingFixture(t)
		unconfigured.rules.configured = false
		result, err := unconfigured.handler.Allocate(context.Background(), allocateCommand(t))
		if err != nil {
			t.Fatalf("allocate: %v", err)
		}
		if result.Outcome() != application.OperatingUndecided ||
			result.UndecidedReason() != application.RuleUnconfigured {
			t.Fatalf("outcome = %q reason = %q（不默认均摊）", result.Outcome(), result.UndecidedReason())
		}
		if len(unconfigured.allocations.records) != 0 {
			t.Fatal("未配置还分了摊")
		}
	})

	t.Run("portions exceeding the source are not accepted", func(t *testing.T) {
		over := allocateCommand(t)
		over.Allocation = "allocation-2"
		over.Portions[0].AmountMinor = 10001
		result, err := fixture.handler.Allocate(context.Background(), over)
		if err != nil {
			t.Fatalf("allocate over: %v", err)
		}
		if result.Outcome() != application.OperatingNotAccepted {
			t.Fatalf("outcome = %q（份额之和不得超过来源）", result.Outcome())
		}
	})

	t.Run("a replay returns the original and a flip is a conflict", func(t *testing.T) {
		replay, err := fixture.handler.Allocate(context.Background(), command)
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.AllocationExistingResult {
			t.Fatalf("outcome = %q", replay.Outcome())
		}
		flipped := allocateCommand(t)
		flipped.Portions[0].AmountMinor = 5000
		conflict, err := fixture.handler.Allocate(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict: %v", err)
		}
		if conflict.Outcome() != application.AllocationConflict {
			t.Fatalf("outcome = %q（改法走重分摊换版本）", conflict.Outcome())
		}
	})
}

// Covers: UC-SA-006「重分摊换版本保留原分摊、来源只引用不修改」的编排面——新版本
// 回指前版；无中生有拒；规则重新核对。
func TestReallocationKeepsTheChain(t *testing.T) {
	fixture := newOperatingFixture(t)
	if _, err := fixture.handler.Allocate(context.Background(), allocateCommand(t)); err != nil {
		t.Fatalf("allocate: %v", err)
	}
	tenant, _ := domain.NewTenantID("tenant-1")

	reallocated, err := fixture.handler.Reallocate(context.Background(), application.ReallocateCommand{
		TenantID:   tenant,
		Allocation: "allocation-1",
		Portions: []application.PortionDirective{
			{Target: "customer-1", AmountMinor: 5000},
			{Target: "customer-2", AmountMinor: 5000},
		},
		NewVersion:  "allocation-1/v2",
		AllocatedAt: allocationAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("reallocate: %v", err)
	}
	if reallocated.Outcome() != application.Reallocated {
		t.Fatalf("outcome = %q, want REALLOCATED", reallocated.Outcome())
	}
	record, _ := reallocated.Allocation()
	predecessor, corrected := record.Allocation.Corrects()
	if !corrected || predecessor.String() != "allocation-1/v1" {
		t.Fatalf("corrects = %q（新版本回指前版）", predecessor)
	}
	if _, sourceMinor := record.Allocation.SourceAmount(); sourceMinor != 10000 {
		t.Fatal("重分摊改了来源金额")
	}

	t.Run("reallocating an absent allocation is refused", func(t *testing.T) {
		missing, err := fixture.handler.Reallocate(context.Background(), application.ReallocateCommand{
			TenantID:    tenant,
			Allocation:  "allocation-9",
			Portions:    []application.PortionDirective{{Target: "customer-1", AmountMinor: 100}},
			NewVersion:  "allocation-9/v2",
			AllocatedAt: allocationAt.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("reallocate absent: %v", err)
		}
		if missing.Outcome() != application.OperatingNotAccepted {
			t.Fatalf("outcome = %q; 重分出了无中生有的归因", missing.Outcome())
		}
	})
}

// Covers: `AT-SA-137`「迟到成本在截点后到达：原快照不变，新版本关联原截点」与 UC-SA-006
// 「指标是派生不可编辑」的编排面——净额只由组成项算出；同口径一版一登重放返原；重派生
// 换版本回指原版。
func TestOperatingResultsDeriveAndRederive(t *testing.T) {
	fixture := newOperatingFixture(t)
	command := deriveCommand(t)

	derived, err := fixture.handler.Derive(context.Background(), command)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if derived.Outcome() != application.ResultDerived {
		t.Fatalf("outcome = %q, want RESULT_DERIVED", derived.Outcome())
	}
	record, _ := derived.Result()
	// 净额锚定本夹具：20000-12000=8000。
	if _, margin := record.Result.Margin(); margin != 8000 {
		t.Fatalf("margin = %d, want 8000（净额只由组成项算出）", margin)
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(fixture.handoff.intents))
	}

	t.Run("a replay returns the original snapshot", func(t *testing.T) {
		replay, err := fixture.handler.Derive(context.Background(), command)
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.ResultExistingResult || fixture.results.saves != 1 {
			t.Fatalf("outcome = %q saves = %d（重放不重派）", replay.Outcome(), fixture.results.saves)
		}
	})

	t.Run("a late cost rederives into a new version", func(t *testing.T) {
		tenant, _ := domain.NewTenantID("tenant-1")
		rederived, err := fixture.handler.Rederive(context.Background(), application.RederiveResultCommand{
			TenantID: tenant,
			Scope:    "customer-1",
			Period:   "2026-08",
			Basis:    domain.ConfirmedBasis,
			Components: []application.ComponentDirective{
				{Source: "confirmed-revenue-1", Effect: domain.IncreasesResult, AmountMinor: 20000},
				{Source: "allocated-cost-1", Effect: domain.DecreasesResult, AmountMinor: 12000},
				{Source: "late-cost-1", Effect: domain.DecreasesResult, AmountMinor: 3000},
			},
			NewVersion: "result/v2",
			AsOf:       operatingAsOf.Add(48 * time.Hour),
		})
		if err != nil {
			t.Fatalf("rederive: %v", err)
		}
		if rederived.Outcome() != application.ResultRederived {
			t.Fatalf("outcome = %q, want RESULT_REDERIVED", rederived.Outcome())
		}
		record, _ := rederived.Result()
		predecessor, corrected := record.Result.Corrects()
		if !corrected || predecessor.String() != "result/v1" {
			t.Fatalf("corrects = %q（新版本关联原截点）", predecessor)
		}
		if _, margin := record.Result.Margin(); margin != 5000 {
			t.Fatalf("margin = %d, want 5000", margin)
		}
	})

	t.Run("rederiving an absent snapshot is refused", func(t *testing.T) {
		tenant, _ := domain.NewTenantID("tenant-1")
		missing, err := fixture.handler.Rederive(context.Background(), application.RederiveResultCommand{
			TenantID:   tenant,
			Scope:      "customer-9",
			Period:     "2026-08",
			Basis:      domain.ConfirmedBasis,
			Components: []application.ComponentDirective{{Source: "x", Effect: domain.IncreasesResult, AmountMinor: 1}},
			NewVersion: "result/v2",
			AsOf:       operatingAsOf.Add(48 * time.Hour),
		})
		if err != nil {
			t.Fatalf("rederive absent: %v", err)
		}
		if missing.Outcome() != application.OperatingNotAccepted {
			t.Fatalf("outcome = %q", missing.Outcome())
		}
	})
}

// Covers: ADR-0029（依赖故障归未决且指名等谁）、ADR-0031（写入代数封闭）与 ADR-0043
// （投递失败不翻结果、重放重发同一份）在本编排的恢复面；未决原因集封闭。
func TestOperatingRecoveryDiscipline(t *testing.T) {
	t.Run("store and view failures are undecided with their reasons", func(t *testing.T) {
		fixture := newOperatingFixture(t)
		fixture.rules.err = errors.New("view down")
		result, err := fixture.handler.Allocate(context.Background(), allocateCommand(t))
		if err != nil {
			t.Fatalf("allocate: %v", err)
		}
		if result.UndecidedReason() != application.RuleViewUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}

		storeFixture := newOperatingFixture(t)
		storeFixture.results.findErr = errors.New("store down")
		result, err = storeFixture.handler.Derive(context.Background(), deriveCommand(t))
		if err != nil {
			t.Fatalf("derive: %v", err)
		}
		if result.UndecidedReason() != application.ResultStoreUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}
	})

	t.Run("a handoff failure keeps the outcome and is resent on replay", func(t *testing.T) {
		fixture := newOperatingFixture(t)
		fixture.handoff.err = errors.New("downstream unavailable")
		first, err := fixture.handler.Derive(context.Background(), deriveCommand(t))
		if err != nil {
			t.Fatalf("derive: %v", err)
		}
		if first.Outcome() != application.ResultDerived || first.OperatingHandoffReference() == "" {
			t.Fatalf("outcome = %q handoff = %q（投递失败不翻结果）", first.Outcome(), first.OperatingHandoffReference())
		}
		fixture.handoff.err = nil
		replay, err := fixture.handler.Derive(context.Background(), deriveCommand(t))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.OperatingHandoffReference() != "" || len(fixture.handoff.intents) != 1 {
			t.Fatalf("intents = %d handoff = %q", len(fixture.handoff.intents), replay.OperatingHandoffReference())
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.OperatingUndecidedReason{
			application.AllocationStoreUnavailable, application.ResultStoreUnavailable,
			application.RuleViewUnavailable, application.RuleUnconfigured,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 4 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.OperatingUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第五个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
