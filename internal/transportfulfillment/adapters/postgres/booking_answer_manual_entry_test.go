package postgres_test

import (
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: 人工录入的承运答复留痕只在事务里写——它与那一份答复同笔，答复回滚时这一条也不该留下。
func TestManualBookingAnswerEntriesRefuseToRunOutsideATransaction(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	entries, err := adapter.NewBookingAnswerManualEntries(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := entries.RecordManualBookingAnswer(t.Context(), ports.ManualBookingAnswer{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外记人工录入留痕：%v，want ErrTransactionRequired", err)
	}
}
