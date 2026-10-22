package postgres_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// Covers: NO CONTEXT「集运包含关系必须无循环」的库面半边（`AT-NO-039`）——环里最短的一种是单元装进自己，它是容纳索引
// 一行之内就判得出的，由迁移 0022 的 CHECK 守；绕过适配器直接写的一行也进不去。更长的环跨行、CHECK 守不住，归编排与锁。
func TestAUnitCannotBeRecordedAsItsOwnMember(t *testing.T) {
	store, transactor, pool := newConsolidationStore(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	saveConsolidation(t, transactor, ctx, store, tenant, openConsolidation(t, "bag-1", "asset-7"))

	_, err := pool.Exec(ctx,
		`INSERT INTO node_operations.containment_current (tenant_id, member_id, unit_id)
		 VALUES ('tenant-a', 'bag-1', 'bag-1')`)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != "containment_current_not_self_contained" {
		t.Fatalf("单元装进自己的那一行进了容纳索引：err=%v", err)
	}
}

// nest 把 member 那只袋装进 unit，经单元库落下容纳行。
func nest(t *testing.T, transactor bentoapp.Transactor, ctx context.Context, store *adapter.ConsolidationUnits, tenant domain.TenantID, unit *domain.ConsolidationUnit, member string) {
	t.Helper()
	if err := unit.AddMember(ref(t, domain.NewHandlingUnitID, member)); err != nil {
		t.Fatalf("把 %s 装进 %s：%v", member, unit.ID(), err)
	}
	updateConsolidation(t, transactor, ctx, store, tenant, unit)
}

func lineageInTx(t *testing.T, transactor bentoapp.Transactor, ctx context.Context, store *adapter.ConsolidationUnits, tenant domain.TenantID, unit string) domain.ContainmentLineage {
	t.Helper()
	id := ref(t, domain.NewConsolidationUnitID, unit)
	var lineage domain.ContainmentLineage
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		read, err := store.LineageForUpdate(txCtx, tenant, id)
		lineage = read
		return err
	})
	return lineage
}

func lineageNames(lineage domain.ContainmentLineage) string {
	names := make([]string, 0, len(lineage))
	for _, unit := range lineage {
		names = append(names, unit.String())
	}
	return strings.Join(names, ",")
}

