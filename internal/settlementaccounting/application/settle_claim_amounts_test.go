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
	claimFormedAt = time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)
	claimNowAt    = time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)
)

type claimAmountStoreDouble struct {
	records map[string]ports.ClaimAmountRecord
	findErr error
}

func newClaimAmountStore() *claimAmountStoreDouble {
	return &claimAmountStoreDouble{records: map[string]ports.ClaimAmountRecord{}}
}

func claimAmountKey(key ports.ClaimAmountKey) string {
	return key.TenantID.String() + "|" + key.Amount.String()
}

func (double *claimAmountStoreDouble) FindByKey(
	_ context.Context,
	key ports.ClaimAmountKey,
) (ports.ClaimAmountRecord, bool, error) {
	if double.findErr != nil {
		return ports.ClaimAmountRecord{}, false, double.findErr
	}
	record, found := double.records[claimAmountKey(key)]
	return record, found, nil
}

func (double *claimAmountStoreDouble) Save(
	_ context.Context,
	record ports.ClaimAmountRecord,
) (ports.ClaimAmountSaveOutcome, error) {
	if _, exists := double.records[claimAmountKey(record.Key)]; exists {
		return ports.ClaimAmountAlreadyFormed, nil
	}
	double.records[claimAmountKey(record.Key)] = record
	return ports.ClaimAmountSaved, nil
}

type receivableStoreDouble struct {
	records map[string]ports.ReceivableRecord
	findErr error
}

func newReceivableStore() *receivableStoreDouble {
	return &receivableStoreDouble{records: map[string]ports.ReceivableRecord{}}
}

func receivableStoreKey(key ports.ReceivableKey) string {
	return key.TenantID.String() + "|" + key.Receivable.String()
}

func (double *receivableStoreDouble) FindByKey(
	_ context.Context,
	key ports.ReceivableKey,
) (ports.ReceivableRecord, bool, error) {
	if double.findErr != nil {
		return ports.ReceivableRecord{}, false, double.findErr
	}
	record, found := double.records[receivableStoreKey(key)]
	return record, found, nil
}

func (double *receivableStoreDouble) Save(
	_ context.Context,
	record ports.ReceivableRecord,
) (ports.ReceivableSaveOutcome, error) {
	if _, exists := double.records[receivableStoreKey(record.Key)]; exists {
		return ports.ReceivableAlreadyFormed, nil
	}
	double.records[receivableStoreKey(record.Key)] = record
	return ports.ReceivableSaved, nil
}

type acknowledgementStoreDouble struct {
	records map[string]ports.AcknowledgementRecord
	findErr error
}

func newAcknowledgementStore() *acknowledgementStoreDouble {
	return &acknowledgementStoreDouble{records: map[string]ports.AcknowledgementRecord{}}
}

func acknowledgementKey(key ports.AcknowledgementKey) string {
	return key.TenantID.String() + "|" + key.Acknowledgement.String()
}

func (double *acknowledgementStoreDouble) FindByKey(
	_ context.Context,
	key ports.AcknowledgementKey,
) (ports.AcknowledgementRecord, bool, error) {
	if double.findErr != nil {
		return ports.AcknowledgementRecord{}, false, double.findErr
	}
	record, found := double.records[acknowledgementKey(key)]
	return record, found, nil
}

func (double *acknowledgementStoreDouble) Save(
	_ context.Context,
	record ports.AcknowledgementRecord,
) (ports.AcknowledgementSaveOutcome, error) {
	if _, exists := double.records[acknowledgementKey(record.Key)]; exists {
		return ports.AcknowledgementAlreadyRecorded, nil
	}
	double.records[acknowledgementKey(record.Key)] = record
	return ports.AcknowledgementSaved, nil
}

type claimAdjustmentStoreDouble struct {
	records map[string]ports.ClaimAdjustmentRecord
	findErr error
}

func newClaimAdjustmentStore() *claimAdjustmentStoreDouble {
	return &claimAdjustmentStoreDouble{records: map[string]ports.ClaimAdjustmentRecord{}}
}

