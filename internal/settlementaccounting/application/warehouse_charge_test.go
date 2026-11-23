package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestAnUnreadableRequestLedgerDoesNotSettle(t *testing.T) {
	unread := errors.New("request ledger unreadable")
	world := &settleWorld{chargesErr: unread}
	outcome, chargeID, err := world.handler(t).Settle(context.Background(), settleTenant(t), settleFact(1))
	if !errors.Is(err, unread) || outcome != "" || chargeID != "" || len(world.saved) != 0 || world.charges.estimated != 0 {
		t.Fatalf("读不动 %s %s %v 请求%d 费用%d", outcome, chargeID, err, len(world.saved), world.charges.estimated)
	}
}

func TestAPriorRequestNamingAMissingChargeDoesNotSettle(t *testing.T) {
	world := &settleWorld{rows: []application.WarehouseChargeRequest{{
		Occurrence: "CHO-1", Version: 1, Status: "FORMED", ChargeID: "CHG-PRIOR",
	}}}
	outcome, _, err := world.handler(t).Settle(context.Background(), settleTenant(t), settleFact(2))
	if err == nil || !strings.Contains(err.Error(), "not on the register") || outcome != "" || len(world.saved) != 0 || world.charges.estimated != 0 {
		t.Fatalf("费用册查无 %s %v 请求%d 费用%d", outcome, err, len(world.saved), world.charges.estimated)
	}
}

func TestNoAdoptionRowStaysNotConfigured(t *testing.T) {
	world := &settleWorld{}
	outcome, chargeID, err := world.handler(t).Settle(context.Background(), settleTenant(t), settleFact(1))
	if err != nil || outcome != application.WarehouseChargeNotConfigured || chargeID != "" ||
		len(world.saved) != 1 || world.saved[0].Status != "NOT_CONFIGURED" || world.charges.estimated != 0 {
		t.Fatalf("没有采用行 %s %s %v %+v", outcome, chargeID, err, world.saved)
	}
}

func TestAnUnreadableAdoptionRegisterDoesNotSettle(t *testing.T) {
	unread := errors.New("adoption ledger unreadable")
	world := &settleWorld{adoptionsErr: unread}
	outcome, _, err := world.handler(t).Settle(context.Background(), settleTenant(t), settleFact(1))
	if !errors.Is(err, unread) || outcome != "" || len(world.saved) != 0 || world.charges.estimated != 0 {
		t.Fatalf("采用册读不动 %s %v 请求%d", outcome, err, len(world.saved))
	}
}

func TestAnAdoptionNamingAMissingAccountDoesNotSettle(t *testing.T) {
	world := &settleWorld{
		adoptions: []application.WarehouseAdoption{settleAdoption()},
		price:     settlePrice(),
	}
	outcome, _, err := world.handler(t).Settle(context.Background(), settleTenant(t), settleFact(1))
	if err == nil || !strings.Contains(err.Error(), "not on the register") || outcome != "" || len(world.saved) != 0 || world.charges.estimated != 0 {
		t.Fatalf("账户查无 %s %v 请求%d 费用%d", outcome, err, len(world.saved), world.charges.estimated)
	}
}

func TestAnUnreadableAccountRegisterDoesNotSettle(t *testing.T) {
	unread := errors.New("account ledger unreadable")
	world := &settleWorld{
		adoptions:    []application.WarehouseAdoption{settleAdoption()},
		price:        settlePrice(),
		account:      settleAccount(t),
		accountFound: true,
		accountErr:   unread,
	}
	outcome, _, err := world.handler(t).Settle(context.Background(), settleTenant(t), settleFact(1))
	if !errors.Is(err, unread) || outcome != "" || len(world.saved) != 0 || world.charges.estimated != 0 {
		t.Fatalf("账户读不动 %s %v 请求%d", outcome, err, len(world.saved))
	}
}

func TestAMissingFeeItemMappingStaysPending(t *testing.T) {
	world := readyToForm(t)
	outcome, chargeID, err := world.handler(t).Settle(context.Background(), settleTenant(t), settleFact(1))
	if err != nil || outcome != application.WarehouseChargePending || chargeID != "" ||
		len(world.saved) != 1 || world.saved[0].Status != "PENDING" || world.charges.estimated != 0 {
		t.Fatalf("没映射 %s %s %v %+v 费用%d", outcome, chargeID, err, world.saved, world.charges.estimated)
	}
}

