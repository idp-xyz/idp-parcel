package postgres_test

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证本上下文各册照 ADR-0144 取页（票 catalogue-read-pagination-rollout/05 完成判据）：每一册走同一套
// 表驱动用例——空册答空页且 total 为 0；翻完第一页再添一行，后面的页不重出已翻过的行、原有的行一行不漏，total 随之变；换一个
// 可排维（只有一维可排的册取反向）照样翻得完；total 随筛选维与 q 变，q 不分大小写、% 按字面。行经各册的写侧适配器或编排真实
// 落库，夹具全为合成值（S 级）。

const pagedTenant = "SYN-TENANT-01"

var pagedAt = time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)

// pagedRegister 是一册真库翻页用例要的几样。add 往册里添第 index 行（1 起）并交回它的标识串；list 读一页；key 从行认出标识串；
// alternateSort 是缺省序之外再翻一遍用的排序；totals 是六行都在之后、按条件数出的总数。
type pagedRegister[Row any] struct {
	catalogue     *cataloguepage.Catalogue
	add           func(index int) string
	list          func(tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[Row], error)
	key           func(row Row) string
	alternateSort string
	totals        map[string]pagedTotal
}

type pagedTotal struct {
	values url.Values
	want   int64
}

func checkPagedRegister[Row any](t *testing.T, register pagedRegister[Row]) {
	t.Helper()
	tenant := deliveryValue(t, domain.NewTenantID, pagedTenant)

	empty, err := register.list(tenant, 2, firstPage(t, register.catalogue))
	if err != nil || len(empty.Rows) != 0 || empty.Total != 0 || empty.Next != "" {
		t.Fatalf("空册应答空页、total 为 0、没有下一页：%+v err=%v", empty, err)
	}

	existing := make([]string, 0, 5)
	for index := 1; index <= 5; index++ {
		existing = append(existing, register.add(index))
	}
	seen := map[string]bool{}
	page, err := register.list(tenant, 2, firstPage(t, register.catalogue))
	if err != nil || len(page.Rows) != 2 || page.Total != 5 || page.Next == "" {
		t.Fatalf("第一页应两行、total 5、有下一页：%d 行 total %d next %q err=%v", len(page.Rows), page.Total, page.Next, err)
	}
	collectPagedKeys(t, register.key, page.Rows, seen)
	register.add(6)
	for turns := 0; page.Next != ""; turns++ {
		if turns > 6 {
			t.Fatal("六行的册翻了七页还没到头")
		}
		page, err = register.list(tenant, 2, nextPage(t, register.catalogue, url.Values{}, page.Next))
		if err != nil || page.Total != 6 {
			t.Fatalf("接着翻：total %d err=%v，添了一行之后应为 6", page.Total, err)
		}
		collectPagedKeys(t, register.key, page.Rows, seen)
	}
	for _, key := range existing {
		if !seen[key] {
			t.Fatalf("翻页期间漏了原有的 %s（翻到的：%v）", key, seen)
		}
	}

	sorted := url.Values{"sort": {register.alternateSort}}
	walked := map[string]bool{}
	page, err = register.list(tenant, 2, pageQuery(t, register.catalogue, sorted))
	for turns := 0; ; turns++ {
		if err != nil || page.Total != 6 || turns > 6 {
			t.Fatalf("按 %s 翻第 %d 页：total %d err=%v", register.alternateSort, turns+1, page.Total, err)
		}
		collectPagedKeys(t, register.key, page.Rows, walked)
		if page.Next == "" {
			break
		}
		page, err = register.list(tenant, 2, nextPage(t, register.catalogue, sorted, page.Next))
	}
	if len(walked) != 6 {
		t.Fatalf("按 %s 翻完应见六行，见 %d 行：%v", register.alternateSort, len(walked), walked)
	}

	for name, total := range register.totals {
		got, err := register.list(tenant, 10, pageQuery(t, register.catalogue, total.values))
		if err != nil || got.Total != total.want || int64(len(got.Rows)) != total.want {
			t.Fatalf("%s（%v）：total %d、%d 行，应为 %d；err=%v", name, total.values, got.Total, len(got.Rows), total.want, err)
		}
	}
}

