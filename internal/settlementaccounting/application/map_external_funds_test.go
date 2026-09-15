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
	fundsOccurredAt = time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	fundsMappedAt   = time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	fundsAppliedAt  = time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	fundsNowAt      = time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC)
)

// fundsFactStoreDouble 照 0021 起的形：一条记录一个版本，键（租户、事实、版本）；FindByKey 交回链头——
// 没有任何一版回指它的那一版（真库靠三道链形约束保证它唯一，替身只在同一事实的记录里找）。
type fundsFactStoreDouble struct {
	records map[string]ports.FundsFactRecord
	findErr error
	// beforeSave 在 Save 查重之前跑一次就清掉：用来在 FindByKey 与 Save 之间塞进另一位写入方——
	// 真库上唯一键把并发采用判成 FundsFactAlreadyAdopted 的正是这一格，替身不开这个口就到不了。
	beforeSave func()
}

func newFundsFactStore() *fundsFactStoreDouble {
	return &fundsFactStoreDouble{records: map[string]ports.FundsFactRecord{}}
}

func fundsFactKey(key ports.FundsFactKey) string {
	return key.TenantID.String() + "|" + key.Fact.String()
}

func fundsFactVersionKey(key ports.FundsFactKey, version domain.FundsFactVersion) string {
	return fundsFactKey(key) + "|" + version.String()
}

func (double *fundsFactStoreDouble) FindByKey(
	_ context.Context,
	key ports.FundsFactKey,
) (ports.FundsFactRecord, bool, error) {
	if double.findErr != nil {
		return ports.FundsFactRecord{}, false, double.findErr
	}
	corrected := map[domain.FundsFactVersion]bool{}
	for _, record := range double.records {
		if record.Key != key {
			continue
		}
		if predecessor, present := record.Fact.Corrects(); present {
			corrected[predecessor] = true
		}
	}
	for _, record := range double.records {
		if record.Key == key && !corrected[record.Fact.Version()] {
			return record, true, nil
		}
	}
	return ports.FundsFactRecord{}, false, nil
}

func (double *fundsFactStoreDouble) FindVersion(
	_ context.Context,
	key ports.FundsFactKey,
	version domain.FundsFactVersion,
) (ports.FundsFactRecord, bool, error) {
	if double.findErr != nil {
		return ports.FundsFactRecord{}, false, double.findErr
	}
	record, found := double.records[fundsFactVersionKey(key, version)]
	return record, found, nil
}

func (double *fundsFactStoreDouble) Save(
	_ context.Context,
	record ports.FundsFactRecord,
) (ports.FundsFactSaveOutcome, error) {
	if double.beforeSave != nil {
		hook := double.beforeSave
		double.beforeSave = nil
		hook()
	}
	if _, exists := double.records[fundsFactVersionKey(record.Key, record.Fact.Version())]; exists {
		return ports.FundsFactAlreadyAdopted, nil
	}
	// 0021 守链形的两道唯一约束在替身里也要在：第二个首版、同一前版的第二次更正都折成`已采用`——
	// 输掉竞态的一方就是从这里拿到 AlreadyAdopted、再读回赢家那一版的。
	predecessor, corrected := record.Fact.Corrects()
	for _, existing := range double.records {
		if existing.Key != record.Key {
			continue
		}
		existingPredecessor, existingCorrected := existing.Fact.Corrects()
		if corrected == existingCorrected && (!corrected || predecessor == existingPredecessor) {
			return ports.FundsFactAlreadyAdopted, nil
		}
	}
	double.records[fundsFactVersionKey(record.Key, record.Fact.Version())] = record
	return ports.FundsFactSaved, nil
}

type fundsMappingStoreDouble struct {
	records map[string]ports.FundsMappingRecord
	findErr error
}

func newFundsMappingStore() *fundsMappingStoreDouble {
	return &fundsMappingStoreDouble{records: map[string]ports.FundsMappingRecord{}}
}

