package postgres_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证价卡内容读口（ports.PriceCardContentRead）：登记一张卡再读回，交出的规范化文档与
// 登记的那一份语义逐位相同（列里是 jsonb，键序不保留，所以比的是解出来的值，不是字节）；元数据照登转写；别的租户、
// 未登过的版本同答「没有」；被改过的快照过不了整图重验、不被当成「这张卡登记的内容」交出。

var demoPriceCardSeeds = []string{
	"price-card-cn-sg.json",
	"price-card-cn-sg-cost.json",
	"price-card-cn-sg-full.json",
	"price-card-cn-sg-full-02.json",
	"price-card-cn-sg-full-03.json",
}

// demoSeedCard 读演示种子文件，返回整份登记、种子里 plan 那一块的解析值。种子就是登记快照的形状，
// 所以拿它既立一张真卡，又当「登记的文档」的参照。
func demoSeedCard(t *testing.T, name string) (domain.PriceCardRegistration, any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "scripts", "demo-seeds", "data", "pricing", name))
	if err != nil {
		t.Fatalf("读演示种子 %s：%v", name, err)
	}
	registration, err := domain.RehydratePriceCardRegistration(raw)
	if err != nil {
		t.Fatalf("重建演示价卡 %s：%v", name, err)
	}
	var seed struct {
		Plan any `json:"plan"`
	}
	if err := json.Unmarshal(raw, &seed); err != nil {
		t.Fatalf("解析种子 %s：%v", name, err)
	}
	return registration, seed.Plan
}

func newPriceCardsWithPool(t *testing.T) (*adapter.PriceCards, *bentopg.DB, func(string, ...any)) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalog, err := adapter.NewPriceCards(db)
	if err != nil {
		t.Fatalf("构造价卡仓储：%v", err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatalf("改库：%v", err)
		}
	}
	return catalog, db, exec
}

// Covers: 答出文档逐字等于登记的文档——五张演示种子卡（含 PPC-5 与 PPC-6、最低收费、偏远查表）各登一遍再读，
// 交出的 document 与种子 plan 块语义逐位相同；元数据（规范化版本、内容摘要、登记责任方）照登转写。
func TestReadPriceCardContentAnswersTheRegisteredDocument(t *testing.T) {
	catalog, db, _ := newPriceCardsWithPool(t)
	ctx := t.Context()

	for _, name := range demoPriceCardSeeds {
		registration, seedPlan := demoSeedCard(t, name)
		if outcome := registerCard(t, catalog, db.Transactor(), ctx, registration); outcome != ports.PriceCardRegistered {
			t.Fatalf("%s 首登 outcome = %d", name, outcome)
		}

		content, found, err := catalog.ReadPriceCardContent(ctx, registration.Tenant(), registration.Plan().Reference())
		if err != nil || !found {
			t.Fatalf("%s 读回：found=%v err=%v", name, found, err)
		}
		var served any
		if err := json.Unmarshal(content.Document, &served); err != nil {
			t.Fatalf("%s 交出的 document 不是 JSON：%v", name, err)
		}
		if !reflect.DeepEqual(served, seedPlan) {
			t.Fatalf("%s 交出的文档与登记的不同：\n got %s", name, content.Document)
		}
		plan := registration.Plan()
		if content.PlanID != plan.Reference().ID() || content.PlanVersion != plan.Reference().Version() ||
			content.Canonicalization != plan.CanonicalizationVersion() || content.ContentDigest != plan.ContentDigest() ||
			content.PublicationApprover != registration.PublicationApprover() || content.RegisteredAt.IsZero() {
			t.Fatalf("%s 元数据 = %+v", name, content)
		}
	}
}

// Covers: 租户隔离与查无——别的租户读同一引用、未登过的版本都答没有（false / nil），越权探针与真不存在不可分辨；
// 引用种类不是 pricing-plan 是调用方的错，答 error。
func TestReadPriceCardContentAnswersAbsenceForOtherTenantsAndUnregisteredVersions(t *testing.T) {
	catalog, db, _ := newPriceCardsWithPool(t)
	ctx := t.Context()
	registration, _ := demoSeedCard(t, "price-card-cn-sg-full-03.json")
	registerCard(t, catalog, db.Transactor(), ctx, registration)

	stranger := evaluationValue(t, domain.NewTenantID, "SYN-TENANT-OTHER")
	if _, found, err := catalog.ReadPriceCardContent(ctx, stranger, registration.Plan().Reference()); err != nil || found {
		t.Fatalf("别的租户读同一引用：found=%v err=%v，想要 false / nil", found, err)
	}
	unregistered, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, registration.Plan().Reference().ID(), "v9")
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := catalog.ReadPriceCardContent(ctx, registration.Tenant(), unregistered); err != nil || found {
		t.Fatalf("未登过的版本号：found=%v err=%v，想要 false / nil", found, err)
	}
	wrongKind, err := domain.NewVersionReferenceIdentity(domain.ArtifactRateTable, registration.Plan().Reference().ID(), "v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := catalog.ReadPriceCardContent(ctx, registration.Tenant(), wrongKind); err == nil {
		t.Fatal("价表种类的引用被当成方案引用读了")
	}
}

// Covers: 读回经整图重验——快照被改过（内容与摘要分岔）答 error，不把一份可能漂离了列面的快照当成「这张卡登记的
// 内容」交给运营；快照按本构建不认的规范化版本折装则答包着 ErrCanonicalizationVersionUnsupported 的 error，
// 与「没有」可分（端点据此另立一格）。
func TestReadPriceCardContentRefusesATamperedOrUnreadableSnapshot(t *testing.T) {
	catalog, db, exec := newPriceCardsWithPool(t)
	ctx := t.Context()
	registration, _ := demoSeedCard(t, "price-card-cn-sg-full-03.json")
	registerCard(t, catalog, db.Transactor(), ctx, registration)
	reference := registration.Plan().Reference()

	exec(`UPDATE parcel_pricing.price_card_version
	         SET snapshot = jsonb_set(snapshot, '{plan,totalChargeBounds,minimum,amount,coefficient}', '"99999"')
	       WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3`,
		registration.Tenant().String(), reference.ID(), reference.Version())
	if _, found, err := catalog.ReadPriceCardContent(ctx, registration.Tenant(), reference); err == nil || found {
		t.Fatalf("被改过的快照被交出了：found=%v err=%v", found, err)
	} else if errors.Is(err, domain.ErrCanonicalizationVersionUnsupported) {
		t.Fatalf("内容被改应是整图重验不过，不是规范化版本不认：%v", err)
	}

	exec(`UPDATE parcel_pricing.price_card_version
	         SET snapshot = jsonb_set(snapshot, '{plan,canonicalization}', '"PPC-99"')
	       WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3`,
		registration.Tenant().String(), reference.ID(), reference.Version())
	if _, found, err := catalog.ReadPriceCardContent(ctx, registration.Tenant(), reference); found || !errors.Is(err, domain.ErrCanonicalizationVersionUnsupported) {
		t.Fatalf("不认的规范化版本：found=%v err=%v，想要 ErrCanonicalizationVersionUnsupported", found, err)
	}
}