func TestAnUnreadableFeeItemMappingDoesNotSettle(t *testing.T) {
	unread := errors.New("mapping ledger unreadable")
	world := readyToForm(t)
	world.mappingErr = unread
	outcome, _, err := world.handler(t).Settle(context.Background(), settleTenant(t), settleFact(1))
	if !errors.Is(err, unread) || outcome != "" || len(world.saved) != 0 || world.charges.estimated != 0 {
		t.Fatalf("映射读不动 %s %v 请求%d", outcome, err, len(world.saved))
	}
}

func TestAnUnreadableConfirmationMomentDoesNotSettle(t *testing.T) {
	world := readyToForm(t)
	world.mapping = settleMapping(t)
	world.mappingFound = true
	world.momentErr = errors.New("confirmation moment unreadable")
	outcome, _, err := world.handler(t).Settle(context.Background(), settleTenant(t), settleFact(1))
	if err == nil || !strings.Contains(err.Error(), "SETTLEMENT_MOMENT_VIEW_UNAVAILABLE") || outcome != "" || len(world.saved) != 0 || world.charges.confirmed != 0 {
		t.Fatalf("确认时点读不动 %s %v 请求%d 确认%d", outcome, err, len(world.saved), world.charges.confirmed)
	}
}

func TestAnUnconfiguredConfirmationMomentFormsWithoutConfirming(t *testing.T) {
	world := readyToForm(t)
	world.mapping = settleMapping(t)
	world.mappingFound = true
	outcome, chargeID, err := world.handler(t).Settle(context.Background(), settleTenant(t), settleFact(1))
	saved := application.WarehouseChargeRequest{}
	if len(world.saved) == 1 {
		saved = world.saved[0]
	}
	if err != nil || outcome != application.WarehouseChargeFormed || chargeID == "" ||
		len(world.saved) != 1 || saved.Status != "FORMED" || saved.RequestID != "CHO-1/1" ||
		saved.Account != "SYN-ACCT-WH" || saved.Scope != "SYN-SCOPE-WH" || saved.Currency != "CNY" ||
		saved.EvaluationID != "WEV-1" || saved.BasisAt.IsZero() ||
		world.charges.estimated != 1 || world.charges.confirmed != 0 {
		t.Fatalf("没登记确认时点 %s %s %v %+v 费用%d 确认%d", outcome, chargeID, err, world.saved, world.charges.estimated, world.charges.confirmed)
	}
}

func TestConfirmingWithoutTheNamedAccountDoesNotSettle(t *testing.T) {
	world := readyToConfirm(t)
	world.accountMissOn = 2
	outcome, _, err := world.handler(t).Settle(context.Background(), settleTenant(t), settleFact(1))
	if err == nil || !strings.Contains(err.Error(), "not on the register") || outcome != "" || len(world.saved) != 0 || world.charges.confirmed != 0 {
		t.Fatalf("确认时账户查无 %s %v 请求%d 确认%d", outcome, err, len(world.saved), world.charges.confirmed)
	}
}

func TestAnUnreadableConfirmationConditionDoesNotSettle(t *testing.T) {
	unread := errors.New("condition ledger unreadable")
	world := readyToConfirm(t)
	world.conditionErr = unread
	outcome, _, err := world.handler(t).Settle(context.Background(), settleTenant(t), settleFact(1))
	if !errors.Is(err, unread) || outcome != "" || len(world.saved) != 0 || world.charges.confirmed != 0 {
		t.Fatalf("确认条件读不动 %s %v 请求%d", outcome, err, len(world.saved))
	}
}

func readyToForm(t *testing.T) *settleWorld {
	t.Helper()
	return &settleWorld{
		adoptions:    []application.WarehouseAdoption{settleAdoption()},
		price:        settlePrice(),
		account:      settleAccount(t),
		accountFound: true,
	}
}

