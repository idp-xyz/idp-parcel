package postgres_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证税费付款核对的结算意图（票 sa-cc/05 完成判据 2）：与核对行同一
// 提交、回滚一并消失、重发同一份不翻倍、两版同区不同 ID、无事务拒、缺键响亮报错、载荷只带引用。
// 信封 ID 由核对幂等键（三维 + 指纹）认领、折成定长指纹形（票 sa-cc/29 裁决 1），分区键到「租户 / 申报范围」
// 带口名段。入队走 EnqueueOnce。

// fingerprintedEventID 按生产同一公式重算信封 ID：口名前缀 + "/" + 各维以 \x00 拼接后的 sha256 十六进制。三只核对口的
// 用例都拿它算 ID 去库里找行——断言的是「可重算、定长、在 eventing.MaxEventIDLength 内」，不再断言字面串接
// （票 sa-cc/29 裁决 1 做法 (5)）。公式在这里复述一遍而不是调生产那只未导出函数：生产改了公式、这里就红。
func fingerprintedEventID(portName string, dimensions ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(dimensions, "\x00")))
	return portName + "/" + hex.EncodeToString(digest[:])
}

const dutyVerificationEventType = "customs-compliance.duty-payment-verification.formed"

type dutyHandoffClock struct{ at time.Time }

func (clock dutyHandoffClock) Now() time.Time { return clock.at }

type dutyHandoffFixture struct {
	registers  *adapter.DutyPaymentReconciliation
	handoff    *adapter.OutboxDutyPaymentVerificationHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newDutyHandoffFixture(t *testing.T) *dutyHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registers, err := adapter.NewDutyPaymentReconciliation(db)
	if err != nil {
		t.Fatalf("构造税费付款核对登记册：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxDutyPaymentVerificationHandoff(db, store, dutyHandoffClock{
		at: time.Date(2026, 9, 11, 6, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &dutyHandoffFixture{registers: registers, handoff: handoff, transactor: db.Transactor(), pool: pool}
}

func (fixture *dutyHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

// saveVerificationWithFundsFact 在同一事务里先铺核对行的外键前置（0016 的核对表引用资金事实表）
// 再落核对行——真库上的「核对形成」就是这两步之后的事。
func (fixture *dutyHandoffFixture) saveVerificationWithFundsFact(
	t *testing.T, ctx context.Context, record ports.DutyVerificationRecord,
) error {
	t.Helper()
	if _, err := fixture.registers.RegisterFundsFact(ctx, tenantA(t), synFundsFact(t, 12500)); err != nil {
		return err
	}
	_, err := fixture.registers.SaveVerification(ctx, record)
	return err
}

func dutyVerificationIntent(t *testing.T, coverage domain.DutyCoverage, digest string) ports.DutyPaymentVerificationHandoffIntent {
	t.Helper()
	record := synVerificationRecord(t, coverage, digest)
	return ports.DutyPaymentVerificationHandoffIntent{Key: record.Key, Verification: record.Verification}
}

func dutyVerificationEventID(key ports.DutyVerificationKey) string {
	return fingerprintedEventID("duty-payment-verification",
		key.TenantID.String(), key.Scope.String(), key.Duty.String(), key.Funds.String(), key.Digest)
}

func dutyVerificationPartitionKey(key ports.DutyVerificationKey) string {
	return key.TenantID.String() + "/duty-payment-verification/" + key.Scope.String()
}

// TestOverlongReferencesStillProduceAnEnvelopeIDWithinTheFrameworkLimit 钉票 sa-cc/29 完成判据 (1)：租户 / 范围 /
// 税费 / 资金四个引用取到旧串接形必然超过 eventing.MaxEventIDLength 的长度（引用多长归实例半边，本仓给不出上界），
// 信封仍入队成功；ID 定长且在上限内；同一核对两次算出同一个 ID（第二次交被 EnqueueOnce 当同一份吞掉，行数仍一）。
func TestOverlongReferencesStillProduceAnEnvelopeIDWithinTheFrameworkLimit(t *testing.T) {
	fixture := newDutyHandoffFixture(t)
	ctx := t.Context()
	long := func(prefix string) string { return prefix + "-" + strings.Repeat("x", 60) }
	duty := viewValue(t, domain.NewAssessedDutyReference, long("SYN-DUTY"))
	funds := viewValue(t, domain.NewExternalFundsFactReference, long("SYN-FUNDS"))
	scope := viewValue(t, domain.NewDecisionScopeReference, long("SYN-UNIT"))
	verification, err := domain.VerifyDutyPayment(duty, funds,
		viewValue(t, domain.NewFundsFactVersion, long("SYN-FUNDS")+"/v1"), scope, synProcedure(t, "SYN-PROC-IMPORT"),
		domain.CoverageFull, domain.DeltaNone, domain.FundsFactValid, dutyRegistryBaseAt)
	if err != nil {
		t.Fatalf("构造核对：%v", err)
	}
	digest := sha256.Sum256([]byte("SYN-CONTENT"))
	key := ports.DutyVerificationKey{
		TenantID: tenantA(t), Duty: duty, Funds: funds, Scope: scope, Digest: hex.EncodeToString(digest[:]),
	}
	concatenated := dutyVerificationPartitionKey(key) + "/" + duty.String() + "/" + funds.String() + "/" + key.Digest
	if len(concatenated) <= eventing.MaxEventIDLength {
		t.Fatalf("夹具没造出超长：串接形 %d 字节没超过上限 %d，本格证不了东西", len(concatenated), eventing.MaxEventIDLength)
	}
	intent := ports.DutyPaymentVerificationHandoffIntent{Key: key, Verification: verification}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDutyPaymentVerification(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDutyPaymentVerification(txCtx, intent)
	})

	eventID := dutyVerificationEventID(key)
	if len(eventID) > eventing.MaxEventIDLength || len(eventID) != len("duty-payment-verification/")+hex.EncodedLen(sha256.Size) {
		t.Fatalf("ID = %q（%d 字节）；该是口名前缀加六十四位十六进制、在上限 %d 内", eventID, len(eventID), eventing.MaxEventIDLength)
	}
	if count := countDutyVerificationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("超长引用下 outbox 行数 = %d，want 1——信封没入队，或两次算出了两个 ID", count)
	}
	if got := partitionKeyOf(t, fixture.pool, eventID); got != dutyVerificationPartitionKey(key) {
		t.Fatalf("分区键 = %q；ID 改指纹形不该动分区键的可读形", got)
	}
}

// TestDutyVerificationIntentCommitsAtomicallyWithTheRecord 证意图与核对行同一提交，且事件类型是本口的。
func TestDutyVerificationIntentCommitsAtomicallyWithTheRecord(t *testing.T) {
	fixture := newDutyHandoffFixture(t)
	ctx := t.Context()
	record := synVerificationRecord(t, domain.CoverageFull, "digest-v1")
	intent := ports.DutyPaymentVerificationHandoffIntent{Key: record.Key, Verification: record.Verification}
	eventID := dutyVerificationEventID(record.Key)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.saveVerificationWithFundsFact(t, txCtx, record); err != nil {
			return err
		}
		return fixture.handoff.HandOffDutyPaymentVerification(txCtx, intent)
	})

	if _, exists, err := fixture.registers.FindVerification(ctx, record.Key); err != nil || !exists {
		t.Fatalf("核对行不在：err=%v exists=%v", err, exists)
	}
	if count := countDutyVerificationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
}

