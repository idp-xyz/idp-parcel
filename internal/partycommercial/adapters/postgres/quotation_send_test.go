package postgres_test

import (
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func TestQuotationSendWritesRefuseToRunOutsideATransaction(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := adapter.NewQuotationSends(db)
	if err != nil {
		t.Fatalf("构造发出记录册：%v", err)
	}
	if _, err := store.AppendQuotationSend(t.Context(), domain.QuotationSend{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务追加发出记录应返回 ErrTransactionRequired，实得：%v", err)
	}
}
