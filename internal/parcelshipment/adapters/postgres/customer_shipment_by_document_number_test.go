package postgres_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证客户按运单号追踪时本上下文那一半的读口（ADR-0356 决定四；票 document-numbering/12 完成判据
// 「PS 真库用例」）：按（租户，客户账户，单据号）读本账户已提交、已接受、已拒绝、已撤回各一份；别的账户的号、别的租户的号与查无
// 同答，且各是同一条语句、账户就在它的参数里；当前有效包裹的次序与分页；无号委托按号不命中。夹具全为合成（S 级）。

// statementRecorder 记下读口那只连接池发出的每条语句与参数。
type statementRecorder struct {
	mu         sync.Mutex
	statements []recordedStatement
}

type recordedStatement struct {
	sql  string
	args []any
}

func (recorder *statementRecorder) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.statements = append(recorder.statements, recordedStatement{sql: data.SQL, args: data.Args})
	return ctx
}

func (recorder *statementRecorder) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (recorder *statementRecorder) take() []recordedStatement {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	taken := recorder.statements
	recorder.statements = nil
	return taken
}

type byNumberFixture struct {
	views      *adapter.ShipmentRequestViews
	requests   *adapter.ShipmentRequests
	transactor bentoapp.Transactor
	recorder   *statementRecorder
}

// newByNumberFixture 给读口另开一只带追踪的连接池：灌数走原池，记下的只有读口自己的语句。
func newByNumberFixture(t *testing.T) *byNumberFixture {
	t.Helper()
	plain := pgtest.Pool(t)
	plainDB, err := bentopg.NewDB(plain, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	requests, err := adapter.NewShipmentRequests(plainDB)
	if err != nil {
		t.Fatal(err)
	}
	config := plain.Config()
	recorder := &statementRecorder{}
	config.ConnConfig.Tracer = recorder
	traced, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatalf("开带追踪的连接池：%v", err)
	}
	// 晚于 pgtest.Pool 登记，先于它的删库执行：库上还有连接时删不掉。
	t.Cleanup(traced.Close)
	tracedDB, err := bentopg.NewDB(traced, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	views, err := adapter.NewShipmentRequestViews(tracedDB)
	if err != nil {
		t.Fatal(err)
	}
	return &byNumberFixture{views: views, requests: requests, transactor: plainDB.Transactor(), recorder: recorder}
}

// insertNumbered 落一份委托并记上单据号（number 为空即不记，读回是上线前开立）。
func (fixture *byNumberFixture) insertNumbered(t *testing.T, request domain.ShipmentRequest, number string) domain.SourceIdentity {
	t.Helper()
	identity := request.CurrentSubmissionVersion().SourceSubmission().Identity()
	if number == "" {
		mustInsert(t, fixture.transactor, t.Context(), fixture.requests, request)
		return identity
	}
	numbered, err := request.RecordDocumentNumbering(numberedShipmentDocument(t, number))
	if err != nil {
		t.Fatalf("记号 %s：%v", number, err)
	}
	mustWithinTransaction(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		if _, err := fixture.requests.Insert(txCtx, identity, numbered); err != nil {
			return err
		}
		return fixture.requests.RecordShipmentDocumentNumbering(txCtx, identity, numbered)
	})
	return identity
}

// advance 读回一份委托，按 decide 推进并保存。
func (fixture *byNumberFixture) advance(t *testing.T, identity domain.SourceIdentity, decide func(domain.ShipmentRequest) (domain.ShipmentRequest, error)) {
	t.Helper()
	loaded, found, err := fixture.requests.FindBySourceIdentity(t.Context(), identity)
	if err != nil || !found {
		t.Fatalf("读回：found=%v err=%v", found, err)
	}
	advanced, err := decide(loaded)
	if err != nil {
		t.Fatalf("推进：%v", err)
	}
	mustSave(t, fixture.transactor, t.Context(), fixture.requests, identity, advanced)
}

func (fixture *byNumberFixture) find(t *testing.T, tenant, customer, number string) (bool, domain.ShipmentRequestState, []domain.EffectiveParcel) {
	t.Helper()
	record, found, err := fixture.views.FindCustomerShipmentByDocumentNumber(t.Context(),
		mustBuild(t, domain.NewTenantID, tenant), mustBuild(t, domain.NewCustomerAccountID, customer),
		mustBuild(t, domain.NewShipmentDocumentNumber, number))
	if err != nil {
		t.Fatalf("按号读 %s/%s/%s：%v", tenant, customer, number, err)
	}
	return found, record.State, record.EffectiveParcels
}

