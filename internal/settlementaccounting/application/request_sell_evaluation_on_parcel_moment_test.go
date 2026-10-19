package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉 ADR-0190 决定二、三的发起编排（票 sell-customer-charge/02）：两种内置时点各发起一份；没登记、时点不符答`未配置`
// 且什么都不落；同一件包裹同一时点再来答`已存在`、交回先到者；链上各格的「没有」与各口的故障各归其格；交接失败答
// `未决`、确定性拒收整笔报错。夹具全是合成串。

var parcelMomentNow = time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)

type parcelResolutionDouble struct {
	resolutions map[string]domain.CommercialResolutionReference
	err         error
}

func (double *parcelResolutionDouble) LoadParcelCommercialResolution(
	_ context.Context, _ domain.TenantID, parcel domain.ParcelReference,
) (domain.CommercialResolutionReference, bool, error) {
	if double.err != nil {
		return domain.CommercialResolutionReference{}, false, double.err
	}
	resolution, found := double.resolutions[parcel.String()]
	return resolution, found, nil
}

type settlementTermsDouble struct {
	terms map[string]domain.AdoptedSettlementTerms
	err   error
}

func (double *settlementTermsDouble) LoadAdoptedSettlementTerms(
	_ context.Context, _ domain.TenantID, resolution domain.CommercialResolutionReference,
) (domain.AdoptedSettlementTerms, bool, error) {
	if double.err != nil {
		return domain.AdoptedSettlementTerms{}, false, double.err
	}
	terms, found := double.terms[resolution.String()]
	return terms, found, nil
}

// parcelAccountsDouble 只认应收行：按四维加方向拼串当键，问错方向就查无。
type parcelAccountsDouble struct {
	accounts map[string]domain.SettlementAccountID
	err      error
}

func parcelAccountKeyString(key domain.SettlementAccountKey) string {
	return key.LegalEntity().String() + "|" + key.Counterparty().String() + "|" + key.Direction().String() + "|" +
		key.Currency().String() + "|" + key.Policy().String()
}

func (double *parcelAccountsDouble) Find(
	_ context.Context, _ domain.TenantID, key domain.SettlementAccountKey,
) (domain.SettlementAccountID, bool, error) {
	if double.err != nil {
		return domain.SettlementAccountID{}, false, double.err
	}
	account, found := double.accounts[parcelAccountKeyString(key)]
	return account, found, nil
}

type parcelMomentAdoptionsDouble struct {
	adoptions map[string]domain.SellBillingMomentAdoption
	err       error
}

func (double *parcelMomentAdoptionsDouble) FindSellBillingMomentAdoption(
	_ context.Context, _ domain.TenantID, account domain.SettlementAccountID,
) (domain.SellBillingMomentAdoption, bool, error) {
	if double.err != nil {
		return domain.SellBillingMomentAdoption{}, false, double.err
	}
	adoption, found := double.adoptions[account.String()]
	return adoption, found, nil
}

type sellRequestRegistryDouble struct {
	byID      map[string]ports.SellEvaluationRequestRecord
	byNatural map[string]ports.SellEvaluationRequestRecord
	findErr   error
	saveErr   error
	saves     int
	// beforeSave 在 Save 查重之前跑一次就清掉：在 FindByNaturalKey 与 Save 之间塞进另一位写入方，真库上自然键唯一
	// 约束判成`已存在`的正是这一格。
	beforeSave func()
}

func newSellRequestRegistry() *sellRequestRegistryDouble {
	return &sellRequestRegistryDouble{
		byID:      map[string]ports.SellEvaluationRequestRecord{},
		byNatural: map[string]ports.SellEvaluationRequestRecord{},
	}
}

func sellRequestIDKey(key ports.SellEvaluationRequestKey) string {
	return key.TenantID.String() + "|" + key.Request.String()
}

func sellRequestNaturalKey(tenant domain.TenantID, key domain.SellEvaluationRequestNaturalKey) string {
	return tenant.String() + "|" + key.Parcel.String() + "|" + key.Moment.String()
}