func readyToConfirm(t *testing.T) *settleWorld {
	t.Helper()
	world := readyToForm(t)
	world.mapping = settleMapping(t)
	world.mappingFound = true
	world.momentFound = true
	world.conditionFound = true
	world.condition = ports.ConfirmationCondition{
		Met:   true,
		Basis: mustValue(t, domain.NewConfirmationBasisReference, "CHO-1/1"),
	}
	return world
}

type settleWorld struct {
	saved          []application.WarehouseChargeRequest
	chargesErr     error
	chargeReads    int
	rows           []application.WarehouseChargeRequest
	adoptions      []application.WarehouseAdoption
	adoptionsErr   error
	price          application.WarehouseChargePrice
	account        domain.SettlementAccount
	accountFound   bool
	accountErr     error
	accountMissOn  int
	mapping        domain.FeeItemMapping
	mappingFound   bool
	mappingErr     error
	momentErr      error
	momentFound    bool
	condition      ports.ConfirmationCondition
	conditionFound bool
	conditionErr   error
	accounts       *accountBook
	charges        *chargeBook
}

func (world *settleWorld) handler(t *testing.T) *application.WarehouseChargeHandler {
	t.Helper()
	world.accounts = &accountBook{
		account: world.account, found: world.accountFound, err: world.accountErr, missOn: world.accountMissOn,
	}
	world.charges = &chargeBook{}
	moments, err := application.NewAdmitSettlementMomentHandler(world)
	if err != nil {
		t.Fatal(err)
	}
	return application.NewWarehouseChargeHandler(
		world, world, world.accounts, world, world.charges, world, world, moments, nil, idSource{},
		settleClock{at: time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)},
	)
}

func (world *settleWorld) SaveAdoption(context.Context, domain.TenantID, application.WarehouseAdoption) error {
	return nil
}

func (world *settleWorld) LoadAdoptions(context.Context, domain.TenantID, string, string) ([]application.WarehouseAdoption, error) {
	if world.adoptionsErr != nil {
		return nil, world.adoptionsErr
	}
	return world.adoptions, nil
}

func (world *settleWorld) SaveRequest(_ context.Context, _ domain.TenantID, request application.WarehouseChargeRequest) error {
	world.saved = append(world.saved, request)
	return nil
}

func (world *settleWorld) LoadRequest(context.Context, domain.TenantID, string, int) (application.WarehouseChargeRequest, bool, error) {
	return application.WarehouseChargeRequest{}, false, nil
}

func (world *settleWorld) LoadCharges(context.Context, domain.TenantID, string) ([]application.WarehouseChargeRequest, error) {
	world.chargeReads++
	if world.chargesErr != nil && world.chargeReads == 1 {
		return nil, world.chargesErr
	}
	return world.rows, nil
}

func (world *settleWorld) Price(context.Context, string, string, time.Time, application.WarehouseOccurrenceFact) (application.WarehouseChargePrice, error) {
	return world.price, nil
}

func (world *settleWorld) FindFeeItemMapping(context.Context, domain.TenantID, domain.PricingChargeCode) (domain.FeeItemMapping, bool, error) {
	if world.mappingErr != nil {
		return domain.FeeItemMapping{}, false, world.mappingErr
	}
	return world.mapping, world.mappingFound, nil
}

func (world *settleWorld) RecordConfirmationBasis(
	context.Context, domain.TenantID, domain.CustomerChargeID, domain.FeeItemReference,
	domain.ConfirmationBasisKind, domain.ConfirmationBasisReference, time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	return 0, nil
}

func (world *settleWorld) LoadConfirmationCondition(
	context.Context, domain.TenantID, domain.CustomerChargeID, domain.FeeItemReference,
) (ports.ConfirmationCondition, bool, error) {
	if world.conditionErr != nil {
		return ports.ConfirmationCondition{}, false, world.conditionErr
	}
	return world.condition, world.conditionFound, nil
}

func (world *settleWorld) LoadSettlementMoment(context.Context, domain.TenantID, domain.SettlementMoment) (bool, error) {
	if world.momentErr != nil {
		return false, world.momentErr
	}
	return world.momentFound, nil
}