func fundsMappingKey(key ports.FundsMappingKey) string {
	return key.TenantID.String() + "|" + key.Mapping.String()
}

func (double *fundsMappingStoreDouble) FindByKey(
	_ context.Context,
	key ports.FundsMappingKey,
) (ports.FundsMappingRecord, bool, error) {
	if double.findErr != nil {
		return ports.FundsMappingRecord{}, false, double.findErr
	}
	record, found := double.records[fundsMappingKey(key)]
	return record, found, nil
}

func (double *fundsMappingStoreDouble) Save(
	_ context.Context,
	record ports.FundsMappingRecord,
) (ports.FundsMappingSaveOutcome, error) {
	if _, exists := double.records[fundsMappingKey(record.Key)]; exists {
		return ports.FundsMappingAlreadyRecorded, nil
	}
	double.records[fundsMappingKey(record.Key)] = record
	return ports.FundsMappingSaved, nil
}

type applicationStoreDouble struct {
	records map[string]ports.SettlementApplicationRecord
	findErr error
}

func newApplicationStore() *applicationStoreDouble {
	return &applicationStoreDouble{records: map[string]ports.SettlementApplicationRecord{}}
}

func applicationKey(key ports.SettlementApplicationKey) string {
	return key.TenantID.String() + "|" + key.Application.String()
}

func (double *applicationStoreDouble) FindByKey(
	_ context.Context,
	key ports.SettlementApplicationKey,
) (ports.SettlementApplicationRecord, bool, error) {
	if double.findErr != nil {
		return ports.SettlementApplicationRecord{}, false, double.findErr
	}
	record, found := double.records[applicationKey(key)]
	return record, found, nil
}

func (double *applicationStoreDouble) Save(
	_ context.Context,
	record ports.SettlementApplicationRecord,
) (ports.SettlementApplicationSaveOutcome, error) {
	if _, exists := double.records[applicationKey(record.Key)]; exists {
		return ports.SettlementApplicationAlreadyApplied, nil
	}
	double.records[applicationKey(record.Key)] = record
	return ports.SettlementApplicationSaved, nil
}

func (double *applicationStoreDouble) Replace(
	_ context.Context,
	record ports.SettlementApplicationRecord,
) (bool, error) {
	if _, exists := double.records[applicationKey(record.Key)]; !exists {
		return false, nil
	}
	double.records[applicationKey(record.Key)] = record
	return true, nil
}

type settlementHandoffDouble struct {
	intents []ports.SettlementApplicationIntent
	err     error
}

func (double *settlementHandoffDouble) HandOffSettlementApplication(
	_ context.Context,
	intent ports.SettlementApplicationIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

// fundsFactHandoffDouble 照 outboxintent.EnqueueOnce 的口径按（租户+事实+版本）认领：同一份
// 意图交两次只算一封——重放在真库上就是这样被吞的，替身不这样做，「重放不翻倍」在应用层
// 就无从断言。
type fundsFactHandoffDouble struct {
	intents map[string]ports.ExternalFundsFactIntent
	err     error
}

func newFundsFactHandoff() *fundsFactHandoffDouble {
	return &fundsFactHandoffDouble{intents: map[string]ports.ExternalFundsFactIntent{}}
}

func (double *fundsFactHandoffDouble) HandOffExternalFundsFact(
	_ context.Context,
	intent ports.ExternalFundsFactIntent,
) error {
	if double.err != nil {
		return double.err
	}
	key := fundsFactKey(intent.Record.Key) + "|" + intent.Record.Fact.Version().String()
	double.intents[key] = intent
	return nil
}

type fundsClock struct{ at time.Time }

func (clock fundsClock) Now() time.Time { return clock.at }

type fundsFixture struct {
	facts        *fundsFactStoreDouble
	mappings     *fundsMappingStoreDouble
	applications *applicationStoreDouble
	handoff      *settlementHandoffDouble
	factHandoff  *fundsFactHandoffDouble
	handler      *application.MapExternalFundsHandler
}

func newFundsFixture(t *testing.T) *fundsFixture {
	t.Helper()
	fixture := &fundsFixture{
		facts:        newFundsFactStore(),
		mappings:     newFundsMappingStore(),
		applications: newApplicationStore(),
		handoff:      &settlementHandoffDouble{},
		factHandoff:  newFundsFactHandoff(),
	}
	fixture.handler = application.NewMapExternalFundsHandler(application.MapExternalFundsDeps{
		Facts:        fixture.facts,
		Mappings:     fixture.mappings,
		Applications: fixture.applications,
		Downstream:   fixture.handoff,
		FactHandoff:  fixture.factHandoff,
		Clock:        fundsClock{at: fundsNowAt},
	})
	return fixture
}

func adoptCommand(t *testing.T, kind domain.FundsFactKind) application.AdoptFundsFactCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.AdoptFundsFactCommand{
		TenantID:    tenant,
		Fact:        "bank-receipt-1",
		Source:      "bank-registration-1",
		Kind:        kind,
		Currency:    "USD",
		AmountMinor: 20000,
		Version:     "bank-receipt-1/v1",
		OccurredAt:  fundsOccurredAt,
	}
}