// TestDutyVerificationIntentRollbackDropsBoth 证回滚时核对行与意图一并消失，再投即一封。
func TestDutyVerificationIntentRollbackDropsBoth(t *testing.T) {
	fixture := newDutyHandoffFixture(t)
	ctx := t.Context()
	record := synVerificationRecord(t, domain.CoverageFull, "digest-v1")
	intent := ports.DutyPaymentVerificationHandoffIntent{Key: record.Key, Verification: record.Verification}
	eventID := dutyVerificationEventID(record.Key)
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := fixture.saveVerificationWithFundsFact(t, txCtx, record); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffDutyPaymentVerification(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.registers.FindVerification(ctx, record.Key); err != nil || exists {
		t.Fatalf("回滚后核对行仍在：err=%v exists=%v", err, exists)
	}
	if count := countDutyVerificationIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.saveVerificationWithFundsFact(t, txCtx, record); err != nil {
			return err
		}
		return fixture.handoff.HandOffDutyPaymentVerification(txCtx, intent)
	})
	if count := countDutyVerificationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

// TestResendingTheSameDutyVerificationIntentIsIdempotent 证 EnqueueOnce 不翻倍：同一版再交仍是一封。
func TestResendingTheSameDutyVerificationIntentIsIdempotent(t *testing.T) {
	fixture := newDutyHandoffFixture(t)
	ctx := t.Context()
	intent := dutyVerificationIntent(t, domain.CoverageFull, "digest-v1")
	eventID := dutyVerificationEventID(intent.Key)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDutyPaymentVerification(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDutyPaymentVerification(txCtx, intent)
	})
	if count := countDutyVerificationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

