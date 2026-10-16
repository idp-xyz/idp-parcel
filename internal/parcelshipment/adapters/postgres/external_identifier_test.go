package postgres_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证外部标识关联册（ADR-0184，迁移 0024）：客户包裹件号随已接受委托的保存同笔
// 写进册里、按键幂等，读面按号码逐字答包裹。夹具全为合成引用（S 级）。

type externalIdentifierFixture struct {
	requests    *adapter.ShipmentRequests
	identifiers *adapter.ExternalIdentifiers
	transactor  bentoapp.Transactor
	pool        *pgxpool.Pool
}

func newExternalIdentifierFixture(t *testing.T) externalIdentifierFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	requests, err := adapter.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	identifiers, err := adapter.NewExternalIdentifiers(db)
	if err != nil {
		t.Fatalf("构造关联册读面：%v", err)
	}
	return externalIdentifierFixture{requests: requests, identifiers: identifiers, transactor: db.Transactor(), pool: pool}
}

// numberedRequest 经领域造一份（tenant，customer，key）下、成员为 parcels、按 numbers 给号的`已提交`委托。
func numberedRequest(
	t *testing.T,
	tenant, customer, key, requestID string,
	parcels []string,
	numbers map[string]string,
) domain.ShipmentRequest {
	t.Helper()
	spec := submittedShipmentRequestSpec(t, key, requestID)
	fingerprint, err := domain.NewSourceSubmissionFingerprint(
		requestScopedIdentity(t, tenant, customer, key),
		mustBuild(t, domain.NewPayloadDigest, "digest-"+key),
		submittedAtFixture.Add(-1),
		submittedAtFixture,
	)
	if err != nil {
		t.Fatalf("来源指纹：%v", err)
	}
	members := make([]domain.DeclaredParcelID, len(parcels))
	for index, parcel := range parcels {
		members[index] = mustBuild(t, domain.NewDeclaredParcelID, parcel)
	}
	candidate, err := domain.NewSubmissionCandidate(fingerprint,
		mustBuild(t, domain.NewSubmissionBatchID, "batch-"+key), mustBuild(t, domain.NewShipmentRequestID, requestID), members)
	if err != nil {
		t.Fatalf("候选：%v", err)
	}
	spec.Candidate = candidate
	spec.Profiles = nil
	for _, parcel := range parcels {
		value, numbered := numbers[parcel]
		if !numbered {
			continue
		}
		declaration, err := domain.DeclareCustomerParcelNumber(
			mustBuild(t, domain.NewDeclaredParcelID, parcel), mustBuild(t, domain.NewExternalIdentifierNumber, value))
		if err != nil {
			t.Fatalf("配对：%v", err)
		}
		spec.CustomerParcelNumbers = append(spec.CustomerParcelNumbers, declaration)
	}
	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("建单：%v", err)
	}
	return request
}

// accept 照 acceptanceDecisionSpec 的配方接受，逐成员可达校验按这份委托自己的成员出。
func accept(t *testing.T, request domain.ShipmentRequest) domain.ShipmentRequest {
	t.Helper()
	spec := acceptanceDecisionSpec(t)
	var checks []domain.AcceptanceCheck
	for _, check := range spec.Checks {
		if check.Group() != domain.NetworkReachabilityCheck {
			checks = append(checks, check)
		}
	}
	for _, parcel := range request.CurrentSubmissionVersion().DeclaredParcelIDs() {
		checks = append(checks, parcelCheck(t, domain.NetworkReachabilityCheck, parcel.String(), domain.CheckPassed, ""))
	}
	spec.Checks = checks
	accepted, err := request.Decide(spec)
	if err != nil {
		t.Fatalf("接受：%v", err)
	}
	if accepted.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED", accepted.State())
	}
	return accepted
}

