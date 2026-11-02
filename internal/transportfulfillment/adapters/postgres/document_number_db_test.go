package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberpg"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfdocnumber "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/docnumber"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证委托与订舱的取号（ADR-0355 决定三、七、八）：TF 的委托、订舱仓储与编号组件的规则册、平台序列
// 接在同一个库上，取号编排的时钟由用例拨，周期与时区因此可断言。开立事务的壳照 parcel-api 那只：答`未决`或编号规则撞号
// 时整笔回滚。

// numberingClock 是取号与落册共用的时钟，用例拨它换周期。
type numberingClock struct{ at time.Time }

func (clock *numberingClock) Now() time.Time { return clock.at }

type registeredAgreementStub struct{}

func (registeredAgreementStub) CheckSupplierAgreement(
	_ context.Context, _ domain.TenantID, _ domain.SupplierAgreementVersion, provider domain.ServiceProviderReference, _ time.Time,
) (ports.SupplierAgreementCheck, error) {
	snapshot, _ := domain.NewAgreementSnapshotReference("agreement-1/v1")
	legalEntity, _ := domain.NewProcurementLegalEntityReference("legal-entity-1")
	return ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementRegistered, Snapshot: snapshot, LegalEntity: legalEntity, Supplier: provider}, nil
}

type anyScheduleStub struct{}

func (anyScheduleStub) FindByKey(_ context.Context, key ports.ScheduleKey) (ports.ScheduleRecord, bool, error) {
	return ports.ScheduleRecord{Key: key}, true, nil
}

type silentCommissionHandoff struct{}

func (silentCommissionHandoff) HandOffTransportCommission(context.Context, ports.TransportCommissionIntent) error {
	return nil
}

type numberingChain struct {
	db          *bentopg.DB
	clock       *numberingClock
	registrar   *docnumber.Registrar
	commissions *adapter.TransportCommissions
	bookings    *adapter.BookingRequests
	catalogue   *adapter.ReviewCatalogue
	handler     *application.CommissionTransportHandler
}

func newNumberingChain(t *testing.T) numberingChain {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	chain := numberingChain{db: db, clock: &numberingClock{at: time.Date(2026, 11, 2, 3, 0, 0, 0, time.UTC)}}
	if chain.commissions, err = adapter.NewTransportCommissions(db); err != nil {
		t.Fatal(err)
	}
	if chain.bookings, err = adapter.NewBookingRequests(db); err != nil {
		t.Fatal(err)
	}
	if chain.catalogue, err = adapter.NewReviewCatalogue(db); err != nil {
		t.Fatal(err)
	}
	catalogue, err := docnumber.NewCatalogue(domain.DocumentNumberTypes())
	if err != nil {
		t.Fatal(err)
	}
	book, err := docnumberpg.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	sequences, err := docnumberpg.NewSequences(db)
	if err != nil {
		t.Fatal(err)
	}
	secrets := docnumber.SecretTable{}
	assigner, err := docnumber.NewAssigner(docnumber.AssignerDeps{
		Catalogue: catalogue, Book: book, Sequences: sequences, Transactions: book, Secrets: secrets, Clock: chain.clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	if chain.registrar, err = docnumber.NewRegistrar(docnumber.RegistrarDeps{Catalogue: catalogue, Book: book, Secrets: secrets, Clock: chain.clock}); err != nil {
		t.Fatal(err)
	}
	issuer, err := tfdocnumber.NewIssuer(assigner)
	if err != nil {
		t.Fatal(err)
	}
	chain.handler = application.NewCommissionTransportHandler(application.CommissionTransportDeps{
		Commissions:       chain.commissions,
		Bookings:          chain.bookings,
		Schedules:         anyScheduleStub{},
		Agreements:        registeredAgreementStub{},
		Downstream:        silentCommissionHandoff{},
		Clock:             chain.clock,
		Numbers:           issuer,
		CommissionNumbers: chain.commissions,
		BookingNumbers:    chain.bookings,
	})
	return chain
}

func (chain numberingChain) registerTimeZone(t *testing.T, tenant string) {
	t.Helper()
	if err := chain.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := chain.registrar.RegisterTimeZone(txCtx, docnumber.RegisterTimeZoneCommand{
			Tenant: tenant, TimeZone: "Asia/Shanghai", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
		})
		return err
	}); err != nil {
		t.Fatalf("登单据编号时区：%v", err)
	}
}