func (double *sellRequestRegistryDouble) Save(
	_ context.Context, record ports.SellEvaluationRequestRecord,
) (ports.EvaluationRequestSaveOutcome, error) {
	if double.beforeSave != nil {
		hook := double.beforeSave
		double.beforeSave = nil
		hook()
	}
	if double.saveErr != nil {
		return ports.EvaluationRequestSaveOutcomeInvalid, double.saveErr
	}
	double.saves++
	natural := sellRequestNaturalKey(record.Key.TenantID, record.Request.NaturalKey())
	if _, exists := double.byID[sellRequestIDKey(record.Key)]; exists {
		return ports.EvaluationRequestAlreadyRequested, nil
	}
	if _, exists := double.byNatural[natural]; exists {
		return ports.EvaluationRequestAlreadyRequested, nil
	}
	double.byID[sellRequestIDKey(record.Key)] = record
	double.byNatural[natural] = record
	return ports.EvaluationRequestSaved, nil
}

func (double *sellRequestRegistryDouble) FindByNaturalKey(
	_ context.Context, tenant domain.TenantID, key domain.SellEvaluationRequestNaturalKey,
) (ports.SellEvaluationRequestRecord, bool, error) {
	if double.findErr != nil {
		return ports.SellEvaluationRequestRecord{}, false, double.findErr
	}
	record, found := double.byNatural[sellRequestNaturalKey(tenant, key)]
	return record, found, nil
}

type sellRequestIdentityDouble struct {
	minted int
	err    error
}

func (double *sellRequestIdentityDouble) MintSellEvaluationRequestID(context.Context) (domain.SellEvaluationRequestID, error) {
	if double.err != nil {
		return domain.SellEvaluationRequestID{}, double.err
	}
	double.minted++
	return domain.NewSellEvaluationRequestID(fmt.Sprintf("SELLREQ-SYN-%d", double.minted))
}

// sellRequestHandoffDouble 照 EnqueueOnce 的口径按（租户、请求 ID）认领：同一份再交一次不翻倍。
type sellRequestHandoffDouble struct {
	intents map[string]ports.SellEvaluationRequestIntent
	calls   int
	err     error
}

func (double *sellRequestHandoffDouble) HandOffSellEvaluationRequest(
	_ context.Context, intent ports.SellEvaluationRequestIntent,
) error {
	double.calls++
	if double.err != nil {
		return double.err
	}
	double.intents[sellRequestIDKey(intent.Record.Key)] = intent
	return nil
}

type parcelMomentFixture struct {
	resolutions *parcelResolutionDouble
	terms       *settlementTermsDouble
	accounts    *parcelAccountsDouble
	adoptions   *parcelMomentAdoptionsDouble
	registry    *sellRequestRegistryDouble
	identity    *sellRequestIdentityDouble
	handoff     *sellRequestHandoffDouble
	handler     *application.RequestSellEvaluationOnParcelMomentHandler
}

// newParcelMomentFixture 把链铺通：包裹 SYN-PARCEL-1 → 解析 SYN-RES-1 → 结算政策四维 → 应收账户 SYN-ACCT-1；账户采用
// 哪一种时点由调用方给，ParcelBillingMomentInvalid 表示这一户没登记。
func newParcelMomentFixture(t *testing.T, adopted domain.ParcelBillingMoment) *parcelMomentFixture {
	t.Helper()
	terms, err := domain.NewAdoptedSettlementTerms(
		value(t, domain.NewLegalEntityReference, "SYN-LE-1"),
		value(t, domain.NewSettlementCounterpartyReference, "SYN-CP-1"),
		value(t, domain.NewCurrencyCode, "XTS"),
		value(t, domain.NewSettlementPolicyReference, "SYN-SETTLE-1"),
	)
	if err != nil {
		t.Fatalf("结算政策四维：%v", err)
	}
	receivable, err := terms.AccountKey(domain.ChargeReceivable)
	if err != nil {
		t.Fatalf("应收账户键：%v", err)
	}
	account := value(t, domain.NewSettlementAccountID, "SYN-ACCT-1")
	fixture := &parcelMomentFixture{
		resolutions: &parcelResolutionDouble{resolutions: map[string]domain.CommercialResolutionReference{
			"SYN-PARCEL-1": value(t, domain.NewCommercialResolutionReference, "SYN-RES-1"),
		}},
		terms: &settlementTermsDouble{terms: map[string]domain.AdoptedSettlementTerms{"SYN-RES-1": terms}},
		accounts: &parcelAccountsDouble{accounts: map[string]domain.SettlementAccountID{
			parcelAccountKeyString(receivable): account,
		}},
		adoptions: &parcelMomentAdoptionsDouble{adoptions: map[string]domain.SellBillingMomentAdoption{}},
		registry:  newSellRequestRegistry(),
		identity:  &sellRequestIdentityDouble{},
		handoff:   &sellRequestHandoffDouble{intents: map[string]ports.SellEvaluationRequestIntent{}},
	}
	if adopted != domain.ParcelBillingMomentInvalid {
		adoption, err := domain.NewSellBillingMomentAdoption(account, adopted, value(t, domain.NewEstimatePricingScope, "SYN-SCOPE-SELL-1"))
		if err != nil {
			t.Fatalf("采用行：%v", err)
		}
		fixture.adoptions.adoptions[account.String()] = adoption
	}
	handler, err := application.NewRequestSellEvaluationOnParcelMomentHandler(application.RequestSellEvaluationOnParcelMomentDeps{
		Resolutions: fixture.resolutions,
		Terms:       fixture.terms,
		Accounts:    fixture.accounts,
		Adoptions:   fixture.adoptions,
		Registry:    fixture.registry,
		Identity:    fixture.identity,
		Downstream:  fixture.handoff,
		Clock:       fixedClock{at: parcelMomentNow},
	})
	if err != nil {
		t.Fatalf("构造发起编排：%v", err)
	}
	fixture.handler = handler
	return fixture
}

