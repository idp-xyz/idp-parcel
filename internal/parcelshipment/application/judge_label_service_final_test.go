package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var (
	labelServiceEstablishedAt = time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	labelServiceResultAt      = labelServiceEstablishedAt.Add(2 * time.Hour)
	labelServiceClosureAt     = labelServiceEstablishedAt.Add(24 * time.Hour)
	labelServicePickupAt      = labelServiceEstablishedAt.Add(6 * time.Hour)
)

type labelTransactionsByParcelDouble struct {
	transactions []domain.LabelTransaction
	err          error
	asked        int
}

func (double *labelTransactionsByParcelDouble) ListByCoveredParcel(
	_ context.Context,
	_ domain.TenantID,
	_ domain.DeclaredParcelID,
) ([]domain.LabelTransaction, error) {
	double.asked++
	if double.err != nil {
		return nil, double.err
	}
	return append([]domain.LabelTransaction(nil), double.transactions...), nil
}

type continuedAttemptRegisterDouble struct {
	register domain.ContinuedAttemptRegister
	found    bool
	err      error
}

func (double *continuedAttemptRegisterDouble) FindByParcel(
	_ context.Context,
	_ domain.TenantID,
	_ domain.DeclaredParcelID,
) (domain.ContinuedAttemptRegister, bool, error) {
	if double.err != nil {
		return domain.ContinuedAttemptRegister{}, false, double.err
	}
	return double.register, double.found, nil
}

type labelValidityDouble struct {
	lapsed     bool
	configured bool
	err        error
}

func (double *labelValidityDouble) JudgeLabelLapsed(
	_ context.Context,
	_ domain.TenantID,
	_ domain.LabelTransaction,
	_ domain.DeclaredParcelID,
	_ time.Time,
) (bool, bool, error) {
	if double.err != nil {
		return false, false, double.err
	}
	return double.lapsed, double.configured, nil
}

type labelServiceFinalFixture struct {
	final        *finalFixture
	transactions *labelTransactionsByParcelDouble
	registers    *continuedAttemptRegisterDouble
	validity     *labelValidityDouble
	conflicts    *conflictRegisterDouble
	lock         *boundaryLockDouble
	handler      *application.JudgeLabelServiceFinalHandler
}

func newLabelServiceFinalFixture(t *testing.T) *labelServiceFinalFixture {
	t.Helper()
	fixture := &labelServiceFinalFixture{
		final:        newFinalFixture(t),
		transactions: &labelTransactionsByParcelDouble{},
		registers:    &continuedAttemptRegisterDouble{},
		validity:     &labelValidityDouble{},
		conflicts:    &conflictRegisterDouble{},
		lock:         &boundaryLockDouble{},
	}
	// 面单渠道服务的终局类型由规则答复给出，这里的取值只是夹具锚点，不是任何产品的声明。
	fixture.final.rules.judgment.Kind = mustValue(t, domain.NewFinalKindReference, "LABEL_SERVICE_FINAL")
	fixture.handler = application.NewJudgeLabelServiceFinalHandler(application.JudgeLabelServiceFinalDeps{
		Transactions:  fixture.transactions,
		Registers:     fixture.registers,
		Cancellations: fixture.final.cancellations,
		Validity:      fixture.validity,
		Conflicts:     fixture.conflicts,
		Adoption:      fixture.final.handler,
		Clock:         fixedClock{at: labelServiceClosureAt.Add(time.Hour)},
		BoundaryLock:  fixture.lock,
	})
	return fixture
}

func labelServiceCommand(t *testing.T) application.JudgeLabelServiceFinalCommand {
	t.Helper()
	return application.JudgeLabelServiceFinalCommand{
		Identity:          sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		Parcel:            mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
	}
}

func labelServicePickup(t *testing.T) domain.CarrierFirstEffectivePickupSpec {
	t.Helper()
	return domain.CarrierFirstEffectivePickupSpec{
		Fact:        mustValue(t, domain.NewCarrierFirstEffectivePickupFactReference, "TF-TRACK-FACT-7"),
		Version:     mustValue(t, domain.NewCarrierFirstEffectivePickupFactVersion, "TF-TRACK-FACT-7/v1"),
		EffectiveAt: labelServicePickupAt,
	}
}