// acceptedFivePieceRequest 是 C1 名下一份五件委托：提交时的次序是 c、a、e、b、d，其中 c、e、d 声明了件号；接受之前客户补充
// 过一次，补充版本把次序改成 a、b、c、d、e（成员集合不变，补充只核集合），接受基线落在补充版本上。
func (fixture *byNumberFixture) acceptedFivePieceRequest(t *testing.T, number string) {
	t.Helper()
	first := []string{"by-number-c", "by-number-a", "by-number-e", "by-number-b", "by-number-d"}
	request := numberedRequest(t, "tenant-1", "customer-1", "key-by-number-accepted", "REQ-BY-NUMBER-ACCEPTED", first,
		map[string]string{"by-number-c": "CUST-C", "by-number-e": "CUST-E", "by-number-d": "CUST-D"})
	identity := fixture.insertNumbered(t, request, number)
	fixture.advance(t, identity, func(loaded domain.ShipmentRequest) (domain.ShipmentRequest, error) {
		reordered := make([]domain.DeclaredParcelID, 0, len(first))
		for _, parcel := range []string{"by-number-a", "by-number-b", "by-number-c", "by-number-d", "by-number-e"} {
			reordered = append(reordered, mustBuild(t, domain.NewDeclaredParcelID, parcel))
		}
		return loaded.FormNewSubmissionVersion(domain.NewSubmissionVersionSpec{
			VersionID:         mustBuild(t, domain.NewSubmissionVersionID, "version-by-number-2"),
			TaskID:            mustBuild(t, domain.NewAcceptanceDecisionTaskID, "task-by-number-2"),
			SourceSubmission:  scopedFingerprint(t, "tenant-1", "customer-1", "key-by-number-accepted-supplement"),
			DeclaredParcelIDs: reordered,
			EstablishedAt:     submittedAtFixture.Add(30 * time.Minute),
		})
	})
	fixture.advance(t, identity, func(loaded domain.ShipmentRequest) (domain.ShipmentRequest, error) {
		return accept(t, loaded), nil
	})
}

// Covers: ADR-0356 决定二、四的提供方半边，`AT-VE-231`、`AT-VE-233` 的 PS 半边——按（租户，客户账户，单据号）读 C1 名下已提交、
// 已接受、已拒绝、已撤回各一份，各答自己的状态；只有已接受带当前有效包裹：基线成员全在，次序与位次照首个提交版本（即首方提交
// 答复的 parcels，补充版本改过的次序不算），件号逐件带回、没声明的件没有。
func TestACustomersOwnShipmentIsReadByNumberInEachState(t *testing.T) {
	fixture := newByNumberFixture(t)

	fixture.insertNumbered(t, numberedRequest(t, "tenant-1", "customer-1", "key-by-number-submitted", "REQ-BY-NUMBER-SUBMITTED",
		[]string{"by-number-s"}, nil), "P20261102000001")
	fixture.acceptedFivePieceRequest(t, "P20261102000002")
	rejected := fixture.insertNumbered(t, numberedRequest(t, "tenant-1", "customer-1", "key-by-number-rejected", "REQ-BY-NUMBER-REJECTED",
		[]string{"by-number-r"}, nil), "P20261102000003")
	fixture.advance(t, rejected, rejectByContract(t))
	withdrawn := fixture.insertNumbered(t, numberedRequest(t, "tenant-1", "customer-1", "key-by-number-withdrawn", "REQ-BY-NUMBER-WITHDRAWN",
		[]string{"by-number-w"}, nil), "P20261102000004")
	fixture.advance(t, withdrawn, withdrawByCustomer(t))

	for number, want := range map[string]domain.ShipmentRequestState{
		"P20261102000001": domain.ShipmentRequestSubmitted,
		"P20261102000003": domain.ShipmentRequestRejected,
		"P20261102000004": domain.ShipmentRequestWithdrawn,
	} {
		found, state, parcels := fixture.find(t, "tenant-1", "customer-1", number)
		if !found || state != want || len(parcels) != 0 {
			t.Errorf("%s：found=%v state=%s parcels=%d, want %s 不带件", number, found, state, len(parcels), want)
		}
	}

	found, state, parcels := fixture.find(t, "tenant-1", "customer-1", "P20261102000002")
	if !found || state != domain.ShipmentRequestAccepted {
		t.Fatalf("已接受那份：found=%v state=%s", found, state)
	}
	var got []string
	for _, parcel := range parcels {
		number, declared := parcel.CustomerParcelNumber()
		entry := fmt.Sprintf("%d:%s", parcel.Position(), parcel.Parcel())
		if declared {
			entry += "=" + number.String()
		}
		got = append(got, entry)
		if len(parcel.Replaces()) != 0 {
			t.Errorf("%s 谱系未建模却带了来源包裹", parcel.Parcel())
		}
	}
	want := "1:by-number-c=CUST-C 2:by-number-a 3:by-number-e=CUST-E 4:by-number-b 5:by-number-d=CUST-D"
	if strings.Join(got, " ") != want {
		t.Fatalf("当前有效包裹 = %s, want %s", strings.Join(got, " "), want)
	}
}