func parcelMomentCommand(t *testing.T, moment domain.ParcelBillingMoment, source string, basis time.Time) application.RequestSellEvaluationOnParcelMomentCommand {
	t.Helper()
	return application.RequestSellEvaluationOnParcelMomentCommand{
		TenantID:       value(t, domain.NewTenantID, "SYN-TENANT-1"),
		Parcel:         value(t, domain.NewParcelReference, "SYN-PARCEL-1"),
		Moment:         moment,
		Source:         value(t, domain.NewSourceFactReference, source),
		PricingBasisAt: basis,
	}
}

func (fixture *parcelMomentFixture) handle(
	t *testing.T, command application.RequestSellEvaluationOnParcelMomentCommand,
) application.RequestSellEvaluationOnParcelMomentResult {
	t.Helper()
	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	return result
}

// assertNothingRequested 钉「不发起」的全部痕迹：没铸 ID、没登记、没交信封。
func (fixture *parcelMomentFixture) assertNothingRequested(t *testing.T) {
	t.Helper()
	if fixture.identity.minted != 0 || fixture.registry.saves != 0 || len(fixture.registry.byID) != 0 ||
		fixture.handoff.calls != 0 {
		t.Fatalf("不发起却留了痕：铸 %d、登记 %d、册上 %d、交接 %d",
			fixture.identity.minted, fixture.registry.saves, len(fixture.registry.byID), fixture.handoff.calls)
	}
}

// Covers: 票 02 完成判据「两种时点各一条」——采用了哪一种，那一种到来就发起一份：账户与结算币种取链与账户键，计价范围
// 取采用册，时点、来源事实与计价基准时点取命令，状态是`已提交`；同一事务交一封信封。
func TestEachBuiltInMomentRequestsOneSellEvaluationForAnAccountThatAdoptedIt(t *testing.T) {
	for _, moment := range []domain.ParcelBillingMoment{
		domain.BillingMomentNetworkIntakeAdopted,
		domain.BillingMomentEffectiveDeliveryFinalized,
	} {
		t.Run(moment.String(), func(t *testing.T) {
			fixture := newParcelMomentFixture(t, moment)
			basis := time.Date(2026, 10, 18, 8, 30, 0, 0, time.UTC)
			result := fixture.handle(t, parcelMomentCommand(t, moment, "SYN-SOURCE-1", basis))

			if result.Outcome() != application.SellEvaluationRequested {
				t.Fatalf("outcome = %q, want SELL_EVALUATION_REQUESTED", result.Outcome())
			}
			record, ok := result.Request()
			if !ok || record.Key.Request.String() != "SELLREQ-SYN-1" || record.Key.TenantID.String() != "SYN-TENANT-1" {
				t.Fatalf("登记的请求 = %+v ok = %v", record.Key, ok)
			}
			request := record.Request
			if request.Parcel().String() != "SYN-PARCEL-1" || request.Moment() != moment ||
				request.Account().String() != "SYN-ACCT-1" || request.SettlementCurrency().String() != "XTS" ||
				request.PricingScope().String() != "SYN-SCOPE-SELL-1" || request.Source().String() != "SYN-SOURCE-1" ||
				!request.PricingBasisAt().Equal(basis) {
				t.Fatalf("请求各格没照链、采用册与命令登记：%+v", request)
			}
			if record.State != domain.SellRequestSubmitted || !record.RecordedAt.Equal(parcelMomentNow) {
				t.Fatalf("状态 = %q 登记时刻 = %v, want SUBMITTED 与时钟给的那一刻", record.State, record.RecordedAt)
			}
			if len(fixture.handoff.intents) != 1 {
				t.Fatalf("信封 %d 封, want 1", len(fixture.handoff.intents))
			}
		})
	}
}

