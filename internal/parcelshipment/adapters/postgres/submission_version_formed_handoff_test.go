package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「新提交版本已形成」意图（ADR-0106 Decision 三）。它守的是
// 那条裁决里最容易悄悄失守的一格：新版本落了库而信封没入队，停在`等待受控补充`并已入账的
// 委托就再也没有投递来续办——读面上看不出来，库里那一行照样是「等补充」，与客户真的还没补
// 长着同一张脸。事件类型字面在这里再写一遍而不导入提供方常量，理由同复核完成那份用例。

const submissionVersionFormedEventType = "parcel-shipment.shipment-request.submission-version-formed"

// supplementFormedAt 是新版本成立的时刻；入队时刻另取，两者拉开才验得出 occurredAt 取的是
// 版本成立而不是入队。
var supplementFormedAt = submittedAtFixture.Add(2 * time.Hour)

type versionFormedClock struct{ at time.Time }

func (clock versionFormedClock) Now() time.Time { return clock.at }

type versionFormedFixture struct {
	requests   *adapter.ShipmentRequests
	handoff    *adapter.OutboxSubmissionVersionFormedHandoff
	store      *outbox.Store
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newVersionFormedFixture(t *testing.T) *versionFormedFixture {
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
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxSubmissionVersionFormedHandoff(db, store, versionFormedClock{
		at: supplementFormedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("构造新版本交接：%v", err)
	}
	return &versionFormedFixture{
		requests: requests, handoff: handoff, store: store, transactor: db.Transactor(), pool: pool,
	}
}

func (fixture *versionFormedFixture) within(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

// seedSubmitted 把一份委托建到库里并读回：后面要 Save，乐观并发认的是库上那一版。
func (fixture *versionFormedFixture) seedSubmitted(
	t *testing.T,
	key, requestID string,
) (domain.SourceIdentity, domain.ShipmentRequest) {
	t.Helper()
	ctx := t.Context()
	identity := requestIdentity(t, key)

	mustInsert(t, fixture.transactor, ctx, fixture.requests, submittedShipmentRequest(t, key, requestID))
	loaded, found, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回 %s：found=%v err=%v", requestID, found, err)
	}
	return identity, loaded
}

// supplemented 在聚合上形成同一委托的新提交版本：成员集合不变（改了要走关联新委托），
// 新版本带自己的来源身份与版本号。
func supplemented(t *testing.T, request domain.ShipmentRequest, key string) domain.ShipmentRequest {
	t.Helper()
	superseded, err := request.FormNewSubmissionVersion(domain.NewSubmissionVersionSpec{
		VersionID:         mustBuild(t, domain.NewSubmissionVersionID, "version-2"),
		TaskID:            mustBuild(t, domain.NewAcceptanceDecisionTaskID, "task-2"),
		SourceSubmission:  requestFingerprint(t, key+"-supplement", "digest-2"),
		DeclaredParcelIDs: request.CurrentSubmissionVersion().DeclaredParcelIDs(),
		EstablishedAt:     supplementFormedAt,
	})
	if err != nil {
		t.Fatalf("形成新提交版本：%v", err)
	}
	return superseded
}

// versionFormedEventID 照 ADR-0043 的认领口径拼：来源身份 + **新**提交版本 + 类型段。版本段
// 取新版本——同一委托的第二次补充是另一封信，取旧版本或省掉它都会被 EnqueueOnce 当重放吞掉。
func versionFormedEventID(identity domain.SourceIdentity, request domain.ShipmentRequest) string {
	return identity.TenantID().String() + "/" +
		identity.CustomerAccountID().String() + "/" +
		identity.Source().String() + "/" +
		identity.RequestKey().String() + "/" +
		request.CurrentSubmissionVersion().VersionID().String() + "/submission-version-formed"
}

func countVersionFormedIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, submissionVersionFormedEventType,
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

// Covers: ADR-0106 Decision 三 —— 新版本与信封同事务成立，且信封签的是新版本。
func TestSubmissionVersionFormedIntentCommitsAtomicallyWithTheNewVersion(t *testing.T) {
	fixture := newVersionFormedFixture(t)
	ctx := t.Context()
	identity, loaded := fixture.seedSubmitted(t, "req-key-1", "request-1")
	superseded := supplemented(t, loaded, "req-key-1")
	eventID := versionFormedEventID(identity, superseded)

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.requests.Save(txCtx, identity, superseded); err != nil {
			return err
		}
		return fixture.handoff.HandOffSubmissionVersionFormed(txCtx,
			ports.SubmissionVersionFormedHandoffIntent{Identity: identity, Request: superseded})
	})

	persisted, found, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回已换代的委托：found=%v err=%v", found, err)
	}
	if got := persisted.CurrentSubmissionVersion().VersionID().String(); got != "version-2" {
		t.Fatalf("当前版本 = %q, want version-2——新版本没落库", got)
	}
	if count := countVersionFormedIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}

	envelope := claimVersionFormedEnvelope(t, fixture.store)
	var payload struct {
		SubmissionVersionID string   `json:"submissionVersionId"`
		DeclaredParcelIDs   []string `json:"declaredParcelIds"`
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("解载荷：%v", err)
	}
	if payload.SubmissionVersionID != "version-2" {
		t.Fatalf("载荷提交版本 = %q, want 新版本 version-2——续办拿旧版本重跑正是 ADR-0106 Context 第 3 条说的白烧", payload.SubmissionVersionID)
	}
	if len(payload.DeclaredParcelIDs) != len(superseded.CurrentSubmissionVersion().DeclaredParcelIDs()) {
		t.Fatalf("载荷成员清单 = %v", payload.DeclaredParcelIDs)
	}
	if !envelope.OccurredAt.Equal(supplementFormedAt) {
		t.Fatalf("occurredAt = %s, want 新版本成立时刻 %s——不是入队时刻", envelope.OccurredAt, supplementFormedAt)
	}
	if envelope.Subject != "request-1" {
		t.Fatalf("subject = %q, want request-1", envelope.Subject)
	}
}

