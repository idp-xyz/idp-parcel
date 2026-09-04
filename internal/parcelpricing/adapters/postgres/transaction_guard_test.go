package postgres_test

import (
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// PBC-08 行为面负向证据（bento-gate-reeval 票 02）：写口在无事务上下文必须被
// RequireExecutor 拒绝。拒绝先于任何入参解读，所以传零值就够；若有人把入参校验挪到
// 守卫之前，断言会以「错误不是 ErrTransactionRequired」如实变红。
func TestPricingWritesRefuseToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()

	evaluations, err := adapter.NewEvaluations(db)
	if err != nil {
		t.Fatalf("构造评价库：%v", err)
	}
	if _, err := evaluations.Save(ctx, domain.PricingEvaluation{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存评价应返回 ErrTransactionRequired，实得：%v", err)
	}

	cards, err := adapter.NewPriceCards(db)
	if err != nil {
		t.Fatalf("构造价卡目录：%v", err)
	}
	if _, err := cards.Register(ctx, domain.PriceCardRegistration{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记价卡应返回 ErrTransactionRequired，实得：%v", err)
	}

	series, err := adapter.NewReferenceSeriesVersions(db)
	if err != nil {
		t.Fatalf("构造参考序列登记册：%v", err)
	}
	if _, err := series.Register(ctx, domain.ReferenceSeriesRegistration{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记参考序列应返回 ErrTransactionRequired，实得：%v", err)
	}

	reviews, err := adapter.NewReferenceSeriesReviews(db)
	if err != nil {
		t.Fatalf("构造复核册：%v", err)
	}
	if _, err := reviews.Record(ctx, domain.SeriesReview{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务记录序列复核应返回 ErrTransactionRequired，实得：%v", err)
	}

	catalogues, err := adapter.NewReferenceCatalogueVersions(db)
	if err != nil {
		t.Fatalf("构造目录登记册：%v", err)
	}
	if _, err := catalogues.Register(ctx, domain.ReferenceCatalogueRegistration{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记参考目录应返回 ErrTransactionRequired，实得：%v", err)
	}

	catalogueReviews, err := adapter.NewReferenceCatalogueReviews(db)
	if err != nil {
		t.Fatalf("构造目录复核册：%v", err)
	}
	if _, err := catalogueReviews.Record(ctx, domain.CatalogueReview{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务记录目录复核应返回 ErrTransactionRequired，实得：%v", err)
	}
}
