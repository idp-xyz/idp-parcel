package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件在真实 PostgreSQL 16 上证资金映射与核销指向的两格金额本体（隔离合成 S）：按租户取、他租户与不存在同答
// 没有；未核销面里的已核销只算未撤销的核销；锁与写同一事务，后到的锁读得到先到那笔已提交的核销；写口无事务即拒；
// 映射目标的 CHECK 收两格新值、仍拒词表外的值。

type claimTargetRig struct {
	targets      *adapter.ClaimSettlementTargets
	amounts      *adapter.CustomerClaimAmounts
	receivables  *adapter.RecoveryReceivables
	acks         *adapter.RecoveryAcknowledgements
	facts        *adapter.ExternalFundsFacts
	mappings     *adapter.FundsMappings
	applications *adapter.SettlementApplications
	transactor   bentoapp.Transactor
	pool         *pgxpool.Pool
}

func newClaimTargetRig(t *testing.T) claimTargetRig {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	rig := claimTargetRig{transactor: db.Transactor(), pool: pool}
	if rig.targets, err = adapter.NewClaimSettlementTargets(db); err != nil {
		t.Fatalf("构造金额本体目标口：%v", err)
	}
	if rig.amounts, err = adapter.NewCustomerClaimAmounts(db); err != nil {
		t.Fatalf("构造索赔金额库：%v", err)
	}
	if rig.receivables, err = adapter.NewRecoveryReceivables(db); err != nil {
		t.Fatalf("构造应追偿库：%v", err)
	}
	if rig.acks, err = adapter.NewRecoveryAcknowledgements(db); err != nil {
		t.Fatalf("构造认可库：%v", err)
	}
	if rig.facts, err = adapter.NewExternalFundsFacts(db); err != nil {
		t.Fatalf("构造资金事实库：%v", err)
	}
	if rig.mappings, err = adapter.NewFundsMappings(db); err != nil {
		t.Fatalf("构造映射库：%v", err)
	}
	if rig.applications, err = adapter.NewSettlementApplications(db); err != nil {
		t.Fatalf("构造核销库：%v", err)
	}
	return rig
}

func claimKey(t *testing.T, kind domain.SettlementTargetKind, target string) domain.ClaimTargetKey {
	t.Helper()
	return domain.ClaimTargetKey{Kind: kind, Target: saValue(t, domain.NewSettlementTargetReference, target)}
}

// seedClaimTargets 在 tenant-a 下登一条客户赔付 5000（compensation-1）与一条追偿认可 6000（acknowledgement-1），
// 在 tenant-b 下登一条同名的客户赔付。
func (rig claimTargetRig) seedClaimTargets(t *testing.T) {
	t.Helper()
	receivable := formedReceivableRecord(t, "tenant-a", "receivable-1")
	saWithin(t, rig.transactor, t.Context(), func(txCtx context.Context) error {
		if _, err := rig.amounts.Save(txCtx, formedClaimAmountRecord(t, "tenant-a", "compensation-1", domain.CustomerCompensationPayable)); err != nil {
			return err
		}
		if _, err := rig.amounts.Save(txCtx, formedClaimAmountRecord(t, "tenant-b", "compensation-1", domain.CustomerCompensationPayable)); err != nil {
			return err
		}
		if _, err := rig.receivables.Save(txCtx, receivable); err != nil {
			return err
		}
		_, err := rig.acks.Save(txCtx, formedAcknowledgementRecord(t, "tenant-a", "acknowledgement-1", receivable.Receivable))
		return err
	})
}

// compensationApplication 是 tenant-a 里一笔指向 compensation-1 的核销连同它的事实与映射，先在测试协程里备好，
// 落库那一步（save）只交回错误，可以放进别的协程里跑。
type compensationApplication struct {
	fact        ports.FundsFactRecord
	mapping     ports.FundsMappingRecord
	application ports.SettlementApplicationRecord
}

func compensationApplicationOf(t *testing.T, id string, amount int64) compensationApplication {
	t.Helper()
	return compensationApplicationIn(t, "tenant-a", id, amount)
}