// errRollBackIssue 让开立事务回滚，同 parcel-api 的壳：`未决`、编号规则撞号，或用例要看回滚释放号码。
var errRollBackIssue = errors.New("roll back the issuing transaction")

// issue 在一笔开立事务里跑 handle；答`未决`或编号规则撞号、或 rollBack 为真时整笔回滚。
func (chain numberingChain) issue(
	t *testing.T,
	rollBack bool,
	handle func(context.Context) (application.CommissionResult, error),
) application.CommissionResult {
	t.Helper()
	var result application.CommissionResult
	err := chain.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		handled, err := handle(txCtx)
		if err != nil {
			return err
		}
		result = handled
		if rollBack || handled.Outcome() == application.CommissionUndecided || handled.Outcome() == application.CommissionNumberingRuleCollision {
			return errRollBackIssue
		}
		return nil
	})
	if err != nil && !errors.Is(err, errRollBackIssue) {
		t.Fatalf("开立事务：%v", err)
	}
	return result
}

func (chain numberingChain) submitCommission(t *testing.T, tenant, commission string, rollBack bool) application.CommissionResult {
	t.Helper()
	command := application.SubmitCommissionCommand{
		TenantID: deliveryValue(t, domain.NewTenantID, tenant), Commission: commission, Provider: "partner-1",
		AgreementObject: "agreement-1", AgreementVersion: "v1", Conditions: "conditions-1", Role: "role-1",
		Responsibility: "responsibility-1", Members: []string{"parcel-1"}, SubmittedAt: chain.clock.at,
	}
	return chain.issue(t, rollBack, func(ctx context.Context) (application.CommissionResult, error) {
		return chain.handler.SubmitCommission(ctx, command)
	})
}

func (chain numberingChain) submitBooking(t *testing.T, tenant, booking, commission string) application.CommissionResult {
	t.Helper()
	command := application.SubmitBookingCommand{
		TenantID: deliveryValue(t, domain.NewTenantID, tenant), Booking: booking, Commission: commission,
		Schedule: "schedule-1", Quantity: 10, Unit: "KG", RequestedAt: chain.clock.at,
	}
	return chain.issue(t, false, func(ctx context.Context) (application.CommissionResult, error) {
		return chain.handler.SubmitBooking(ctx, command)
	})
}

func commissionNumber(t *testing.T, result application.CommissionResult) string {
	t.Helper()
	record, present := result.Commission()
	if !present {
		t.Fatalf("没有回显委托：%s", result.Outcome())
	}
	number, numbered := record.DocumentNumber.Number()
	if !numbered {
		t.Fatalf("委托没有单据号：%s", result.Outcome())
	}
	return number.String()
}