func mapCommand(t *testing.T) application.MapFundsCommand {
	t.Helper()
	tenant, _ := domain.NewTenantID("tenant-1")
	return application.MapFundsCommand{
		TenantID:   tenant,
		Mapping:    "mapping-1",
		Fact:       "bank-receipt-1",
		TargetKind: domain.TargetStatement,
		Target:     "STMT-2026-08-001",
		Basis:      "payment-instruction-1",
		MappedAt:   fundsMappedAt,
	}
}

func applyCommand(t *testing.T) application.ApplySettlementCommand {
	t.Helper()
	tenant, _ := domain.NewTenantID("tenant-1")
	return application.ApplySettlementCommand{
		TenantID:       tenant,
		Application:    "application-1",
		Fact:           "bank-receipt-1",
		TargetCurrency: "USD",
		MappingRefs:    []string{"mapping-1"},
		Allocations: []application.AllocationDirective{{
			Mapping:     "mapping-1",
			TargetKind:  domain.TargetStatement,
			Target:      "STMT-2026-08-001",
			Direction:   domain.AllocationDebit,
			AmountMinor: 15000,
		}},
		Basis:     "unique-match-evidence-1",
		AppliedAt: fundsAppliedAt,
	}
}

// Covers: `AT-SA-101`「采用只形成引用和待匹配入口，不直接成为已核销」的编排面——
// 事实归银行/支付系统拥有，同一引用只采用一次，异内容冲突（外部更正走版本链）。
func TestAFundsFactAdoptsOnceAsAReference(t *testing.T) {
	fixture := newFundsFixture(t)
	command := adoptCommand(t, domain.FundsReceiptConfirmed)

	first, err := fixture.handler.AdoptFact(context.Background(), command)
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if first.Outcome() != application.FundsFactAdopted {
		t.Fatalf("outcome = %q, want FUNDS_FACT_ADOPTED", first.Outcome())
	}

	replay, err := fixture.handler.AdoptFact(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.FundsFactExisting {
		t.Fatalf("outcome = %q", replay.Outcome())
	}

	flipped := command
	flipped.AmountMinor = 21000
	conflict, err := fixture.handler.AdoptFact(context.Background(), flipped)
	if err != nil {
		t.Fatalf("conflict adopt: %v", err)
	}
	if conflict.Outcome() != application.FundsFactConflict {
		t.Fatalf("outcome = %q（外部更正走版本链不顶替）", conflict.Outcome())
	}
}

// Covers: sa-cc/03 裁决「付款人维取 A、在 SA 可缺席」的编排面——来源提供的付款人随采用命令进来、
// 落在事实上；不提供照样采用、事实上显式缺席；付款人是内容的一维，同引用换付款人是冲突而不是重放。
func TestAdoptingCarriesTheSourceProvidedPayerAndTreatsItAsContent(t *testing.T) {
	fixture := newFundsFixture(t)

	withPayer := adoptCommand(t, domain.FundsReceiptConfirmed)
	withPayer.Payer = "payer-customer-7"
	adopted, err := fixture.handler.AdoptFact(context.Background(), withPayer)
	if err != nil || adopted.Outcome() != application.FundsFactAdopted {
		t.Fatalf("adopt with payer: outcome = %q err = %v", adopted.Outcome(), err)
	}
	record, ok := adopted.Fact()
	if !ok {
		t.Fatal("采用成功该带记录")
	}
	if payer, provided := record.Fact.Payer(); !provided || payer.String() != "payer-customer-7" {
		t.Fatalf("Payer() = (%q, %v), want (payer-customer-7, true)", payer, provided)
	}

	changedPayer := withPayer
	changedPayer.Payer = "payer-customer-8"
	conflict, err := fixture.handler.AdoptFact(context.Background(), changedPayer)
	if err != nil || conflict.Outcome() != application.FundsFactConflict {
		t.Fatalf("同引用换付款人：outcome = %q err = %v, want FUNDS_FACT_CONFLICT", conflict.Outcome(), err)
	}

	withoutPayer := adoptCommand(t, domain.FundsReceiptConfirmed)
	withoutPayer.Fact = "bank-receipt-2"
	withoutPayer.Version = "bank-receipt-2/v1"
	adoptedWithout, err := fixture.handler.AdoptFact(context.Background(), withoutPayer)
	if err != nil || adoptedWithout.Outcome() != application.FundsFactAdopted {
		t.Fatalf("adopt without payer: outcome = %q err = %v——来源未提供付款人不拒绝采用", adoptedWithout.Outcome(), err)
	}
	record, _ = adoptedWithout.Fact()
	if _, provided := record.Fact.Payer(); provided {
		t.Fatal("来源未提供付款人，事实上却有付款人")
	}
}

// Covers: UC-SA-005「金额相同/同一客户/同一时间不单独证明映射」与「付款失败的事实
// 不可映射」的编排面——显式依据必备（缺依据未受理）；失败付款 → UNFUNDABLE_FACT
// 业务负向（ErrUnfundableFact 哨兵分格）；未采用的事实映射不了。
func TestMappingNeedsBasisAndFundableFact(t *testing.T) {
	fixture := newFundsFixture(t)
	if _, err := fixture.handler.AdoptFact(context.Background(), adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}

	mapped, err := fixture.handler.Map(context.Background(), mapCommand(t))
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if mapped.Outcome() != application.FundsMapped {
		t.Fatalf("outcome = %q, want FUNDS_MAPPED", mapped.Outcome())
	}

	t.Run("a coincidence alone cannot prove a mapping", func(t *testing.T) {
		bare := mapCommand(t)
		bare.Mapping = "mapping-2"
		bare.Basis = " "
		result, err := fixture.handler.Map(context.Background(), bare)
		if err != nil {
			t.Fatalf("map: %v", err)
		}
		if result.Outcome() != application.FundsNotAccepted {
			t.Fatalf("outcome = %q（缺显式依据）", result.Outcome())
		}
	})

	t.Run("a failed payment cannot be mapped", func(t *testing.T) {
		failed := newFundsFixture(t)
		failedAdopt := adoptCommand(t, domain.FundsPaymentFailed)
		failedAdopt.Fact = "bank-failure-1"
		failedAdopt.Version = "bank-failure-1/v1"
		if _, err := failed.handler.AdoptFact(context.Background(), failedAdopt); err != nil {
			t.Fatalf("adopt failed fact: %v", err)
		}
		failedMap := mapCommand(t)
		failedMap.Fact = "bank-failure-1"
		result, err := failed.handler.Map(context.Background(), failedMap)
		if err != nil {
			t.Fatalf("map failed: %v", err)
		}
		if result.Outcome() != application.UnfundableFactOutcome {
			t.Fatalf("outcome = %q, want UNFUNDABLE_FACT", result.Outcome())
		}
	})

	t.Run("mapping an unadopted fact is not accepted", func(t *testing.T) {
		missing := mapCommand(t)
		missing.Mapping = "mapping-3"
		missing.Fact = "bank-receipt-9"
		result, err := fixture.handler.Map(context.Background(), missing)
		if err != nil {
			t.Fatalf("map: %v", err)
		}
		if result.Outcome() != application.FundsNotAccepted {
			t.Fatalf("outcome = %q（先采用再映射）", result.Outcome())
		}
	})

	t.Run("a replay returns the original mapping", func(t *testing.T) {
		replay, err := fixture.handler.Map(context.Background(), mapCommand(t))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.FundsMappingExisting {
			t.Fatalf("outcome = %q", replay.Outcome())
		}
	})
}

// Covers: `AT-SA-112/113`「核销撤销恢复未结金额，原收款引用与原分配不删」与 UC-SA-005
// 「核销是显式判断不是自动推导」的编排面——净额越界 APPLICATION_IMBALANCE、跨币种
// CROSS_CURRENCY 各归业务负向；撤销一次为限；意图交下游已结视图。
func TestApplicationConservesAndReversesOnce(t *testing.T) {
	fixture := newFundsFixture(t)
	if _, err := fixture.handler.AdoptFact(context.Background(), adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if _, err := fixture.handler.Map(context.Background(), mapCommand(t)); err != nil {
		t.Fatalf("map: %v", err)
	}

	applied, err := fixture.handler.Apply(context.Background(), applyCommand(t))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied.Outcome() != application.SettlementApplied {
		t.Fatalf("outcome = %q, want SETTLEMENT_APPLIED", applied.Outcome())
	}
	record, _ := applied.Application()
	// 净额锚定本夹具：借 15000 ≤ 事实 20000。
	if record.Application.AppliedMinor() != 15000 {
		t.Fatalf("applied = %d, want 15000", record.Application.AppliedMinor())
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(fixture.handoff.intents))
	}

	t.Run("an over-fact allocation is an imbalance", func(t *testing.T) {
		over := applyCommand(t)
		over.Application = "application-2"
		over.Allocations[0].AmountMinor = 20001
		result, err := fixture.handler.Apply(context.Background(), over)
		if err != nil {
			t.Fatalf("apply over: %v", err)
		}
		if result.Outcome() != application.ApplicationImbalanceOutcome {
			t.Fatalf("outcome = %q, want APPLICATION_IMBALANCE（超额分别表达不静默截断）", result.Outcome())
		}
	})

	t.Run("a cross-currency application without conversion is refused", func(t *testing.T) {
		cross := applyCommand(t)
		cross.Application = "application-3"
		cross.TargetCurrency = "EUR"
		result, err := fixture.handler.Apply(context.Background(), cross)
		if err != nil {
			t.Fatalf("apply cross: %v", err)
		}
		if result.Outcome() != application.CrossCurrencyOutcome {
			t.Fatalf("outcome = %q, want CROSS_CURRENCY", result.Outcome())
		}
	})

	t.Run("an unbacked allocation is not accepted", func(t *testing.T) {
		unbacked := applyCommand(t)
		unbacked.Application = "application-4"
		unbacked.Allocations[0].Target = "STMT-2026-08-999"
		result, err := fixture.handler.Apply(context.Background(), unbacked)
		if err != nil {
			t.Fatalf("apply unbacked: %v", err)
		}
		if result.Outcome() != application.FundsNotAccepted {
			t.Fatalf("outcome = %q（凭巧合分钱在领域被拒，AT-SA-168）", result.Outcome())
		}
	})

	t.Run("a reversal appends once and resends the intent", func(t *testing.T) {
		tenant, _ := domain.NewTenantID("tenant-1")
		reversed, err := fixture.handler.Reverse(context.Background(), application.ReverseApplicationCommand{
			TenantID:    tenant,
			Application: "application-1",
			Basis:       "customer-refund-decision",
			ReversedAt:  fundsAppliedAt.Add(24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("reverse: %v", err)
		}
		if reversed.Outcome() != application.ApplicationReversedOutcome {
			t.Fatalf("outcome = %q, want APPLICATION_REVERSED", reversed.Outcome())
		}
		record, _ := reversed.Application()
		if _, _, isReversed := record.Application.Reversed(); !isReversed {
			t.Fatal("撤销没有落在核销上")
		}
		if len(fixture.handoff.intents) != 2 {
			t.Fatalf("intents = %d, want 2（撤销版重新交下游）", len(fixture.handoff.intents))
		}

		again, err := fixture.handler.Reverse(context.Background(), application.ReverseApplicationCommand{
			TenantID:    tenant,
			Application: "application-1",
			Basis:       "second-thoughts",
			ReversedAt:  fundsAppliedAt.Add(48 * time.Hour),
		})
		if err != nil {
			t.Fatalf("reverse again: %v", err)
		}
		if again.Outcome() != application.ApplicationAlreadyReversed {
			t.Fatalf("outcome = %q, want ALREADY_REVERSED（不二撤）", again.Outcome())
		}
	})
}

// Covers: ADR-0029（依赖故障归未决且指名等谁）、ADR-0031（写入代数封闭）与 ADR-0043
// （投递失败不翻结果、重放重发同一份）在本编排的恢复面；未决原因集封闭。
func TestFundsRecoveryDiscipline(t *testing.T) {
	t.Run("store failures are undecided with their reasons", func(t *testing.T) {
		fixture := newFundsFixture(t)
		fixture.facts.findErr = errors.New("store down")
		result, err := fixture.handler.AdoptFact(context.Background(), adoptCommand(t, domain.FundsReceiptConfirmed))
		if err != nil {
			t.Fatalf("adopt: %v", err)
		}
		if result.UndecidedReason() != application.FundsFactStoreUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}

		mappingFixture := newFundsFixture(t)
		if _, err := mappingFixture.handler.AdoptFact(context.Background(), adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
			t.Fatalf("adopt: %v", err)
		}
		mappingFixture.mappings.findErr = errors.New("mapping store down")
		result, err = mappingFixture.handler.Map(context.Background(), mapCommand(t))
		if err != nil {
			t.Fatalf("map: %v", err)
		}
		if result.UndecidedReason() != application.FundsMappingStoreUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}
	})

	t.Run("a handoff failure keeps the outcome and is resent on replay", func(t *testing.T) {
		fixture := newFundsFixture(t)
		if _, err := fixture.handler.AdoptFact(context.Background(), adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
			t.Fatalf("adopt: %v", err)
		}
		if _, err := fixture.handler.Map(context.Background(), mapCommand(t)); err != nil {
			t.Fatalf("map: %v", err)
		}
		fixture.handoff.err = errors.New("downstream unavailable")
		first, err := fixture.handler.Apply(context.Background(), applyCommand(t))
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		if first.Outcome() != application.SettlementApplied || first.FundsHandoffReference() == "" {
			t.Fatalf("outcome = %q handoff = %q（投递失败不翻结果）", first.Outcome(), first.FundsHandoffReference())
		}
		fixture.handoff.err = nil
		replay, err := fixture.handler.Apply(context.Background(), applyCommand(t))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.FundsHandoffReference() != "" || len(fixture.handoff.intents) != 1 {
			t.Fatalf("intents = %d handoff = %q", len(fixture.handoff.intents), replay.FundsHandoffReference())
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.FundsUndecidedReason{
			application.FundsFactStoreUnavailable, application.FundsMappingStoreUnavailable,
			application.ApplicationStoreUnavailable,
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
		if application.FundsUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第四个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