func compensationApplicationIn(t *testing.T, tenant, id string, amount int64) compensationApplication {
	t.Helper()
	fact := adoptedFactRecord(t, tenant, "bank-fact-"+id, domain.FundsReceiptConfirmed, 8000)
	mapping := mappingRecord(t, tenant, "mapping-"+id, fact.Fact, domain.TargetCustomerCompensation, "compensation-1")
	application, err := domain.ApplySettlement(fact.Fact, []domain.FundsMapping{mapping.Mapping},
		[]domain.SettlementAllocation{{
			Mapping: mapping.Mapping.Mapping(), TargetKind: domain.TargetCustomerCompensation, Target: mapping.Mapping.Target(),
			Direction: domain.AllocationDebit, AmountMinor: amount,
		}},
		saValue(t, domain.NewApplicationReference, id),
		saValue(t, domain.NewApplicationBasisReference, "payment-instruction-1"),
		fundsAppliedAt)
	if err != nil {
		t.Fatalf("构造核销：%v", err)
	}
	return compensationApplication{fact: fact, mapping: mapping, application: ports.SettlementApplicationRecord{
		Key:           ports.SettlementApplicationKey{TenantID: saTenant(t, tenant), Application: application.Application()},
		ContentDigest: "digest-" + id,
		Application:   application,
		RecordedAt:    fundsAppliedAt,
	}}
}

func (rig claimTargetRig) save(ctx context.Context, prepared compensationApplication) error {
	if _, err := rig.facts.Save(ctx, prepared.fact); err != nil {
		return err
	}
	if _, err := rig.mappings.Save(ctx, prepared.mapping); err != nil {
		return err
	}
	_, err := rig.applications.Save(ctx, prepared.application)
	return err
}

// Covers: 两格目标的存在性与租户一致性——按租户取，他租户的与不存在的同答没有（ADR-0029）；金额本体取的是那一条自己的
// 金额与币种：客户赔付取赔付金额，追偿认可取认可金额而不是应追偿金额。
func TestClaimTargetsAreFoundOnlyInTheirOwnTenant(t *testing.T) {
	rig := newClaimTargetRig(t)
	rig.seedClaimTargets(t)
	ctx := t.Context()

	for _, probe := range []struct {
		tenant string
		key    domain.ClaimTargetKey
		want   bool
	}{
		{"tenant-a", claimKey(t, domain.TargetCustomerCompensation, "compensation-1"), true},
		{"tenant-a", claimKey(t, domain.TargetRecoveryAcknowledgement, "acknowledgement-1"), true},
		{"tenant-c", claimKey(t, domain.TargetCustomerCompensation, "compensation-1"), false},
		{"tenant-b", claimKey(t, domain.TargetRecoveryAcknowledgement, "acknowledgement-1"), false},
		{"tenant-a", claimKey(t, domain.TargetCustomerCompensation, "compensation-never"), false},
		{"tenant-a", claimKey(t, domain.TargetRecoveryAcknowledgement, "receivable-1"), false},
		{"tenant-a", claimKey(t, domain.TargetCustomerCompensation, "acknowledgement-1"), false},
	} {
		found, err := rig.targets.FindClaimTarget(ctx, saTenant(t, probe.tenant), probe.key)
		if err != nil {
			t.Fatalf("%s 找 %s/%s：%v", probe.tenant, probe.key.Kind, probe.key.Target, err)
		}
		if found != probe.want {
			t.Fatalf("%s 找 %s/%s = %v，要 %v", probe.tenant, probe.key.Kind, probe.key.Target, found, probe.want)
		}
	}

	saWithin(t, rig.transactor, ctx, func(txCtx context.Context) error {
		compensation, found, err := rig.targets.LockClaimTarget(txCtx, saTenant(t, "tenant-a"),
			claimKey(t, domain.TargetCustomerCompensation, "compensation-1"))
		if err != nil || !found {
			t.Fatalf("锁客户赔付：found=%v err=%v", found, err)
		}
		if compensation.AmountMinor() != 5000 || compensation.AppliedMinor() != 0 || compensation.Currency().String() != "USD" {
			t.Fatalf("客户赔付未核销面 = %s %d/%d", compensation.Currency(), compensation.AppliedMinor(), compensation.AmountMinor())
		}
		acknowledgement, found, err := rig.targets.LockClaimTarget(txCtx, saTenant(t, "tenant-a"),
			claimKey(t, domain.TargetRecoveryAcknowledgement, "acknowledgement-1"))
		if err != nil || !found {
			t.Fatalf("锁追偿认可：found=%v err=%v", found, err)
		}
		if acknowledgement.AmountMinor() != 6000 {
			t.Fatalf("追偿认可金额本体 = %d，要认可的 6000 而不是应追偿的 10000", acknowledgement.AmountMinor())
		}
		if _, found, err := rig.targets.LockClaimTarget(txCtx, saTenant(t, "tenant-c"),
			claimKey(t, domain.TargetCustomerCompensation, "compensation-1")); err != nil || found {
			t.Fatalf("别的租户锁到了这条赔付：found=%v err=%v", found, err)
		}
		return nil
	})
}