// Covers: 票 02 完成判据「没登记」——这一户在采用册上没有行，答`未配置`，不铸、不登、不交。
func TestAnAccountWithoutABillingMomentAdoptionIsUnconfigured(t *testing.T) {
	fixture := newParcelMomentFixture(t, domain.ParcelBillingMomentInvalid)
	result := fixture.handle(t, parcelMomentCommand(t, domain.BillingMomentNetworkIntakeAdopted, "SYN-SOURCE-1", parcelMomentNow))

	if result.Outcome() != application.SellEvaluationRequestUnconfigured ||
		result.UnconfiguredReason() != application.SellRequestBillingMomentNotAdopted {
		t.Fatalf("outcome = %q reason = %q, want 未配置 / SELL_BILLING_MOMENT_NOT_ADOPTED", result.Outcome(), result.UnconfiguredReason())
	}
	if _, ok := result.Request(); ok {
		t.Fatal("未配置不该交回请求")
	}
	fixture.assertNothingRequested(t)
}

// Covers: 票 02 完成判据「时点不符」——一户只采用一种时点，另一种到来答`未配置`，两个方向都不发起。
func TestAMomentOtherThanTheAdoptedOneIsUnconfigured(t *testing.T) {
	for adopted, arriving := range map[domain.ParcelBillingMoment]domain.ParcelBillingMoment{
		domain.BillingMomentNetworkIntakeAdopted:       domain.BillingMomentEffectiveDeliveryFinalized,
		domain.BillingMomentEffectiveDeliveryFinalized: domain.BillingMomentNetworkIntakeAdopted,
	} {
		t.Run(arriving.String(), func(t *testing.T) {
			fixture := newParcelMomentFixture(t, adopted)
			result := fixture.handle(t, parcelMomentCommand(t, arriving, "SYN-SOURCE-1", parcelMomentNow))
			if result.Outcome() != application.SellEvaluationRequestUnconfigured ||
				result.UnconfiguredReason() != application.SellRequestBillingMomentDiffers {
				t.Fatalf("outcome = %q reason = %q, want 未配置 / SELL_BILLING_MOMENT_DIFFERS", result.Outcome(), result.UnconfiguredReason())
			}
			fixture.assertNothingRequested(t)
		})
	}
}

// Covers: 票 02 完成判据「重放」——同一件包裹同一时点再来（来源事实与基准时点换了也一样）答`已存在`，交回先到者的 ID，
// 不另铸、不另登；再交的是同一封，信封不翻倍。
func TestTheSameParcelAndMomentAgainIsTheExistingRequest(t *testing.T) {
	fixture := newParcelMomentFixture(t, domain.BillingMomentEffectiveDeliveryFinalized)
	first := fixture.handle(t, parcelMomentCommand(t, domain.BillingMomentEffectiveDeliveryFinalized, "SYN-FINAL-V1", parcelMomentNow))
	if first.Outcome() != application.SellEvaluationRequested {
		t.Fatalf("首拍 outcome = %q", first.Outcome())
	}

	again := fixture.handle(t, parcelMomentCommand(t, domain.BillingMomentEffectiveDeliveryFinalized, "SYN-FINAL-V2",
		parcelMomentNow.Add(time.Hour)))
	if again.Outcome() != application.SellEvaluationRequestExisting {
		t.Fatalf("重放 outcome = %q, want EXISTING_SELL_EVALUATION_REQUEST", again.Outcome())
	}
	record, ok := again.Request()
	if !ok || record.Key.Request.String() != "SELLREQ-SYN-1" || record.Request.Source().String() != "SYN-FINAL-V1" {
		t.Fatalf("重放交回 %+v ok = %v, want 先到者原样", record.Key, ok)
	}
	if fixture.identity.minted != 1 || fixture.registry.saves != 1 || len(fixture.handoff.intents) != 1 {
		t.Fatalf("重放铸 %d、登记 %d、信封 %d, want 都只有首拍那一次", fixture.identity.minted, fixture.registry.saves,
			len(fixture.handoff.intents))
	}
}

