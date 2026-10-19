package partycommercial_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/adapters/registrationjson"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/partycommercial"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证核对缝的整条读路（票 demo-chain-gaps/22 完成判据「真库测试覆盖 SYN 两份协议」）：
// 演示种子里的供应商协议经 PC 发布用例落库，再经 PC 点读口与本包适配器核出各格。种子从仓库里的发布批读，不在
// 测试里抄一份——两份各改各的，测试就会对着一份种子里已经不存在的协议证明它可用。

// 本文件核的演示租户供应商协议：干线一份由外包段采用，末端一份暂不被任何委托采用（.scratch/demo-chain-gaps/tf-chain-plan.md
// 「跨票通则」X1）。
const (
	trunkAgreement    = "SYN-SUPPLIER-TRUNK-CN-SG-01"
	lastMileAgreement = "SYN-SUPPLIER-LASTMILE-SG-01"
)

type publishClock struct{ at time.Time }

func (clock publishClock) Now() time.Time { return clock.at }

// silentRegistrationHandoff 收下「参数已登记」意图不入队：本测试证的是协议读路，那封信封的投递由 PC 自己的测试证。
type silentRegistrationHandoff struct{}

func (silentRegistrationHandoff) HandOffOperatorRegistrationCompleted(context.Context, pcports.OperatorRegistrationCompletedIntent) error {
	return nil
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("工作目录：%v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("找不到 go.mod")
		}
		dir = parent
	}
}

// seededSupplierAgreements 读演示发布批里的供应商协议项，按对象号索引。解码与 parcel-commercial CLI 同一形状、
// 同样拒未知字段。
func seededSupplierAgreements(t *testing.T) map[string]registrationjson.PublicationItemDocument {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repositoryRoot(t), "scripts/demo-seeds/data/commercial/publish-batch.json"))
	if err != nil {
		t.Fatalf("读演示发布批：%v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var batch registrationjson.PublicationBatchDocument
	if err := decoder.Decode(&batch); err != nil {
		t.Fatalf("解码演示发布批：%v", err)
	}
	agreements := map[string]registrationjson.PublicationItemDocument{}
	for _, item := range batch.Items {
		if item.Kind == "SUPPLIER_AGREEMENT" {
			agreements[item.ObjectID] = item
		}
	}
	return agreements
}

func publishSeededAgreement(
	t *testing.T,
	db *bentopg.DB,
	handler *pcapplication.PublishCommercialAuthorityHandler,
	item registrationjson.PublicationItemDocument,
) {
	t.Helper()
	command, err := registrationjson.PublicationCommand(item)
	if err != nil {
		t.Fatalf("%s：发布翻译：%v", item.ObjectID, err)
	}
	var result pcapplication.PublishCommercialAuthorityResult
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var handleErr error
		result, handleErr = handler.Handle(txCtx, command)
		return handleErr
	}); err != nil {
		t.Fatalf("%s：发布：%v", item.ObjectID, err)
	}
	if result.Outcome() != pcapplication.CommercialVersionPublishedEffective {
		t.Fatalf("%s：发布结果 = %s，want PUBLISHED_EFFECTIVE", item.ObjectID, result.Outcome())
	}
	bodySaved := false
	for _, report := range result.Declarations() {
		if report.Channel == pcapplication.SupplierAgreementBodyChannel && report.Outcome == pcports.DeclarationSaved {
			bodySaved = true
		}
	}
	if !bodySaved {
		t.Fatalf("%s：正文没随发布登记：%v", item.ObjectID, result.Declarations())
	}
}