type accountBook struct {
	account domain.SettlementAccount
	found   bool
	err     error
	missOn  int
	reads   int
}

func (book *accountBook) FindByID(context.Context, domain.TenantID, domain.SettlementAccountID) (domain.SettlementAccount, bool, error) {
	book.reads++
	if book.err != nil {
		return domain.SettlementAccount{}, false, book.err
	}
	if book.missOn > 0 && book.reads == book.missOn {
		return domain.SettlementAccount{}, false, nil
	}
	return book.account, book.found, nil
}

type chargeBook struct {
	estimated int
	confirmed int
	held      domain.CustomerCharge
	hasHeld   bool
}

func (book *chargeBook) SaveEstimated(context.Context, domain.TenantID, domain.CustomerCharge) (ports.ChargeSaveOutcome, error) {
	book.estimated++
	return ports.ChargeSaved, nil
}

func (book *chargeBook) SaveConfirmed(context.Context, domain.TenantID, domain.CustomerCharge) (ports.ChargeSaveOutcome, error) {
	book.confirmed++
	return ports.ChargeSaved, nil
}

func (book *chargeBook) FindByID(_ context.Context, _ domain.TenantID, id domain.CustomerChargeID) (domain.CustomerCharge, bool, error) {
	if book.hasHeld && book.held.ID() == id {
		return book.held, true, nil
	}
	return domain.CustomerCharge{}, false, nil
}

type scriptedDifference struct {
	adopted bool
	outcome application.ReweighDifferenceOutcome
	seen    application.ReweighDifferenceCommand
	calls   int
}

func (script *scriptedDifference) ReweighAdjustmentAdopted(context.Context, domain.TenantID, domain.SettlementAccountID) (bool, error) {
	return script.adopted, nil
}

func (script *scriptedDifference) SettleReweigh(_ context.Context, command application.ReweighDifferenceCommand) (application.ReweighDifferenceOutcome, error) {
	script.calls++
	script.seen = command
	return script.outcome, nil
}

func TestAConfirmedWarehouseCorrectionSettlesTheDifferenceWhenAdopted(t *testing.T) {
	world := &settleWorld{
		adoptions: []application.WarehouseAdoption{settleAdoption()},
		price:     settlePrice(), account: settleAccount(t), accountFound: true,
		mapping: settleMapping(t), mappingFound: true,
	}
	handler := world.handler(t)
	charge := confirmedWarehouseCharge(t)
	world.charges.held = charge
	world.charges.hasHeld = true
	world.rows = []application.WarehouseChargeRequest{{
		Occurrence: "CHO-1", Version: 1, Status: "CONFIRMED", ChargeID: charge.ID().String(),
	}}
	script := &scriptedDifference{adopted: true, outcome: application.ReweighDifferenceRecorded}
	handler.WithConfirmedDifference(script, script)
	outcome, id, err := handler.Settle(context.Background(), settleTenant(t), settleFact(2))
	if err != nil || outcome != application.WarehouseChargeDifference || id != charge.ID().String() || script.calls != 1 || script.seen.SettlementMinor != 400 {
		t.Fatalf("差额 %s %s 调用 %d 金额 %d err %v", outcome, id, script.calls, script.seen.SettlementMinor, err)
	}
	script.adopted = false
	again, _, err := handler.Settle(context.Background(), settleTenant(t), settleFact(3))
	world.rows = append(world.rows, application.WarehouseChargeRequest{Occurrence: "CHO-1", Version: 2, Status: "DIFFERENCE", ChargeID: charge.ID().String()})
	if err != nil || again != application.WarehouseChargeNotConfigured || script.calls != 1 {
		t.Fatalf("没采用 %s 调用 %d err %v", again, script.calls, err)
	}
}