func (fixture externalIdentifierFixture) insertAndAccept(t *testing.T, request domain.ShipmentRequest) {
	t.Helper()
	ctx := t.Context()
	identity := request.CurrentSubmissionVersion().SourceSubmission().Identity()
	mustInsert(t, fixture.transactor, ctx, fixture.requests, request)
	loaded, found, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回：found = %v, err = %v", found, err)
	}
	mustSave(t, fixture.transactor, ctx, fixture.requests, identity, accept(t, loaded))
}

func (fixture externalIdentifierFixture) parcelsOf(t *testing.T, tenant, number string) []string {
	t.Helper()
	parcels, err := fixture.identifiers.FindParcelsByExternalIdentifier(t.Context(),
		mustBuild(t, domain.NewTenantID, tenant), mustBuild(t, domain.NewExternalIdentifierNumber, number))
	if err != nil {
		t.Fatalf("按号码查 %s：%v", number, err)
	}
	values := make([]string, len(parcels))
	for index, parcel := range parcels {
		values[index] = parcel.String()
	}
	return values
}

func (fixture externalIdentifierFixture) relationRows(t *testing.T) int {
	t.Helper()
	var count int
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM parcel_shipment.external_identifier_relation`).Scan(&count); err != nil {
		t.Fatalf("数关联行：%v", err)
	}
	return count
}

// Covers: ADR-0184 决定三、四——已提交时册里没有行；接受那一次保存同笔写下给了号的成员，快照往返带回配对；
// 已接受委托再保存一次不重复成行；形成依据与分配方落成接受决定与客户账户。
func TestAcceptanceWritesCustomerParcelNumbersIntoTheRegisterOnce(t *testing.T) {
	fixture := newExternalIdentifierFixture(t)
	ctx := t.Context()
	request := numberedRequest(t, "tenant-1", "customer-1", "req-key-1", "request-1",
		[]string{"parcel-1", "parcel-2"}, map[string]string{"parcel-1": "CUST-0001"})
	mustInsert(t, fixture.transactor, ctx, fixture.requests, request)
	if got := fixture.parcelsOf(t, "tenant-1", "CUST-0001"); len(got) != 0 {
		t.Fatalf("已提交即可识别：%v", got)
	}

	loaded := mustFind(t, fixture.requests, ctx, "req-key-1")
	if got := loaded.CustomerParcelNumbers(); len(got) != 1 || got[0].Number().String() != "CUST-0001" {
		t.Fatalf("快照没带回配对：%+v", got)
	}
	accepted := accept(t, loaded)
	mustSave(t, fixture.transactor, ctx, fixture.requests, requestIdentity(t, "req-key-1"), accepted)
	if got := fixture.parcelsOf(t, "tenant-1", "CUST-0001"); !slices.Equal(got, []string{"parcel-1"}) {
		t.Fatalf("接受后按号码查得 %v, want [parcel-1]", got)
	}

	reloaded := mustFind(t, fixture.requests, ctx, "req-key-1")
	mustSave(t, fixture.transactor, ctx, fixture.requests, requestIdentity(t, "req-key-1"), reloaded)
	if rows := fixture.relationRows(t); rows != 1 {
		t.Fatalf("关联行 = %d, want 1——已接受委托再保存不重复成行", rows)
	}
	var allocator, scope, basis string
	if err := fixture.pool.QueryRow(ctx,
		`SELECT allocator, scope, basis FROM parcel_shipment.external_identifier_relation`).Scan(&allocator, &scope, &basis); err != nil {
		t.Fatalf("读关联行：%v", err)
	}
	if allocator != "customer-1" || scope != "customer-1" || basis != "accept-1" {
		t.Fatalf("allocator / scope / basis = %q / %q / %q", allocator, scope, basis)
	}
}

// Covers: PS CONTEXT「同一号码命中多件包裹即如实交回多个候选」——两个客户账户用了同一个号，两件都交回；号码逐字比，
// 大小写不同查不到；他租户查不到。
func TestTheSameNumberFromTwoCustomersAnswersBothParcelsVerbatimWithinTheTenant(t *testing.T) {
	fixture := newExternalIdentifierFixture(t)
	fixture.insertAndAccept(t, numberedRequest(t, "tenant-1", "customer-1", "req-key-a", "request-a",
		[]string{"parcel-a"}, map[string]string{"parcel-a": "SHARED-01"}))
	fixture.insertAndAccept(t, numberedRequest(t, "tenant-1", "customer-2", "req-key-b", "request-b",
		[]string{"parcel-b"}, map[string]string{"parcel-b": "SHARED-01"}))

	if got := fixture.parcelsOf(t, "tenant-1", "SHARED-01"); !slices.Equal(got, []string{"parcel-a", "parcel-b"}) {
		t.Fatalf("同号两件查得 %v", got)
	}
	if got := fixture.parcelsOf(t, "tenant-1", "shared-01"); len(got) != 0 {
		t.Fatalf("大小写不同也查得到：%v", got)
	}
	if got := fixture.parcelsOf(t, "tenant-2", "SHARED-01"); len(got) != 0 {
		t.Fatalf("他租户查得到：%v", got)
	}
}

// Covers: 被拒绝的委托不形成关联——接受前的声明包裹不是正式包裹，拒绝之后也不会成为。
func TestARejectedRequestLeavesNoRelation(t *testing.T) {
	fixture := newExternalIdentifierFixture(t)
	ctx := t.Context()
	request := numberedRequest(t, "tenant-1", "customer-1", "req-key-1", "request-1",
		[]string{"parcel-1", "parcel-2"}, map[string]string{"parcel-1": "CUST-0001"})
	mustInsert(t, fixture.transactor, ctx, fixture.requests, request)
	spec := acceptanceDecisionSpec(t)
	spec.Checks[0] = versionCheck(t, domain.CustomerRelationshipCheck, domain.CheckFailed, "NO_RELATIONSHIP")
	rejected, err := mustFind(t, fixture.requests, ctx, "req-key-1").Decide(spec)
	if err != nil || rejected.State() != domain.ShipmentRequestRejected {
		t.Fatalf("拒绝：state = %q, err = %v", rejected.State(), err)
	}
	var saved bool
	mustWithinTransaction(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		_, saveErr := fixture.requests.Save(txCtx, requestIdentity(t, "req-key-1"), rejected)
		saved = saveErr == nil
		return saveErr
	})
	if !saved || fixture.relationRows(t) != 0 {
		t.Fatalf("拒绝后关联行 = %d", fixture.relationRows(t))
	}
}

// Covers: ADR-0184 决定三「与来源事实同一笔事务形成」——关联写不进去时接受那一笔整笔回滚：委托仍是已提交、
// 版本不动，不会出现「委托已接受而节点认不出它的包裹」那一刻。本用例的库是每用例一份的物理拷贝，改名只伤自己。
func TestAFailedRelationWriteRollsTheAcceptanceBack(t *testing.T) {
	fixture := newExternalIdentifierFixture(t)
	ctx := t.Context()
	request := numberedRequest(t, "tenant-1", "customer-1", "req-key-1", "request-1",
		[]string{"parcel-1", "parcel-2"}, map[string]string{"parcel-1": "CUST-0001"})
	mustInsert(t, fixture.transactor, ctx, fixture.requests, request)
	accepted := accept(t, mustFind(t, fixture.requests, ctx, "req-key-1"))

	if _, err := fixture.pool.Exec(ctx,
		`ALTER TABLE parcel_shipment.external_identifier_relation RENAME TO external_identifier_relation_away`); err != nil {
		t.Fatalf("挪走关联表：%v", err)
	}
	err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, saveErr := fixture.requests.Save(txCtx, requestIdentity(t, "req-key-1"), accepted)
		return saveErr
	})
	if err == nil {
		t.Fatal("关联写不进去，保存却成功了")
	}
	if _, err := fixture.pool.Exec(ctx,
		`ALTER TABLE parcel_shipment.external_identifier_relation_away RENAME TO external_identifier_relation`); err != nil {
		t.Fatalf("挪回关联表：%v", err)
	}
	after := mustFind(t, fixture.requests, ctx, "req-key-1")
	if after.State() != domain.ShipmentRequestSubmitted || after.Revision() != 1 {
		t.Fatalf("回滚后 state = %q / revision = %d, want SUBMITTED / 1", after.State(), after.Revision())
	}
}

// 以下证渠道来源（票 ps-external-mark-relations/03）：面单交易仓储保存时同笔写渠道包裹件号与它的适用变化。

type channelFixture struct {
	transactions *adapter.LabelTransactions
	identifiers  *adapter.ExternalIdentifiers
	transactor   bentoapp.Transactor
	pool         *pgxpool.Pool
}

func newChannelFixture(t *testing.T) channelFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	transactions, err := adapter.NewLabelTransactions(db)
	if err != nil {
		t.Fatalf("构造面单交易仓储：%v", err)
	}
	identifiers, err := adapter.NewExternalIdentifiers(db)
	if err != nil {
		t.Fatalf("构造关联册读面：%v", err)
	}
	return channelFixture{transactions: transactions, identifiers: identifiers, transactor: db.Transactor(), pool: pool}
}

func (fixture channelFixture) reload(t *testing.T, transactionID string) domain.LabelTransaction {
	t.Helper()
	found, exists, err := fixture.transactions.FindByID(t.Context(),
		mustBuild(t, domain.NewTenantID, "tenant-a"), mustBuild(t, domain.NewLabelTransactionID, transactionID))
	if err != nil || !exists {
		t.Fatalf("读回 %s：exists = %v, err = %v", transactionID, exists, err)
	}
	return found
}

func (fixture channelFixture) save(t *testing.T, transaction domain.LabelTransaction) {
	t.Helper()
	var result ports.LabelTransactionSaveResult
	mustWithinTransaction(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = fixture.transactions.Save(txCtx, transaction)
		return err
	})
	if result.Outcome != ports.LabelTransactionSaved {
		t.Fatalf("保存结果 = %s", result.Outcome)
	}
}

// recordChannel 把一笔交易从建立推到结果落库：插入、提交渠道、记录结果，每一步读回再保存。
func (fixture channelFixture) recordChannel(
	t *testing.T,
	established domain.LabelTransaction,
	observedAt time.Time,
	results ...domain.LabelTransactionParcelResultSpec,
) domain.LabelTransaction {
	t.Helper()
	mustInsertLabelTransaction(t, fixture.transactor, t.Context(), fixture.transactions, established)
	submitted, err := fixture.reload(t, established.ID().String()).SubmitToChannel(observedAt.Add(-time.Minute))
	if err != nil {
		t.Fatalf("提交渠道：%v", err)
	}
	fixture.save(t, submitted)
	outcome := domain.LabelTransactionSucceeded
	for _, result := range results {
		if !result.Accepted {
			outcome = domain.LabelTransactionPartiallySucceeded
		}
	}
	recorded, err := fixture.reload(t, established.ID().String()).RecordChannelResult(domain.RecordChannelResultSpec{
		Outcome: outcome, ParcelResults: results, ObservedAt: observedAt,
	})
	if err != nil {
		t.Fatalf("记录渠道结果：%v", err)
	}
	fixture.save(t, recorded)
	return fixture.reload(t, established.ID().String())
}

func (fixture channelFixture) parcelsOf(t *testing.T, number string) []string {
	t.Helper()
	parcels, err := fixture.identifiers.FindParcelsByExternalIdentifier(t.Context(),
		mustBuild(t, domain.NewTenantID, "tenant-a"), mustBuild(t, domain.NewExternalIdentifierNumber, number))
	if err != nil {
		t.Fatalf("按号码查 %s：%v", number, err)
	}
	values := make([]string, len(parcels))
	for index, parcel := range parcels {
		values[index] = parcel.String()
	}
	return values
}

func acceptedChannelResult(t *testing.T, parcel, number string) domain.LabelTransactionParcelResultSpec {
	t.Helper()
	return domain.LabelTransactionParcelResultSpec{
		Parcel:     mustBuild(t, domain.NewDeclaredParcelID, parcel),
		Accepted:   true,
		Identifier: mustBuild(t, domain.NewChannelParcelIdentifier, number),
	}
}

func (fixture channelFixture) appendFollowUp(t *testing.T, transactionID string, kind domain.FollowUpActionKind, at time.Time, parcels ...string) {
	t.Helper()
	scope := make([]domain.DeclaredParcelID, 0, len(parcels))
	for _, parcel := range parcels {
		scope = append(scope, mustBuild(t, domain.NewDeclaredParcelID, parcel))
	}
	appended, err := fixture.reload(t, transactionID).AppendFollowUpAction(domain.FollowUpActionSpec{
		Kind: kind, Parcels: scope, Reason: mustBuild(t, domain.NewChannelResultReasonReference, "SYN-REASON"), OccurredAt: at,
	})
	if err != nil {
		t.Fatalf("后续动作：%v", err)
	}
	fixture.save(t, appended)
}

func (fixture channelFixture) changeKinds(t *testing.T) map[string]string {
	t.Helper()
	rows, err := fixture.pool.Query(t.Context(),
		`SELECT identifier_number, change_kind FROM parcel_shipment.external_identifier_relation_change`)
	if err != nil {
		t.Fatalf("读变化行：%v", err)
	}
	defer rows.Close()
	kinds := map[string]string{}
	for rows.Next() {
		var number, kind string
		if err := rows.Scan(&number, &kind); err != nil {
			t.Fatalf("读变化行：%v", err)
		}
		kinds[number] = kind
	}
	return kinds
}

// Covers: 受理的渠道包裹件号随结果那一笔保存进册、未受理的不进；渠道退款不改；逐件作废之后那个号零候选、
// 另一件照常识别；作废只留一行终态，交易再保存也不重复。
func TestChannelResultsFormNumbersAndAVoidRetiresOnlyTheVoidedParcel(t *testing.T) {
	fixture := newChannelFixture(t)
	observed := labelEstablishedAt.Add(2 * time.Minute)
	fixture.recordChannel(t, establishedLabelTransactionFixture(t, "tenant-a", "label-txn-1", "parcel-1", "parcel-2", "parcel-3"), observed,
		acceptedChannelResult(t, "parcel-1", "CH-0001"), acceptedChannelResult(t, "parcel-2", "CH-0002"),
		domain.LabelTransactionParcelResultSpec{
			Parcel: mustBuild(t, domain.NewDeclaredParcelID, "parcel-3"),
			Reason: mustBuild(t, domain.NewChannelResultReasonReference, "ADDRESS_UNSUPPORTED"),
		})
	if got := fixture.parcelsOf(t, "CH-0001"); !slices.Equal(got, []string{"parcel-1"}) {
		t.Fatalf("CH-0001 查得 %v", got)
	}

	fixture.appendFollowUp(t, "label-txn-1", domain.ChannelRefundAction, observed.Add(time.Minute))
	if got := fixture.parcelsOf(t, "CH-0002"); !slices.Equal(got, []string{"parcel-2"}) {
		t.Fatalf("退款后 CH-0002 查得 %v——退款不等于号作废", got)
	}

	fixture.appendFollowUp(t, "label-txn-1", domain.ChannelVoidAction, observed.Add(2*time.Minute), "parcel-1")
	if got := fixture.parcelsOf(t, "CH-0001"); len(got) != 0 {
		t.Fatalf("作废后 CH-0001 仍查得 %v", got)
	}
	if got := fixture.parcelsOf(t, "CH-0002"); !slices.Equal(got, []string{"parcel-2"}) {
		t.Fatalf("作废 parcel-1 把 CH-0002 也带走了：%v", got)
	}
	fixture.save(t, fixture.reload(t, "label-txn-1"))
	if kinds := fixture.changeKinds(t); len(kinds) != 1 || kinds["CH-0001"] != "VOIDED" {
		t.Fatalf("变化行 = %v, want 只有 CH-0001 VOIDED", kinds)
	}
}

// Covers: 换单形成的新交易对同一包裹受理出新号，原交易那条关联转已被替代并回指新号：旧号零候选、新号识别出同一件；
// 已有终态的关联不再变——先被作废的号，后来的替代写不进第二个终态。
func TestAReplacementSupersedesThePriorNumberAndATerminalStateIsFinal(t *testing.T) {
	fixture := newChannelFixture(t)
	observed := labelEstablishedAt.Add(2 * time.Minute)
	prior := fixture.recordChannel(t, establishedLabelTransactionFixture(t, "tenant-a", "label-txn-1", "parcel-1", "parcel-2"), observed,
		acceptedChannelResult(t, "parcel-1", "CH-OLD-1"), acceptedChannelResult(t, "parcel-2", "CH-OLD-2"))
	fixture.appendFollowUp(t, "label-txn-1", domain.ChannelVoidAction, observed.Add(time.Minute), "parcel-2")

	link, err := domain.EstablishPriorLabelTransactionLink(fixture.reload(t, "label-txn-1"), domain.LabelTransactionReplacement)
	if err != nil {
		t.Fatalf("前序关系：%v", err)
	}
	if prior.State() != domain.LabelTransactionSucceeded {
		t.Fatalf("原交易 state = %s", prior.State())
	}
	replacing := establishedLabelTransactionFixture(t, "tenant-a", "label-txn-2", "parcel-1", "parcel-2")
	spec := domain.EstablishLabelTransactionSpec{
		Tenant: replacing.Tenant(), ID: replacing.ID(), CoveredParcels: replacing.CoveredParcels(),
		ChannelAccount: replacing.ChannelAccount(), AccountHolder: replacing.AccountHolder(),
		ServiceProvider:        mustBuild(t, domain.NewChannelServiceProviderReference, "party-channel-2"),
		SettlementCounterparty: replacing.SettlementCounterparty(), Contract: replacing.Contract(), Rate: replacing.Rate(),
		ResponsibilityBasis: replacing.ResponsibilityBasis(), EstablishedAt: observed.Add(time.Hour), PriorLink: link,
	}
	established, err := domain.EstablishLabelTransaction(spec)
	if err != nil {
		t.Fatalf("建立替代交易：%v", err)
	}
	fixture.recordChannel(t, established, observed.Add(3*time.Hour),
		acceptedChannelResult(t, "parcel-1", "CH-NEW-1"), acceptedChannelResult(t, "parcel-2", "CH-NEW-2"))

	if got := fixture.parcelsOf(t, "CH-OLD-1"); len(got) != 0 {
		t.Fatalf("被替代的旧号仍查得 %v", got)
	}
	if got := fixture.parcelsOf(t, "CH-NEW-1"); !slices.Equal(got, []string{"parcel-1"}) {
		t.Fatalf("新号查得 %v", got)
	}
	kinds := fixture.changeKinds(t)
	if kinds["CH-OLD-1"] != "REPLACED" || kinds["CH-OLD-2"] != "VOIDED" || len(kinds) != 2 {
		t.Fatalf("变化行 = %v, want CH-OLD-1 REPLACED、CH-OLD-2 仍是先到的 VOIDED", kinds)
	}
	var successorAllocator, successorNumber string
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT successor_allocator, successor_number FROM parcel_shipment.external_identifier_relation_change
		  WHERE identifier_number = 'CH-OLD-1'`).Scan(&successorAllocator, &successorNumber); err != nil {
		t.Fatalf("读后继：%v", err)
	}
	if successorAllocator != "party-channel-2" || successorNumber != "CH-NEW-1" {
		t.Fatalf("后继 = %s / %s", successorAllocator, successorNumber)
	}
}
