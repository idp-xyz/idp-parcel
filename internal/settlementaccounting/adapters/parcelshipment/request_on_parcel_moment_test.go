package parcelshipment_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelshipment"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉 SA 读 PS 两路计费时点的翻译（票 sell-customer-charge/02）：按信封四维回读，只有`采用`的收寄与已终局的有效交付
// 才发起；命令的时点、来源事实引用与计价基准时点各取对的那一格；读不回是可见性滞后（续办），键对不上与译不动是硬
// 失败；编排的答案按「重投会不会改变结果」落成消费两格。发起编排接真的，它的各口是替身。

var (
	intakeOccurredAt   = time.Date(2026, 10, 18, 7, 15, 0, 0, time.UTC)
	deliveryOccurredAt = time.Date(2026, 10, 19, 15, 40, 0, 0, time.UTC)
	adoptionRecordedAt = time.Date(2026, 10, 19, 16, 0, 0, 0, time.UTC)
)

// --- 发起编排各口的替身：链铺通到 SYN-ACCT-1，采用哪一种时点由用例给。

type chainResolutions struct{ err error }

func (double chainResolutions) LoadParcelCommercialResolution(
	_ context.Context, _ sadomain.TenantID, _ sadomain.ParcelReference,
) (sadomain.CommercialResolutionReference, bool, error) {
	if double.err != nil {
		return sadomain.CommercialResolutionReference{}, false, double.err
	}
	reference, err := sadomain.NewCommercialResolutionReference("SYN-RES-1")
	return reference, true, err
}

type chainTerms struct {
	terms sadomain.AdoptedSettlementTerms
}

func (double chainTerms) LoadAdoptedSettlementTerms(
	context.Context, sadomain.TenantID, sadomain.CommercialResolutionReference,
) (sadomain.AdoptedSettlementTerms, bool, error) {
	return double.terms, true, nil
}

type chainAccounts struct{ account sadomain.SettlementAccountID }

func (double chainAccounts) Find(
	context.Context, sadomain.TenantID, sadomain.SettlementAccountKey,
) (sadomain.SettlementAccountID, bool, error) {
	return double.account, true, nil
}

type chainAdoptions struct {
	adoption sadomain.SellBillingMomentAdoption
	adopted  bool
}

func (double chainAdoptions) FindSellBillingMomentAdoption(
	context.Context, sadomain.TenantID, sadomain.SettlementAccountID,
) (sadomain.SellBillingMomentAdoption, bool, error) {
	return double.adoption, double.adopted, nil
}

type chainRegistry struct {
	records map[string]saports.SellEvaluationRequestRecord
	findErr error
}

func (double *chainRegistry) Save(
	_ context.Context, record saports.SellEvaluationRequestRecord,
) (saports.EvaluationRequestSaveOutcome, error) {
	natural := record.Request.Parcel().String() + "|" + record.Request.Moment().String()
	if _, exists := double.records[natural]; exists {
		return saports.EvaluationRequestAlreadyRequested, nil
	}
	double.records[natural] = record
	return saports.EvaluationRequestSaved, nil
}

func (double *chainRegistry) FindByNaturalKey(
	_ context.Context, _ sadomain.TenantID, key sadomain.SellEvaluationRequestNaturalKey,
) (saports.SellEvaluationRequestRecord, bool, error) {
	if double.findErr != nil {
		return saports.SellEvaluationRequestRecord{}, false, double.findErr
	}
	record, found := double.records[key.Parcel.String()+"|"+key.Moment.String()]
	return record, found, nil
}

type chainIdentity struct{ minted int }

func (double *chainIdentity) MintSellEvaluationRequestID(context.Context) (sadomain.SellEvaluationRequestID, error) {
	double.minted++
	return sadomain.NewSellEvaluationRequestID(fmt.Sprintf("SELLREQ-SYN-%d", double.minted))
}

type chainHandoff struct {
	intents []saports.SellEvaluationRequestIntent
	err     error
}

func (double *chainHandoff) HandOffSellEvaluationRequest(_ context.Context, intent saports.SellEvaluationRequestIntent) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type chainClock struct{}

func (chainClock) Now() time.Time { return adoptionRecordedAt }

// recordingRequester 先记下命令再交真编排，让用例既看得到译出的命令，也走得到编排的真答案。
type recordingRequester struct {
	commands []saapplication.RequestSellEvaluationOnParcelMomentCommand
	inner    *saapplication.RequestSellEvaluationOnParcelMomentHandler
}

