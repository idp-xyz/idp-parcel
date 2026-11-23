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

func TestQuotationReplyWritesRefuseToRunOutsideATransaction(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := adapter.NewQuotationReplies(db)
	if err != nil {
		t.Fatalf("构造答复册：%v", err)
	}
	if _, err := store.AppendQuotationReply(t.Context(), domain.QuotationReply{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务追加答复应返回 ErrTransactionRequired，实得：%v", err)
	}
}