// TestTwoVerificationVersionsOfOneScopeAreTwoEnvelopesInOnePartition 钉 ID 与分区键的分工：
// 同一范围的核对随迟到事实换指纹换版，两版**都入队**（ID 含指纹，第二版不被 EnqueueOnce 当成
// 首版的重放吞掉）**且同分区**（分区键到「租户 / 申报范围」带口名段，后一版排在前一版后面，
// ADR-0069 决定二）。指纹进分区键每版自成一区，SA 读到的核对版本就没有先后可言。
func TestTwoVerificationVersionsOfOneScopeAreTwoEnvelopesInOnePartition(t *testing.T) {
	fixture := newDutyHandoffFixture(t)
	ctx := t.Context()
	first := dutyVerificationIntent(t, domain.CoverageFull, "digest-v1")
	second := dutyVerificationIntent(t, domain.CoverageNone, "digest-v2")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.handoff.HandOffDutyPaymentVerification(txCtx, first); err != nil {
			return err
		}
		return fixture.handoff.HandOffDutyPaymentVerification(txCtx, second)
	})

	firstID, secondID := dutyVerificationEventID(first.Key), dutyVerificationEventID(second.Key)
	if count := countDutyVerificationIntents(t, fixture.pool, firstID); count != 1 {
		t.Fatalf("首版行数 = %d，want 1", count)
	}
	if count := countDutyVerificationIntents(t, fixture.pool, secondID); count != 1 {
		t.Fatalf("改判版行数 = %d，want 1——ID 不带指纹时第二版会被 EnqueueOnce 静默吞掉", count)
	}
	const wantPartition = "tenant-a/duty-payment-verification/SYN-UNIT-01"
	if got := partitionKeyOf(t, fixture.pool, firstID); got != wantPartition {
		t.Fatalf("首版分区键 = %q，want %q", got, wantPartition)
	}
	if got := partitionKeyOf(t, fixture.pool, secondID); got != wantPartition {
		t.Fatalf("改判版分区键 = %q；两版不同分区就没有先后可言", got)
	}
}

// TestDutyVerificationPayloadCarriesOnlyReferences 证票面红线「信封不带金额结论」：载荷恰是
// 租户、申报范围、税费引用、资金事实引用与版本指纹这几个引用键，三轴与依据一个都不在里面——
// 覆盖 / 差额 / 有效性由 SA 按引用回读 CC 的核对册，CC 是权威。
func TestDutyVerificationPayloadCarriesOnlyReferences(t *testing.T) {
	fixture := newDutyHandoffFixture(t)
	ctx := t.Context()
	intent := dutyVerificationIntent(t, domain.CoverageFull, "digest-v1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDutyPaymentVerification(txCtx, intent)
	})

	var raw []byte
	if err := fixture.pool.QueryRow(ctx,
		`SELECT payload FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		dutyVerificationEventID(intent.Key),
	).Scan(&raw); err != nil {
		t.Fatalf("读取载荷：%v", err)
	}
	var payload map[string]string
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("译载荷：%v", err)
	}
	want := map[string]string{
		"tenantId": "tenant-a",
		"scope":    "SYN-UNIT-01",
		"duty":     "SYN-DUTY-01/v1",
		"funds":    "SYN-FUNDS-01",
		"digest":   "digest-v1",
	}
	if len(payload) != len(want) {
		t.Fatalf("载荷键集 = %v，want 恰好这几个引用键 %v", payload, want)
	}
	for field, value := range want {
		if payload[field] != value {
			t.Fatalf("载荷 %s = %q，want %q", field, payload[field], value)
		}
	}
	if got := dutyVerificationIntentType(t, fixture.pool, dutyVerificationEventID(intent.Key)); got != dutyVerificationEventType {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestDutyVerificationIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newDutyHandoffFixture(t)
	intent := dutyVerificationIntent(t, domain.CoverageFull, "digest-v1")
	eventID := dutyVerificationEventID(intent.Key)
	if err := fixture.handoff.HandOffDutyPaymentVerification(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countDutyVerificationIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

// TestAForeignDutyVerificationIntentIsLoud 证缺幂等键或缺核对时刻的意图是装配缺陷，响亮报错不入队。
func TestAForeignDutyVerificationIntentIsLoud(t *testing.T) {
	fixture := newDutyHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDutyPaymentVerification(txCtx, ports.DutyPaymentVerificationHandoffIntent{
			Key: ports.DutyVerificationKey{TenantID: tenantA(t)},
		})
	}); err == nil {
		t.Fatal("缺幂等键的意图必须响亮报错")
	}
	complete := dutyVerificationIntent(t, domain.CoverageFull, "digest-v1")
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDutyPaymentVerification(txCtx, ports.DutyPaymentVerificationHandoffIntent{
			Key: complete.Key,
		})
	}); err == nil {
		t.Fatal("缺核对对象的意图必须响亮报错——OccurredAt 无从取")
	}
}

func countDutyVerificationIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, dutyVerificationEventType,
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func dutyVerificationIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
	t.Helper()
	var eventType string
	err := pool.QueryRow(t.Context(),
		`SELECT event_type FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		eventID,
	).Scan(&eventType)
	if err != nil {
		t.Fatalf("读事件类型：%v", err)
	}
	return eventType
}