// labelTransactionFor 造一笔覆盖 parcel-1、已记下渠道结果的交易；accepted 决定本包裹是受理还是未受理。
func labelTransactionFor(t *testing.T, id string, accepted bool) domain.LabelTransaction {
	t.Helper()
	parcel := mustValue(t, domain.NewDeclaredParcelID, "parcel-1")
	transaction, err := domain.EstablishLabelTransaction(domain.EstablishLabelTransactionSpec{
		Tenant:                 mustValue(t, domain.NewTenantID, "tenant-1"),
		ID:                     mustValue(t, domain.NewLabelTransactionID, id),
		CoveredParcels:         []domain.DeclaredParcelID{parcel},
		ChannelAccount:         mustValue(t, domain.NewChannelAccountReference, "channel-account-1"),
		AccountHolder:          mustValue(t, domain.NewChannelAccountHolderReference, "party-holder-1"),
		ServiceProvider:        mustValue(t, domain.NewChannelServiceProviderReference, "party-channel-1"),
		SettlementCounterparty: mustValue(t, domain.NewSettlementCounterpartyReference, "party-settlement-1"),
		Contract:               mustValue(t, domain.NewChannelContractReference, "contract-1"),
		Rate:                   mustValue(t, domain.NewChannelRateReference, "rate-1"),
		ResponsibilityBasis:    mustValue(t, domain.NewResponsibilityBasisSnapshotReference, "basis-snapshot-1"),
		EstablishedAt:          labelServiceEstablishedAt,
	})
	if err != nil {
		t.Fatalf("establish %s: %v", id, err)
	}
	transaction, err = transaction.SubmitToChannel(labelServiceEstablishedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("submit %s: %v", id, err)
	}
	result := domain.LabelTransactionParcelResultSpec{Parcel: parcel}
	outcome := domain.LabelTransactionFailed
	if accepted {
		result.Accepted = true
		result.Identifier = mustValue(t, domain.NewChannelParcelIdentifier, "CHN-"+id)
		outcome = domain.LabelTransactionSucceeded
	} else {
		result.Reason = mustValue(t, domain.NewChannelResultReasonReference, "ADDRESS_REJECTED")
	}
	transaction, err = transaction.RecordChannelResult(domain.RecordChannelResultSpec{
		Outcome:       outcome,
		ParcelResults: []domain.LabelTransactionParcelResultSpec{result},
		ObservedAt:    labelServiceResultAt,
	})
	if err != nil {
		t.Fatalf("record result on %s: %v", id, err)
	}
	return transaction
}

func closedLabelServiceRegister(t *testing.T) domain.ContinuedAttemptRegister {
	t.Helper()
	register, err := domain.OpenContinuedAttemptRegister(
		mustValue(t, domain.NewTenantID, "tenant-1"), mustValue(t, domain.NewDeclaredParcelID, "parcel-1"))
	if err != nil {
		t.Fatalf("open register: %v", err)
	}
	register, err = register.Append(domain.ContinuedAttemptDecisionSpec{
		ID:                mustValue(t, domain.NewContinuedAttemptDecisionID, "closure-1"),
		Kind:              domain.ControlledClosureDecision,
		Decider:           mustValue(t, domain.NewDeciderReference, "OPS-MANAGER-1"),
		AuthorityRole:     mustValue(t, domain.NewContinuedAttemptAuthorityRoleReference, "PC-CLOSURE-ROLE-1"),
		AuthoritySnapshot: mustValue(t, domain.NewContinuedAttemptAuthoritySnapshot, "PC-AUTH-SNAPSHOT-1"),
		Reason:            mustValue(t, domain.NewContinuedAttemptReasonReference, "CHANNEL_SUSPENDED"),
		EffectiveAt:       labelServiceClosureAt,
		CutoffBoundary:    mustValue(t, domain.NewAuthoritativeCutoffBoundary, "PS-CUTOFF-1"),
		ClosureResponsibilitySource: mustValue(t,
			domain.NewClosureResponsibilitySourceReference, "PC-CHANNEL-ACCOUNT-SUSPENSION-1"),
	}, false)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	return register
}