// Covers: 票 demo-chain-gaps/22 完成判据——SYN 干线与末端协议各按自己的供应商在册，交回种子里的责任法人与 PC 拼出的快照；
// 干线协议填末端承运商答服务方不符（票 23 的演示停点）；生效前答未生效；没发布的版本与他租户答未在册。
func TestTheSyntheticSupplierAgreementsAreCheckedThroughTheRealPointRead(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造发布登记册：%v", err)
	}
	decisionAt := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	handler := pcapplication.NewPublishCommercialAuthorityHandler(registry, publishClock{at: decisionAt}, silentRegistrationHandoff{})

	seeded := seededSupplierAgreements(t)
	for _, object := range []string{trunkAgreement, lastMileAgreement} {
		item, present := seeded[object]
		if !present {
			t.Fatalf("演示发布批里没有 %s", object)
		}
		if item.Declarations == nil || item.Declarations.SupplierAgreementBody == nil {
			t.Fatalf("%s 在种子里没带正文", object)
		}
		publishSeededAgreement(t, db, handler, item)
	}

	contents, err := pcpostgres.NewSupplierAgreementContents(db)
	if err != nil {
		t.Fatalf("构造协议读口：%v", err)
	}
	source, err := adapter.NewSupplierAgreementSource(contents)
	if err != nil {
		t.Fatalf("构造核对源：%v", err)
	}
	check := func(tenant, object, version, provider string, at time.Time) tfports.SupplierAgreementCheck {
		t.Helper()
		agreement, err := tfdomain.NewSupplierAgreementVersion(object, version)
		if err != nil {
			t.Fatalf("协议版本：%v", err)
		}
		answer, err := source.CheckSupplierAgreement(t.Context(),
			mustBuild(t, tfdomain.NewTenantID, tenant), agreement,
			mustBuild(t, tfdomain.NewServiceProviderReference, provider), at)
		if err != nil {
			t.Fatalf("%s %s/%s：核对：%v", tenant, object, version, err)
		}
		return answer
	}

	for _, object := range []string{trunkAgreement, lastMileAgreement} {
		item := seeded[object]
		body := item.Declarations.SupplierAgreementBody
		tenant := string(bytes.Trim(item.TenantID, `"`))

		registered := check(tenant, object, item.Version, body.Supplier, decisionAt)
		if registered.Standing != tfports.SupplierAgreementRegistered {
			t.Fatalf("%s：standing = %s，want AGREEMENT_REGISTERED", object, registered.Standing)
		}
		if registered.LegalEntity.String() != body.LegalEntity {
			t.Fatalf("%s：法人 = %q，want 种子里的 %q", object, registered.LegalEntity, body.LegalEntity)
		}
		if want := object + "/" + item.Version; registered.Snapshot.String() != want {
			t.Fatalf("%s：快照 = %q，want %q", object, registered.Snapshot, want)
		}

		if before := check(tenant, object, item.Version, body.Supplier, body.EffectiveStartsAt.Add(-time.Second)); before.Standing != tfports.SupplierAgreementNotEffective {
			t.Fatalf("%s 生效前：standing = %s，want AGREEMENT_NOT_EFFECTIVE", object, before.Standing)
		}
		if unpublished := check(tenant, object, item.Version+"-unpublished", body.Supplier, decisionAt); unpublished.Standing != tfports.SupplierAgreementNotRegistered {
			t.Fatalf("%s 未发布的版本：standing = %s，want AGREEMENT_NOT_REGISTERED", object, unpublished.Standing)
		}
		if elsewhere := check(tenant+"-OTHER", object, item.Version, body.Supplier, decisionAt); elsewhere.Standing != tfports.SupplierAgreementNotRegistered {
			t.Fatalf("%s 他租户：standing = %s，want AGREEMENT_NOT_REGISTERED", object, elsewhere.Standing)
		}
	}

	trunk := seeded[trunkAgreement]
	lastMileSupplier := seeded[lastMileAgreement].Declarations.SupplierAgreementBody.Supplier
	mismatch := check(string(bytes.Trim(trunk.TenantID, `"`)), trunkAgreement, trunk.Version, lastMileSupplier, decisionAt)
	if mismatch.Standing != tfports.SupplierAgreementProviderMismatch {
		t.Fatalf("干线协议填末端承运商：standing = %s，want PROVIDER_MISMATCH", mismatch.Standing)
	}
	if mismatch.Supplier.String() != trunk.Declarations.SupplierAgreementBody.Supplier {
		t.Fatalf("服务方不符时说出的协议供应商 = %q", mismatch.Supplier)
	}
}