func (requester *recordingRequester) Handle(
	ctx context.Context, command saapplication.RequestSellEvaluationOnParcelMomentCommand,
) (saapplication.RequestSellEvaluationOnParcelMomentResult, error) {
	requester.commands = append(requester.commands, command)
	return requester.inner.Handle(ctx, command)
}

type requesterFixture struct {
	requester   *recordingRequester
	resolutions *chainResolutions
	registry    *chainRegistry
	handoff     *chainHandoff
}

// newRequesterFixture 装一只真发起编排；adopted 为 ParcelBillingMomentInvalid 表示这一户没登记采用。
func newRequesterFixture(t *testing.T, adopted sadomain.ParcelBillingMoment) *requesterFixture {
	t.Helper()
	terms, err := sadomain.NewAdoptedSettlementTerms(
		must(t, sadomain.NewLegalEntityReference, "SYN-LE-1"),
		must(t, sadomain.NewSettlementCounterpartyReference, "SYN-CP-1"),
		must(t, sadomain.NewCurrencyCode, "XTS"),
		must(t, sadomain.NewSettlementPolicyReference, "SYN-SETTLE-1"),
	)
	if err != nil {
		t.Fatalf("四维：%v", err)
	}
	account := must(t, sadomain.NewSettlementAccountID, "SYN-ACCT-1")
	adoptions := chainAdoptions{}
	if adopted != sadomain.ParcelBillingMomentInvalid {
		adoption, err := sadomain.NewSellBillingMomentAdoption(account, adopted, must(t, sadomain.NewEstimatePricingScope, "SYN-SCOPE-1"))
		if err != nil {
			t.Fatalf("采用：%v", err)
		}
		adoptions = chainAdoptions{adoption: adoption, adopted: true}
	}
	fixture := &requesterFixture{
		resolutions: &chainResolutions{},
		registry:    &chainRegistry{records: map[string]saports.SellEvaluationRequestRecord{}},
		handoff:     &chainHandoff{},
	}
	handler, err := saapplication.NewRequestSellEvaluationOnParcelMomentHandler(saapplication.RequestSellEvaluationOnParcelMomentDeps{
		Resolutions: fixture.resolutions,
		Terms:       chainTerms{terms: terms},
		Accounts:    chainAccounts{account: account},
		Adoptions:   adoptions,
		Registry:    fixture.registry,
		Identity:    &chainIdentity{},
		Downstream:  fixture.handoff,
		Clock:       chainClock{},
	})
	if err != nil {
		t.Fatalf("构造发起编排：%v", err)
	}
	fixture.requester = &recordingRequester{inner: handler}
	return fixture
}

// --- 提供方读口替身。

type intakeAdoptionsDouble struct {
	records map[psports.IntakeAdoptionKey]psports.IntakeAdoptionRecord
	err     error
}

func (double *intakeAdoptionsDouble) FindByKey(
	_ context.Context, key psports.IntakeAdoptionKey,
) (psports.IntakeAdoptionRecord, bool, error) {
	if double.err != nil {
		return psports.IntakeAdoptionRecord{}, false, double.err
	}
	record, found := double.records[key]
	return record, found, nil
}

type finalOutcomesDouble struct {
	records map[psports.FinalAdoptionKey]psports.FinalOutcomeRecord
	err     error
}

func (double *finalOutcomesDouble) FindByKey(
	_ context.Context, key psports.FinalAdoptionKey,
) (psports.FinalOutcomeRecord, bool, error) {
	if double.err != nil {
		return psports.FinalOutcomeRecord{}, false, double.err
	}
	record, found := double.records[key]
	return record, found, nil
}