// Covers: 「核销余额按目标各自算」与 `AT-SA-113`「核销撤销恢复相应未结金额」——已核销只算指向这一条、尚未撤销的核销；
// 别的租户里同名目标上的核销不算进来；被指向的金额本体那一行一字不动。
func TestAClaimTargetStandingCountsOnlyLiveApplicationsOfItsOwn(t *testing.T) {
	rig := newClaimTargetRig(t)
	rig.seedClaimTargets(t)
	ctx := t.Context()
	key := claimKey(t, domain.TargetCustomerCompensation, "compensation-1")
	before := claimAmountRow(t, rig.pool, "tenant-a", "compensation-1")

	firstPrepared := compensationApplicationOf(t, "application-1", 3000)
	secondPrepared := compensationApplicationOf(t, "application-2", 1500)
	// 别的租户在同名的 compensation-1 上也核了一笔：tenant-a 的已核销不该把它算进来。
	foreignPrepared := compensationApplicationIn(t, "tenant-b", "application-foreign", 2000)
	saWithin(t, rig.transactor, ctx, func(txCtx context.Context) error {
		if err := rig.save(txCtx, firstPrepared); err != nil {
			return err
		}
		if err := rig.save(txCtx, foreignPrepared); err != nil {
			return err
		}
		return rig.save(txCtx, secondPrepared)
	})
	first := firstPrepared.application
	standingOf := func() domain.ClaimTargetStanding {
		var standing domain.ClaimTargetStanding
		saWithin(t, rig.transactor, ctx, func(txCtx context.Context) error {
			var found bool
			var err error
			standing, found, err = rig.targets.LockClaimTarget(txCtx, saTenant(t, "tenant-a"), key)
			if err == nil && !found {
				t.Fatal("锁不到在册的赔付")
			}
			return err
		})
		return standing
	}
	if standing := standingOf(); standing.AppliedMinor() != 4500 || standing.RemainingMinor() != 500 {
		t.Fatalf("两笔核销后已核销 / 未核销 = %d / %d，要 4500 / 500", standing.AppliedMinor(), standing.RemainingMinor())
	}

	reversed, err := first.Application.Reverse(saValue(t, domain.NewApplicationBasisReference, "payment-returned-1"), fundsReversedAt)
	if err != nil {
		t.Fatalf("撤销：%v", err)
	}
	first.Application = reversed
	saWithin(t, rig.transactor, ctx, func(txCtx context.Context) error {
		_, err := rig.applications.Replace(txCtx, first)
		return err
	})
	if standing := standingOf(); standing.AppliedMinor() != 1500 {
		t.Fatalf("撤销第一笔后已核销 = %d，要 1500", standing.AppliedMinor())
	}
	if after := claimAmountRow(t, rig.pool, "tenant-a", "compensation-1"); after != before {
		t.Fatalf("核销与撤销改了金额本体那一行：%+v → %+v", before, after)
	}
	if other := claimAmountRow(t, rig.pool, "tenant-b", "compensation-1"); other.amount != 5000 {
		t.Fatalf("别的租户的同名赔付被动了：%+v", other)
	}
}

