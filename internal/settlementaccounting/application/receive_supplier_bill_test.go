package application_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	billArrivedAt  = time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	billRecordedAt = time.Date(2026, 8, 12, 9, 30, 0, 0, time.UTC)
)

func billValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type billStoreDouble struct {
	records     map[string]ports.BillReceptionRecord
	findErr     error
	saveErr     error
	saveResult  ports.BillSaveOutcome
	forceResult bool
	missNext    bool
	saves       int
}

func newBillStore() *billStoreDouble {
	return &billStoreDouble{records: map[string]ports.BillReceptionRecord{}}
}

func billStoreKey(key ports.BillReceptionKey) string {
	return key.TenantID.String() + "|" + key.Claim.String() + "|" + key.Version.String()
}

func (double *billStoreDouble) FindByKey(
	_ context.Context,
	key ports.BillReceptionKey,
) (ports.BillReceptionRecord, bool, error) {
	if double.findErr != nil {
		return ports.BillReceptionRecord{}, false, double.findErr
	}
	if double.missNext {
		double.missNext = false
		return ports.BillReceptionRecord{}, false, nil
	}
	record, found := double.records[billStoreKey(key)]
	return record, found, nil
}

func (double *billStoreDouble) Save(
	_ context.Context,
	record ports.BillReceptionRecord,
) (ports.BillSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.BillSaveOutcomeInvalid, double.saveErr
	}
	if double.forceResult {
		return double.saveResult, nil
	}
	if _, exists := double.records[billStoreKey(record.Key)]; exists {
		return ports.BillAlreadyRecorded, nil
	}
	double.records[billStoreKey(record.Key)] = record
	return ports.BillSaved, nil
}

type costViewDouble struct {
	costs map[string]domain.SupplierExpectedCost
	err   error
}

func (double *costViewDouble) LoadExpectedCost(
	_ context.Context,
	_ domain.TenantID,
	version domain.SupplierCostVersionID,
) (domain.SupplierExpectedCost, bool, error) {
	if double.err != nil {
		return domain.SupplierExpectedCost{}, false, double.err
	}
	cost, found := double.costs[version.String()]
	return cost, found, nil
}

type authorityViewDouble struct {
	configured bool
	err        error
}

func (double *authorityViewDouble) LoadSupplierAuditAuthority(
	_ context.Context,
	_ domain.TenantID,
	_ domain.SupplierPartyReference,
	_ domain.LegalEntityReference,
) (domain.AuditorReference, bool, error) {
	if double.err != nil {
		return domain.AuditorReference{}, false, double.err
	}
	if !double.configured {
		return domain.AuditorReference{}, false, nil
	}
	auditor, err := domain.NewAuditorReference("auditor-1")
	return auditor, true, err
}

type billHandoffDouble struct {
	intents []ports.SupplierBillHandoffIntent
	err     error
}

