package postgres_test

import (
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Covers: AT-WF-014（商品的核心）——一笔带别名、申报与声明重量的修订落册后读回是同一份请求；数值 0.50 与 5.50 按数值读回。
func TestGoodsRegistryRoundTripsARevision(t *testing.T) {
	stores := newGoodsStores(t)
	revision := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 1, "REQ-1", nil)
	if result := saveGoods(t, stores, revision); result.Outcome != ports.GoodsRevisionSaved {
		t.Fatalf("保存 = %s", result.Outcome)
	}
	loaded, found, err := stores.goods.LoadLatestGoods(t.Context(), revision.Tenant(), revision.Owner(), revision.Code())
	if err != nil || !found {
		t.Fatalf("读回 %v %v", found, err)
	}
	if !loaded.Request().SameAs(revision.Request()) || loaded.Registrant() != revision.Registrant() || !loaded.RegisteredAt().Equal(revision.RegisteredAt()) {
		t.Fatalf("读回的请求与落册的不是同一份")
	}
	if price, currency, has := loaded.Content().Declaration().UnitPrice(); !has || price.String() != "5.5" || currency.String() != "USD" {
		t.Fatalf("申报单价读回 %s %s（%v），want 5.5 USD", price, currency, has)
	}
	if weight, has := loaded.Content().DeclaredMeasurements()[0].Weight(); !has || weight.Value().String() != "0.5" {
		t.Fatalf("声明重量读回 %s（%v），want 0.5", weight.Value(), has)
	}
}

func TestGoodsRegistryRoundTripsAnOnBehalfRevision(t *testing.T) {
	stores := newGoodsStores(t)
	submission, err := domain.NewOnBehalfSubmission(
		valueOf(t, domain.NewRequestKey, "DEC-1"), valueOf(t, domain.NewEvidenceReference, "AUTH-1"))
	if err != nil {
		t.Fatal(err)
	}
	revision, err := domain.NewGoodsRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), 1, goodsContentFixture(t, nil), submission,
		valueOf(t, domain.NewRegistrant, "operator-1"), baseInstant)
	if err != nil {
		t.Fatal(err)
	}
	if result := saveGoods(t, stores, revision); result.Outcome != ports.GoodsRevisionSaved {
		t.Fatalf("代录 = %s", result.Outcome)
	}
	loaded, found, err := stores.goods.LoadGoodsRevisionBySubmission(t.Context(), revision.Tenant(), submission)
	if err != nil || !found || !loaded.Request().SameAs(revision.Request()) || loaded.Submission().Path() != domain.SubmissionOnBehalf {
		t.Fatalf("代录读回 %v %v", found, err)
	}
	if evidence, has := loaded.Submission().EvidenceReference(); !has || evidence.String() != "AUTH-1" {
		t.Fatalf("依据引用 = %q（%v）", evidence, has)
	}
	if _, has := loaded.Submission().Source(); has {
		t.Fatal("代录不该有来源")
	}
}

func TestGoodsRegistryAnswersReplayAndConflictForOneSubmissionKey(t *testing.T) {
	stores := newGoodsStores(t)
	first := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 1, "REQ-1", nil)
	if result := saveGoods(t, stores, first); result.Outcome != ports.GoodsRevisionSaved {
		t.Fatalf("首次 = %s", result.Outcome)
	}
	replay, err := domain.NewGoodsRevision(
		first.Tenant(), first.Owner(), first.Code(), 1, first.Content(), first.Submission(),
		valueOf(t, domain.NewRegistrant, "operator-2"), baseInstant.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if result := saveGoods(t, stores, replay); result.Outcome != ports.GoodsRevisionAlreadyRegistered {
		t.Fatalf("重放 = %s", result.Outcome)
	}
	other := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 1, "REQ-1", func(spec *domain.GoodsContentSpec) {
		spec.Name = valueOf(t, domain.NewGoodsName, "另一份")
	})
	if result := saveGoods(t, stores, other); result.Outcome != ports.GoodsSourceConflict {
		t.Fatalf("换内容 = %s", result.Outcome)
	}
	loaded, found, err := stores.goods.LoadGoodsRevisionBySubmission(t.Context(), first.Tenant(), first.Submission())
	if err != nil || !found || loaded.Content().Name().String() != "合成商品" || loaded.Registrant().String() != "operator-1" {
		t.Fatalf("在册的被改了：%q %q %v %v", loaded.Content().Name(), loaded.Registrant(), found, err)
	}
}