// Covers: AT-TF-371、AT-TF-372（真库面）——委托进入已提交、订舱登记时在开立事务里取号，默认模板按租户的单据编号时区出
// 日期；同一开立意图重放答原号、序列不动；两个租户各从 1 数；跨过上海时间的月界从 1 重数（UTC 那一刻仍在上个月）；开立
// 回滚连号码一起释放，下一份取到同一个号；租户没登时区则照常开立、记无号原因；订舱册带订舱与委托两个号。
func TestCommissionAndBookingNumbersAreIssuedInTheIssuingTransaction(t *testing.T) {
	chain := newNumberingChain(t)
	chain.registerTimeZone(t, "tenant-a")
	chain.registerTimeZone(t, "tenant-b")

	first := chain.submitCommission(t, "tenant-a", "commission-1", false)
	if first.Outcome() != application.CommissionSubmitted || commissionNumber(t, first) != "TC-202611-00001" {
		t.Fatalf("首份委托 = %s %s", first.Outcome(), commissionNumber(t, first))
	}
	replay := chain.submitCommission(t, "tenant-a", "commission-1", false)
	if replay.Outcome() != application.CommissionExistingResult || commissionNumber(t, replay) != "TC-202611-00001" {
		t.Fatalf("重放 = %s %s", replay.Outcome(), commissionNumber(t, replay))
	}
	if got := commissionNumber(t, chain.submitCommission(t, "tenant-a", "commission-2", false)); got != "TC-202611-00002" {
		t.Fatalf("第二份委托 = %s——重放占了号", got)
	}
	if got := commissionNumber(t, chain.submitCommission(t, "tenant-b", "commission-1", false)); got != "TC-202611-00001" {
		t.Fatalf("另一租户 = %s，want 各自从 1 数", got)
	}

	booking := chain.submitBooking(t, "tenant-a", "booking-1", "commission-1")
	record, _ := booking.Booking()
	if number, _ := record.DocumentNumber.Number(); booking.Outcome() != application.BookingSubmitted || number.String() != "BK-202611-00001" {
		t.Fatalf("订舱 = %s %s", booking.Outcome(), number)
	}
	replayed, _ := chain.submitBooking(t, "tenant-a", "booking-1", "commission-1").Booking()
	if number, _ := replayed.DocumentNumber.Number(); number.String() != "BK-202611-00001" {
		t.Fatalf("订舱重放 = %s", number)
	}

	// 上海 12 月 1 日 00:30，UTC 仍是 11 月 30 日：周期按租户的单据编号时区切。
	chain.clock.at = time.Date(2026, 11, 30, 16, 30, 0, 0, time.UTC)
	if got := commissionNumber(t, chain.submitCommission(t, "tenant-a", "commission-3", false)); got != "TC-202612-00001" {
		t.Fatalf("跨月 = %s", got)
	}
	rolledBack := chain.submitCommission(t, "tenant-a", "commission-4", true)
	if commissionNumber(t, rolledBack) != "TC-202612-00002" {
		t.Fatalf("回滚的那一份 = %s", commissionNumber(t, rolledBack))
	}
	if _, found, err := chain.commissions.FindByKey(t.Context(), ports.TransportCommissionKey{
		TenantID: deliveryValue(t, domain.NewTenantID, "tenant-a"), Commission: deliveryValue(t, domain.NewTransportCommissionReference, "commission-4"),
	}); err != nil || found {
		t.Fatalf("回滚的委托仍在册：%v %v", found, err)
	}
	if got := commissionNumber(t, chain.submitCommission(t, "tenant-a", "commission-5", false)); got != "TC-202612-00002" {
		t.Fatalf("回滚之后 = %s，want 释放的那个号", got)
	}

	unconfigured := chain.submitCommission(t, "tenant-c", "commission-1", false)
	stored, found, err := chain.commissions.FindByKey(t.Context(), ports.TransportCommissionKey{
		TenantID: deliveryValue(t, domain.NewTenantID, "tenant-c"), Commission: deliveryValue(t, domain.NewTransportCommissionReference, "commission-1"),
	})
	if err != nil || !found || unconfigured.Outcome() != application.CommissionSubmitted {
		t.Fatalf("没登时区的租户 = %s %v %v", unconfigured.Outcome(), found, err)
	}
	if absence, ok := stored.DocumentNumber.Absence(); !ok || absence != domain.DocumentNumberingTimeZoneNotConfigured {
		t.Fatalf("没登时区读回 %s %v", absence, ok)
	}

	rows, err := chain.catalogue.ListBookingRequests(t.Context(), deliveryValue(t, domain.NewTenantID, "tenant-a"), 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("订舱册：%v %d", err, len(rows))
	}
	if rows[0].CommissionDocumentNumber == nil {
		t.Fatal("订舱册那一行没带委托号格")
	}
	commissionNumber, _ := rows[0].CommissionDocumentNumber.Number()
	if number, _ := rows[0].DocumentNumber.Number(); number.String() != "BK-202611-00001" || commissionNumber.String() != "TC-202611-00001" {
		t.Fatalf("订舱册那一行：%s / %s", number, commissionNumber)
	}
}

