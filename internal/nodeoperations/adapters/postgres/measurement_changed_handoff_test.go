package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

type measurementNoticeClock struct{ at time.Time }

func (clock measurementNoticeClock) Now() time.Time { return clock.at }

func TestMeasurementChangeNoticeRefusesToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox：%v", err)
	}
	notice, err := adapter.NewOutboxMeasurementChangeNotice(db, store, measurementNoticeClock{at: time.Now()})
	if err != nil {
		t.Fatalf("构造测量变化通知：%v", err)
	}
	if err := notice.NoticeMeasurementChanged(context.Background(), ports.MeasurementChanged{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务通知应返回 ErrTransactionRequired，实得：%v", err)
	}
}