// Covers: 锁与写同一事务——先到的事务锁住这条赔付、落一笔核销后才提交；后到的事务在锁上等（看 pg_stat_activity 认），
// 先到的提交后它读到的已核销含那一笔，而不是锁之前的快照。
func TestASecondLockOnAClaimTargetSeesTheFirstApplicationOnceCommitted(t *testing.T) {
	rig := newClaimTargetRig(t)
	rig.seedClaimTargets(t)
	ctx := t.Context()
	key := claimKey(t, domain.TargetCustomerCompensation, "compensation-1")
	tenant := saTenant(t, "tenant-a")

	prepared := compensationApplicationOf(t, "application-first", 4000)
	locked := make(chan struct{})
	release := make(chan struct{})
	// 断言失败时也要放开先到的事务：它不交回连接，用例收尾关池就一直等。清理后进先出，排在关池之前。
	var releaseOnce sync.Once
	releaseFirst := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseFirst)
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- rig.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			if _, _, err := rig.targets.LockClaimTarget(txCtx, tenant, key); err != nil {
				return err
			}
			close(locked)
			<-release
			return rig.save(txCtx, prepared)
		})
	}()
	select {
	case <-locked:
	case err := <-firstDone:
		t.Fatalf("先到的事务没锁上就收了：%v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("先到的事务十秒内没锁上")
	}

	secondDone := make(chan domain.ClaimTargetStanding, 1)
	secondErr := make(chan error, 1)
	go func() {
		var standing domain.ClaimTargetStanding
		err := rig.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var err error
			standing, _, err = rig.targets.LockClaimTarget(txCtx, tenant, key)
			return err
		})
		secondErr <- err
		secondDone <- standing
	}()

	// 池只有两条连接，两条事务各占一条；看锁走自己的连接，不然这一句排在池上永远等不到。
	watcher, err := pgx.ConnectConfig(ctx, rig.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("开看锁连接：%v", err)
	}
	t.Cleanup(func() { _ = watcher.Close(context.Background()) })
	deadline := time.Now().Add(10 * time.Second)
	for !waitingOnALock(t, watcher) {
		if time.Now().After(deadline) {
			t.Fatal("后到的事务十秒内没有在锁上等——锁没有落在金额本体那一行上")
		}
		time.Sleep(10 * time.Millisecond)
	}
	releaseFirst()
	if err := <-firstDone; err != nil {
		t.Fatalf("先到的事务：%v", err)
	}
	if err := <-secondErr; err != nil {
		t.Fatalf("后到的事务：%v", err)
	}
	if standing := <-secondDone; standing.AppliedMinor() != 4000 {
		t.Fatalf("后到的锁读到已核销 %d，要 4000——读的是锁之前的快照", standing.AppliedMinor())
	}
}

// Covers: PBC-08——锁口无环境事务即拒，不在自动提交里读一份不受锁保护的已核销。
func TestLockingAClaimTargetRefusesToRunOutsideATransaction(t *testing.T) {
	rig := newClaimTargetRig(t)
	rig.seedClaimTargets(t)
	_, _, err := rig.targets.LockClaimTarget(t.Context(), saTenant(t, "tenant-a"),
		claimKey(t, domain.TargetCustomerCompensation, "compensation-1"))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务锁金额本体：err = %v，要 ErrTransactionRequired", err)
	}
}

// Covers: 迁移放开映射目标 CHECK——两格新值进得去，词表外的值仍拒；对账单等既有四格照旧（存量行共存）。
func TestTheMappingTargetCheckAdmitsTheClaimAmountKindsOnly(t *testing.T) {
	rig := newClaimTargetRig(t)
	insert := func(id, kind string) error {
		_, err := rig.pool.Exec(t.Context(),
			`INSERT INTO settlement_accounting.funds_mapping
				(tenant_id, mapping_id, fact_id, target_kind, target_ref, basis, mapped_at, content_digest, recorded_at)
			 VALUES ('tenant-a', $1, 'f-1', $2, 't-1', 'b', now(), 'd', now())`, id, kind)
		return err
	}
	for _, kind := range []string{"STATEMENT", "PAYABLE", "CREDIT_NOTE", "ACCOUNT", "CUSTOMER_COMPENSATION", "RECOVERY_ACKNOWLEDGEMENT"} {
		if err := insert("m-"+kind, kind); err != nil {
			t.Fatalf("目标 %s 进不了映射库：%v", kind, err)
		}
	}
	for _, kind := range []string{"RECOVERY_RECEIVABLE", "CHARGE", "CLAIM_AMOUNT"} {
		if err := insert("m-bad-"+kind, kind); err == nil {
			t.Fatalf("词表外的目标 %s 溜进了映射库", kind)
		}
	}
}

