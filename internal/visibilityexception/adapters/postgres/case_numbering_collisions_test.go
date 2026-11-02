package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件对真实 PostgreSQL 16 证案件撞号册（VE 迁移 0033）：记下的命中原样读回、同键再撞覆盖成最近一次并立刻到期、推后与
// 销账、这一原因的发作期一落撞号那一行随之销掉、写口无事务即拒。测试输入是隔离合成，只记 S。

func collisionOf(t *testing.T, tenant, parcel, fact string, at time.Time, template string) ports.CaseNumberingCollision {
	t.Helper()
	return ports.CaseNumberingCollision{
		Tenant:   factValue(t, domain.NewTenantID, tenant),
		Hit:      episodeHit(t, parcel, "DEPARTURE_GATE_BLOCKED", fact, at),
		Template: template,
		At:       at.Add(time.Minute),
	}
}

// Covers: AT-VE-228 真库半边——案件撞号册：命中各样原样读回（因果条件、规则版本、可信度、事实依据、判断时间）、撞号那一刻
// 就到期；同一原因再撞保留首次命中、次数加一、换成最近撞上的格式；推后之后不到期；销掉之后不在；跨租户按到期序取。
func TestTheCaseNumberingCollisionBookKeepsTheHitForTheRedrive(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	book, err := adapter.NewCaseNumberingCollisions(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	inTx := func(fn func(context.Context) error) {
		t.Helper()
		if err := db.Transactor().WithinTransaction(ctx, fn); err != nil {
			t.Fatalf("事务内写入失败：%v", err)
		}
	}

	first := collisionOf(t, "SYN-TENANT-01", "SYN-PARCEL-01", "movement-fact/SYN-PARCEL-01/MF-1", factBaseAt, "SYN-{seq:5}")
	other := collisionOf(t, "SYN-TENANT-02", "SYN-PARCEL-01", "movement-fact/SYN-PARCEL-01/MF-1", factBaseAt.Add(time.Hour), "B-{seq:5}")
	inTx(func(txCtx context.Context) error {
		if err := book.RecordCaseNumberingCollision(txCtx, first); err != nil {
			return err
		}
		return book.RecordCaseNumberingCollision(txCtx, other)
	})
	due, err := book.DueCaseNumberingCollisions(ctx, factBaseAt.Add(2*time.Hour), 10)
	if err != nil || len(due) != 2 {
		t.Fatalf("到期 %d 件 %v", len(due), err)
	}
	got := due[0]
	if got.Tenant.String() != "SYN-TENANT-01" || got.Hit.Cause != first.Hit.Cause || got.Hit.Rule != first.Hit.Rule ||
		got.Hit.Confidence != first.Hit.Confidence || !got.Hit.At.Equal(first.Hit.At) || len(got.Hit.Basis) != 1 ||
		got.Hit.Basis[0] != first.Hit.Basis[0] || got.Template != "SYN-{seq:5}" || !got.At.Equal(first.At) {
		t.Fatalf("读回走样：%+v，want %+v", got, first)
	}
	if notYet, _ := book.DueCaseNumberingCollisions(ctx, factBaseAt, 10); len(notYet) != 0 {
		t.Fatalf("撞号之前就到期了：%+v", notYet)
	}

	// 同一原因再撞（命中晚了一小时）：首次命中不动，次数加一、换格式，立刻到期。
	again := first
	again.Hit.At = factBaseAt.Add(time.Hour)
	again.Template = "SYN2-{seq:5}"
	again.At = factBaseAt.Add(3 * time.Hour)
	inTx(func(txCtx context.Context) error {
		if err := book.PostponeCaseNumberingCollision(txCtx, first.Key(), factBaseAt.Add(24*time.Hour)); err != nil {
			return err
		}
		return book.RecordCaseNumberingCollision(txCtx, again)
	})
	var collisions int
	var template string
	var hitAt time.Time
	if err := pool.QueryRow(ctx, `SELECT collisions, template, hit_at FROM visibility_exception.case_numbering_collision
		WHERE tenant_id = 'SYN-TENANT-01'`).Scan(&collisions, &template, &hitAt); err != nil || collisions != 2 ||
		template != "SYN2-{seq:5}" || !hitAt.Equal(first.Hit.At) {
		t.Fatalf("再撞：%d 次 %q 命中 %v %v，want 两次、新格式、首次命中", collisions, template, hitAt, err)
	}
	if due, _ := book.DueCaseNumberingCollisions(ctx, again.At, 10); len(due) != 2 {
		t.Fatalf("再撞之后没立刻到期：%d", len(due))
	}

	inTx(func(txCtx context.Context) error {
		if err := book.PostponeCaseNumberingCollision(txCtx, first.Key(), again.At.Add(time.Hour)); err != nil {
			return err
		}
		return book.ResolveCaseNumberingCollision(txCtx, other.Key())
	})
	if due, _ := book.DueCaseNumberingCollisions(ctx, again.At.Add(time.Minute), 10); len(due) != 0 {
		t.Fatalf("推后与销账之后仍到期：%+v", due)
	}
	if due, _ := book.DueCaseNumberingCollisions(ctx, again.At.Add(2*time.Hour), 10); len(due) != 1 || due[0].Tenant.String() != "SYN-TENANT-01" {
		t.Fatalf("推后到期之后：%+v", due)
	}
}

// Covers: AT-VE-228 真库半边——这一原因的发作期一落，撞号册上它那一行就销掉：重驱建成的、或另一份命中先走通的都一样；别的
// 原因的行不动。
func TestSavingARaisedEpisodeResolvesItsCollision(t *testing.T) {
	fixture := newFactFixture(t)
	book, err := adapter.NewCaseNumberingCollisions(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	fact := "fact/parcel-1"
	collided := collisionOf(t, "tenant-a", "parcel-1", fact, factBaseAt, "SYN-{seq:5}")
	untouched := collisionOf(t, "tenant-a", "parcel-2", "fact/parcel-2", factBaseAt, "SYN-{seq:5}")
	collided.Hit.Kind = factValue(t, domain.NewExceptionSignalKindReference, "STALLED")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := book.RecordCaseNumberingCollision(txCtx, collided); err != nil {
			return err
		}
		return book.RecordCaseNumberingCollision(txCtx, untouched)
	})

	episode := openedEpisodeFor(t, "ep-1", "parcel-1", "STALLED", fact, factBaseAt)
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.episodes.SaveRaised(txCtx, raisedRecord(t, episode, "tenant-a", "parcel-1", "STALLED", domain.ManualReviewRequired))
	})
	due, err := book.DueCaseNumberingCollisions(ctx, factBaseAt.Add(time.Hour), 10)
	if err != nil || len(due) != 1 || due[0].Hit.Parcel.String() != "parcel-2" {
		t.Fatalf("落了发作期之后撞号册 = %+v %v，want 只剩另一原因那一行", due, err)
	}
}

// Covers: 案件撞号册的写口与两种锁行无事务即拒（ErrTransactionRequired），不改用连接池——锁只在事务里才锁得住。
func TestCaseNumberingCollisionWritesRefuseToRunOutsideATransaction(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	book, err := adapter.NewCaseNumberingCollisions(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	collision := collisionOf(t, "tenant-a", "parcel-1", "fact/parcel-1", factBaseAt, "SYN-{seq:5}")
	if err := book.RecordCaseNumberingCollision(ctx, collision); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务记撞号应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := book.PostponeCaseNumberingCollision(ctx, collision.Key(), factBaseAt); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务推后应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := book.ResolveCaseNumberingCollision(ctx, collision.Key()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务销账应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := book.HoldCaseNumberingCollision(ctx, collision.Key()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务锁那一行应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, _, err := book.ClaimDueCaseNumberingCollision(ctx, collision.Key(), factBaseAt); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务抢那一行应返回 ErrTransactionRequired，实得：%v", err)
	}
}