func claimAdjustmentKey(key ports.ClaimAdjustmentKey) string {
	return key.TenantID.String() + "|" + key.Adjustment.String()
}

func (double *claimAdjustmentStoreDouble) FindByKey(
	_ context.Context,
	key ports.ClaimAdjustmentKey,
) (ports.ClaimAdjustmentRecord, bool, error) {
	if double.findErr != nil {
		return ports.ClaimAdjustmentRecord{}, false, double.findErr
	}
	record, found := double.records[claimAdjustmentKey(key)]
	return record, found, nil
}

func (double *claimAdjustmentStoreDouble) Save(
	_ context.Context,
	record ports.ClaimAdjustmentRecord,
) (ports.ClaimAdjustmentSaveOutcome, error) {
	if _, exists := double.records[claimAdjustmentKey(record.Key)]; exists {
		return ports.ClaimAdjustmentAlreadyFormed, nil
	}
	double.records[claimAdjustmentKey(record.Key)] = record
	return ports.ClaimAdjustmentSaved, nil
}

type claimRuleViewDouble struct {
	configured bool
	err        error
}

func (double *claimRuleViewDouble) LoadClaimAmountRule(
	_ context.Context,
	_ domain.TenantID,
	_ domain.ResponsibilityConclusionReference,
) (domain.AmountRuleVersionReference, bool, error) {
	if double.err != nil {
		return domain.AmountRuleVersionReference{}, false, double.err
	}
	if !double.configured {
		return domain.AmountRuleVersionReference{}, false, nil
	}
	rule, err := domain.NewAmountRuleVersionReference("claim-rule/v2")
	return rule, true, err
}

type claimHandoffDouble struct {
	intents []ports.ClaimSettlementIntent
	err     error
}

func (double *claimHandoffDouble) HandOffClaimSettlement(
	_ context.Context,
	intent ports.ClaimSettlementIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type claimClock struct{ at time.Time }

func (clock claimClock) Now() time.Time { return clock.at }

type claimFixture struct {
	amounts          *claimAmountStoreDouble
	receivables      *receivableStoreDouble
	acknowledgements *acknowledgementStoreDouble
	adjustments      *claimAdjustmentStoreDouble
	rules            *claimRuleViewDouble
	handoff          *claimHandoffDouble
	handler          *application.SettleClaimAmountsHandler
}

func newClaimFixture(t *testing.T) *claimFixture {
	t.Helper()
	fixture := &claimFixture{
		amounts:          newClaimAmountStore(),
		receivables:      newReceivableStore(),
		acknowledgements: newAcknowledgementStore(),
		adjustments:      newClaimAdjustmentStore(),
		rules:            &claimRuleViewDouble{configured: true},
		handoff:          &claimHandoffDouble{},
	}
	fixture.handler = application.NewSettleClaimAmountsHandler(application.SettleClaimAmountsDeps{
		Amounts:          fixture.amounts,
		Receivables:      fixture.receivables,
		Acknowledgements: fixture.acknowledgements,
		Adjustments:      fixture.adjustments,
		Rules:            fixture.rules,
		Downstream:       fixture.handoff,
		Clock:            claimClock{at: claimNowAt},
	})
	return fixture
}

func formClaimAmountCommand(t *testing.T) application.FormClaimAmountCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.FormClaimAmountCommand{
		TenantID:       tenant,
		Amount:         "claim-amount-1",
		Kind:           domain.CustomerCompensationPayable,
		ClaimItem:      "claim-item-1",
		Responsibility: "responsibility-conclusion-1",
		LegalEntity:    "legal-entity-1",
		Currency:       "USD",
		AmountMinor:    9000,
		Period:         "2026-09",
		FormedAt:       claimFormedAt,
	}
}