func intakeKey(t *testing.T) psports.IntakeAdoptionKey {
	t.Helper()
	return psports.IntakeAdoptionKey{
		TenantID: must(t, psdomain.NewTenantID, "SYN-TENANT-1"),
		Parcel:   must(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-1"),
		Kind:     psdomain.NodeIntakeSource,
		Version:  must(t, psdomain.NewSourceResultVersion, "SYN-INTAKE-V1"),
	}
}

func adoptedIntake(t *testing.T, key psports.IntakeAdoptionKey) psports.IntakeAdoptionRecord {
	t.Helper()
	source, err := psdomain.NewIntakeSource(psdomain.IntakeSourceSpec{
		Kind:       key.Kind,
		Object:     must(t, psdomain.NewSourceObjectReference, "SYN-UNIT-1"),
		Parcel:     key.Parcel,
		Place:      must(t, psdomain.NewIntakePlaceReference, "SYN-NODE-1"),
		Control:    must(t, psdomain.NewIntakeControlReference, "SYN-CONTROL-1"),
		Version:    key.Version,
		OccurredAt: intakeOccurredAt,
	})
	if err != nil {
		t.Fatalf("来源：%v", err)
	}
	intake, err := psdomain.AdoptNetworkIntake(source, must(t, psdomain.NewSubmissionVersionID, "SYN-SUB-V1"))
	if err != nil {
		t.Fatalf("采用：%v", err)
	}
	return psports.IntakeAdoptionRecord{
		Key:       key,
		Adopted:   true,
		Intake:    intake,
		AdoptedAt: adoptionRecordedAt,
	}
}

func recordedIntakeOf(key psports.IntakeAdoptionKey) sainbox.RecordedNetworkIntake {
	return sainbox.RecordedNetworkIntake{
		TenantID: key.TenantID.String(),
		Parcel:   key.Parcel.String(),
		Kind:     key.Kind.String(),
		Version:  key.Version.String(),
	}
}

func finalKey(t *testing.T, kind psdomain.ResponsibilityOutcomeKind) psports.FinalAdoptionKey {
	t.Helper()
	return psports.FinalAdoptionKey{
		TenantID: must(t, psdomain.NewTenantID, "SYN-TENANT-1"),
		Parcel:   must(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-1"),
		Kind:     kind,
		Version:  must(t, psdomain.NewResponsibilityOutcomeVersion, "SYN-OUTCOME-V1"),
	}
}

func finalizedRecord(t *testing.T, key psports.FinalAdoptionKey) psports.FinalOutcomeRecord {
	t.Helper()
	outcome, err := psdomain.NewResponsibilityOutcome(psdomain.ResponsibilityOutcomeSpec{
		Kind:       key.Kind,
		Parcel:     key.Parcel,
		Decision:   must(t, psdomain.NewResponsibilityDecisionReference, "SYN-DECISION-1"),
		Execution:  must(t, psdomain.NewExecutionEvidenceReference, "SYN-POD-1"),
		Version:    key.Version,
		OccurredAt: deliveryOccurredAt,
	})
	if err != nil {
		t.Fatalf("责任结果：%v", err)
	}
	final, err := psdomain.FormParcelFinalOutcome(psdomain.ParcelFinalOutcomeSpec{
		Version:     must(t, psdomain.NewFinalOutcomeVersionID, "SYN-FIN-1"),
		Parcel:      key.Parcel,
		Kind:        must(t, psdomain.NewFinalKindReference, "SYN-DELIVERED"),
		Source:      outcome,
		RuleVersion: must(t, psdomain.NewFinalRuleVersionReference, "SYN-FINAL-RULE-V1"),
	})
	if err != nil {
		t.Fatalf("终局：%v", err)
	}
	return psports.FinalOutcomeRecord{Key: key, Finalized: true, Final: final, AdoptedAt: adoptionRecordedAt}
}

func formedFinalOf(key psports.FinalAdoptionKey) sainbox.FormedFinalOutcome {
	return sainbox.FormedFinalOutcome{
		TenantID: key.TenantID.String(),
		Parcel:   key.Parcel.String(),
		Kind:     key.Kind.String(),
		Version:  key.Version.String(),
	}
}

// Covers: 收寄采用那一路主径——采用的记录译成`有效网络收寄已采用`：基准时点是责任起点（不是采认时刻），来源事实引用
// 是带前缀的采用键；编排发起一份，消费入账。
func TestAnAdoptedNetworkIntakeRequestsAtItsResponsibilityStart(t *testing.T) {
	key := intakeKey(t)
	fixture := newRequesterFixture(t, sadomain.BillingMomentNetworkIntakeAdopted)
	adoptions := &intakeAdoptionsDouble{records: map[psports.IntakeAdoptionKey]psports.IntakeAdoptionRecord{key: adoptedIntake(t, key)}}
	handler, err := adapter.NewRequestOnNetworkIntakeAdapter(adoptions, fixture.requester)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if err := handler.HandleRecordedNetworkIntake(t.Context(), recordedIntakeOf(key)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(fixture.requester.commands) != 1 {
		t.Fatalf("命令 %d 份, want 1", len(fixture.requester.commands))
	}
	command := fixture.requester.commands[0]
	if command.Moment != sadomain.BillingMomentNetworkIntakeAdopted || !command.PricingBasisAt.Equal(intakeOccurredAt) ||
		command.Source.String() != "network-intake/SYN-PARCEL-1/NODE_INTAKE/SYN-INTAKE-V1" ||
		command.TenantID.String() != "SYN-TENANT-1" || command.Parcel.String() != "SYN-PARCEL-1" {
		t.Fatalf("命令 = %+v", command)
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("信封 %d 封, want 1", len(fixture.handoff.intents))
	}
}

// Covers: `不采用`的收寄不是这一拍：入账，不问编排。
func TestARefusedNetworkIntakeIsAcknowledgedWithoutRequesting(t *testing.T) {
	key := intakeKey(t)
	refused := psports.IntakeAdoptionRecord{
		Key:          key,
		RefusalBasis: must(t, psdomain.NewCheckReason, "RESPONSIBILITY_ALREADY_STARTED/SYN"),
		AdoptedAt:    adoptionRecordedAt,
	}
	fixture := newRequesterFixture(t, sadomain.BillingMomentNetworkIntakeAdopted)
	handler, err := adapter.NewRequestOnNetworkIntakeAdapter(
		&intakeAdoptionsDouble{records: map[psports.IntakeAdoptionKey]psports.IntakeAdoptionRecord{key: refused}}, fixture.requester)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := handler.HandleRecordedNetworkIntake(t.Context(), recordedIntakeOf(key)); err != nil {
		t.Fatalf("不采用应入账：%v", err)
	}
	if len(fixture.requester.commands) != 0 {
		t.Fatalf("不采用却问了编排 %d 次", len(fixture.requester.commands))
	}
}

// Covers: 终局那一路主径——已终局的有效交付译成`有效交付已终局`，基准时点是终局生效时点（责任结果的业务时间）。
func TestAFinalizedEffectiveDeliveryRequestsAtItsEffectiveTime(t *testing.T) {
	key := finalKey(t, psdomain.EffectiveDeliveryOutcome)
	fixture := newRequesterFixture(t, sadomain.BillingMomentEffectiveDeliveryFinalized)
	finals := &finalOutcomesDouble{records: map[psports.FinalAdoptionKey]psports.FinalOutcomeRecord{key: finalizedRecord(t, key)}}
	handler, err := adapter.NewRequestOnFinalOutcomeAdapter(finals, fixture.requester)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if err := handler.HandleFormedFinalOutcome(t.Context(), formedFinalOf(key)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(fixture.requester.commands) != 1 {
		t.Fatalf("命令 %d 份, want 1", len(fixture.requester.commands))
	}
	command := fixture.requester.commands[0]
	if command.Moment != sadomain.BillingMomentEffectiveDeliveryFinalized || !command.PricingBasisAt.Equal(deliveryOccurredAt) ||
		command.Source.String() != "final-outcome/SYN-PARCEL-1/EFFECTIVE_DELIVERY/SYN-OUTCOME-V1" {
		t.Fatalf("命令 = %+v", command)
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("信封 %d 封, want 1", len(fixture.handoff.intents))
	}
}

// Covers: 只认已终局的有效交付——不采用的终局、别的责任结果种类的终局都入账、不问编排。
func TestOnlyAFinalizedEffectiveDeliveryIsTheBillingMoment(t *testing.T) {
	notFinalizedKey := finalKey(t, psdomain.EffectiveDeliveryOutcome)
	returnedKey := finalKey(t, psdomain.ReturnCompletedOutcome)
	cases := map[string]struct {
		key    psports.FinalAdoptionKey
		record psports.FinalOutcomeRecord
	}{
		"不采用的终局": {notFinalizedKey, psports.FinalOutcomeRecord{
			Key: notFinalizedKey, RefusalBasis: must(t, psdomain.NewCheckReason, "FINAL_RULE_NOT_SATISFIED/SYN"),
			AdoptedAt: adoptionRecordedAt,
		}},
		"退运完成的终局": {returnedKey, finalizedRecord(t, returnedKey)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newRequesterFixture(t, sadomain.BillingMomentEffectiveDeliveryFinalized)
			handler, err := adapter.NewRequestOnFinalOutcomeAdapter(
				&finalOutcomesDouble{records: map[psports.FinalAdoptionKey]psports.FinalOutcomeRecord{tc.key: tc.record}},
				fixture.requester)
			if err != nil {
				t.Fatalf("构造：%v", err)
			}
			if err := handler.HandleFormedFinalOutcome(t.Context(), formedFinalOf(tc.key)); err != nil {
				t.Fatalf("应入账：%v", err)
			}
			if len(fixture.requester.commands) != 0 {
				t.Fatalf("不是这一拍却问了编排 %d 次", len(fixture.requester.commands))
			}
		})
	}
}

// Covers: 回读不到是可见性滞后（续办哨兵），读口故障同格；键对不上是硬失败，不进续办；信封上的种类译不动是硬失败。
func TestReadingBackTheProviderRecordSortsLagFromBreakage(t *testing.T) {
	key := intakeKey(t)
	fixture := newRequesterFixture(t, sadomain.BillingMomentNetworkIntakeAdopted)
	build := func(t *testing.T, adoptions *intakeAdoptionsDouble) *adapter.RequestOnNetworkIntakeAdapter {
		t.Helper()
		handler, err := adapter.NewRequestOnNetworkIntakeAdapter(adoptions, fixture.requester)
		if err != nil {
			t.Fatalf("构造：%v", err)
		}
		return handler
	}

	empty := &intakeAdoptionsDouble{records: map[psports.IntakeAdoptionKey]psports.IntakeAdoptionRecord{}}
	if err := build(t, empty).HandleRecordedNetworkIntake(t.Context(), recordedIntakeOf(key)); !errors.Is(err, adapter.ErrIntakeAdoptionNotVisible) {
		t.Fatalf("读不回 err = %v, want ErrIntakeAdoptionNotVisible", err)
	}
	broken := &intakeAdoptionsDouble{err: errors.New("syn: read down")}
	if err := build(t, broken).HandleRecordedNetworkIntake(t.Context(), recordedIntakeOf(key)); !errors.Is(err, adapter.ErrIntakeAdoptionNotVisible) {
		t.Fatalf("读口故障 err = %v, want ErrIntakeAdoptionNotVisible", err)
	}

	other := key
	other.Version = must(t, psdomain.NewSourceResultVersion, "SYN-INTAKE-OTHER")
	mismatched := &intakeAdoptionsDouble{records: map[psports.IntakeAdoptionKey]psports.IntakeAdoptionRecord{key: adoptedIntake(t, other)}}
	if err := build(t, mismatched).HandleRecordedNetworkIntake(t.Context(), recordedIntakeOf(key)); !errors.Is(err, adapter.ErrAdoptionRecordInconsistent) {
		t.Fatalf("键对不上 err = %v, want ErrAdoptionRecordInconsistent", err)
	}

	unknownKind := recordedIntakeOf(key)
	unknownKind.Kind = "VEHICLE_ARRIVAL"
	if err := build(t, empty).HandleRecordedNetworkIntake(t.Context(), unknownKind); !errors.Is(err, adapter.ErrUntranslatableAnswer) {
		t.Fatalf("集外来源类型 err = %v, want ErrUntranslatableAnswer", err)
	}

	finalHandler, err := adapter.NewRequestOnFinalOutcomeAdapter(&finalOutcomesDouble{
		records: map[psports.FinalAdoptionKey]psports.FinalOutcomeRecord{},
	}, fixture.requester)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := finalHandler.HandleFormedFinalOutcome(t.Context(), formedFinalOf(finalKey(t, psdomain.EffectiveDeliveryOutcome))); !errors.Is(err, adapter.ErrFinalOutcomeNotVisible) {
		t.Fatalf("终局读不回 err = %v, want ErrFinalOutcomeNotVisible", err)
	}
	if len(fixture.requester.commands) != 0 {
		t.Fatalf("回读没成却问了编排 %d 次", len(fixture.requester.commands))
	}
}

// Covers: 编排答案落成消费两格——未配置、已存在入账；未决交回续办哨兵；确定性拒收原样上抛、不是续办。
func TestTheRequestAnswerBecomesTheConsumptionGrade(t *testing.T) {
	key := intakeKey(t)
	records := map[psports.IntakeAdoptionKey]psports.IntakeAdoptionRecord{key: adoptedIntake(t, key)}
	handlerFor := func(t *testing.T, fixture *requesterFixture) *adapter.RequestOnNetworkIntakeAdapter {
		t.Helper()
		handler, err := adapter.NewRequestOnNetworkIntakeAdapter(&intakeAdoptionsDouble{records: records}, fixture.requester)
		if err != nil {
			t.Fatalf("构造：%v", err)
		}
		return handler
	}

	t.Run("未配置入账", func(t *testing.T) {
		fixture := newRequesterFixture(t, sadomain.ParcelBillingMomentInvalid)
		if err := handlerFor(t, fixture).HandleRecordedNetworkIntake(t.Context(), recordedIntakeOf(key)); err != nil {
			t.Fatalf("未配置应入账：%v", err)
		}
		if len(fixture.handoff.intents) != 0 {
			t.Fatal("未配置却交了信封")
		}
	})
	t.Run("时点不符入账", func(t *testing.T) {
		fixture := newRequesterFixture(t, sadomain.BillingMomentEffectiveDeliveryFinalized)
		if err := handlerFor(t, fixture).HandleRecordedNetworkIntake(t.Context(), recordedIntakeOf(key)); err != nil {
			t.Fatalf("时点不符应入账：%v", err)
		}
		if len(fixture.handoff.intents) != 0 {
			t.Fatal("时点不符却交了信封")
		}
	})
	t.Run("已存在入账", func(t *testing.T) {
		fixture := newRequesterFixture(t, sadomain.BillingMomentNetworkIntakeAdopted)
		handler := handlerFor(t, fixture)
		for range 2 {
			if err := handler.HandleRecordedNetworkIntake(t.Context(), recordedIntakeOf(key)); err != nil {
				t.Fatalf("应入账：%v", err)
			}
		}
		if len(fixture.registry.records) != 1 {
			t.Fatalf("请求 %d 份, want 1", len(fixture.registry.records))
		}
	})
	t.Run("未决续办", func(t *testing.T) {
		fixture := newRequesterFixture(t, sadomain.BillingMomentNetworkIntakeAdopted)
		fixture.resolutions.err = errors.New("syn: parcel shipment down")
		err := handlerFor(t, fixture).HandleRecordedNetworkIntake(t.Context(), recordedIntakeOf(key))
		if !errors.Is(err, adapter.ErrSellEvaluationRequestUndecided) {
			t.Fatalf("err = %v, want ErrSellEvaluationRequestUndecided", err)
		}
	})
	t.Run("确定性拒收硬失败", func(t *testing.T) {
		fixture := newRequesterFixture(t, sadomain.BillingMomentNetworkIntakeAdopted)
		fixture.handoff.err = fmt.Errorf("syn: %w", saports.ErrSellEvaluationRequestHandoffRejected)
		err := handlerFor(t, fixture).HandleRecordedNetworkIntake(t.Context(), recordedIntakeOf(key))
		if !errors.Is(err, saports.ErrSellEvaluationRequestHandoffRejected) || errors.Is(err, adapter.ErrSellEvaluationRequestUndecided) {
			t.Fatalf("err = %v, want 确定性拒收且不是续办哨兵", err)
		}
	})
}

func TestTheParcelMomentAdaptersRefuseMissingHalves(t *testing.T) {
	fixture := newRequesterFixture(t, sadomain.BillingMomentNetworkIntakeAdopted)
	if _, err := adapter.NewRequestOnNetworkIntakeAdapter(nil, fixture.requester); err == nil {
		t.Fatal("缺采用读口仍装配成功")
	}
	if _, err := adapter.NewRequestOnNetworkIntakeAdapter(&intakeAdoptionsDouble{}, nil); err == nil {
		t.Fatal("缺编排仍装配成功")
	}
	if _, err := adapter.NewRequestOnFinalOutcomeAdapter(nil, fixture.requester); err == nil {
		t.Fatal("缺终局读口仍装配成功")
	}
	if _, err := adapter.NewRequestOnFinalOutcomeAdapter(&finalOutcomesDouble{}, nil); err == nil {
		t.Fatal("缺编排仍装配成功")
	}
}
