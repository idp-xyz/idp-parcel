package main

import (
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
)

// pricingParameterHandoffForTest 装计价的「参数已登记」交接口，供用例照生产登记口的装法登卡与复核：价卡登记与复核用例缺了
// 它就不受理（ADR-0229 决定五），而登卡的那几条用例要的是生产的那一条登记路。
func pricingParameterHandoffForTest(t *testing.T, db *bentopg.DB) *pppostgres.OutboxEvaluationResumptionHandoffs {
	t.Helper()
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := pppostgres.NewOutboxEvaluationResumptionHandoffs(db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造参数已登记交接口：%v", err)
	}
	return handoff
}

// pricedScopesForTest 装序列与目录复核问「绑了它的卡在哪些范围上」的读口，同生产装法：卡册本身。
func pricedScopesForTest(t *testing.T, db *bentopg.DB) *pppostgres.PriceCards {
	t.Helper()
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		t.Fatalf("构造价卡库：%v", err)
	}
	return cards
}
