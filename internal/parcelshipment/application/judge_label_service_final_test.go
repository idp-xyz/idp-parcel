package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
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
}

func (double *labelTransactionsByParcelDouble) ListByCoveredParcel(
	_ context.Context,
	_ domain.TenantID,
	_ domain.DeclaredParcelID,
) ([]domain.LabelTransaction, error) {
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
	handler      *application.JudgeLabelServiceFinalHandler
}

func newLabelServiceFinalFixture(t *testing.T) *labelServiceFinalFixture {
	t.Helper()
	fixture := &labelServiceFinalFixture{
		final:        newFinalFixture(t),
		transactions: &labelTransactionsByParcelDouble{},
		registers:    &continuedAttemptRegisterDouble{},
		validity:     &labelValidityDouble{},
	}
	// 面单渠道服务的终局类型由规则答复给出，这里的取值只是夹具锚点，不是任何产品的声明。
	fixture.final.rules.judgment.Kind = mustValue(t, domain.NewFinalKindReference, "LABEL_SERVICE_FINAL")
	fixture.handler = application.NewJudgeLabelServiceFinalHandler(application.JudgeLabelServiceFinalDeps{
		Transactions:  fixture.transactions,
		Registers:     fixture.registers,
		Cancellations: fixture.final.cancellations,
		Validity:      fixture.validity,
		Adoption:      fixture.final.handler,
		Clock:         fixedClock{at: labelServiceClosureAt.Add(time.Hour)},
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
		Fact:        mustValue(t, domain.NewCarrierTrackingFactReference, "TF-TRACK-FACT-7"),
		Version:     mustValue(t, domain.NewCarrierTrackingFactVersion, "TF-TRACK-FACT-7/v1"),
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

// Covers: `AT-PS-095`「已有有效取消结果，随后收到首次有效收寄 → 取消在先，不形成非取消终局」
// 与 `AT-PS-100`「两格已判出但终局规则声明尚未包含面单渠道服务 → 保持终局未决」：取消在先不走
// 采用路径；终局规则的实例半边——PC 词汇表未声明面单渠道两格时，采用路径如实停在
// FINAL_RULE_UNCONFIGURED，判断本身仍交回（机制半边可测）。
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

// Covers: ADR-0029 依赖故障归未决并指名等谁；未受理分格；未决原因集封闭；本编排的依赖里没有
// 任何写口——它只判断，形成走既有采用路径。
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
		if application.LabelServiceUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第五个未决原因带了标签——封闭集合被悄悄放开")
		}
	})

	t.Run("the judgment holds no write port of its own", func(t *testing.T) {
		deps := reflect.TypeOf(application.JudgeLabelServiceFinalDeps{})
		for index := 0; index < deps.NumField(); index++ {
			field := deps.Field(index)
			if field.Name == "Adoption" || field.Type.Kind() != reflect.Interface {
				continue
			}
			for method := 0; method < field.Type.NumMethod(); method++ {
				name := field.Type.Method(method).Name
				if name == "Save" || name == "Insert" || name == "Replace" {
					t.Fatalf("依赖 %s 带了写法 %s——判断不写，形成走既有采用路径", field.Name, name)
				}
			}
		}
	})
}