func formReceivableCommand(t *testing.T) application.FormReceivableCommand {
	t.Helper()
	tenant, _ := domain.NewTenantID("tenant-1")
	return application.FormReceivableCommand{
		TenantID:       tenant,
		Receivable:     "receivable-1",
		Matter:         "recovery-matter-1",
		Responsibility: "responsibility-conclusion-1",
		Counterparty:   "carrier-1",
		LegalEntity:    "legal-entity-1",
		Currency:       "USD",
		AmountMinor:    7000,
		FormedAt:       claimFormedAt,
	}
}

// Covers: `AT-SA-144`「没有责任结论就没有金额」与 `AT-SA-147`「保存采用的规则版本」的
// 编排面——规则版本由视图核对（未配置→未决不默认限额）；赔付/退款两格完备性领域把门
// （退款要原费用、赔付不带）；幂等分重放/冲突；意图交对账单纳入。
func TestClaimAmountsFormOnlyFromConclusionsAndRules(t *testing.T) {
	fixture := newClaimFixture(t)
	command := formClaimAmountCommand(t)

	first, err := fixture.handler.FormClaimAmount(context.Background(), command)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if first.Outcome() != application.ClaimAmountFormed {
		t.Fatalf("outcome = %q, want CLAIM_AMOUNT_FORMED", first.Outcome())
	}
	record, _ := first.ClaimAmount()
	if record.Amount.RuleVersion().String() != "claim-rule/v2" {
		t.Fatal("规则版本没有来自视图核对")
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(fixture.handoff.intents))
	}

	t.Run("an unconfigured rule catalogue is undecided", func(t *testing.T) {
		unconfigured := newClaimFixture(t)
		unconfigured.rules.configured = false
		result, err := unconfigured.handler.FormClaimAmount(context.Background(), formClaimAmountCommand(t))
		if err != nil {
			t.Fatalf("form: %v", err)
		}
		if result.Outcome() != application.ClaimUndecided ||
			result.UndecidedReason() != application.ClaimRuleUnconfigured {
			t.Fatalf("outcome = %q reason = %q（不默认限额）", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("a compensation carrying an original charge is not accepted", func(t *testing.T) {
		mixed := formClaimAmountCommand(t)
		mixed.Amount = "claim-amount-2"
		mixed.OriginalCharge = "charge-1"
		result, err := fixture.handler.FormClaimAmount(context.Background(), mixed)
		if err != nil {
			t.Fatalf("form: %v", err)
		}
		if result.Outcome() != application.ClaimNotAccepted {
			t.Fatalf("outcome = %q（赔付不带原费用，两族不混 AT-SA-153）", result.Outcome())
		}
	})

	t.Run("a replay returns the original and a flip is a conflict", func(t *testing.T) {
		replay, err := fixture.handler.FormClaimAmount(context.Background(), command)
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.ClaimAmountExisting {
			t.Fatalf("outcome = %q", replay.Outcome())
		}
		flipped := formClaimAmountCommand(t)
		flipped.AmountMinor = 9500
		conflict, err := fixture.handler.FormClaimAmount(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict: %v", err)
		}
		if conflict.Outcome() != application.ClaimAmountConflict {
			t.Fatalf("outcome = %q（改额走调整，不顶替原金额）", conflict.Outcome())
		}
	})
}

// Covers: `AT-SA-149`「责任条件满足即可独立形成，不等待对方响应」与 `AT-SA-150`「认可
// 覆盖接受范围」的编排面——应追偿独立形成；认可依附既有应追偿（无中生有拒）；全额
// 认可等额、部分认可小于额、超额拒（AcknowledgeRecovery 把门）；认可≠到账。
func TestRecoveryAndAcknowledgementAreLayered(t *testing.T) {
	fixture := newClaimFixture(t)
	formed, err := fixture.handler.FormReceivable(context.Background(), formReceivableCommand(t))
	if err != nil {
		t.Fatalf("form receivable: %v", err)
	}
	if formed.Outcome() != application.ReceivableFormed {
		t.Fatalf("outcome = %q, want RECEIVABLE_FORMED", formed.Outcome())
	}
	tenant, _ := domain.NewTenantID("tenant-1")

	acknowledged, err := fixture.handler.Acknowledge(context.Background(), application.AcknowledgeCommand{
		TenantID:          tenant,
		Acknowledgement:   "acknowledgement-1",
		Receivable:        "receivable-1",
		Response:          "carrier-response-1",
		Standing:          domain.ResponsePartiallyAccepted,
		AcknowledgedMinor: 4000,
		AcknowledgedAt:    claimFormedAt.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	if acknowledged.Outcome() != application.RecoveryAcknowledged {
		t.Fatalf("outcome = %q, want RECOVERY_ACKNOWLEDGED", acknowledged.Outcome())
	}
	record, _ := acknowledged.Acknowledgement()
	// 差额锚定本夹具：7000-4000=3000 未认可范围可见。
	if record.Acknowledgement.UnacknowledgedMinor() != 3000 {
		t.Fatalf("unacknowledged = %d, want 3000", record.Acknowledgement.UnacknowledgedMinor())
	}

	t.Run("a full acceptance must equal the receivable", func(t *testing.T) {
		mismatch, err := fixture.handler.Acknowledge(context.Background(), application.AcknowledgeCommand{
			TenantID:          tenant,
			Acknowledgement:   "acknowledgement-2",
			Receivable:        "receivable-1",
			Response:          "carrier-response-2",
			Standing:          domain.ResponseAccepted,
			AcknowledgedMinor: 6999,
			AcknowledgedAt:    claimFormedAt.Add(24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("acknowledge: %v", err)
		}
		if mismatch.Outcome() != application.ClaimNotAccepted {
			t.Fatalf("outcome = %q（全额认可必须等额）", mismatch.Outcome())
		}
	})

	t.Run("acknowledging an absent receivable is refused", func(t *testing.T) {
		missing, err := fixture.handler.Acknowledge(context.Background(), application.AcknowledgeCommand{
			TenantID:          tenant,
			Acknowledgement:   "acknowledgement-3",
			Receivable:        "receivable-9",
			Response:          "carrier-response-3",
			Standing:          domain.ResponseAccepted,
			AcknowledgedMinor: 100,
			AcknowledgedAt:    claimFormedAt.Add(24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("acknowledge: %v", err)
		}
		if missing.Outcome() != application.ClaimNotAccepted {
			t.Fatalf("outcome = %q; 没有应追偿就没有可认可的范围", missing.Outcome())
		}
	})
}

// Covers: `AT-SA-152`「追加调整不改写原金额、真实收付与核销历史」的编排面——目标按
// 类别核对存在（无中生有拒）；带原因与新责任依据（领域把门）；幂等分重放/冲突。
func TestAdjustmentsAttachToExistingAmounts(t *testing.T) {
	fixture := newClaimFixture(t)
	if _, err := fixture.handler.FormClaimAmount(context.Background(), formClaimAmountCommand(t)); err != nil {
		t.Fatalf("form amount: %v", err)
	}
	tenant, _ := domain.NewTenantID("tenant-1")
	adjust := application.AdjustClaimAmountCommand{
		TenantID:    tenant,
		Adjustment:  "claim-adjustment-1",
		TargetKind:  domain.AdjustsCustomerClaimAmount,
		Target:      "claim-amount-1",
		Reason:      domain.ResponsibilityRevised,
		Basis:       "responsibility-conclusion-2",
		Direction:   domain.AdjustmentCredit,
		Currency:    "USD",
		AmountMinor: 1500,
		Period:      "2026-10",
		FormedAt:    claimFormedAt.Add(48 * time.Hour),
	}

	formed, err := fixture.handler.Adjust(context.Background(), adjust)
	if err != nil {
		t.Fatalf("adjust: %v", err)
	}
	if formed.Outcome() != application.ClaimAdjustmentFormed {
		t.Fatalf("outcome = %q, want CLAIM_ADJUSTMENT_FORMED", formed.Outcome())
	}

	t.Run("adjusting an absent target is refused", func(t *testing.T) {
		missing := adjust
		missing.Adjustment = "claim-adjustment-2"
		missing.Target = "claim-amount-9"
		result, err := fixture.handler.Adjust(context.Background(), missing)
		if err != nil {
			t.Fatalf("adjust: %v", err)
		}
		if result.Outcome() != application.ClaimNotAccepted {
			t.Fatalf("outcome = %q; 调整出了无中生有的金额", result.Outcome())
		}
	})

	t.Run("a receivable-kind target is checked in its own store", func(t *testing.T) {
		if _, err := fixture.handler.FormReceivable(context.Background(), formReceivableCommand(t)); err != nil {
			t.Fatalf("form receivable: %v", err)
		}
		onReceivable := adjust
		onReceivable.Adjustment = "claim-adjustment-3"
		onReceivable.TargetKind = domain.AdjustsRecoveryReceivable
		onReceivable.Target = "receivable-1"
		result, err := fixture.handler.Adjust(context.Background(), onReceivable)
		if err != nil {
			t.Fatalf("adjust: %v", err)
		}
		if result.Outcome() != application.ClaimAdjustmentFormed {
			t.Fatalf("outcome = %q", result.Outcome())
		}
	})

	t.Run("a replay returns the original adjustment", func(t *testing.T) {
		replay, err := fixture.handler.Adjust(context.Background(), adjust)
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.ClaimAdjustmentExisting {
			t.Fatalf("outcome = %q", replay.Outcome())
		}
	})
}

// Covers: ADR-0029（依赖故障归未决且指名等谁）、ADR-0031（写入代数封闭）与 ADR-0043
// （投递失败不翻结果、重放重发同一份）在本编排的恢复面；未决原因集封闭。
func TestClaimSettlementRecoveryDiscipline(t *testing.T) {
	t.Run("store and view failures are undecided with their reasons", func(t *testing.T) {
		fixture := newClaimFixture(t)
		fixture.rules.err = errors.New("view down")
		result, err := fixture.handler.FormClaimAmount(context.Background(), formClaimAmountCommand(t))
		if err != nil {
			t.Fatalf("form: %v", err)
		}
		if result.UndecidedReason() != application.ClaimRuleViewUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}

		storeFixture := newClaimFixture(t)
		storeFixture.receivables.findErr = errors.New("store down")
		result, err = storeFixture.handler.FormReceivable(context.Background(), formReceivableCommand(t))
		if err != nil {
			t.Fatalf("form: %v", err)
		}
		if result.UndecidedReason() != application.ReceivableStoreUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}
	})

	t.Run("a handoff failure keeps the outcome and is resent on replay", func(t *testing.T) {
		fixture := newClaimFixture(t)
		fixture.handoff.err = errors.New("downstream unavailable")
		first, err := fixture.handler.FormClaimAmount(context.Background(), formClaimAmountCommand(t))
		if err != nil {
			t.Fatalf("form: %v", err)
		}
		if first.Outcome() != application.ClaimAmountFormed || first.ClaimHandoffReference() == "" {
			t.Fatalf("outcome = %q handoff = %q（投递失败不翻结果）", first.Outcome(), first.ClaimHandoffReference())
		}
		fixture.handoff.err = nil
		replay, err := fixture.handler.FormClaimAmount(context.Background(), formClaimAmountCommand(t))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.ClaimHandoffReference() != "" || len(fixture.handoff.intents) != 1 {
			t.Fatalf("intents = %d handoff = %q", len(fixture.handoff.intents), replay.ClaimHandoffReference())
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.ClaimUndecidedReason{
			application.ClaimAmountStoreUnavailable, application.ReceivableStoreUnavailable,
			application.AcknowledgementStoreUnavailable, application.ClaimAdjustmentStoreUnavailable,
			application.ClaimRuleViewUnavailable, application.ClaimRuleUnconfigured,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 6 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.ClaimUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第七个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