// Covers: AT-TF-371（真库面：开始不换号）；ADR-0355 决定八的最后一道保险与迁移 0057 的形状——同一租户同一个号第二次记即撞
// 唯一约束，他租户可用；一份单据只记一次号，开始经 Replace 落、不动号；号与无号原因互斥、原因只认那一个取值、号不得空白；
// 从没记过号的旧行读回「本规则生效前开立」。
func TestTheDocumentNumberColumnsGuardUniquenessAndShape(t *testing.T) {
	commissions, bookings, _, transactor, pool := newCommissionStores(t)
	ctx := t.Context()
	for _, record := range []ports.TransportCommissionRecord{
		submittedCommission(t, "tenant-a", "commission-1"),
		submittedCommission(t, "tenant-a", "commission-2"),
		submittedCommission(t, "tenant-b", "commission-1"),
	} {
		mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			_, err := commissions.Save(txCtx, record)
			return err
		})
	}
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := bookings.Save(txCtx, submittedBookingRecord(t, "tenant-a", "booking-1"))
		return err
	})

	legacy, _, err := commissions.FindByKey(ctx, submittedCommission(t, "tenant-a", "commission-2").Key)
	if absence, ok := legacy.DocumentNumber.Absence(); err != nil || !ok || absence != domain.IssuedBeforeDocumentNumbering {
		t.Fatalf("没记过号的行读回 %s %v %v", absence, ok, err)
	}

	number := domain.NumberedDocument(deliveryValue(t, domain.NewDocumentNumber, "TC-202611-00001"))
	record := func(tenant, commission string, numbering domain.DocumentNumbering) error {
		return transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			return commissions.RecordCommissionDocumentNumber(txCtx, submittedCommission(t, tenant, commission).Key, numbering)
		})
	}
	if err := record("tenant-a", "commission-1", number); err != nil {
		t.Fatalf("记号：%v", err)
	}
	if err := record("tenant-a", "commission-2", number); err == nil {
		t.Fatal("同一租户同一个号第二次记应撞唯一约束")
	}
	if err := record("tenant-b", "commission-1", number); err != nil {
		t.Fatalf("他租户同一个号：%v", err)
	}
	if err := record("tenant-a", "commission-1", domain.NumberedDocument(deliveryValue(t, domain.NewDocumentNumber, "TC-202611-00009"))); err == nil {
		t.Fatal("已记过号的委托又记了一次")
	}
	if err := record("tenant-a", "commission-2", domain.DocumentNumbering{}); err == nil {
		t.Fatal("「本规则生效前开立」不该写到新开立的单据上")
	}
	used, err := commissions.CommissionDocumentNumberUsed(ctx, deliveryValue(t, domain.NewTenantID, "tenant-a"), deliveryValue(t, domain.NewDocumentNumber, "TC-202611-00001"))
	if err != nil || !used {
		t.Fatalf("已用的号答 %v %v", used, err)
	}
	if used, _ := commissions.CommissionDocumentNumberUsed(ctx, deliveryValue(t, domain.NewTenantID, "tenant-c"), deliveryValue(t, domain.NewDocumentNumber, "TC-202611-00001")); used {
		t.Fatal("他租户的号不该算本租户已用")
	}
	stored, _, _ := commissions.FindByKey(ctx, submittedCommission(t, "tenant-a", "commission-1").Key)
	if got, _ := stored.DocumentNumber.Number(); got.String() != "TC-202611-00001" {
		t.Fatalf("读回的号 = %s", got)
	}
	// 开始与取消经 Replace 落，只写转换那几列：号不动（ADR-0355 决定三「开立后不变」）。
	started, err := stored.Commission.MarkTransportStarted(deliveryValue(t, domain.NewParticipationBasisReference, "basis-1"), commissionAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("开始：%v", err)
	}
	stored.Commission = started
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := commissions.Replace(txCtx, stored)
		return err
	})
	afterStart, _, _ := commissions.FindByKey(ctx, stored.Key)
	if got, _ := afterStart.DocumentNumber.Number(); got.String() != "TC-202611-00001" {
		t.Fatalf("开始之后的号 = %s", got)
	}

	for name, statement := range map[string]string{
		"号与原因同在": `UPDATE transport_fulfillment.transport_commission SET document_number_absence = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED'
			WHERE tenant_id = 'tenant-a' AND commission_id = 'commission-1'`,
		"认不得的原因": `UPDATE transport_fulfillment.transport_commission SET document_number_absence = 'SOMETHING_ELSE'
			WHERE tenant_id = 'tenant-a' AND commission_id = 'commission-2'`,
		"空白的号": `UPDATE transport_fulfillment.transport_commission SET document_number = ' '
			WHERE tenant_id = 'tenant-a' AND commission_id = 'commission-2'`,
		"订舱号与原因同在": `UPDATE transport_fulfillment.booking_request SET document_number = 'BK-1', document_number_absence = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED'
			WHERE tenant_id = 'tenant-a' AND booking_id = 'booking-1'`,
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("%s 应被 CHECK 拒", name)
		}
	}

	bookingKey := submittedBookingRecord(t, "tenant-a", "booking-1").Key
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		return bookings.RecordBookingDocumentNumber(txCtx, bookingKey, domain.UnnumberedDocument(domain.DocumentNumberingTimeZoneNotConfigured))
	})
	booked, _, err := bookings.FindByKey(ctx, bookingKey)
	if absence, ok := booked.DocumentNumber.Absence(); err != nil || !ok || absence != domain.DocumentNumberingTimeZoneNotConfigured {
		t.Fatalf("订舱无号原因读回 %s %v %v", absence, ok, err)
	}
}