func (double *billHandoffDouble) HandOffSupplierBill(
	_ context.Context,
	intent ports.SupplierBillHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type billClock struct{ at time.Time }

func (clock billClock) Now() time.Time { return clock.at }

type billFixture struct {
	store     *billStoreDouble
	costs     *costViewDouble
	authority *authorityViewDouble
	handoff   *billHandoffDouble
	handler   *application.ReceiveSupplierBillHandler
}

func newBillFixture(t *testing.T) *billFixture {
	t.Helper()
	fixture := &billFixture{
		store:     newBillStore(),
		costs:     &costViewDouble{costs: map[string]domain.SupplierExpectedCost{}},
		authority: &authorityViewDouble{configured: true},
		handoff:   &billHandoffDouble{},
	}
	fixture.handler = application.NewReceiveSupplierBillHandler(application.ReceiveSupplierBillDeps{
		Receptions: fixture.store,
		Costs:      fixture.costs,
		Authority:  fixture.authority,
		Downstream: fixture.handoff,
		Clock:      billClock{at: billRecordedAt},
	})
	fixture.costs.costs["cost/v1"] = fixtureExpectedCost(t, "cost/v1", 12000)
	fixture.costs.costs["cost/v2"] = fixtureExpectedCost(t, "cost/v2", 2500)
	return fixture
}

func fixtureExpectedCost(t *testing.T, version string, settlementMinor int64) domain.SupplierExpectedCost {
	t.Helper()
	occurrence, err := domain.NewTransportChargeOccurrence(
		billValue(t, domain.NewChargeOccurrenceID, "occurrence-1"),
		billValue(t, domain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT"),
		billValue(t, domain.NewOccurrenceVersion, "occurrence/v1"),
		billArrivedAt.Add(-24*time.Hour),
	)
	if err != nil {
		t.Fatalf("new occurrence: %v", err)
	}
	cost, err := domain.FormSupplierExpectedCost(domain.SupplierExpectedCostSpec{
		Version:            billValue(t, domain.NewSupplierCostVersionID, version),
		Occurrence:         occurrence,
		FeeItem:            billValue(t, domain.NewFeeItemReference, "fee-linehaul"),
		RuleVersion:        billValue(t, domain.NewPurchaseRuleVersionReference, "purchase-rule/v1"),
		Agreement:          billValue(t, domain.NewSupplierAgreementReference, "agreement-1"),
		Evaluation:         billValue(t, domain.NewBuyEvaluationReference, "buy-evaluation-1"),
		OriginalCurrency:   billValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:      settlementMinor,
		SettlementCurrency: billValue(t, domain.NewCurrencyCode, "USD"),
		SettlementMinor:    settlementMinor,
	})
	if err != nil {
		t.Fatalf("form expected cost: %v", err)
	}
	return cost
}

func billCommand(t *testing.T, claimID string) application.ReceiveSupplierBillCommand {
	t.Helper()
	return application.ReceiveSupplierBillCommand{
		TenantID: billValue(t, domain.NewTenantID, "tenant-1"),
		Claim: domain.SupplierBillClaimSpec{
			Claim:       billValue(t, domain.NewBillClaimID, claimID),
			Version:     billValue(t, domain.NewBillClaimVersion, claimID+"/v1"),
			Supplier:    billValue(t, domain.NewSupplierPartyReference, "partner-1"),
			LegalEntity: billValue(t, domain.NewLegalEntityReference, "legal-1"),
			Period:      billValue(t, domain.NewBillingPeriodReference, "period-2026-08"),
			Currency:    billValue(t, domain.NewCurrencyCode, "USD"),
			Lines: []domain.BillLine{
				{
					Line:         billValue(t, domain.NewBillLineReference, "line-1"),
					FeeItem:      billValue(t, domain.NewFeeItemReference, "fee-linehaul"),
					ClaimedMinor: 12000,
				},
				{
					Line:         billValue(t, domain.NewBillLineReference, "line-2"),
					FeeItem:      billValue(t, domain.NewFeeItemReference, "fee-fuel"),
					ClaimedMinor: 3000,
				},
			},
			ReceivedAt: billArrivedAt,
		},
		Directives: []application.LineMatchDirective{
			{
				Line:            billValue(t, domain.NewBillLineReference, "line-1"),
				Classification:  domain.LineMatched,
				ExpectedVersion: billValue(t, domain.NewSupplierCostVersionID, "cost/v1"),
			},
			{
				Line:            billValue(t, domain.NewBillLineReference, "line-2"),
				Classification:  domain.PriceVariance,
				ExpectedVersion: billValue(t, domain.NewSupplierCostVersionID, "cost/v2"),
				Basis:           billValue(t, domain.NewMatchBasisReference, "price-evidence-1"),
			},
		},
	}
}

// Covers: `AT-SA-082`/`AT-SA-083`/`AT-SA-087`——接收提交后逐行匹配并存（已匹配与价差
// 分开保留，争议不吞并无争议行）；记录形状上没有应付字段（匹配完成≠应付，审核另步）；
// 意图交出一份。
func TestABillIsReceivedWithPerLineMatches(t *testing.T) {
	recordType := reflect.TypeOf(ports.BillReceptionRecord{})
	for index := 0; index < recordType.NumField(); index++ {
		name := strings.ToLower(recordType.Field(index).Name)
		if strings.Contains(name, "payable") || strings.Contains(name, "payment") {
			t.Fatalf("BillReceptionRecord 携带 %q——接收记录就能被读成应付", recordType.Field(index).Name)
		}
	}

	fixture := newBillFixture(t)
	result, err := fixture.handler.Handle(context.Background(), billCommand(t, "bill-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.BillReceived {
		t.Fatalf("outcome = %q, want BILL_RECEIVED", result.Outcome())
	}
	record, present := result.Record()
	if !present {
		t.Fatal("no record returned")
	}
	// 计数锚定本夹具：两行（12000 已匹配 + 3000 对 2500 价差，USD，账期 2026-08）。
	if len(record.Matches) != 2 {
		t.Fatalf("matches = %d, want 2", len(record.Matches))
	}
	if record.Matches[0].Classification() != domain.LineMatched ||
		record.Matches[1].Classification() != domain.PriceVariance {
		t.Fatalf("classifications = %q/%q", record.Matches[0].Classification(), record.Matches[1].Classification())
	}
	if record.Matches[1].VarianceMinor() != 500 {
		t.Fatalf("variance = %d, want +500", record.Matches[1].VarianceMinor())
	}
	if !record.AuditAuthorityConfigured || result.AuditUndecided() {
		t.Fatal("授权已配置却被读成审核未决")
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(fixture.handoff.intents))
	}
}

// Covers: `AT-SA-092`「同一主张重复到达 → 返回原结果，不重复匹配或审核」与 `AT-SA-093`
// 「同一身份携带不同金额 → 版本冲突，不按最后到达覆盖」；投递失败不翻结果、重放重发
// 同一份（AT-SA-098 接收半程）。
func TestReplayConflictAndIntentRecovery(t *testing.T) {
	fixture := newBillFixture(t)
	fixture.handoff.err = errors.New("downstream unavailable")
	command := billCommand(t, "bill-1")

	first, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if first.Outcome() != application.BillReceived {
		t.Fatalf("outcome = %q（投递失败不翻结果）", first.Outcome())
	}
	if first.BillHandoffReference() == "" {
		t.Fatal("投递失败没有留下续办引用")
	}

	fixture.handoff.err = nil
	replay, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.BillExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT", replay.Outcome())
	}
	if replay.BillHandoffReference() != "" || len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d handoff = %q（重放重发同一份）", len(fixture.handoff.intents), replay.BillHandoffReference())
	}
	if fixture.store.saves != 1 {
		t.Fatalf("saves = %d, want 1（重放不重复提交）", fixture.store.saves)
	}

	t.Run("a different content under the same identity is a version conflict", func(t *testing.T) {
		flipped := billCommand(t, "bill-1")
		flipped.Claim.Lines[0].ClaimedMinor = 99999
		result, err := fixture.handler.Handle(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict handle: %v", err)
		}
		if result.Outcome() != application.BillVersionConflict {
			t.Fatalf("outcome = %q, want VERSION_CONFLICT", result.Outcome())
		}
		kept := fixture.store.records[billStoreKey(ports.BillReceptionKey{
			TenantID: flipped.TenantID, Claim: flipped.Claim.Claim, Version: flipped.Claim.Version,
		})]
		if kept.Claim.TotalClaimedMinor() != 15000 {
			t.Fatal("冲突覆盖了原主张")
		}
	})

	t.Run("a concurrent loser reads back the winner", func(t *testing.T) {
		fixture.store.missNext = true
		fixture.store.forceResult = true
		fixture.store.saveResult = ports.BillAlreadyRecorded
		loser, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("loser handle: %v", err)
		}
		fixture.store.forceResult = false
		if loser.Outcome() != application.BillExistingResult {
			t.Fatalf("outcome = %q, want EXISTING_RESULT", loser.Outcome())
		}
	})
}

