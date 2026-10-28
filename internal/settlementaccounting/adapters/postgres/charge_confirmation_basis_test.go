package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"

	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
)

// 钉 ADR-0336 落在库面的三口：确认条件目录的受控登记分重放与冲突，确认依据按（费用、费用项目、种类）记一次、后到的同种
// 依据不改先到的引用、记下之后条件核对答已满足，结算账户按标识整户读回。真库，合成串。

// Covers: AT-SA-220——同费用项目同种重放、异种冲突不覆盖。
func TestTheConfirmationConditionRegisterSeparatesReplayFromConflict(t *testing.T) {
	db := catalogueDB(t)
	conditions, err := adapter.NewChargeConfirmationConditions(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := registerTenant(t)
	feeItem := mustAccountRef(t, domain.NewFeeItemReference, "SYN-FEE-FREIGHT")
	final := domain.EffectiveDeliveryFinalizedBasis()
	other := mustAccountRef(t, domain.NewConfirmationBasisKind, "SYN-OTHER-BASIS")
	at := time.Date(2026, 10, 28, 3, 0, 0, 0, time.UTC)

	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		for _, step := range []struct {
			kind domain.ConfirmationBasisKind
			want ports.CatalogueRegistrationEffect
		}{{final, ports.CatalogueRegistered}, {final, ports.CatalogueReplay}, {other, ports.CatalogueConflict}} {
			if effect, err := conditions.SaveConfirmationCondition(ctx, tenant, feeItem, step.kind, at); err != nil || effect != step.want {
				return errOrEffect(err, effect, step.want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// 冲突那一次没改册上那一行：依据没到时缺口点名的仍是先登的种类。
	condition, configured, err := conditions.LoadConfirmationCondition(t.Context(), tenant,
		mustAccountRef(t, domain.NewCustomerChargeID, "SYN-CHARGE-1"), feeItem)
	if err != nil || !configured || condition.Met || condition.Gap != adapter.ConfirmationBasisAbsent+final.String() {
		t.Fatalf("条件 = %+v configured=%v err=%v", condition, configured, err)
	}
}

// Covers: AT-SA-218——依据记一次，后到的同种依据不改先到的引用，条件核对随之答已满足。
func TestARecordedBasisMeetsTheConditionOnceAndKeepsTheFirstReference(t *testing.T) {
	db := catalogueDB(t)
	conditions, err := adapter.NewChargeConfirmationConditions(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := registerTenant(t)
	charge := mustAccountRef(t, domain.NewCustomerChargeID, "SYN-CHARGE-1")
	feeItem := mustAccountRef(t, domain.NewFeeItemReference, "SYN-FEE-FREIGHT")
	kind := domain.EffectiveDeliveryFinalizedBasis()
	first := mustAccountRef(t, domain.NewConfirmationBasisReference, "final-outcome/SYN-PARCEL-1/EFFECTIVE_DELIVERY/V1")
	later := mustAccountRef(t, domain.NewConfirmationBasisReference, "final-outcome/SYN-PARCEL-1/EFFECTIVE_DELIVERY/V2")
	at := time.Date(2026, 10, 28, 3, 0, 0, 0, time.UTC)

	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if effect, err := conditions.SaveConfirmationCondition(ctx, tenant, feeItem, kind, at); err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		if effect, err := conditions.RecordConfirmationBasis(ctx, tenant, charge, feeItem, kind, first, at); err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		if effect, err := conditions.RecordConfirmationBasis(ctx, tenant, charge, feeItem, kind, later, at); err != nil || effect != ports.CatalogueReplay {
			return errOrEffect(err, effect, ports.CatalogueReplay)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	condition, configured, err := conditions.LoadConfirmationCondition(t.Context(), tenant, charge, feeItem)
	if err != nil || !configured || !condition.Met || condition.Basis.String() != first.String() {
		t.Fatalf("条件 = %+v configured=%v err=%v，want 已满足、依据是先到的那份", condition, configured, err)
	}
}

// Covers: AT-SA-217——确认事实要按请求指名的账户标识整户读回。
func TestASettlementAccountReadsBackByID(t *testing.T) {
	db := catalogueDB(t)
	accounts, err := adapter.NewSettlementAccounts(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := settlementTenant(t)
	account := settlementAccountOutsideATransaction(t)

	if _, found, err := accounts.FindByID(t.Context(), tenant, account.ID()); err != nil || found {
		t.Fatalf("空册 found=%v err=%v", found, err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := accounts.Save(ctx, tenant, account, time.Now().UTC())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := accounts.FindByID(t.Context(), tenant, account.ID())
	if err != nil || !found || !loaded.SameRegistration(account) {
		t.Fatalf("读回 found=%v err=%v", found, err)
	}
	if loaded.Responsibility().String() != "CONTRACT-1" || loaded.Key().LegalEntity().String() != "LE-1" {
		t.Fatalf("读回 = 成立依据 %s 法人 %s", loaded.Responsibility(), loaded.Key().LegalEntity())
	}
}

// Covers: AT-SA-218、AT-SA-219——包裹确认锁让同一件包裹的两头排队：一笔持锁未提交时，另一笔取同一件包裹的锁要等；别的包裹
// 不等；持锁那一笔提交之后取得到。
func TestTheParcelConfirmationLockQueuesTheSameParcelOnly(t *testing.T) {
	db := catalogueDB(t)
	locks, err := adapter.NewParcelConfirmationLocks(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := registerTenant(t)
	parcel := mustAccountRef(t, domain.NewParcelReference, "SYN-PARCEL-LOCK-1")
	other := mustAccountRef(t, domain.NewParcelReference, "SYN-PARCEL-LOCK-2")

	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- db.Transactor().WithinTransaction(context.Background(), func(ctx context.Context) error {
			if err := locks.SerializeParcelConfirmation(ctx, tenant, parcel); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		return locks.SerializeParcelConfirmation(ctx, tenant, other)
	}); err != nil {
		t.Fatalf("别的包裹也等了：%v", err)
	}
	waiting, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	err = db.Transactor().WithinTransaction(waiting, func(ctx context.Context) error {
		return locks.SerializeParcelConfirmation(ctx, tenant, parcel)
	})
	cancel()
	if err == nil {
		t.Fatal("同一件包裹在对方持锁时取到了锁")
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		return locks.SerializeParcelConfirmation(ctx, tenant, parcel)
	}); err != nil {
		t.Fatalf("对方提交之后仍取不到：%v", err)
	}
}

// 包裹确认锁是事务级锁，事务外取等于没锁：按框架合同拒。
func TestTheParcelConfirmationLockRefusesToRunOutsideATransaction(t *testing.T) {
	locks, err := adapter.NewParcelConfirmationLocks(catalogueDB(t))
	if err != nil {
		t.Fatal(err)
	}
	err = locks.SerializeParcelConfirmation(t.Context(), registerTenant(t), mustAccountRef(t, domain.NewParcelReference, "SYN-PARCEL-LOCK-1"))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外取锁应答 ErrTransactionRequired，实得：%v", err)
	}
}