// Covers: ADR-0356 决定四「账户在键上」与越权风险点 1，`AT-VE-232` 的 PS 半边——C2 名下的号、只在另一租户的号、本租户没有的号、
// 大小写折过的号、无号委托的委托标识，C1 问来都答 found 为假；每一问恰是一条语句、同一段 SQL，账户与号都在它的参数里（不是先按
// 号读出再比账户）。另一租户有与 C1 同号的委托时，两边各读到自己的那一份。
func TestOtherAccountsOtherTenantsAndMissingNumbersAnswerAlikeInOneStatement(t *testing.T) {
	fixture := newByNumberFixture(t)

	fixture.acceptedFivePieceRequest(t, "P20261102000002")
	fixture.insertNumbered(t, numberedRequest(t, "tenant-1", "customer-2", "key-by-number-other-account", "REQ-BY-NUMBER-OTHER-ACCOUNT",
		[]string{"by-number-o"}, nil), "P20261102000005")
	fixture.insertNumbered(t, numberedRequest(t, "tenant-2", "customer-1", "key-by-number-other-tenant", "REQ-BY-NUMBER-OTHER-TENANT",
		[]string{"by-number-t"}, nil), "P20261102000006")
	fixture.insertNumbered(t, numberedRequest(t, "tenant-2", "customer-1", "key-by-number-same-number", "REQ-BY-NUMBER-SAME-NUMBER",
		[]string{"by-number-t2"}, nil), "P20261102000002")
	fixture.insertNumbered(t, numberedRequest(t, "tenant-1", "customer-1", "key-by-number-legacy", "REQ-BY-NUMBER-LEGACY",
		[]string{"by-number-l"}, nil), "")
	fixture.recorder.take()

	var shape string
	for name, number := range map[string]string{
		"另一账户的号":      "P20261102000005",
		"只在另一租户的号":    "P20261102000006",
		"本租户没有的号":     "P20261102999999",
		"大小写折过的号":     "p20261102000002",
		"无号委托的委托标识当号": "REQ-BY-NUMBER-LEGACY",
	} {
		record, found, err := fixture.views.FindCustomerShipmentByDocumentNumber(t.Context(),
			mustBuild(t, domain.NewTenantID, "tenant-1"), mustBuild(t, domain.NewCustomerAccountID, "customer-1"),
			mustBuild(t, domain.NewShipmentDocumentNumber, number))
		if err != nil || found || record.State != domain.ShipmentRequestStateInvalid || len(record.EffectiveParcels) != 0 {
			t.Errorf("%s：found=%v record=%+v err=%v, want 查无", name, found, record, err)
		}
		statements := fixture.recorder.take()
		if len(statements) != 1 {
			t.Fatalf("%s：发了 %d 条语句，want 1", name, len(statements))
		}
		if shape == "" {
			shape = statements[0].sql
		} else if statements[0].sql != shape {
			t.Errorf("%s 走了另一段 SQL：%s", name, statements[0].sql)
		}
		args := fmt.Sprint(statements[0].args...)
		if !strings.Contains(args, "customer-1") || !strings.Contains(args, number) || !strings.Contains(args, "tenant-1") {
			t.Errorf("%s：租户、账户与号没都进这条语句的参数：%v", name, statements[0].args)
		}
	}

	if found, state, _ := fixture.find(t, "tenant-1", "customer-1", "P20261102000002"); !found || state != domain.ShipmentRequestAccepted {
		t.Fatalf("本租户本账户的号：found=%v state=%s", found, state)
	}
	if found, state, _ := fixture.find(t, "tenant-2", "customer-1", "P20261102000002"); !found || state != domain.ShipmentRequestSubmitted {
		t.Fatalf("另一租户的同号：found=%v state=%s, want 它自己那份已提交", found, state)
	}
}

// Covers: ADR-0356 决定三「分页」的提供方半边——真库上经应用读口按页取：每页两件、严格接在上一页末件之后，Total 是五件、每页
// 相同，末页之后没有下一页；拼起来的次序就是首个提交版本的次序。
func TestEffectiveParcelsArePagedInFirstVersionOrderAgainstARealDatabase(t *testing.T) {
	fixture := newByNumberFixture(t)
	fixture.acceptedFivePieceRequest(t, "P20261102000002")
	handler, err := application.NewReadCustomerShipmentByDocumentNumberHandler(fixture.views)
	if err != nil {
		t.Fatal(err)
	}

	var read []string
	after := 0
	for page := 1; page <= 4; page++ {
		result, err := handler.Handle(t.Context(), application.ReadCustomerShipmentByDocumentNumberQuery{
			Tenant:        mustBuild(t, domain.NewTenantID, "tenant-1"),
			Customer:      mustBuild(t, domain.NewCustomerAccountID, "customer-1"),
			Number:        mustBuild(t, domain.NewShipmentDocumentNumber, "P20261102000002"),
			AfterPosition: after,
			Limit:         2,
		})
		if err != nil || !result.Found() || result.Total() != 5 {
			t.Fatalf("第 %d 页：found=%v total=%d err=%v", page, result.Found(), result.Total(), err)
		}
		for _, parcel := range result.Parcels() {
			read = append(read, parcel.Parcel().String())
			after = parcel.Position()
		}
		if !result.More() {
			break
		}
	}
	if strings.Join(read, ",") != "by-number-c,by-number-a,by-number-e,by-number-b,by-number-d" {
		t.Fatalf("逐页读到 %v", read)
	}
}