// Covers: 实例半边纪律「审核授权未配置即未决」——接收与匹配照常成立（机制半边），
// AuditUndecided 为真且记录上授权未配置；不虚构授权人、不默认放行、不铸应付。
func TestUnconfiguredAuditAuthorityLeavesAuditUndecided(t *testing.T) {
	fixture := newBillFixture(t)
	fixture.authority.configured = false

	result, err := fixture.handler.Handle(context.Background(), billCommand(t, "bill-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.BillReceived {
		t.Fatalf("outcome = %q（接收是机制半边，照常成立）", result.Outcome())
	}
	if !result.AuditUndecided() {
		t.Fatal("授权未配置没有被读成审核未决——默认放行正是被禁止的那条路")
	}
	record, _ := result.Record()
	if record.AuditAuthorityConfigured {
		t.Fatal("记录把未配置写成了已配置")
	}
	if len(record.Matches) != 2 {
		t.Fatalf("matches = %d, want 2（匹配不因授权缺席而丢）", len(record.Matches))
	}
}

// Covers: `AT-SA-096`「币种不同且无换算依据 → 保持待判断」与提交矛盾/依赖故障分格
// （ADR-0029）：跨币种 → 未决(CONVERSION_UNCONFIGURED)；指错预期成本版本 → 未受理；
// 预期成本视图故障 → 未决(EXPECTED_COST_UNAVAILABLE)；漏行裁决 → 未受理。都不落库。
func TestUndecidedAndNotAcceptedSplitByRecovery(t *testing.T) {
	t.Run("cross-currency stays undecided for configuration", func(t *testing.T) {
		fixture := newBillFixture(t)
		command := billCommand(t, "bill-1")
		command.Claim.Currency = billValue(t, domain.NewCurrencyCode, "EUR")
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.BillUndecided {
			t.Fatalf("outcome = %q, want BILL_UNDECIDED", result.Outcome())
		}
		if result.UndecidedReason() != application.BillConversionUnconfigured {
			t.Fatalf("reason = %q, want CONVERSION_UNCONFIGURED", result.UndecidedReason())
		}
		if len(fixture.store.records) != 0 {
			t.Fatal("待判断的提交落了库")
		}
	})

	t.Run("a missing expected cost version is not accepted", func(t *testing.T) {
		fixture := newBillFixture(t)
		command := billCommand(t, "bill-1")
		command.Directives[0].ExpectedVersion = billValue(t, domain.NewSupplierCostVersionID, "cost/v9")
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.BillNotAccepted {
			t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
		}
	})

	t.Run("an unavailable cost view is undecided", func(t *testing.T) {
		fixture := newBillFixture(t)
		fixture.costs.err = errors.New("view down")
		result, err := fixture.handler.Handle(context.Background(), billCommand(t, "bill-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.BillUndecided || result.UndecidedReason() != application.BillExpectedCostUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("a line without a directive is not accepted", func(t *testing.T) {
		fixture := newBillFixture(t)
		command := billCommand(t, "bill-1")
		command.Directives = command.Directives[:1]
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.BillNotAccepted {
			t.Fatalf("outcome = %q; 漏行会让未匹配金额凭空消失", result.Outcome())
		}
		if len(fixture.store.records) != 0 || len(fixture.handoff.intents) != 0 {
			t.Fatal("未受理的提交落了库或交了意图")
		}
	})

	t.Run("a store failure is undecided with its reason", func(t *testing.T) {
		fixture := newBillFixture(t)
		fixture.store.findErr = errors.New("store down")
		result, err := fixture.handler.Handle(context.Background(), billCommand(t, "bill-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.BillUndecided || result.UndecidedReason() != application.BillStoreUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("an unexpected save outcome is a programming error", func(t *testing.T) {
		fixture := newBillFixture(t)
		fixture.store.forceResult = true
		fixture.store.saveResult = ports.BillSaveOutcome(99)
		if _, err := fixture.handler.Handle(context.Background(), billCommand(t, "bill-1")); !errors.Is(err, application.ErrUnexpectedBillSave) {
			t.Fatalf("error = %v, want ErrUnexpectedBillSave", err)
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.BillUndecidedReason{
			application.BillStoreUnavailable, application.BillExpectedCostUnavailable, application.BillConversionUnconfigured,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 3 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.BillUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第四个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