// Covers: ADR-0106 Decision 三的另一半 —— 一起消失。回滚后新版本不该留在库上，否则委托就成了
// 「已换代但没有任何投递会来推它」的死格，与决定三禁止的「只翻转不给触发」是同一种永久停滞。
func TestSubmissionVersionFormedIntentRollbackDropsBoth(t *testing.T) {
	fixture := newVersionFormedFixture(t)
	ctx := t.Context()
	identity, loaded := fixture.seedSubmitted(t, "req-key-1", "request-1")
	superseded := supplemented(t, loaded, "req-key-1")
	eventID := versionFormedEventID(identity, superseded)
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.requests.Save(txCtx, identity, superseded); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffSubmissionVersionFormed(txCtx,
			ports.SubmissionVersionFormedHandoffIntent{Identity: identity, Request: superseded}); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	persisted, found, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回委托：found=%v err=%v", found, err)
	}
	if got := persisted.CurrentSubmissionVersion().VersionID().String(); got == "version-2" {
		t.Error("回滚后新版本仍留在库上")
	}
	if count := countVersionFormedIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}
}

// Covers: ADR-0043 —— 重放重发同一份。同一提交版本至多形成一次，认领键因此稳定。
func TestResendingTheSameSubmissionVersionFormedIntentIsIdempotent(t *testing.T) {
	fixture := newVersionFormedFixture(t)
	ctx := t.Context()
	identity, loaded := fixture.seedSubmitted(t, "req-key-1", "request-1")
	superseded := supplemented(t, loaded, "req-key-1")
	eventID := versionFormedEventID(identity, superseded)
	intent := ports.SubmissionVersionFormedHandoffIntent{Identity: identity, Request: superseded}

	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffSubmissionVersionFormed(txCtx, intent)
	})
	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffSubmissionVersionFormed(txCtx, intent)
	})
	if count := countVersionFormedIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

// Covers: PBC-08 行为面 —— 写口在无事务上下文必须被 RequireExecutor 拒绝。这一条随适配器逐个
// 成立，不能靠别的口的同款用例代证。
func TestSubmissionVersionFormedIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newVersionFormedFixture(t)
	identity, loaded := fixture.seedSubmitted(t, "req-key-1", "request-1")
	superseded := supplemented(t, loaded, "req-key-1")
	eventID := versionFormedEventID(identity, superseded)

	err := fixture.handoff.HandOffSubmissionVersionFormed(t.Context(),
		ports.SubmissionVersionFormedHandoffIntent{Identity: identity, Request: superseded})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countVersionFormedIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

// Covers: 聚合上没有被换代的前版却走到这个口 = 边界壳接在了首次提交上而不是补充上。响亮报错而
// 不是入一封没有依据的信封——后者会让续办门拿首版再驱一条根本没停过的链。
func TestASubmissionVersionFormedIntentWithoutASupersededVersionIsLoud(t *testing.T) {
	fixture := newVersionFormedFixture(t)
	ctx := t.Context()
	identity, loaded := fixture.seedSubmitted(t, "req-key-1", "request-1")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffSubmissionVersionFormed(txCtx,
			ports.SubmissionVersionFormedHandoffIntent{Identity: identity, Request: loaded})
	}); err == nil {
		t.Fatal("聚合仍是首版时必须响亮报错")
	}
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffSubmissionVersionFormed(txCtx, ports.SubmissionVersionFormedHandoffIntent{})
	}); err == nil {
		t.Fatal("缺来源身份的意图必须响亮报错")
	}
}

// claimVersionFormedEnvelope 按派发一拍的同一条认领路径取回信封：要验的正是「库里那一封」，
// 自己拼等于把发布侧那半换成手抄本。认领时刻取在入队时刻之后——夹具的业务时刻落在真实
// 时钟前方，按真实时钟认领会一封也看不见。
func claimVersionFormedEnvelope(t *testing.T, store *outbox.Store) eventing.Envelope {
	t.Helper()
	deliveries, err := store.Claim(t.Context(), eventing.OutboxClaim{
		Now:         supplementFormedAt.Add(2 * time.Hour),
		Limit:       10,
		LeaseFor:    time.Minute,
		MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("认领待发信封：%v", err)
	}
	for _, delivery := range deliveries {
		if string(delivery.Envelope.Type) == submissionVersionFormedEventType {
			return delivery.Envelope
		}
	}
	t.Fatalf("认领到 %d 封，其中没有「新提交版本已形成」", len(deliveries))
	return eventing.Envelope{}
}