func collectPagedKeys[Row any](t *testing.T, key func(Row) string, rows []Row, seen map[string]bool) {
	t.Helper()
	for _, row := range rows {
		identity := key(row)
		if seen[identity] {
			t.Fatalf("%s 在后面的页上重出", identity)
		}
		seen[identity] = true
	}
}

// Covers: ADR-0144 决定一、四、五——运输履约查阅页 registry 分派的十三册与外部承运轨迹事实当前版，每一册都过一遍上面那套用例。
func TestEveryTransportFulfillmentCataloguePagesWithoutRepeatsOrGapsAndCountsUnderItsConditions(t *testing.T) {
	t.Run("transport-schedule", func(t *testing.T) {
		stores := newReviewCatalogueStores(t)
		checkPagedRegister(t, pagedRegister[ports.TransportScheduleCatalogueRow]{
			catalogue: ports.TransportSchedulePages,
			add: func(index int) string {
				id := fmt.Sprintf("SYN-SCHED-%d", index)
				saveScheduleRecord(t, stores, t.Context(),
					catalogueScheduleRecord(t, pagedTenant, id, scheduleDeparts.Add(time.Duration(index)*time.Hour)))
				return id
			},
			list: func(tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.TransportScheduleCatalogueRow], error) {
				return stores.catalogue.ListTransportSchedules(t.Context(), tenant, limit, query)
			},
			key:           func(row ports.TransportScheduleCatalogueRow) string { return row.ScheduleID },
			alternateSort: "scheduleId",
			totals: map[string]pagedTotal{
				"来历为临时班次":     {url.Values{"origin": {"AD_HOC"}}, 6},
				"来历为物化":       {url.Values{"origin": {"TEMPLATE"}}, 0},
				"q 不分大小写命中一班": {url.Values{"q": {"syn-sched-3"}}, 1},
				"q 的 % 按字面":   {url.Values{"q": {"%"}}, 0},
			},
		})
	})

	t.Run("transport-schedule-template", func(t *testing.T) {
		stores := newScheduleRegistryStores(t)
		tenant := deliveryValue(t, domain.NewTenantID, pagedTenant)
		checkPagedRegister(t, pagedRegister[ports.ScheduleTemplateCatalogueRow]{
			catalogue: ports.ScheduleTemplatePages,
			add: func(index int) string {
				code := fmt.Sprintf("SYN-TPL-%d", index)
				wantScheduleOutcome(t, stores.registerTemplate(t,
					syntheticTemplateCommand(tenant, code, fmt.Sprintf("SYN-CONN-%d", index), "22:00")),
					application.ScheduleTemplateRegistered)
				return code
			},
			list: func(tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.ScheduleTemplateCatalogueRow], error) {
				return stores.catalogue.ListScheduleTemplates(t.Context(), tenant, limit, query)
			},
			key:           func(row ports.ScheduleTemplateCatalogueRow) string { return row.Template },
			alternateSort: "-recordedAt",
			totals: map[string]pagedTotal{
				"q 命中代码":    {url.Values{"q": {"syn-tpl-3"}}, 1},
				"q 命中连接":    {url.Values{"q": {"SYN-CONN-4"}}, 1},
				"q 的 % 按字面": {url.Values{"q": {"%"}}, 0},
			},
		})
	})

	t.Run("capacity-pool", func(t *testing.T) {
		stores := newReviewCatalogueStores(t)
		checkPagedRegister(t, pagedRegister[ports.CapacityPoolCatalogueRow]{
			catalogue: ports.CapacityPoolPages,
			add: func(index int) string {
				schedule := fmt.Sprintf("SYN-SCHED-%d", index)
				savePoolRecord(t, stores, t.Context(), establishedPoolRecord(t, pagedTenant, schedule, 100))
				return schedule + "/WEIGHT"
			},
			list: func(tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.CapacityPoolCatalogueRow], error) {
				return stores.catalogue.ListCapacityPools(t.Context(), tenant, limit, query)
			},
			key:           func(row ports.CapacityPoolCatalogueRow) string { return row.PoolID },
			alternateSort: "-poolId",
			totals: map[string]pagedTotal{
				"按重量维":      {url.Values{"dimension": {"WEIGHT"}}, 6},
				"按体积维":      {url.Values{"dimension": {"VOLUME"}}, 0},
				"q 命中班次":    {url.Values{"q": {"syn-sched-3"}}, 1},
				"q 的 % 按字面": {url.Values{"q": {"%"}}, 0},
			},
		})
	})

	t.Run("transport-handover", func(t *testing.T) {
		stores := newReviewCatalogueStores(t)
		checkPagedRegister(t, pagedRegister[ports.TransportHandoverCatalogueRow]{
			catalogue: ports.TransportHandoverPages,
			add: func(index int) string {
				object, version := fmt.Sprintf("SYN-PARCEL-%d", index), fmt.Sprintf("SYN-HV-%d", index)
				mustSaveHandover(t, stores.transactor, t.Context(), stores.handovers, catalogueHandoverRecord(t, pagedTenant, object, version))
				return object + "|" + version
			},
			list: func(tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.TransportHandoverCatalogueRow], error) {
				return stores.catalogue.ListTransportHandovers(t.Context(), tenant, limit, query)
			},
			key:           func(row ports.TransportHandoverCatalogueRow) string { return row.Object + "|" + row.Version },
			alternateSort: "object",
			totals: map[string]pagedTotal{
				"已交接":       {url.Values{"verdict": {"HANDED_OVER"}}, 6},
				"拒收":        {url.Values{"verdict": {"REFUSED"}}, 0},
				"q 命中对象":    {url.Values{"q": {"syn-parcel-3"}}, 1},
				"q 的 % 按字面": {url.Values{"q": {"%"}}, 0},
			},
		})
	})

	t.Run("effective-delivery", func(t *testing.T) {
		stores := newReviewCatalogueStores(t)
		checkPagedRegister(t, pagedRegister[ports.EffectiveDeliveryCatalogueRow]{
			catalogue: ports.EffectiveDeliveryPages,
			add: func(index int) string {
				object := fmt.Sprintf("SYN-PARCEL-%d", index)
				mustSaveDelivery(t, stores.transactor, t.Context(), stores.deliveries, catalogueDeliveryRecord(t, pagedTenant, object,
					fmt.Sprintf("SYN-ATTEMPT-%d", index), fmt.Sprintf("SYN-POD-%d", index), fmt.Sprintf("SYN-DV-%d", index),
					deliveredAtFixture.Add(time.Duration(index)*time.Minute), "", time.Time{}))
				return object
			},
			list: func(tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.EffectiveDeliveryCatalogueRow], error) {
				return stores.catalogue.ListEffectiveDeliveries(t.Context(), tenant, limit, query)
			},
			key:           func(row ports.EffectiveDeliveryCatalogueRow) string { return row.Object },
			alternateSort: "object",
			totals: map[string]pagedTotal{
				"门禁读数待定":    {url.Values{"gateReading": {deliveryGateReadingFixture(t).Answer().String()}}, 6},
				"门禁读数阻断":    {url.Values{"gateReading": {"BLOCKED"}}, 0},
				"q 命中证明":    {url.Values{"q": {"syn-pod-3"}}, 1},
				"q 的 % 按字面": {url.Values{"q": {"%"}}, 0},
			},
		})
	})

	t.Run("carrier-master-document", func(t *testing.T) {
		stores := newReviewCatalogueStores(t)
		documents, err := adapter.NewMasterDocuments(stores.db)
		if err != nil {
			t.Fatal(err)
		}
		checkPagedRegister(t, pagedRegister[ports.CarrierMasterDocumentCatalogueRow]{
			catalogue: ports.CarrierMasterDocumentPages,
			add: func(index int) string {
				document := fmt.Sprintf("SYN-MAWB-%d", index)
				registered, err := domain.RegisterMasterDocument(domain.MasterDocumentSpec{
					TenantID: segmentRef(t, domain.NewTenantID, pagedTenant),
					Document: segmentRef(t, domain.NewMasterDocumentReference, document),
					Version:  segmentRef(t, domain.NewMasterDocumentVersion, "SYN-MDV-1"),
					Issuer:   segmentRef(t, domain.NewMasterDocumentIssuerReference, "SYN-PARTY-CARRIER-X"),
					Scope:    segmentRef(t, domain.NewTransportScopeReference, "SYN-LANE-1"),
				})
				if err != nil {
					t.Fatalf("形成总单夹具：%v", err)
				}
				mustSaveMasterDocument(t, stores.transactor, t.Context(), documents,
					masterDocumentRecordOf(registered, masterDocumentRecordedAt.Add(time.Duration(index)*time.Minute)))
				return document + "|SYN-MDV-1"
			},
			list: func(tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.CarrierMasterDocumentCatalogueRow], error) {
				return stores.catalogue.ListCarrierMasterDocuments(t.Context(), tenant, limit, query)
			},
			key:           func(row ports.CarrierMasterDocumentCatalogueRow) string { return row.Document + "|" + row.Version },
			alternateSort: "document",
			totals: map[string]pagedTotal{
				"有效":        {url.Values{"standing": {"IN_FORCE"}}, 6},
				"已撤销":       {url.Values{"standing": {"REVOKED"}}, 0},
				"q 命中总单号":   {url.Values{"q": {"syn-mawb-3"}}, 1},
				"q 的 % 按字面": {url.Values{"q": {"%"}}, 0},
			},
		})
	})

	t.Run("transport-preparation", func(t *testing.T) {
		stores := newPreparationStores(t)
		tenant := deliveryValue(t, domain.NewTenantID, pagedTenant)
		checkPagedRegister(t, pagedRegister[ports.TransportPreparationCatalogueRow]{
			catalogue: ports.TransportPreparationPages,
			add: func(index int) string {
				object, leg := fmt.Sprintf("SYN-PARCEL-%d", index), fmt.Sprintf("SYN-RPV-%d#1", index)
				result := stores.run(t, application.PrepareOnPlannedLegsCommand{
					TenantID: tenant,
					Object:   object,
					Route:    application.PlannedRouteFormed,
					Legs: []domain.PreparationLeg{
						preparationLeg(t, leg, "SYN-CONN-EMPTY", departure.Add(-time.Hour), departure.Add(12*time.Hour)),
					},
				})
				if result.Outcome() != application.PreparationRunAnswered || len(result.Legs()) != 1 ||
					result.Legs()[0].Answer != application.LegPreparationFormed {
					t.Fatalf("准备 %s：%s（%s）", object, result.Outcome(), result.UndecidedReason())
				}
				return leg + "|" + object
			},
			list: func(tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.TransportPreparationCatalogueRow], error) {
				return stores.catalogue.ListTransportPreparations(t.Context(), tenant, limit, query)
			},
			key:           func(row ports.TransportPreparationCatalogueRow) string { return row.PlannedLeg + "|" + row.Object },
			alternateSort: "object",
			totals: map[string]pagedTotal{
				"未配置":       {url.Values{"outcome": {domain.PreparationNotConfigured.String()}}, 6},
				"运输机会已准备":   {url.Values{"outcome": {domain.PreparationOpportunityPrepared.String()}}, 0},
				"q 命中段":     {url.Values{"q": {"syn-rpv-3"}}, 1},
				"q 的 % 按字面": {url.Values{"q": {"%"}}, 0},
			},
		})
	})

	t.Run("delivery-dispatch-pending", func(t *testing.T) {
		register, transactor, db := newDeliveryDispatchPendings(t)
		catalogue, err := adapter.NewReviewCatalogue(db)
		if err != nil {
			t.Fatal(err)
		}
		checkPagedRegister(t, pagedRegister[ports.DeliveryDispatchPendingRecord]{
			catalogue: ports.DeliveryDispatchPendingPages,
			add: func(index int) string {
				key := pendingKey(t, pagedTenant, fmt.Sprintf("SYN-SEG-%d", index), fmt.Sprintf("SYN-PARCEL-%d", index))
				withinPendingTransaction(t, transactor, func(ctx context.Context) error {
					return register.RecordMissing(ctx, key, pendingBasis(t, "SYN-BASIS/v1"), []string{"DELIVERY_CONDITION"},
						pendingAnsweredAt.Add(time.Duration(index)*time.Minute))
				})
				return key.Segment.String() + "|" + key.Object.String()
			},
			list: func(tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.DeliveryDispatchPendingRecord], error) {
				return catalogue.ListDeliveryDispatchPendings(t.Context(), tenant, limit, query)
			},
			key: func(row ports.DeliveryDispatchPendingRecord) string {
				return row.Key.Segment.String() + "|" + row.Key.Object.String()
			},
			alternateSort: "-firstAnsweredAt",
			totals: map[string]pagedTotal{
				"q 命中对象":    {url.Values{"q": {"syn-parcel-3"}}, 1},
				"q 命中入场依据":  {url.Values{"q": {"syn-basis/"}}, 6},
				"q 的 % 按字面": {url.Values{"q": {"%"}}, 0},
			},
		})
	})

	t.Run("delivery-dispatch-reask-cadence", func(t *testing.T) {
		register, db := newReaskCadences(t)
		catalogue, err := adapter.NewReviewCatalogue(db)
		if err != nil {
			t.Fatal(err)
		}
		checkPagedRegister(t, pagedRegister[ports.DeliveryDispatchReaskCadenceRecord]{
			catalogue: ports.DeliveryDispatchReaskCadencePages,
			add: func(index int) string {
				version := fmt.Sprintf("SYN-REASK/v%d", index)
				var saved bool
				if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
					var err error
					saved, err = register.Save(ctx, reaskCadenceRecord(t, pagedTenant, version, time.Hour, pagedAt.Add(time.Duration(index)*time.Minute)))
					return err
				}); err != nil || !saved {
					t.Fatalf("登记 %s：saved=%v err=%v", version, saved, err)
				}
				return version
			},
			list: func(tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.DeliveryDispatchReaskCadenceRecord], error) {
				return catalogue.ListDeliveryDispatchReaskCadences(t.Context(), tenant, limit, query)
			},
			key:           func(row ports.DeliveryDispatchReaskCadenceRecord) string { return row.Cadence.Version().String() },
			alternateSort: "recordedAt",
			totals: map[string]pagedTotal{
				"q 命中版本":    {url.Values{"q": {"syn-reask/v3"}}, 1},
				"q 命中依据":    {url.Values{"q": {"SYN-BASIS/"}}, 6},
				"q 的 % 按字面": {url.Values{"q": {"%"}}, 0},
			},
		})
	})

	t.Run("actual-fulfillment-unformed", func(t *testing.T) {
		stores := newReviewCatalogueStores(t)
		register, err := adapter.NewActualFulfillmentUnformeds(stores.db)
		if err != nil {
			t.Fatal(err)
		}
		agreement, err := domain.NewSupplierAgreementVersion("SYN-AGREEMENT-1", "v1")
		if err != nil {
			t.Fatal(err)
		}
		checkPagedRegister(t, pagedRegister[ports.ActualFulfillmentUnformedRecord]{
			catalogue: ports.ActualFulfillmentUnformedPages,
			add: func(index int) string {
				key := unformedKey(t, pagedTenant, fmt.Sprintf("SYN-SEG-%d", index), fmt.Sprintf("SYN-PARCEL-%d", index))
				reason := ports.UnformedNotConfigured
				if index%2 == 0 {
					reason = ports.UnformedAgreementTerminated
				}
				entry := ports.ActualFulfillmentUnformedEntry{
					Key:        key,
					Commission: deliveryValue(t, domain.NewTransportCommissionReference, "SYN-COMMISSION-1"),
					Revision:   1,
					EndedAt:    pagedAt.Add(time.Duration(index) * time.Minute),
					Reason:     reason,
					Agreement:  agreement,
				}
				mustWithinDeliveryTransaction(t, stores.transactor, t.Context(), func(txCtx context.Context) error {
					return register.RecordUnformed(txCtx, entry, pagedAt.Add(time.Hour))
				})
				return key.Segment.String() + "|" + key.Object.String()
			},
			list: func(tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.ActualFulfillmentUnformedRecord], error) {
				return stores.catalogue.ListActualFulfillmentUnformed(t.Context(), tenant, limit, query)
			},
			key: func(row ports.ActualFulfillmentUnformedRecord) string {
				return row.Entry.Key.Segment.String() + "|" + row.Entry.Key.Object.String()
			},
			alternateSort: "-commission",
			totals: map[string]pagedTotal{
				"未配置":       {url.Values{"reason": {"NOT_CONFIGURED"}}, 3},
				"维内为或":      {url.Values{"reason": {"NOT_CONFIGURED", "AGREEMENT_TERMINATED"}}, 6},
				"形成缺陷":      {url.Values{"reason": {"FORMATION_DEFECT"}}, 0},
				"维与 q 为与":   {url.Values{"reason": {"AGREEMENT_TERMINATED"}, "q": {"SYN-PARCEL-3"}}, 0},
				"q 命中委托":    {url.Values{"q": {"syn-commission-1"}}, 6},
				"q 的 % 按字面": {url.Values{"q": {"%"}}, 0},
			},
		})
	})

	t.Run("booking-request", func(t *testing.T) {
		stores := newBookingCatalogueStores(t)
		checkPagedRegister(t, pagedRegister[ports.BookingRequestCatalogueRow]{
			catalogue: ports.BookingRequestPages,
			add: func(index int) string {
				booking := fmt.Sprintf("SYN-BOOKING-%d", index)
				stores.book(t, pagedTenant, booking, time.Duration(index)*time.Minute)
				return booking
			},
			list: func(tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.BookingRequestCatalogueRow], error) {
				return stores.catalogue.ListBookingRequests(t.Context(), tenant, limit, query)
			},
			key:           func(row ports.BookingRequestCatalogueRow) string { return row.Booking },
			alternateSort: "booking",
			totals: map[string]pagedTotal{
				"q 命中订舱":    {url.Values{"q": {"syn-booking-3"}}, 1},
				"q 命中委托":    {url.Values{"q": {"SYN-COMMISSION-48"}}, 6},
				"q 的 % 按字面": {url.Values{"q": {"%"}}, 0},
			},
		})
	})

	t.Run("booking-answer-bounce", func(t *testing.T) {
		stores := newBookingCatalogueStores(t)
		tenant := deliveryValue(t, domain.NewTenantID, pagedTenant)
		checkPagedRegister(t, pagedRegister[ports.BookingAnswerBounceCatalogueRow]{
			catalogue: ports.BookingAnswerBouncePages,
			add: func(index int) string {
				booking, answer := fmt.Sprintf("SYN-BOOKING-%d", index), fmt.Sprintf("SYN-ANSWER-%d", index)
				basis := ports.BouncedBookingNotRegistered
				if index%2 == 0 {
					basis = ports.BouncedSourceNotServiceProvider
				}
				mustWithinDeliveryTransaction(t, stores.transactor, t.Context(), func(txCtx context.Context) error {
					return stores.bounces.RecordBounce(txCtx, ports.BookingAnswerBounce{
						Key:        ports.BookingKey{TenantID: tenant, Booking: deliveryValue(t, domain.NewBookingReference, booking)},
						Source:     "SYN-PARTY-CARRIER-01",
						Acceptance: deliveryValue(t, domain.NewCarrierAcceptanceReference, answer),
						Basis:      basis,
						BouncedAt:  catalogueBookingAt.Add(time.Duration(index) * time.Minute),
					})
				})
				return "SYN-PARTY-CARRIER-01|" + booking + "|" + answer + "|" + basis.String()
			},
			list: func(tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.BookingAnswerBounceCatalogueRow], error) {
				return stores.catalogue.ListBookingAnswerBounces(t.Context(), tenant, limit, query)
			},
			key: func(row ports.BookingAnswerBounceCatalogueRow) string {
				return row.Source + "|" + row.Booking + "|" + row.Answer + "|" + row.Basis
			},
			alternateSort: "-count",
			totals: map[string]pagedTotal{
				"订舱不在册":     {url.Values{"basis": {"BOOKING_NOT_REGISTERED"}}, 3},
				"按一个订舱":     {url.Values{"booking": {"SYN-BOOKING-2"}}, 1},
				"订舱维内为或":    {url.Values{"booking": {"SYN-BOOKING-2", "SYN-BOOKING-5"}}, 2},
				"按答复方":      {url.Values{"source": {"SYN-PARTY-CARRIER-01"}}, 6},
				"别的答复方":     {url.Values{"source": {"SYN-PARTY-CARRIER-99"}}, 0},
				"q 命中答复":    {url.Values{"q": {"syn-answer-3"}}, 1},
				"q 的 % 按字面": {url.Values{"q": {"%"}}, 0},
			},
		})
	})

	t.Run("pending-attempt-result", func(t *testing.T) {
		rig := newCorrectionRig(t)
		checkPagedRegister(t, pagedRegister[ports.PendingAttemptResultRow]{
			catalogue: ports.PendingAttemptResultPages,
			add: func(index int) string {
				if index%2 == 1 {
					source := fmt.Sprintf("SYN-DEVICE-%d", index)
					rig.savePickupAttempt(t, pendingPickupRecord(t, pagedTenant, source))
					return ports.PendingAttemptSidePickup + "|attempt-1|parcel-2|" + source
				}
				attempt := fmt.Sprintf("SYN-ATTEMPT-%d", index)
				mustWithinDeliveryTransaction(t, rig.transactor, t.Context(), func(txCtx context.Context) error {
					_, err := rig.deliveries.Save(txCtx, pendingDeliveryRecord(t, pagedTenant, attempt))
					return err
				})
				return ports.PendingAttemptSideDelivery + "|" + attempt + "|parcel-2|"
			},
			list: func(tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.PendingAttemptResultRow], error) {
				return rig.catalogue.ListPendingAttemptResults(t.Context(), tenant, limit, query)
			},
			key: func(row ports.PendingAttemptResultRow) string {
				return row.Side + "|" + row.Attempt + "|" + row.Object + "|" + row.SourceID
			},
			alternateSort: "-occurredAt",
			totals: map[string]pagedTotal{
				"揽收一侧":      {url.Values{"side": {ports.PendingAttemptSidePickup}}, 3},
				"两侧":        {url.Values{"side": {ports.PendingAttemptSidePickup, ports.PendingAttemptSideDelivery}}, 6},
				"q 命中来源身份":  {url.Values{"q": {"syn-device-3"}}, 1},
				"q 命中派送尝试":  {url.Values{"q": {"syn-attempt-"}}, 3},
				"q 的 % 按字面": {url.Values{"q": {"%"}}, 0},
			},
		})
	})

	t.Run("external-tracking-facts", func(t *testing.T) {
		repository, transactor, _ := newExternalTrackingFacts(t)
		source := segmentRef(t, domain.NewTrackingSourceReference, "SYN-SOURCE-A")
		checkPagedRegister(t, pagedRegister[ports.ExternalTrackingFactReviewRow]{
			catalogue: ports.ExternalTrackingFactPages,
			add: func(index int) string {
				fact, version := fmt.Sprintf("SYN-EXTF-%d", index), fmt.Sprintf("SYN-EXTV-%d", index)
				record := reviewFactRecord(t, pagedTenant, source.String(), fact, version, pagedAt.Add(time.Duration(index)*time.Minute))
				if outcome := mustSaveTrackingFact(t, transactor, t.Context(), repository, record); outcome != ports.ExternalTrackingFactSaved {
					t.Fatalf("事实 %s 落库 outcome = %s", fact, outcome)
				}
				return fact + "|" + version
			},
			list: func(tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.ExternalTrackingFactReviewRow], error) {
				return repository.ListCurrentExternalTrackingFacts(t.Context(), tenant, source, ports.EveryCurrentVersion, limit, query)
			},
			key:           func(row ports.ExternalTrackingFactReviewRow) string { return row.Fact + "|" + row.Version },
			alternateSort: "-receivedAt",
			totals: map[string]pagedTotal{
				"待判断":       {url.Values{"effectiveBasis": {"PENDING"}}, 6},
				"所有者显式判断":   {url.Values{"effectiveBasis": {"JUDGED_EXPLICITLY"}}, 0},
				"q 命中事实":    {url.Values{"q": {"syn-extf-3"}}, 1},
				"q 的 % 按字面": {url.Values{"q": {"%"}}, 0},
			},
		})
	})
}
