package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证服务产品形态册（ADR-0050）：形态随整册一次取回并挂回自己的
// 版本、只登版本不登形态是一格合法的缺席、版本非生效时形态不进视图、登记形态推动该范围的
// ViewRevision、撞键按形态判重放、他租形态不入本租户的册。
//
// 两条防御分支在今天的封闭集下**够不着，因此没有用例**：`内容冲突`（同一产品版本登记成另一
// 种形态）与「形态取值不认识」。两者都要求库里出现 NETWORK_SERVICE 之外的取值，而迁移 0008
// 的 CHECK 与 domain.ServiceProductForm 今天各自只有这一个值。它们不是死代码——它们正是这
// 一对（SQL CHECK 与 Go 封闭集，两份要同步扩展的东西）走散时唯一会报的地方；`PAR-COM-12`
// 解封、第二种形态落地那一天，两条同时变得够得着，届时补用例。

func TestServiceProductFormRoundTripsWithTheRegistry(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := productVersionInTenant(t, "tenant-1", "product-1", "v1", "digest-product-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	mustSaveServiceProduct(t, transactor, ctx, repository, serviceProductOf(t, version))

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}

	products := registry.ServiceProducts()
	if len(products) != 1 {
		t.Fatalf("读回 %d 份产品形态，want 1", len(products))
	}
	if products[0].Form() != domain.NetworkServiceForm {
		t.Fatalf("form = %q", products[0].Form())
	}
	if !products[0].Version().SameVersionAs(version) {
		t.Fatal("形态挂回了另一个版本")
	}
}

// Covers: ADR-0050 决定四「产品缺席不使解析从`唯一解析`退化为`无适用依据`」。
//
// 只登版本、不登形态是一格**合法的缺席**：版本照常入册参与选择，形态不可观察而已。它因此
// 不需要一个「未配置」标记列来表达，也不该让装载报错——这正是形态册有意不设第三种取值的
// 理由，用例把它钉住。
func TestAProductVersionWithoutARegisteredFormStillLoads(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := productVersionInTenant(t, "tenant-1", "product-1", "v1", "digest-product-1")
	mustSaveVersion(t, transactor, ctx, repository, version)

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	if registry.Count() != 1 {
		t.Fatalf("版本数 = %d, want 1", registry.Count())
	}
	if products := registry.ServiceProducts(); len(products) != 0 {
		t.Fatalf("没登记形态却读回 %d 份产品", len(products))
	}
}

// Covers: `CommercialAuthority` 注释「把它们漏在外面会让 ViewRevision 按不完整的内容派生，
// 从而在视图其实已经变了的时候答『还是同一个视图』」——装配点在装载侧。
//
// 形态从缺席变为在场必须推动该范围的修订，否则一次带产品的重解会被认成原解析（AT-PC-024）。
// 这条用例守的是「新册确实接进了 ViewRevision 的派生」，而不只是「新册能读回来」。
func TestRegisteringAFormMovesTheScopeViewRevision(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()
	tenant, scope := pcTenant(t, "tenant-1"), pcScope(t)

	version := productVersionInTenant(t, "tenant-1", "product-1", "v1", "digest-product-1")
	mustSaveVersion(t, transactor, ctx, repository, version)

	beforeRegistry, err := repository.LoadForScope(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("登记形态前读回：%v", err)
	}
	before := beforeRegistry.ViewRevision(tenant, scope)

	mustSaveServiceProduct(t, transactor, ctx, repository, serviceProductOf(t, version))

	afterRegistry, err := repository.LoadForScope(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("登记形态后读回：%v", err)
	}
	if afterRegistry.ViewRevision(tenant, scope) == before {
		t.Fatal("形态从缺席变为在场，范围修订却没动——装载没把形态册接进派生")
	}
}

// Covers: 形态行在册、但它的版本不再`已生效`时不进登记册（本切片设计的装载纪律）。
//
// 跳过是安全的，因为解析选候选本来就跳过非生效版本；而它不会让失效检测漏掉任何东西——版本
// 状态本身参与 ViewRevision 派生，生效与收尾这两次转变照样推动修订。
//
// 用直接 SQL 造这个状态：SaveVersion 只增不改，登记册今天没有把一份已入册版本推进到下一个
// 生命周期位置的写入路径，所以这一格在库上真实存在、却造不出来。
func TestAFormWhoseVersionIsNotEffectiveStaysOutOfTheRegistry(t *testing.T) {
	repository, transactor, pool := newPublications(t)
	ctx := t.Context()

	version := publishedProductVersion(t, "product-1", "v1", "digest-product-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	insertFormRow(t, pool, ctx, "tenant-1", "product-1", "v1", "NETWORK_SERVICE")

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	if registry.Count() != 1 {
		t.Fatalf("版本数 = %d, want 1", registry.Count())
	}
	if products := registry.ServiceProducts(); len(products) != 0 {
		t.Fatalf("尚未生效版本的形态进了登记册：%d 份", len(products))
	}
}

// Covers: 撞键不覆盖（ADR-0031 同款代数）。同一产品版本重登同一形态是重放，不是第二次登记。
func TestSavingTheSameFormTwiceIsAReplay(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := productVersionInTenant(t, "tenant-1", "product-1", "v1", "digest-product-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	product := serviceProductOf(t, version)
	mustSaveServiceProduct(t, transactor, ctx, repository, product)

	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SaveServiceProduct(txCtx, product)
		if err != nil {
			return err
		}
		if outcome != ports.ServiceProductAlreadyRegistered {
			t.Fatalf("replay outcome = %q, want ALREADY_REGISTERED", outcome)
		}
		return nil
	})
}