func confirmedWarehouseCharge(t *testing.T) domain.CustomerCharge {
	t.Helper()
	formed, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID:               mustValue(t, domain.NewCustomerChargeID, "CHG-CONFIRMED"),
		FeeItem:          mustValue(t, domain.NewFeeItemReference, "SYN-FEE-WH"),
		Evaluation:       mustValue(t, domain.NewSellEvaluationReference, "WEV-0"),
		OriginalCurrency: mustValue(t, domain.NewCurrencyCode, "CNY"), OriginalMinor: 100,
		SettlementCurrency: mustValue(t, domain.NewCurrencyCode, "CNY"), SettlementMinor: 100,
		Stage: domain.ChargeEstimated, FormedAt: time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := formed.Confirm(domain.ConfirmedChargeFacts{
		ResponsibleEntity:    mustValue(t, domain.NewLegalEntityReference, "SYN-LE"),
		Counterparty:         mustValue(t, domain.NewSettlementCounterpartyReference, "SYN-OWNER-01"),
		Direction:            domain.ChargeReceivable,
		SettlementAccount:    mustValue(t, domain.NewSettlementAccountID, "SYN-ACCT-WH"),
		ContractBasis:        mustValue(t, domain.NewContractBasisReference, "SYN-CONTRACT"),
		PrimaryChargingScope: mustValue(t, domain.NewChargingScopeReference, "SYN-SCOPE-WH"),
		SourceFact:           mustValue(t, domain.NewSourceFactReference, "CHO-1/1"),
	}, mustValue(t, domain.NewConfirmationBasisReference, "WAREHOUSE_WORK_OCCURRED/CHO-1/1"), time.Date(2026, 11, 20, 3, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return confirmed
}

type idSource struct{}

func (idSource) Next() (string, error) { return "CHG-1", nil }

type settleClock struct{ at time.Time }

func (clock settleClock) Now() time.Time { return clock.at }

func settleFact(version int) application.WarehouseOccurrenceFact {
	return application.WarehouseOccurrenceFact{
		ID: "CHO-1", Version: version, Kind: "INBOUND_HANDLING", Owner: "SYN-OWNER-01", Warehouse: "SYN-WH-01",
		Occurred: time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC),
	}
}

func settleAdoption() application.WarehouseAdoption {
	return application.WarehouseAdoption{
		Owner: "SYN-OWNER-01", Warehouse: "SYN-WH-01", Revision: 1,
		Effective: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		Account:   "SYN-ACCT-WH", Scope: "SYN-SCOPE-WH", Basis: "SYN-BASIS", Registrant: "operator-1",
	}
}

func settlePrice() application.WarehouseChargePrice {
	return application.WarehouseChargePrice{
		EvaluationID: "WEV-1", Status: "COMPLETED", TotalMinor: 400, Currency: "CNY", Codes: []string{"INBOUND_PIECE"},
	}
}

func settleAccount(t *testing.T) domain.SettlementAccount {
	t.Helper()
	key, err := domain.NewSettlementAccountKey(
		mustValue(t, domain.NewLegalEntityReference, "SYN-LE"),
		mustValue(t, domain.NewSettlementCounterpartyReference, "SYN-OWNER-01"),
		domain.ChargeReceivable,
		mustValue(t, domain.NewCurrencyCode, "CNY"),
		mustValue(t, domain.NewSettlementPolicyReference, "SYN-POL"),
	)
	if err != nil {
		t.Fatal(err)
	}
	account, err := domain.NewSettlementAccount(
		mustValue(t, domain.NewSettlementAccountID, "SYN-ACCT-WH"),
		key, domain.SettlementCounterpartyReference{}, false,
		mustValue(t, domain.NewResponsibilityBasis, "SYN-CONTRACT"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func settleMapping(t *testing.T) domain.FeeItemMapping {
	t.Helper()
	mapping, err := domain.NewFeeItemMapping(
		mustValue(t, domain.NewPricingChargeCode, "INBOUND_PIECE"),
		mustValue(t, domain.NewFeeItemReference, "SYN-FEE-WH"),
		mustValue(t, domain.NewFeeItemMappingBasisReference, "SYN-BASIS"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return mapping
}

func settleTenant(t *testing.T) domain.TenantID {
	t.Helper()
	return mustValue(t, domain.NewTenantID, "SYN-TENANT-01")
}

func mustValue[T any](t *testing.T, build func(string) (T, error), value string) T {
	t.Helper()
	built, err := build(value)
	if err != nil {
		t.Fatal(err)
	}
	return built
}