// Covers: AT-TF-374（真库面）——引用委托、订舱的册读回时连读所引单据的号格：总单按键读、读当前版与查阅页列册都带所引委托的号与所引
// 订舱的无号原因，所引订舱不在册时那一格为 nil（不编一个「上线前开立」）；未形成册列册时带所在委托的号。
func TestRegistersThatReferToACommissionOrBookingReadItsNumber(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	transactor := db.Transactor()
	commissions, _ := adapter.NewTransportCommissions(db)
	bookings, _ := adapter.NewBookingRequests(db)
	documents, _ := adapter.NewMasterDocuments(db)
	catalogue, _ := adapter.NewReviewCatalogue(db)
	unformeds, _ := adapter.NewActualFulfillmentUnformeds(db)

	commission := submittedCommission(t, "tenant-1", "commission-1")
	booking := submittedBookingRecord(t, "tenant-1", "booking-1")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := commissions.Save(txCtx, commission); err != nil {
			return err
		}
		if err := commissions.RecordCommissionDocumentNumber(txCtx, commission.Key,
			domain.NumberedDocument(deliveryValue(t, domain.NewDocumentNumber, "TC-202611-00004"))); err != nil {
			return err
		}
		if _, err := bookings.Save(txCtx, booking); err != nil {
			return err
		}
		return bookings.RecordBookingDocumentNumber(txCtx, booking.Key, domain.UnnumberedDocument(domain.DocumentNumberingTimeZoneNotConfigured))
	})
	withNumbers := masterDocumentRecord(t, masterDocumentFixtureOptions{document: "MAWB-1", version: "v1", commission: "commission-1", booking: "booking-1"})
	withGhost := masterDocumentRecord(t, masterDocumentFixtureOptions{document: "MAWB-2", version: "v1", commission: "commission-1", booking: "booking-ghost"})
	mustSaveMasterDocument(t, transactor, ctx, documents, withNumbers)
	mustSaveMasterDocument(t, transactor, ctx, documents, withGhost)

	check := func(name string, commissionNumbering, bookingNumbering *domain.DocumentNumbering, wantBooking bool) {
		t.Helper()
		if commissionNumbering == nil {
			t.Fatalf("%s：所引委托没带号格", name)
		}
		if number, _ := commissionNumbering.Number(); number.String() != "TC-202611-00004" {
			t.Fatalf("%s：所引委托号 = %s", name, number)
		}
		if !wantBooking {
			if bookingNumbering != nil {
				t.Fatalf("%s：不在册的订舱不该带号格", name)
			}
			return
		}
		if bookingNumbering == nil {
			t.Fatalf("%s：所引订舱没带号格", name)
		}
		if absence, ok := bookingNumbering.Absence(); !ok || absence != domain.DocumentNumberingTimeZoneNotConfigured {
			t.Fatalf("%s：所引订舱无号原因 = %s %v", name, absence, ok)
		}
	}
	found, _, err := documents.FindByKey(ctx, withNumbers.Key)
	if err != nil {
		t.Fatal(err)
	}
	check("按键读", found.CommissionDocumentNumber, found.BookingDocumentNumber, true)
	current, _, err := documents.FindCurrent(ctx, withGhost.Key.TenantID, withGhost.Key.Document)
	if err != nil {
		t.Fatal(err)
	}
	check("读当前版（订舱不在册）", current.CommissionDocumentNumber, current.BookingDocumentNumber, false)
	rows, err := catalogue.ListCarrierMasterDocuments(ctx, withNumbers.Key.TenantID, 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("总单册：%v %d", err, len(rows))
	}
	for _, row := range rows {
		check("总单册 "+row.Document, row.CommissionDocumentNumber, row.BookingDocumentNumber, row.Document == "MAWB-1")
	}

	key := unformedKey(t, "tenant-1", "SEG-A", "parcel-1")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		return unformeds.RecordUnformed(txCtx, unformedEntry(t, key, time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC), ports.UnformedNotConfigured),
			time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC))
	})
	listed, err := catalogue.ListActualFulfillmentUnformed(ctx, key.TenantID, 10)
	if err != nil || len(listed) != 1 || listed[0].CommissionDocumentNumber == nil {
		t.Fatalf("未形成册：%v %d", err, len(listed))
	}
	if number, _ := listed[0].CommissionDocumentNumber.Number(); number.String() != "TC-202611-00004" {
		t.Fatalf("未形成册的委托号 = %s", number)
	}
}