// Covers: 并发下另一位先落了自然键：Save 答`已存在`，读回赢家作答，不报错也不另起一份。
func TestALosingConcurrentRequestAnswersWithTheWinner(t *testing.T) {
	fixture := newParcelMomentFixture(t, domain.BillingMomentNetworkIntakeAdopted)
	command := parcelMomentCommand(t, domain.BillingMomentNetworkIntakeAdopted, "SYN-SOURCE-1", parcelMomentNow)
	fixture.registry.beforeSave = func() {
		winner, err := domain.SubmitSellEvaluationRequest(domain.SellEvaluationRequestSpec{
			ID:                 value(t, domain.NewSellEvaluationRequestID, "SELLREQ-WINNER"),
			Parcel:             command.Parcel,
			Account:            value(t, domain.NewSettlementAccountID, "SYN-ACCT-1"),
			SettlementCurrency: value(t, domain.NewCurrencyCode, "XTS"),
			PricingScope:       value(t, domain.NewEstimatePricingScope, "SYN-SCOPE-SELL-1"),
			Moment:             command.Moment,
			Source:             command.Source,
			PricingBasisAt:     parcelMomentNow,
		})
		if err != nil {
			t.Fatalf("赢家：%v", err)
		}
		record := ports.SellEvaluationRequestRecord{
			Key:     ports.SellEvaluationRequestKey{TenantID: command.TenantID, Request: winner.ID()},
			Request: winner, RecordedAt: parcelMomentNow, State: domain.SellRequestSubmitted,
		}
		fixture.registry.byID[sellRequestIDKey(record.Key)] = record
		fixture.registry.byNatural[sellRequestNaturalKey(command.TenantID, winner.NaturalKey())] = record
	}

	result := fixture.handle(t, command)
	record, ok := result.Request()
	if result.Outcome() != application.SellEvaluationRequestExisting || !ok || record.Key.Request.String() != "SELLREQ-WINNER" {
		t.Fatalf("outcome = %q 请求 = %q, want 已存在且交回赢家", result.Outcome(), record.Key.Request)
	}
}

// Covers: 链上三格的「没有」各归其格，都不发起：包裹不属任何已接受委托是`包裹无商业解析`，闭包没采用结算政策与账户
// 不在册上是`未配置`。
func TestEachGapOnTheParcelToAccountChainStopsWithoutRequesting(t *testing.T) {
	cases := map[string]struct {
		breakChain func(*parcelMomentFixture)
		outcome    application.SellEvaluationRequestOutcome
		reason     application.SellEvaluationRequestUnconfiguredReason
	}{
		"包裹无商业解析": {
			breakChain: func(fixture *parcelMomentFixture) { delete(fixture.resolutions.resolutions, "SYN-PARCEL-1") },
			outcome:    application.SellEvaluationRequestParcelUnresolved,
		},
		"闭包没采用结算政策": {
			breakChain: func(fixture *parcelMomentFixture) { delete(fixture.terms.terms, "SYN-RES-1") },
			outcome:    application.SellEvaluationRequestUnconfigured,
			reason:     application.SellRequestSettlementPolicyNotAdopted,
		},
		"账户不在册上": {
			breakChain: func(fixture *parcelMomentFixture) {
				fixture.accounts.accounts = map[string]domain.SettlementAccountID{}
			},
			outcome: application.SellEvaluationRequestUnconfigured,
			reason:  application.SellRequestSettlementAccountNotRegistered,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newParcelMomentFixture(t, domain.BillingMomentNetworkIntakeAdopted)
			tc.breakChain(fixture)
			result := fixture.handle(t, parcelMomentCommand(t, domain.BillingMomentNetworkIntakeAdopted, "SYN-SOURCE-1", parcelMomentNow))
			if result.Outcome() != tc.outcome || result.UnconfiguredReason() != tc.reason {
				t.Fatalf("outcome = %q reason = %q, want %q / %q", result.Outcome(), result.UnconfiguredReason(), tc.outcome, tc.reason)
			}
			fixture.assertNothingRequested(t)
		})
	}
}

