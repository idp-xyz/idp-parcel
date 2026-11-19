package postgres_test

import (
	"context"
	"fmt"
	"testing"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证 FindByIDs：结算的查询面按一批评价标识读回评价，给每笔费用配「计费重来源」。它的
// SQL 只被替身测过，这里让它真跑一遍：读回的就是点名的那几份且与存进去的一致；别的租户的评价不被读回（ADR-0003）；
// 点名了不存在的标识不报错、只是不出现；空输入答空、不碰库。结果不保证次序，调用方按评价标识对号，所以这里也按标识比。

func saveEvaluations(t *testing.T, repository *adapter.Evaluations, transactor bentoapp.Transactor, evaluations ...domain.PricingEvaluation) {
	t.Helper()
	for _, evaluation := range evaluations {
		inEvaluationTx(t, transactor, t.Context(), func(txCtx context.Context) error {
			outcome, err := repository.Save(txCtx, evaluation)
			if err == nil && outcome != ports.EvaluationSaved {
				return fmt.Errorf("评价 %s 没有落库：outcome=%d", evaluation.ID(), outcome)
			}
			return err
		})
	}
}

func evaluationIDs(evaluations ...domain.PricingEvaluation) []domain.EvaluationID {
	ids := make([]domain.EvaluationID, 0, len(evaluations))
	for _, evaluation := range evaluations {
		ids = append(ids, evaluation.ID())
	}
	return ids
}

// assertReadBack 要求读回的恰是 want 这几份：每个标识至多一次，快照与存进去的逐字节相同，状态相同。
func assertReadBack(t *testing.T, label string, got []domain.PricingEvaluation, want ...domain.PricingEvaluation) {
	t.Helper()
	byID := make(map[string]domain.PricingEvaluation, len(got))
	for _, evaluation := range got {
		if _, repeated := byID[evaluation.ID().String()]; repeated {
			t.Fatalf("%s：标识 %s 读回了不止一份", label, evaluation.ID())
		}
		byID[evaluation.ID().String()] = evaluation
	}
	if len(byID) != len(want) {
		t.Fatalf("%s：读回 %d 份，想要 %d 份", label, len(byID), len(want))
	}
	for _, saved := range want {
		read, found := byID[saved.ID().String()]
		if !found {
			t.Fatalf("%s：缺 %s", label, saved.ID())
		}
		if read.Status() != saved.Status() || snapshotOf(t, read) != snapshotOf(t, saved) {
			t.Fatalf("%s：%s 读回的内容与存进去的不同（状态 %s / %s）", label, saved.ID(), read.Status(), saved.Status())
		}
	}
}

func TestEvaluationsAreReadBackInBatchByTheirIDs(t *testing.T) {
	repository, transactor := newEvaluations(t)
	ctx := t.Context()

	completed := evaluatedFixture(t, "eval-batch-completed", "tenant-a", "Z1")
	failed := evaluatedFixture(t, "eval-batch-failed", "tenant-a", "Z9")
	referenced := referencedFixture(t, "eval-batch-referenced", "tenant-a", "Z1", "EVREQ-SYN-BATCH")
	untouched := evaluatedFixture(t, "eval-batch-untouched", "tenant-a", "Z1")
	if completed.Status() == failed.Status() {
		t.Fatalf("夹具两份状态相同（%q）；想要一份算成、一份没算成", completed.Status())
	}
	saveEvaluations(t, repository, transactor, completed, failed, referenced, untouched)
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	got, err := repository.FindByIDs(ctx, tenant, evaluationIDs(completed, failed, referenced))
	if err != nil {
		t.Fatalf("按一批标识读回：%v", err)
	}
	assertReadBack(t, "三份", got, completed, failed, referenced)

	// 只点名一份就只回一份；没点名的那份在库里也不跟着出来。
	got, err = repository.FindByIDs(ctx, tenant, evaluationIDs(failed))
	if err != nil {
		t.Fatalf("按一个标识读回：%v", err)
	}
	assertReadBack(t, "一份", got, failed)

	// 同一标识点名两次也只回一份：调用方按标识对号，重复的一行会让对号落进歧义。
	got, err = repository.FindByIDs(ctx, tenant, evaluationIDs(completed, completed))
	if err != nil {
		t.Fatalf("重复点名：%v", err)
	}
	assertReadBack(t, "重复点名", got, completed)

	// 带回指的那份读回后回指仍在：按批读与按标识读共用同一套比对列。
	got, err = repository.FindByIDs(ctx, tenant, evaluationIDs(referenced))
	if err != nil || len(got) != 1 {
		t.Fatalf("读回带回指的那份：%v，%d 份", err, len(got))
	}
	if reference, has := got[0].RequestReference(); !has || reference.String() != "EVREQ-SYN-BATCH" {
		t.Fatalf("读回的回指 = %q present=%v", reference, has)
	}
}

func TestBatchReadNeverReturnsAnotherTenantsEvaluation(t *testing.T) {
	repository, transactor := newEvaluations(t)
	ctx := t.Context()

	mine := evaluatedFixture(t, "eval-batch-mine", "tenant-a", "Z1")
	theirs := evaluatedFixture(t, "eval-batch-theirs", "tenant-b", "Z1")
	saveEvaluations(t, repository, transactor, mine, theirs)
	tenantA := evaluationValue(t, domain.NewTenantID, "tenant-a")
	tenantB := evaluationValue(t, domain.NewTenantID, "tenant-b")

	// 评价标识全局唯一，所以「别的租户同标识的评价」就是别的租户名下的那一行：本租户点名它读不到，也不拖累同批里本租户自己的。
	got, err := repository.FindByIDs(ctx, tenantA, evaluationIDs(mine, theirs))
	if err != nil {
		t.Fatalf("租户 A 点名两份：%v", err)
	}
	assertReadBack(t, "租户 A", got, mine)

	got, err = repository.FindByIDs(ctx, tenantA, evaluationIDs(theirs))
	if err != nil || len(got) != 0 {
		t.Fatalf("租户 A 单点租户 B 的评价读到了：err=%v %d 份", err, len(got))
	}

	// 反过来同理，且租户 B 点名自己的照常读回。
	got, err = repository.FindByIDs(ctx, tenantB, evaluationIDs(mine, theirs))
	if err != nil {
		t.Fatalf("租户 B 点名两份：%v", err)
	}
	assertReadBack(t, "租户 B", got, theirs)

	// 一个库里根本没有的租户什么也读不到。
	nobody := evaluationValue(t, domain.NewTenantID, "tenant-nobody")
	got, err = repository.FindByIDs(ctx, nobody, evaluationIDs(mine, theirs))
	if err != nil || len(got) != 0 {
		t.Fatalf("无此租户读到了：err=%v %d 份", err, len(got))
	}
}

func TestBatchReadSkipsIDsThatDoNotExist(t *testing.T) {
	repository, transactor := newEvaluations(t)
	ctx := t.Context()

	first := evaluatedFixture(t, "eval-batch-first", "tenant-a", "Z1")
	second := evaluatedFixture(t, "eval-batch-second", "tenant-a", "Z1")
	saveEvaluations(t, repository, transactor, first, second)
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")
	absent := func(id string) domain.EvaluationID { return evaluationValue(t, domain.NewEvaluationID, id) }

	got, err := repository.FindByIDs(ctx, tenant, []domain.EvaluationID{
		first.ID(), absent("eval-batch-absent-1"), second.ID(), absent("eval-batch-absent-2"),
	})
	if err != nil {
		t.Fatalf("夹着不存在的标识读回：%v", err)
	}
	assertReadBack(t, "夹着不存在的", got, first, second)

	// 点名的全不存在：空结果、不报错。
	got, err = repository.FindByIDs(ctx, tenant, []domain.EvaluationID{absent("eval-batch-absent-1")})
	if err != nil || len(got) != 0 {
		t.Fatalf("全不存在：err=%v %d 份", err, len(got))
	}
}

// 空输入答空、不报错，且不向库发任何查询：这里把连接池关掉再问，真发了查询就会报「池已关」。关池之前先在活的库上问一遍，
// 证明空输入答的是空而不是「这个租户的全部」。
func TestBatchReadOfNoIDsIsEmptyAndSendsNoQuery(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewEvaluations(db)
	if err != nil {
		t.Fatalf("构造评价登记册：%v", err)
	}
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	stored := evaluatedFixture(t, "eval-batch-stored", "tenant-a", "Z1")
	saveEvaluations(t, repository, db.Transactor(), stored)
	for name, ids := range map[string][]domain.EvaluationID{"nil": nil, "空切片": {}} {
		got, err := repository.FindByIDs(ctx, tenant, ids)
		if err != nil || len(got) != 0 {
			t.Fatalf("活库上空输入（%s）：err=%v %d 份", name, err, len(got))
		}
	}

	pool.Close()
	if _, err := repository.FindByIDs(ctx, tenant, evaluationIDs(stored)); err == nil {
		t.Fatalf("对照：池关了之后点名一份竟没报错，说明下面的空输入没法证明没发查询")
	}
	for name, ids := range map[string][]domain.EvaluationID{"nil": nil, "空切片": {}} {
		got, err := repository.FindByIDs(ctx, tenant, ids)
		if err != nil || len(got) != 0 {
			t.Fatalf("池关了之后空输入（%s）：err=%v %d 份，想要不碰库", name, err, len(got))
		}
	}
}