func TestGoodsRegistryRejectsARevisionThatDoesNotContinueTheSequence(t *testing.T) {
	stores := newGoodsStores(t)
	skipped := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 2, "REQ-2", nil)
	if result := saveGoods(t, stores, skipped); result.Outcome != ports.GoodsBasisStale {
		t.Fatalf("跳号 = %s", result.Outcome)
	}
	first := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 1, "REQ-1", nil)
	second := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 2, "REQ-2", func(spec *domain.GoodsContentSpec) {
		spec.Name = valueOf(t, domain.NewGoodsName, "第二笔")
	})
	if result := saveGoods(t, stores, first); result.Outcome != ports.GoodsRevisionSaved {
		t.Fatalf("修订 1 = %s", result.Outcome)
	}
	if result := saveGoods(t, stores, second); result.Outcome != ports.GoodsRevisionSaved {
		t.Fatalf("修订 2 = %s", result.Outcome)
	}
}

func TestGoodsRegistryRejectsAnOverlappingIdentifierAndAllowsTheOthers(t *testing.T) {
	stores := newGoodsStores(t)
	held := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-OTHER", 1, "REQ-OTHER", nil)
	if result := saveGoods(t, stores, held); result.Outcome != ports.GoodsRevisionSaved {
		t.Fatalf("占码的商品 = %s", result.Outcome)
	}
	collision := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 1, "REQ-1", func(spec *domain.GoodsContentSpec) {
		spec.Aliases = []domain.GoodsAlias{domain.NewGoodsAlias(
			valueOf(t, domain.NewGoodsAliasCode, "SYN-OTHER"), domain.GoodsAliasBarcode, domain.PlatformName{}, domain.AliasValidity{},
		)}
	})
	result := saveGoods(t, stores, collision)
	if result.Outcome != ports.GoodsIdentifierConflict || result.ConflictingCode.String() != "SYN-OTHER" {
		t.Fatalf("撞码 = %s %q", result.Outcome, result.ConflictingCode)
	}
	touching := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 1, "REQ-1", func(spec *domain.GoodsContentSpec) {
		spec.Aliases = []domain.GoodsAlias{domain.NewGoodsAlias(
			valueOf(t, domain.NewGoodsAliasCode, "SYN-BAR-01"), domain.GoodsAliasBarcode, domain.PlatformName{},
			domain.NewAliasValidity(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Time{}),
		)}
	})
	if result := saveGoods(t, stores, touching); result.Outcome != ports.GoodsRevisionSaved {
		t.Fatalf("区间首尾相接 = %s，want SAVED", result.Outcome)
	}
	otherOwner := goodsRevisionFixture(t, "SYN-OWNER-02", "SYN-OTHER", 1, "REQ-OWNER-2", nil)
	if result := saveGoods(t, stores, otherOwner); result.Outcome != ports.GoodsRevisionSaved {
		t.Fatalf("别的货主 = %s", result.Outcome)
	}
}

func TestGoodsRegistryRefusesASaveOutsideATransaction(t *testing.T) {
	stores := newGoodsStores(t)
	_, err := stores.goods.SaveGoodsRevision(t.Context(), goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 1, "REQ-1", nil))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务 SaveGoodsRevision 应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestGoodsRevisionCheckRejectsAZeroDeclaredPrice(t *testing.T) {
	stores := newGoodsStores(t)
	_, err := stores.pool.Exec(t.Context(),
		`INSERT INTO warehouse_fulfillment.goods_revision (
		    tenant_id, owner_account_id, goods_code, revision, goods_name, base_unit,
		    track_batch, track_expiry, composite, submission_path, submission_source, request_key,
		    registrant, registered_at, declared_unit_price, declared_currency)
		 VALUES ('SYN-TENANT-01','SYN-OWNER-01','SYN-GOODS-01',1,'合成商品','件',
		         false,false,false,'OWNER','SYN-CHANNEL-01','REQ-ZERO',
		         'operator-1',$1, 0, 'USD')`, baseInstant)
	if err == nil {
		t.Fatal("申报单价为零应被 CHECK 拒绝")
	}
}