// Covers: `AT-PS-094`「首次有效收寄事实到达，一笔交易仍结果不确定 → 形成非取消终局，来源只引用
// 收寄事实与版本」——CONTEXT 生命周期「`transport-fulfillment` 提供实际承运商首次有效收寄事件 →
// 面单渠道服务非取消终局结果」在编排面：判断交给既有终局采用路径，终局落进同一个 FinalOutcomeStore
// （委托完成派生从那里读到它），来源只引用 TF 事实（引用@版本），生效时间取收寄有效时间；同一
// 事实版本重放返原，不形成第二终局。
func TestAFirstEffectivePickupIsAdoptedAsTheLabelServiceFinal(t *testing.T) {
	fixture := newLabelServiceFinalFixture(t)
	command := labelServiceCommand(t)
	command.FirstEffectivePickup = labelServicePickup(t)

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.LabelServiceFinalAdopted {
		t.Fatalf("outcome = %q, want LABEL_SERVICE_FINAL_ADOPTED", result.Outcome())
	}
	if result.Verdict().Judgment() != domain.LabelServiceFinalByFirstPickup {
		t.Fatalf("judgment = %s", result.Verdict().Judgment())
	}
	adoption, delegated := result.Adoption()
	if !delegated || adoption.Outcome() != application.ParcelFinalFormed {
		t.Fatalf("adoption = %v/%q, want FINAL_FORMED", delegated, adoption.Outcome())
	}
	record, _ := adoption.Record()
	source := record.Final.Source()
	if source.Kind() != domain.LabelServiceOutcome ||
		source.Version().String() != "TF-TRACK-FACT-7/v1" ||
		source.Execution().String() != "TF-TRACK-FACT-7@TF-TRACK-FACT-7/v1" {
		t.Fatalf("source = %s/%s/%s；来源只引用 TF 事实与版本", source.Kind(), source.Version(), source.Execution())
	}
	if !record.Final.EffectiveAt().Equal(labelServicePickupAt) {
		t.Fatalf("effective at = %s，want 收寄有效时间", record.Final.EffectiveAt())
	}
	if record.Final.Kind().String() != "LABEL_SERVICE_FINAL" {
		t.Fatalf("final kind = %s；终局类型来自规则答复", record.Final.Kind())
	}
	current, found, _ := fixture.final.finals.FindCurrentFinal(context.Background(),
		mustValue(t, domain.NewTenantID, "tenant-1"), mustValue(t, domain.NewDeclaredParcelID, "parcel-1"))
	if !found || !current.Finalized {
		t.Fatal("面单渠道服务的终局没有落进当前有效终局——委托完成与取消核验都会看不见它")
	}
	if summary, present := adoption.Completion(); !present || summary.Finalized() != 1 {
		t.Fatalf("completion = %v %d；终局落库即进委托完成派生", present, summary.Finalized())
	}

	t.Run("the same fact version replays to the existing final", func(t *testing.T) {
		replay, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		adoption, _ := replay.Adoption()
		if adoption.Outcome() != application.ParcelFinalExistingResult || fixture.final.finals.saved != 1 {
			t.Fatalf("adoption = %q saved = %d", adoption.Outcome(), fixture.final.finals.saved)
		}
	})
}