// Covers: NO CONTEXT「嵌套集运时通过直接父级链路派生间接包含」——祖先链沿当前容纳索引逐级上溯、自身在先、到根为止；
// 根的链路只有自己；不在册的单元链路也只有自己（在不在册归单元库答）；关闭的单元不再是任何东西的直接父级（关闭时
// 容纳行已释放），链路在它下面那一级就到头；别的租户的同名袋不进链路。
func TestLineageForUpdateWalksTheDirectParentChainToTheRoot(t *testing.T) {
	store, transactor, _ := newConsolidationStore(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	other := ref(t, domain.NewTenantID, "tenant-b")

	bagA, bagB, bagC := openConsolidation(t, "bag-a", "asset-a"), openConsolidation(t, "bag-b", "asset-b"), openConsolidation(t, "bag-c", "asset-c")
	for _, bag := range []*domain.ConsolidationUnit{bagA, bagB, bagC} {
		saveConsolidation(t, transactor, ctx, store, tenant, bag)
	}
	nest(t, transactor, ctx, store, tenant, bagA, "bag-b")
	nest(t, transactor, ctx, store, tenant, bagB, "bag-c")

	foreignA := openConsolidation(t, "bag-a", "asset-a")
	foreignB := openConsolidation(t, "bag-b", "asset-b")
	saveConsolidation(t, transactor, ctx, store, other, foreignA)
	saveConsolidation(t, transactor, ctx, store, other, foreignB)
	nest(t, transactor, ctx, store, other, foreignB, "bag-a")

	if got := lineageNames(lineageInTx(t, transactor, ctx, store, tenant, "bag-c")); got != "bag-c,bag-b,bag-a" {
		t.Fatalf("bag-c 的链路 = %q，要自身在先逐级到根", got)
	}
	if got := lineageNames(lineageInTx(t, transactor, ctx, store, tenant, "bag-a")); got != "bag-a" {
		t.Fatalf("根的链路 = %q，要只有自己；另一个租户里 bag-a 在 bag-b 里不该混进来", got)
	}
	if got := lineageNames(lineageInTx(t, transactor, ctx, store, tenant, "bag-x")); got != "bag-x" {
		t.Fatalf("不在册单元的链路 = %q，要只有自己", got)
	}

	if err := bagB.Close(ref(t, domain.NewWorkBasisReference, "DISPOSITION/9"), consolidationAt); err != nil {
		t.Fatalf("带处置依据关闭 bag-b：%v", err)
	}
	updateConsolidation(t, transactor, ctx, store, tenant, bagB)
	if got := lineageNames(lineageInTx(t, transactor, ctx, store, tenant, "bag-c")); got != "bag-c" {
		t.Fatalf("父级关闭后 bag-c 的链路 = %q，要只有自己", got)
	}
}

// Covers: 册已成环时祖先链不是答案——两行直接写成互装（CHECK 只拦自装），上溯走回自己即交回 ErrContainmentLineageCyclic，
// 不死循环、不在环上再判一次（ADR-0022「返回 error 就是没形成」；与读不动分开，这一种重试不会好）。
func TestAnAlreadyCyclicIndexIsAnErrorNotALineage(t *testing.T) {
	store, transactor, pool := newConsolidationStore(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	for _, id := range []string{"bag-a", "bag-b"} {
		saveConsolidation(t, transactor, ctx, store, tenant, openConsolidation(t, id, "asset-"+id))
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO node_operations.containment_current (tenant_id, member_id, unit_id)
		 VALUES ('tenant-a', 'bag-a', 'bag-b'), ('tenant-a', 'bag-b', 'bag-a')`); err != nil {
		t.Fatalf("直接写两行互装：%v", err)
	}

	bagA := ref(t, domain.NewConsolidationUnitID, "bag-a")
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := store.LineageForUpdate(txCtx, tenant, bagA)
		return err
	})
	if !errors.Is(err, ports.ErrContainmentLineageCyclic) {
		t.Fatalf("成环的册读出了链路：err=%v", err)
	}
}

// Covers: 并发互装只有一方成立的锁半边（`AT-NO-039`）——第二笔事务在祖先链那一问上等第一笔提交，提交后读到的链路已含
// 第一笔落下的那一级；锁随事务释放。没有这把锁，两笔各读到对方还是根、双双放行。
func TestLineageForUpdateWaitsForTheConcurrentMembershipWrite(t *testing.T) {
	store, transactor, _ := newConsolidationStore(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	bagA, bagB := openConsolidation(t, "bag-a", "asset-a"), openConsolidation(t, "bag-b", "asset-b")
	saveConsolidation(t, transactor, ctx, store, tenant, bagA)
	saveConsolidation(t, transactor, ctx, store, tenant, bagB)

	nested := ref(t, domain.NewHandlingUnitID, "bag-b")
	firstHolds := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	// 用例半途失败时也要放第一笔走、等两笔退出：悬着的事务会拖住本用例库的收尾（DROP DATABASE 等连接释放）。
	var workers sync.WaitGroup
	t.Cleanup(func() { unblock(); workers.Wait() })

	firstDone := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		firstDone <- transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			if _, err := store.LineageForUpdate(txCtx, tenant, bagA.ID()); err != nil {
				return err
			}
			if err := bagA.AddMember(nested); err != nil {
				return err
			}
			if err := store.Update(txCtx, tenant, bagA); err != nil {
				return err
			}
			close(firstHolds)
			<-release
			return nil
		})
	}()
	select {
	case <-firstHolds:
	case err := <-firstDone:
		t.Fatalf("第一笔没拿住就退出了：%v", err)
	}

	secondDone := make(chan string, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		var lineage domain.ContainmentLineage
		err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			read, err := store.LineageForUpdate(txCtx, tenant, bagB.ID())
			lineage = read
			return err
		})
		if err != nil {
			secondDone <- "error: " + err.Error()
			return
		}
		secondDone <- lineageNames(lineage)
	}()

	select {
	case got := <-secondDone:
		t.Fatalf("第二笔没有等第一笔提交就读到了链路 %q", got)
	case <-time.After(300 * time.Millisecond):
	}
	unblock()
	if err := <-firstDone; err != nil {
		t.Fatalf("第一笔提交：%v", err)
	}
	select {
	case got := <-secondDone:
		if got != "bag-b,bag-a" {
			t.Fatalf("第一笔提交后第二笔读到的链路 = %q，要已含第一笔落下的那一级", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("第一笔提交后第二笔仍没放行")
	}
}
