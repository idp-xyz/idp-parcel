package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

func identityKey(t *testing.T, tenant, sourceID string) ports.ActualMeasurementKey {
	t.Helper()
	return ports.ActualMeasurementKey{TenantID: ref(t, domain.NewTenantID, tenant), SourceID: sourceID}
}

// 来源身份锁是事务级锁，事务外取等于没锁：按框架合同拒。
func TestTheMeasurementSourceIdentityLockRefusesToRunOutsideATransaction(t *testing.T) {
	measurements, _ := newActualMeasurements(t)
	err := measurements.SerializeSourceIdentity(context.Background(), identityKey(t, "tenant-a", "same"))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外取锁应答 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: 测量与作废共用的来源身份锁只让同一（租户，来源身份）排队：一笔持锁未提交时，另一笔取同一来源身份的锁要等；
// 同租户的别的来源身份、别的租户的同一来源身份都不等；持锁那一笔提交之后取得到。
func TestTheMeasurementSourceIdentityLockQueuesTheSameIdentityOnly(t *testing.T) {
	measurements, transactor := newActualMeasurements(t)
	held := identityKey(t, "tenant-a", "same")

	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- transactor.WithinTransaction(context.Background(), func(ctx context.Context) error {
			if err := measurements.SerializeSourceIdentity(ctx, held); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked

	for name, key := range map[string]ports.ActualMeasurementKey{
		"同租户别的来源身份":  identityKey(t, "tenant-a", "other"),
		"别的租户同一来源身份": identityKey(t, "tenant-b", "same"),
	} {
		if err := transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			return measurements.SerializeSourceIdentity(ctx, key)
		}); err != nil {
			t.Fatalf("%s也等了：%v", name, err)
		}
	}
	waiting, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	err := transactor.WithinTransaction(waiting, func(ctx context.Context) error {
		return measurements.SerializeSourceIdentity(ctx, held)
	})
	cancel()
	if err == nil {
		t.Fatal("同一来源身份在对方持锁时取到了锁")
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		return measurements.SerializeSourceIdentity(ctx, held)
	}); err != nil {
		t.Fatalf("对方提交之后仍取不到：%v", err)
	}
}
