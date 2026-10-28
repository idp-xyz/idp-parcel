package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 钉迁移 0030 的三本册（ADR-0342）：申报通道、发送事实、技术回执各自同内容重放、异内容冲突不覆盖；发送事实按消息号找得回；回执
// 按到达先后列出；写口在事务外按框架合同拒。真库，合成串。

// sendsFixture 让三本新册与申报链库各走各的事务：两只 bentopg.DB 包着同一个连接池，事务属主不能混用。
type sendsFixture struct {
	*declarationFixture
	db *bentopg.DB
}

func (fixture sendsFixture) sendTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.db.Transactor().WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func newSends(t *testing.T) (*adapter.SubmissionSends, sendsFixture) {
	t.Helper()
	fixture := newDeclarationFixture(t)
	db, err := bentopg.NewDB(fixture.pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	sends, err := adapter.NewSubmissionSends(db)
	if err != nil {
		t.Fatal(err)
	}
	return sends, sendsFixture{declarationFixture: fixture, db: db}
}

func TestSubmissionSendWritesRefuseToRunOutsideATransaction(t *testing.T) {
	sends, err := adapter.NewSubmissionSends(func() *bentopg.DB {
		db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
		if err != nil {
			t.Fatal(err)
		}
		return db
	}())
	if err != nil {
		t.Fatal(err)
	}
	tenant := declarationValue(t, domain.NewTenantID, "tenant-a")
	version := declarationValue(t, domain.NewSubmissionVersionID, "v-1")
	channel, _ := domain.NewDeclarationChannel("SYN-CHANNEL-01", "https://broker.syn.example/d", "")
	fact, _ := domain.RecordSubmissionSend(version, 1, time.Now(), domain.AttemptAcknowledged, "MSG-1", "")
	receipt, _ := domain.ReceiveTechnicalReceipt("R-1", version, 1, domain.AttemptAcknowledged, time.Now(), time.Now(), "d")
	if _, err := sends.SaveDeclarationChannel(t.Context(), tenant, channel, time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveDeclarationChannel err = %v", err)
	}
	if _, err := sends.SaveSendFact(t.Context(), tenant, fact, time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveSendFact err = %v", err)
	}
	if _, err := sends.SaveTechnicalReceipt(t.Context(), tenant, receipt, time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveTechnicalReceipt err = %v", err)
	}
}

// Covers: AT-CC-453（ADR-0342 决定三）——一个发送目标一份通道：同内容重放、异内容冲突，册上那一份不动。
func TestADeclarationChannelRegistersOnce(t *testing.T) {
	sends, fixture := newSends(t)
	ctx := context.Background()
	tenant := declarationValue(t, domain.NewTenantID, "tenant-a")
	first, _ := domain.NewDeclarationChannel("SYN-CHANNEL-01", "https://broker.syn.example/d", "SYN_BROKER_TOKEN")
	other, _ := domain.NewDeclarationChannel("SYN-CHANNEL-01", "https://other.syn.example/d", "")
	var effects []ports.SubmissionSendEffect
	fixture.sendTx(t, ctx, func(txCtx context.Context) error {
		for _, channel := range []domain.DeclarationChannel{first, first, other} {
			effect, err := sends.SaveDeclarationChannel(txCtx, tenant, channel, time.Now())
			if err != nil {
				return err
			}
			effects = append(effects, effect)
		}
		return nil
	})
	if effects[0] != ports.SubmissionSendRegistered || effects[1] != ports.SubmissionSendReplayed || effects[2] != ports.SubmissionSendConflict {
		t.Fatalf("落点 = %v", effects)
	}
	loaded, found, err := sends.LoadDeclarationChannel(ctx, tenant, "SYN-CHANNEL-01")
	if err != nil || !found || !loaded.SameChannel(first) {
		t.Fatalf("读回 = %+v found=%v err=%v", loaded, found, err)
	}
}

// Covers: AT-CC-454、AT-CC-455（ADR-0342 决定二、四）——一份尝试一笔发送事实：同内容重放、异内容冲突；按消息号找得回；技术回执同来源
// 身份同内容重放、异内容冲突，按到达先后列出。
func TestSendFactsAndReceiptsAppendWithoutOverwriting(t *testing.T) {
	sends, fixture := newSends(t)
	ctx := context.Background()
	record := submissionRecord(t, "tenant-a", "unit-1", "proc-export", "v-1")
	tenant := record.Key.TenantID
	version := record.Version.ID()
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.submissions.Save(txCtx, record)
		return err
	})
	sentAt := declarationFixedAt.Add(time.Hour)
	fact, _ := domain.RecordSubmissionSend(version, 1, sentAt, domain.AttemptPendingConfirmation, "MSG-1", "timeout")
	differs, _ := domain.RecordSubmissionSend(version, 1, sentAt, domain.AttemptAcknowledged, "MSG-1", "")
	first, _ := domain.ReceiveTechnicalReceipt("R-1", version, 1, domain.AttemptAcknowledged, sentAt, sentAt.Add(time.Minute), "d1")
	changed, _ := domain.ReceiveTechnicalReceipt("R-1", version, 1, domain.AttemptFailed, sentAt, sentAt.Add(time.Minute), "d1-changed")
	second, _ := domain.ReceiveTechnicalReceipt("R-2", version, 1, domain.AttemptFailed, sentAt, sentAt.Add(2*time.Minute), "d2")
	var effects []ports.SubmissionSendEffect
	fixture.sendTx(t, ctx, func(txCtx context.Context) error {
		for _, step := range []func() (ports.SubmissionSendEffect, error){
			func() (ports.SubmissionSendEffect, error) { return sends.SaveSendFact(txCtx, tenant, fact, time.Now()) },
			func() (ports.SubmissionSendEffect, error) { return sends.SaveSendFact(txCtx, tenant, fact, time.Now()) },
			func() (ports.SubmissionSendEffect, error) {
				return sends.SaveSendFact(txCtx, tenant, differs, time.Now())
			},
			func() (ports.SubmissionSendEffect, error) {
				return sends.SaveTechnicalReceipt(txCtx, tenant, first, time.Now())
			},
			func() (ports.SubmissionSendEffect, error) {
				return sends.SaveTechnicalReceipt(txCtx, tenant, first, time.Now())
			},
			func() (ports.SubmissionSendEffect, error) {
				return sends.SaveTechnicalReceipt(txCtx, tenant, changed, time.Now())
			},
			func() (ports.SubmissionSendEffect, error) {
				return sends.SaveTechnicalReceipt(txCtx, tenant, second, time.Now())
			},
		} {
			effect, err := step()
			if err != nil {
				return err
			}
			effects = append(effects, effect)
		}
		return nil
	})
	want := []ports.SubmissionSendEffect{ports.SubmissionSendRegistered, ports.SubmissionSendReplayed, ports.SubmissionSendConflict,
		ports.SubmissionSendRegistered, ports.SubmissionSendReplayed, ports.SubmissionSendConflict, ports.SubmissionSendRegistered}
	for index := range want {
		if effects[index] != want[index] {
			t.Fatalf("落点 = %v，want %v", effects, want)
		}
	}
	byMessage, found, err := sends.FindSendFactByChannelMessage(ctx, tenant, "MSG-1")
	if err != nil || !found || byMessage.Outcome() != domain.AttemptPendingConfirmation || byMessage.Version() != version {
		t.Fatalf("按消息号 = %+v found=%v err=%v", byMessage, found, err)
	}
	receipts, err := sends.ListTechnicalReceipts(ctx, tenant, version, 1)
	if err != nil || len(receipts) != 2 || receipts[0].Source() != "R-1" || receipts[0].Outcome() != domain.AttemptAcknowledged ||
		receipts[1].Source() != "R-2" || !receipts[0].OccurredAt().Equal(sentAt) {
		t.Fatalf("回执 = %+v err = %v", receipts, err)
	}
}