// Covers: `AT-PS-096`（关闭 + 全部未受理 → 终局失败结果，证据是生效关闭）、`AT-PS-097`（成功且
// 未作废、有效期规则未配置 → 不形成）与 `AT-PS-098` 的失效半边——CONTEXT 生命周期「当前受控关闭
// 已经生效且未被重开，全部相关面单交易均已定案为明确失败……→ 终局失败结果」在编排面：全部交易按包裹整册
// 取（含边界后交易，不筛），登记册未开册按空册读，证据是生效的关闭决定（来源版本即它的标识）。
func TestTheClosurePathAdoptsAFailureFinalAcrossAllTransactions(t *testing.T) {
	fixture := newLabelServiceFinalFixture(t)
	fixture.transactions.transactions = []domain.LabelTransaction{
		labelTransactionFor(t, "label-txn-1", false),
		labelTransactionFor(t, "label-txn-2", false),
	}
	fixture.registers.register, fixture.registers.found = closedLabelServiceRegister(t), true

	result, err := fixture.handler.Handle(context.Background(), labelServiceCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	adoption, delegated := result.Adoption()
	if result.Outcome() != application.LabelServiceFinalAdopted || !delegated || adoption.Outcome() != application.ParcelFinalFormed {
		t.Fatalf("outcome = %q adoption = %v/%q", result.Outcome(), delegated, adoption.Outcome())
	}
	record, _ := adoption.Record()
	source := record.Final.Source()
	if source.Kind() != domain.LabelServiceFailure || source.Version().String() != "closure-1" ||
		source.Execution().String() != "CONTINUED-ATTEMPT-CLOSURE/closure-1" {
		t.Fatalf("source = %s/%s/%s；关闭路径的证据是生效的关闭决定", source.Kind(), source.Version(), source.Execution())
	}
	if !record.Final.EffectiveAt().Equal(labelServiceClosureAt) {
		t.Fatalf("effective at = %s", record.Final.EffectiveAt())
	}

	t.Run("a still-open parcel or a usable label result forms nothing", func(t *testing.T) {
		open := newLabelServiceFinalFixture(t)
		open.transactions.transactions = []domain.LabelTransaction{labelTransactionFor(t, "label-txn-1", false)}
		result, err := open.handler.Handle(context.Background(), labelServiceCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.LabelServiceNotFinalOutcome || result.Verdict().Reason() != domain.ContinuedAttemptStillOpen {
			t.Fatalf("outcome = %q reason = %s；未开册即空册，无关闭即开放", result.Outcome(), result.Verdict().Reason())
		}
		if _, delegated := result.Adoption(); delegated || open.final.finals.saved != 0 {
			t.Fatal("不形成的判断走到了采用路径")
		}

		usable := newLabelServiceFinalFixture(t)
		usable.transactions.transactions = []domain.LabelTransaction{labelTransactionFor(t, "label-txn-1", true)}
		usable.registers.register, usable.registers.found = closedLabelServiceRegister(t), true
		result, err = usable.handler.Handle(context.Background(), labelServiceCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.LabelServiceNotFinalOutcome || result.Verdict().Reason() != domain.UsableLabelResultOutstanding {
			t.Fatalf("outcome = %q reason = %s", result.Outcome(), result.Verdict().Reason())
		}
	})

	t.Run("a lapse ruled by the configured validity rule unblocks the outcome", func(t *testing.T) {
		lapsed := newLabelServiceFinalFixture(t)
		lapsed.transactions.transactions = []domain.LabelTransaction{labelTransactionFor(t, "label-txn-1", true)}
		lapsed.registers.register, lapsed.registers.found = closedLabelServiceRegister(t), true
		lapsed.validity.configured, lapsed.validity.lapsed = true, true
		result, err := lapsed.handler.Handle(context.Background(), labelServiceCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		adoption, _ := result.Adoption()
		record, _ := adoption.Record()
		if result.Verdict().Judgment() != domain.LabelServiceFinalOutcomeByClosure || record.Final.Source().Kind() != domain.LabelServiceOutcome {
			t.Fatalf("judgment = %s；不可逆失效的成功不再阻止终局服务结果", result.Verdict().Judgment())
		}

		unconfigured := newLabelServiceFinalFixture(t)
		unconfigured.transactions.transactions = []domain.LabelTransaction{labelTransactionFor(t, "label-txn-1", true)}
		unconfigured.registers.register, unconfigured.registers.found = closedLabelServiceRegister(t), true
		unconfigured.validity.configured, unconfigured.validity.lapsed = false, true
		result, err = unconfigured.handler.Handle(context.Background(), labelServiceCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.LabelServiceNotFinalOutcome {
			t.Fatalf("outcome = %q；有效期规则未配置时不按墙钟推算失效", result.Outcome())
		}
	})
}

// Covers: `AT-PS-095`「已有有效取消结果，随后收到的首次有效收寄发生晚于取消的业务时间 → 取消在先，
// 不形成非取消终局」与 `AT-PS-100`「两格已判出但终局规则声明尚未包含面单渠道服务 → 保持终局未决」：取消在先不走
// 采用路径；终局规则的实例半边——租户未登记终局规则声明（PAR-COM-17）时，采用路径如实停在
// FINAL_RULE_UNCONFIGURED，判断本身仍交回（机制半边可测）。声明已登而缺面单两行是另一格：
// 「此产品下这种结果不形成终局」，未决原因是 FinalRuleNotSatisfiedYet，译法钉在 adapters/partycommercial
// 的 TestFinalJudgmentTranslatesLabelServiceRows（pc-gaps/12 之后 PC 词汇表有这两行，未配置只剩没登这一种来源）。
func TestCancellationAndUnconfiguredRulesStopTheLabelServiceFinalHonestly(t *testing.T) {
	t.Run("a standing cancellation is not overridden", func(t *testing.T) {
		fixture := newLabelServiceFinalFixture(t)
		fixture.final.cancellations.cancelled = true
		fixture.final.cancellations.cancellation = cancelledAt(t, labelServiceEstablishedAt.Add(time.Hour))
		command := labelServiceCommand(t)
		command.FirstEffectivePickup = labelServicePickup(t)
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.LabelServiceCancellationStandsOutcome {
			t.Fatalf("outcome = %q", result.Outcome())
		}
		if _, delegated := result.Adoption(); delegated || fixture.final.finals.saved != 0 {
			t.Fatal("取消在先仍走到了采用路径")
		}
		if len(fixture.conflicts.records) != 0 {
			t.Fatal("取消在先不是冲突，却记了冲突")
		}
	})

	t.Run("an unconfigured final rule leaves the adoption undecided", func(t *testing.T) {
		fixture := newLabelServiceFinalFixture(t)
		fixture.final.rules.configured = false
		command := labelServiceCommand(t)
		command.FirstEffectivePickup = labelServicePickup(t)
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		adoption, delegated := result.Adoption()
		if result.Outcome() != application.LabelServiceFinalAdopted || !delegated {
			t.Fatalf("outcome = %q delegated = %v", result.Outcome(), delegated)
		}
		if adoption.Outcome() != application.ParcelFinalUndecided || adoption.UndecidedReason() != application.FinalRuleUnconfigured {
			t.Fatalf("adoption = %q/%q；规则未配置停在未决，不默认收寄即终局", adoption.Outcome(), adoption.UndecidedReason())
		}
		if result.Verdict().Judgment() != domain.LabelServiceFinalByFirstPickup {
			t.Fatal("判断本身应照常交回，等的是规则声明")
		}
	})
}

// Covers: `AT-PS-108`（ADR-0254 决定二第 3 格的面单渠道一侧）——有效取消已在，此后到的首次有效收寄
// 发生不晚于取消的业务时间：不形成非取消终局，也不按取消在先作答，答`不形成`并指名冲突，不走采用
// 路径；冲突记进冲突册按包裹查得出，已提交的取消不被碰。冲突记不下时按依赖未决交回——那一格重投会
// 改变结果，而冲突那一格入账。
func TestALateEarlierPickupIsAFactConflictWithTheStandingCancellation(t *testing.T) {
	arrange := func(t *testing.T) (*labelServiceFinalFixture, application.JudgeLabelServiceFinalCommand) {
		fixture := newLabelServiceFinalFixture(t)
		fixture.final.cancellations.cancelled = true
		fixture.final.cancellations.cancellation = cancelledAt(t, labelServicePickupAt.Add(time.Hour))
		command := labelServiceCommand(t)
		command.FirstEffectivePickup = labelServicePickup(t)
		return fixture, command
	}

	t.Run("the judgment names the conflict and records it", func(t *testing.T) {
		fixture, command := arrange(t)
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.LabelServiceNotFinalOutcome ||
			result.Verdict().Reason() != domain.LabelServiceCancellationOrderConflict {
			t.Fatalf("outcome = %q reason = %q, want LABEL_SERVICE_NOT_FINAL/CANCELLATION_ORDER_CONFLICT",
				result.Outcome(), result.Verdict().Reason())
		}
		if result.Outcome() == application.LabelServiceCancellationStandsOutcome ||
			result.Outcome() == application.LabelServiceJudgmentUndecided {
			t.Fatal("冲突既不是取消在先，也不是读口未决")
		}
		if _, delegated := result.Adoption(); delegated || fixture.final.finals.saved != 0 {
			t.Fatal("冲突走到了采用路径")
		}
		if len(fixture.conflicts.records) != 1 {
			t.Fatalf("冲突册行数 = %d, want 1", len(fixture.conflicts.records))
		}
		recorded := fixture.conflicts.records[0]
		if recorded.TenantID.String() != "tenant-1" || recorded.Conflict.Cancellation().String() != "cancel-1" ||
			recorded.Conflict.Boundary().Kind != domain.CarrierFirstEffectivePickupBoundary ||
			recorded.Conflict.Boundary().Reference.String() != "TF-TRACK-FACT-7@TF-TRACK-FACT-7/v1" {
			t.Fatalf("冲突 = %+v", recorded)
		}

		replay, err := fixture.handler.Handle(context.Background(), command)
		if err != nil || replay.Outcome() != application.LabelServiceNotFinalOutcome {
			t.Fatalf("重投同一封：outcome = %q err = %v——重投推不动，答同一格", replay.Outcome(), err)
		}
		if len(fixture.conflicts.records) != 1 {
			t.Fatal("同一份冲突重投记了第二行")
		}
	})

	t.Run("a conflict that cannot be recorded is a dependency gap", func(t *testing.T) {
		fixture, command := arrange(t)
		fixture.conflicts.err = errors.New("conflict register down")
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.LabelServiceJudgmentUndecided ||
			result.UndecidedReason() != application.LabelConflictRegisterUnavailable {
			t.Fatalf("outcome = %q reason = %q, want JUDGMENT_UNDECIDED/CONFLICT_REGISTER_UNAVAILABLE",
				result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("an unwired conflict register does not pretend to have recorded it", func(t *testing.T) {
		fixture, command := arrange(t)
		fixture.handler = application.NewJudgeLabelServiceFinalHandler(application.JudgeLabelServiceFinalDeps{
			Transactions:  fixture.transactions,
			Registers:     fixture.registers,
			Cancellations: fixture.final.cancellations,
			Validity:      fixture.validity,
			Adoption:      fixture.final.handler,
			Clock:         fixedClock{at: labelServiceClosureAt.Add(time.Hour)},
			BoundaryLock:  fixture.lock,
		})
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.UndecidedReason() != application.LabelConflictRegisterUnavailable {
			t.Fatalf("reason = %q, want CONFLICT_REGISTER_UNAVAILABLE", result.UndecidedReason())
		}
	})
}

// Covers: `AT-PS-108` 与 ADR-0254 决定二第 3 格对同一包裹的各路触发——冲突一经收寄那一路判出并记进册，
// 交易与关闭两路（不带收寄）再触发时也答`不形成`/CANCELLATION_ORDER_CONFLICT，不答取消在先：先查册上有没有
// 指向这份取消的冲突。查不动按依赖未决；册上没有（或只有指向别的取消的行）照旧取消在先；册未接入不假装查过。
func TestTheClosurePathsAnswerTheRecordedConflictToo(t *testing.T) {
	arrange := func(t *testing.T) *labelServiceFinalFixture {
		fixture := newLabelServiceFinalFixture(t)
		fixture.final.cancellations.cancelled = true
		fixture.final.cancellations.cancellation = cancelledAt(t, labelServicePickupAt.Add(time.Hour))
		return fixture
	}
	recordConflictThroughThePickupRoute := func(t *testing.T, fixture *labelServiceFinalFixture) {
		command := labelServiceCommand(t)
		command.FirstEffectivePickup = labelServicePickup(t)
		if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
			t.Fatalf("pickup route: %v", err)
		}
		if len(fixture.conflicts.records) != 1 {
			t.Fatalf("收寄那一路没有记下冲突：%d 行", len(fixture.conflicts.records))
		}
	}

	t.Run("a recorded conflict on this cancellation", func(t *testing.T) {
		fixture := arrange(t)
		recordConflictThroughThePickupRoute(t, fixture)

		result, err := fixture.handler.Handle(context.Background(), labelServiceCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.LabelServiceNotFinalOutcome ||
			result.Verdict().Reason() != domain.LabelServiceCancellationOrderConflict {
			t.Fatalf("outcome = %q reason = %q, want LABEL_SERVICE_NOT_FINAL/CANCELLATION_ORDER_CONFLICT",
				result.Outcome(), result.Verdict().Reason())
		}
		if len(fixture.conflicts.records) != 1 {
			t.Fatal("关闭那一路又记了一行冲突——它只读册，不判新冲突")
		}
		if _, delegated := result.Adoption(); delegated {
			t.Fatal("冲突走到了采用路径")
		}
	})

	t.Run("no conflict on the register keeps the cancellation", func(t *testing.T) {
		fixture := arrange(t)
		result, err := fixture.handler.Handle(context.Background(), labelServiceCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.LabelServiceCancellationStandsOutcome {
			t.Fatalf("outcome = %q, want CANCELLATION_STANDS", result.Outcome())
		}
	})

	t.Run("a conflict on another cancellation does not count", func(t *testing.T) {
		fixture := arrange(t)
		recordConflictThroughThePickupRoute(t, fixture)
		other := cancelledAt(t, labelServicePickupAt.Add(time.Hour))
		spec := domain.ParcelCancellationSpec{
			ID:          mustValue(t, domain.NewParcelCancellationID, "cancel-2"),
			Parcel:      other.Parcel(),
			Requester:   other.Requester(),
			Authority:   other.Authority(),
			Reason:      other.Reason(),
			RequestedAt: other.RequestedAt(),
		}
		another, err := domain.DecideParcelCancellation(spec, domain.CurrentIntakeFact{})
		if err != nil {
			t.Fatalf("decide: %v", err)
		}
		fixture.final.cancellations.cancellation = another
		result, err := fixture.handler.Handle(context.Background(), labelServiceCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.LabelServiceCancellationStandsOutcome {
			t.Fatalf("outcome = %q, want CANCELLATION_STANDS——册上那行指向的不是这份取消", result.Outcome())
		}
	})

	t.Run("an unreadable register is a dependency gap", func(t *testing.T) {
		fixture := arrange(t)
		fixture.conflicts.err = errors.New("conflict register down")
		result, err := fixture.handler.Handle(context.Background(), labelServiceCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.LabelServiceJudgmentUndecided ||
			result.UndecidedReason() != application.LabelConflictRegisterUnavailable {
			t.Fatalf("outcome = %q reason = %q, want JUDGMENT_UNDECIDED/CONFLICT_REGISTER_UNAVAILABLE",
				result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("an unwired register does not pretend to have looked", func(t *testing.T) {
		fixture := arrange(t)
		fixture.handler = application.NewJudgeLabelServiceFinalHandler(application.JudgeLabelServiceFinalDeps{
			Transactions:  fixture.transactions,
			Registers:     fixture.registers,
			Cancellations: fixture.final.cancellations,
			Validity:      fixture.validity,
			Adoption:      fixture.final.handler,
			Clock:         fixedClock{at: labelServiceClosureAt.Add(time.Hour)},
			BoundaryLock:  fixture.lock,
		})
		result, err := fixture.handler.Handle(context.Background(), labelServiceCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.UndecidedReason() != application.LabelConflictRegisterUnavailable {
			t.Fatalf("reason = %q, want CONFLICT_REGISTER_UNAVAILABLE", result.UndecidedReason())
		}
	})
}

// Covers: ADR-0029 依赖故障归未决并指名等谁；未受理分格；未决原因集封闭；本编排的依赖里除冲突册外
// 没有写口——它只判断，形成走既有采用路径。
func TestLabelServiceFinalRecoveryDiscipline(t *testing.T) {
	stops := map[string]struct {
		arrange func(*labelServiceFinalFixture)
		reason  application.LabelServiceUndecidedReason
	}{
		"transactions view down": {func(f *labelServiceFinalFixture) { f.transactions.err = errors.New("down") }, application.LabelTransactionsUnavailable},
		"register down":          {func(f *labelServiceFinalFixture) { f.registers.err = errors.New("down") }, application.ContinuedAttemptRegisterUnavailable},
		"cancellation view down": {func(f *labelServiceFinalFixture) { f.final.cancellations.err = errors.New("down") }, application.LabelCancellationViewUnavailable},
		"validity rule down": {func(f *labelServiceFinalFixture) {
			f.transactions.transactions = []domain.LabelTransaction{labelTransactionFor(t, "label-txn-1", true)}
			f.registers.register, f.registers.found = closedLabelServiceRegister(t), true
			f.validity.err = errors.New("down")
		}, application.LabelValidityRuleUnavailable},
	}
	for name, stop := range stops {
		t.Run(name, func(t *testing.T) {
			fixture := newLabelServiceFinalFixture(t)
			stop.arrange(fixture)
			result, err := fixture.handler.Handle(context.Background(), labelServiceCommand(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.LabelServiceJudgmentUndecided || result.UndecidedReason() != stop.reason {
				t.Fatalf("outcome = %q reason = %q, want UNDECIDED/%q", result.Outcome(), result.UndecidedReason(), stop.reason)
			}
		})
	}

	t.Run("a blank parcel or a half pickup reference is not accepted", func(t *testing.T) {
		fixture := newLabelServiceFinalFixture(t)
		command := labelServiceCommand(t)
		command.Parcel = domain.DeclaredParcelID{}
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.LabelServiceJudgmentNotAccepted {
			t.Fatalf("outcome = %q", result.Outcome())
		}

		half := labelServiceCommand(t)
		half.FirstEffectivePickup = labelServicePickup(t)
		half.FirstEffectivePickup.EffectiveAt = time.Time{}
		result, err = fixture.handler.Handle(context.Background(), half)
		if err != nil {
			t.Fatalf("handle half: %v", err)
		}
		if result.Outcome() != application.LabelServiceJudgmentNotAccepted {
			t.Fatalf("outcome = %q；有效时间待判断的事实不能当收寄终局的依据", result.Outcome())
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.LabelServiceUndecidedReason{
			application.LabelTransactionsUnavailable, application.ContinuedAttemptRegisterUnavailable,
			application.LabelCancellationViewUnavailable, application.LabelValidityRuleUnavailable,
			application.LabelConflictRegisterUnavailable,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 5 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.LabelServiceUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("集合外的未决原因带了标签——封闭集合被悄悄放开")
		}
	})

	// 冲突册是唯一许可的写口，且只许它：它不形成终局，只把判出的冲突留下（ADR-0254 决定二）。
	// 名单之外任何依赖带写法，仍是判断在形成终局之外另写了东西。
	t.Run("the judgment holds no write port of its own beyond the conflict register", func(t *testing.T) {
		deps := reflect.TypeOf(application.JudgeLabelServiceFinalDeps{})
		register := reflect.TypeOf((*ports.CancellationOrderConflictRegister)(nil)).Elem()
		for index := 0; index < deps.NumField(); index++ {
			field := deps.Field(index)
			if field.Name == "Adoption" || field.Type.Kind() != reflect.Interface {
				continue
			}
			// 放过的只是冲突册本身：字段名对得上而类型换成别的写口，照样拦。
			exempt := field.Name == "Conflicts" && field.Type == register
			for method := 0; method < field.Type.NumMethod(); method++ {
				name := field.Type.Method(method).Name
				if exempt && name == "Record" {
					continue
				}
				if name == "Save" || name == "Insert" || name == "Replace" || name == "Record" {
					t.Fatalf("依赖 %s 带了写法 %s——判断不写，形成走既有采用路径", field.Name, name)
				}
			}
		}
	})
}

// Covers: PS CONTEXT「同一包裹的取消提交、收寄采用与面单渠道服务的终局判断依次越过提交边界」在面单渠道终局判断这一侧——锁指名
// 命令的（租户，包裹），取在读已成立的取消之前；取不到锁或没装锁就不读取消与交易、不交采用、不记冲突，停在
// CANCELLATION_VIEW_UNAVAILABLE。
func TestTheLabelServiceJudgmentQueuesOnItsParcelBeforeReadingTheCancellation(t *testing.T) {
	reads := func(fixture *labelServiceFinalFixture) int {
		return fixture.final.cancellations.asked + fixture.transactions.asked
	}
	command := func(t *testing.T) application.JudgeLabelServiceFinalCommand {
		command := labelServiceCommand(t)
		command.FirstEffectivePickup = labelServicePickup(t)
		return command
	}

	t.Run("the lock comes before every read it guards", func(t *testing.T) {
		fixture := newLabelServiceFinalFixture(t)
		fixture.lock.onLock = func() {
			if n := reads(fixture); n != 0 {
				t.Errorf("取锁之前已读了 %d 道——锁外读到的「没有取消」正是要防的那一份旧状态", n)
			}
		}
		result, err := fixture.handler.Handle(context.Background(), command(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.LabelServiceFinalAdopted {
			t.Fatalf("outcome = %q, want LABEL_SERVICE_FINAL_ADOPTED", result.Outcome())
		}
		if len(fixture.lock.locked) != 1 || fixture.lock.locked[0] != "tenant-1/parcel-1" {
			t.Fatalf("取锁 = %v, want 恰一次、指名 tenant-1/parcel-1", fixture.lock.locked)
		}
	})

	stops := map[string]func(*labelServiceFinalFixture){
		"an unavailable lock": func(fixture *labelServiceFinalFixture) { fixture.lock.err = errors.New("lock wait cancelled") },
		"an unwired lock": func(fixture *labelServiceFinalFixture) {
			fixture.handler = application.NewJudgeLabelServiceFinalHandler(application.JudgeLabelServiceFinalDeps{
				Transactions:  fixture.transactions,
				Registers:     fixture.registers,
				Cancellations: fixture.final.cancellations,
				Validity:      fixture.validity,
				Conflicts:     fixture.conflicts,
				Adoption:      fixture.final.handler,
				Clock:         fixedClock{at: labelServiceClosureAt.Add(time.Hour)},
			})
		},
	}
	for name, arrange := range stops {
		t.Run(name, func(t *testing.T) {
			fixture := newLabelServiceFinalFixture(t)
			arrange(fixture)
			result, err := fixture.handler.Handle(context.Background(), command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.LabelServiceJudgmentUndecided ||
				result.UndecidedReason() != application.LabelCancellationViewUnavailable {
				t.Fatalf("outcome = %q reason = %q, want JUDGMENT_UNDECIDED/CANCELLATION_VIEW_UNAVAILABLE",
					result.Outcome(), result.UndecidedReason())
			}
			if _, delegated := result.Adoption(); delegated || reads(fixture) != 0 || len(fixture.conflicts.records) != 0 {
				t.Fatalf("没排上队却交了采用（%t）、读了 %d 道、记了 %d 行冲突",
					delegated, reads(fixture), len(fixture.conflicts.records))
			}
		})
	}
}