// Covers: 两格目标的映射与核销经真库往返不变形——映射行与核销分配 jsonb 里的目标种类按同一张词表读回。
func TestMappingsAndApplicationsOnClaimTargetsRoundTrip(t *testing.T) {
	rig := newClaimTargetRig(t)
	ctx := t.Context()
	fact := adoptedFactRecord(t, "tenant-a", "bank-fact-roundtrip", domain.FundsReceiptConfirmed, 8000)
	compensation := mappingRecord(t, "tenant-a", "mapping-compensation", fact.Fact, domain.TargetCustomerCompensation, "compensation-1")
	recovery := mappingRecord(t, "tenant-a", "mapping-recovery", fact.Fact, domain.TargetRecoveryAcknowledgement, "acknowledgement-1")
	application, err := domain.ApplySettlement(fact.Fact, []domain.FundsMapping{compensation.Mapping, recovery.Mapping},
		[]domain.SettlementAllocation{
			{Mapping: compensation.Mapping.Mapping(), TargetKind: domain.TargetCustomerCompensation, Target: compensation.Mapping.Target(),
				Direction: domain.AllocationDebit, AmountMinor: 3000},
			{Mapping: recovery.Mapping.Mapping(), TargetKind: domain.TargetRecoveryAcknowledgement, Target: recovery.Mapping.Target(),
				Direction: domain.AllocationDebit, AmountMinor: 2000},
		},
		saValue(t, domain.NewApplicationReference, "application-roundtrip"),
		saValue(t, domain.NewApplicationBasisReference, "payment-instruction-1"),
		fundsAppliedAt)
	if err != nil {
		t.Fatalf("构造核销：%v", err)
	}
	record := ports.SettlementApplicationRecord{
		Key:           ports.SettlementApplicationKey{TenantID: saTenant(t, "tenant-a"), Application: application.Application()},
		ContentDigest: "digest-roundtrip", Application: application, RecordedAt: fundsAppliedAt,
	}
	saWithin(t, rig.transactor, ctx, func(txCtx context.Context) error {
		if _, err := rig.facts.Save(txCtx, fact); err != nil {
			return err
		}
		for _, mapping := range []ports.FundsMappingRecord{compensation, recovery} {
			if _, err := rig.mappings.Save(txCtx, mapping); err != nil {
				return err
			}
		}
		_, err := rig.applications.Save(txCtx, record)
		return err
	})
	for _, mapping := range []ports.FundsMappingRecord{compensation, recovery} {
		found, exists, err := rig.mappings.FindByKey(ctx, mapping.Key)
		if err != nil || !exists || found.Mapping.TargetKind() != mapping.Mapping.TargetKind() || found.Mapping.Target() != mapping.Mapping.Target() {
			t.Fatalf("映射 %s 读回 = %+v（exists=%v err=%v）", mapping.Key.Mapping, found.Mapping, exists, err)
		}
	}
	found, exists, err := rig.applications.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("核销读回：exists=%v err=%v", exists, err)
	}
	if got := found.Application.ClaimTargetDemands(); len(got) != 2 ||
		got[0].Key.Kind != domain.TargetCustomerCompensation || got[1].Key.Kind != domain.TargetRecoveryAcknowledgement {
		t.Fatalf("读回的分配 = %+v", got)
	}
}

type claimAmountSnapshot struct {
	amount     int64
	currency   string
	digest     string
	recordedAt time.Time
}

func claimAmountRow(t *testing.T, pool *pgxpool.Pool, tenant, id string) claimAmountSnapshot {
	t.Helper()
	var row claimAmountSnapshot
	if err := pool.QueryRow(t.Context(),
		`SELECT amount_minor, currency, content_digest, recorded_at
		   FROM settlement_accounting.customer_claim_amount WHERE tenant_id = $1 AND amount_id = $2`,
		tenant, id).Scan(&row.amount, &row.currency, &row.digest, &row.recordedAt); err != nil {
		t.Fatalf("直读金额本体：%v", err)
	}
	return row
}

// waitingOnALock 只看本测试库：pgtest 给每个用例一个物理库，别的用例的等待不该算进来。
func waitingOnALock(t *testing.T, watcher *pgx.Conn) bool {
	t.Helper()
	var waiting int
	if err := watcher.QueryRow(t.Context(),
		`SELECT count(*) FROM pg_stat_activity
		  WHERE wait_event_type = 'Lock' AND datname = current_database()`).Scan(&waiting); err != nil {
		t.Fatalf("看锁：%v", err)
	}
	return waiting > 0
}