// Covers: 依赖故障一律`未决`并点名停在哪一口；交接口故障也是`未决`——调用方整笔回滚，请求行不能没有信封独自提交。
func TestADependencyFailureIsUndecidedAndNamesTheStop(t *testing.T) {
	broken := errors.New("syn: dependency down")
	cases := map[string]struct {
		breakDependency func(*parcelMomentFixture)
		reason          application.SellEvaluationRequestUndecidedReason
	}{
		"小包托运读口": {func(f *parcelMomentFixture) { f.resolutions.err = broken }, application.SellRequestCommercialResolutionUnavailable},
		"商业闭包读口": {func(f *parcelMomentFixture) { f.terms.err = broken }, application.SellRequestSettlementTermsUnavailable},
		"账户册":    {func(f *parcelMomentFixture) { f.accounts.err = broken }, application.SellRequestSettlementAccountUnavailable},
		"采用册":    {func(f *parcelMomentFixture) { f.adoptions.err = broken }, application.SellRequestBillingMomentAdoptionUnavailable},
		"请求册读":   {func(f *parcelMomentFixture) { f.registry.findErr = broken }, application.SellRequestRegistryUnavailable},
		"请求册写":   {func(f *parcelMomentFixture) { f.registry.saveErr = broken }, application.SellRequestRegistryUnavailable},
		"铸造口":    {func(f *parcelMomentFixture) { f.identity.err = broken }, application.SellRequestIdentityUnavailable},
		"交接口":    {func(f *parcelMomentFixture) { f.handoff.err = broken }, application.SellRequestHandoffUnavailable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newParcelMomentFixture(t, domain.BillingMomentNetworkIntakeAdopted)
			tc.breakDependency(fixture)
			result := fixture.handle(t, parcelMomentCommand(t, domain.BillingMomentNetworkIntakeAdopted, "SYN-SOURCE-1", parcelMomentNow))
			if result.Outcome() != application.SellEvaluationRequestUndecided || result.UndecidedReason() != tc.reason {
				t.Fatalf("outcome = %q reason = %q, want 未决 / %q", result.Outcome(), result.UndecidedReason(), tc.reason)
			}
			if _, ok := result.Request(); ok {
				t.Fatal("未决不该交回请求")
			}
		})
	}
}

// Covers: 信封被框架确定性拒收时整笔报错（errors.Is ports.ErrSellEvaluationRequestHandoffRejected），不折成`未决`。
func TestADeterministicallyRejectedEnvelopeFailsTheWholeRequest(t *testing.T) {
	fixture := newParcelMomentFixture(t, domain.BillingMomentNetworkIntakeAdopted)
	fixture.handoff.err = fmt.Errorf("syn: %w", ports.ErrSellEvaluationRequestHandoffRejected)

	_, err := fixture.handler.Handle(context.Background(),
		parcelMomentCommand(t, domain.BillingMomentNetworkIntakeAdopted, "SYN-SOURCE-1", parcelMomentNow))
	if !errors.Is(err, ports.ErrSellEvaluationRequestHandoffRejected) {
		t.Fatalf("err = %v, want 确定性拒收原样可辨", err)
	}
}

// Covers: 命令缺格答`未受理`，链一口都不问。
func TestAnIncompleteMomentIsNotAccepted(t *testing.T) {
	for name, mutate := range map[string]func(*application.RequestSellEvaluationOnParcelMomentCommand){
		"缺租户": func(c *application.RequestSellEvaluationOnParcelMomentCommand) { c.TenantID = domain.TenantID{} },
		"缺包裹": func(c *application.RequestSellEvaluationOnParcelMomentCommand) { c.Parcel = domain.ParcelReference{} },
		"缺时点": func(c *application.RequestSellEvaluationOnParcelMomentCommand) {
			c.Moment = domain.ParcelBillingMomentInvalid
		},
		"缺来源事实": func(c *application.RequestSellEvaluationOnParcelMomentCommand) {
			c.Source = domain.SourceFactReference{}
		},
		"缺计价基准时点": func(c *application.RequestSellEvaluationOnParcelMomentCommand) { c.PricingBasisAt = time.Time{} },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newParcelMomentFixture(t, domain.BillingMomentNetworkIntakeAdopted)
			fixture.resolutions.err = errors.New("syn: 不该问到链")
			command := parcelMomentCommand(t, domain.BillingMomentNetworkIntakeAdopted, "SYN-SOURCE-1", parcelMomentNow)
			mutate(&command)
			if result := fixture.handle(t, command); result.Outcome() != application.SellEvaluationRequestNotAccepted {
				t.Fatalf("outcome = %q, want SELL_EVALUATION_REQUEST_NOT_ACCEPTED", result.Outcome())
			}
			fixture.assertNothingRequested(t)
		})
	}
}

func TestTheSellMomentRequestHandlerRefusesAMissingDependency(t *testing.T) {
	_, err := application.NewRequestSellEvaluationOnParcelMomentHandler(application.RequestSellEvaluationOnParcelMomentDeps{})
	if !errors.Is(err, application.ErrNilDependency) {
		t.Fatalf("err = %v, want ErrNilDependency", err)
	}
}