// Covers: `AT-PC-014` 的形态册半边（ADR-0040 / ADR-0003）：租户是身份的一部分，不是过滤器。
// 两个租户可以合法共用同一个产品标识与版本号，一方登记的形态不得出现在另一方的册里。
func TestAnotherTenantsFormDoesNotEnterThisScopesRegistry(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	mine := productVersionInTenant(t, "tenant-1", "product-1", "v1", "digest-mine")
	theirs := productVersionInTenant(t, "tenant-2", "product-1", "v1", "digest-theirs")
	mustSaveVersion(t, transactor, ctx, repository, mine)
	mustSaveVersion(t, transactor, ctx, repository, theirs)
	mustSaveServiceProduct(t, transactor, ctx, repository, serviceProductOf(t, theirs))

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	if registry.Count() != 1 {
		t.Fatalf("本租户册里有 %d 个版本，want 1（他租版本漏进来了）", registry.Count())
	}
	if products := registry.ServiceProducts(); len(products) != 0 {
		t.Fatalf("他租登记的形态进了本租户的册：%d 份", len(products))
	}
}

// ---- 夹具 ----

func mustSaveServiceProduct(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialPublications,
	product domain.ServiceProduct,
) {
	t.Helper()
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SaveServiceProduct(txCtx, product)
		if err != nil {
			return err
		}
		if outcome != ports.ServiceProductSaved {
			t.Fatalf("save outcome = %q, want SAVED", outcome)
		}
		return nil
	})
}

func serviceProductOf(t *testing.T, version domain.CommercialVersion) domain.ServiceProduct {
	t.Helper()
	product, err := domain.NewServiceProduct(version, domain.NetworkServiceForm)
	if err != nil {
		t.Fatalf("new service product: %v", err)
	}
	return product
}

// productVersionInTenant 造一份指定租户下`已生效`的服务产品版本。租户要能指定，因为跨租户
// 那条用例的整个前提就是两个租户共用同一个产品标识与版本号。
func productVersionInTenant(t *testing.T, tenant, objectID, label, digest string) domain.CommercialVersion {
	t.Helper()
	return rehydratedProductVersion(t, tenant, objectID, label, digest, domain.CommercialVersionEffective)
}

// publishedProductVersion 造一份`已发布未生效`的服务产品版本：已入册，但还没到自己声明的
// 生效边界，因此不参与新的解析选择。
func publishedProductVersion(t *testing.T, objectID, label, digest string) domain.CommercialVersion {
	t.Helper()
	return rehydratedProductVersion(t, "tenant-1", objectID, label, digest, domain.CommercialVersionPublished)
}

// rehydratedProductVersion 走领域重建门而不是逐步走发布生命周期：夹具要的是一份合法的已发布
// 版本，而重建门的校验正是「什么算合法」的单一权威（同 effectiveVersionOfKind 的理由）。
func rehydratedProductVersion(
	t *testing.T,
	tenant, objectID, label, digest string,
	status domain.CommercialVersionStatus,
) domain.CommercialVersion {
	t.Helper()

	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := domain.NewApprovalBasis(
		pcValue(t, domain.NewApprovalReference, "approval-1"),
		pcValue(t, domain.NewCommercialSourceReference, "source-1"),
		approvedAtFixture,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}

	spec := domain.RehydrateCommercialVersionSpec{
		TenantID:      pcTenant(t, tenant),
		Kind:          domain.ServiceProductObject,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, label),
		Scope:         pcScope(t),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, digest),
		Effective:     interval,
		Status:        status,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
	}
	// `已发布未生效`的版本不得带生效痕迹，重建门会拒；只有已生效那一格才填生效时刻。
	if status == domain.CommercialVersionEffective {
		spec.EffectiveAt = effectiveAtRow
	}

	version, err := domain.RehydrateCommercialVersion(spec)
	if err != nil {
		t.Fatalf("重建服务产品版本：%v", err)
	}
	return version
}

// insertFormRow 绕过 SaveServiceProduct 直接落一行形态。它只用于造那些**写侧构造不出、库上
// 却真实存在**的状态：SaveServiceProduct 收的是 domain.ServiceProduct，而后者要求版本已生效。
func insertFormRow(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	tenant, objectID, label, form string,
) {
	t.Helper()
	if _, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.service_product_form
			(tenant_id, object_kind, object_id, version_label, form)
		 VALUES ($1, 1, $2, $3, $4)`,
		tenant, objectID, label, form,
	); err != nil {
		t.Fatalf("直接落形态行：%v", err)
	}
}
