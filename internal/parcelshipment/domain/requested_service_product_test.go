package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件钉的是票 psb/17 的 PS 半边：委托声明的服务产品从服务要求段按封闭条目名读出、随提交版本留下，供商业依据
// 解析键回读。

func TestTheRequestedServiceProductIsReadByItsClosedEntryNameOnly(t *testing.T) {
	entries := []domain.CanonicalContentEntry{
		contentEntry(t, "service_product", "express"),
		contentEntry(t, "requestedServiceProduct", "SYN-OTHER"),
		contentEntry(t, domain.RequestedServiceProductEntryName, " SYN-PROD-CN-SG-EXPRESS "),
	}

	product := domain.RequestedServiceProductOf(entries)
	if !product.Declared() || product.String() != " SYN-PROD-CN-SG-EXPRESS " {
		t.Fatalf("product = %q declared = %v；只认封闭条目名，值原样不去空白", product.String(), product.Declared())
	}
	if domain.RequestedServiceProductOf(entries[:2]).Declared() {
		t.Fatal("租户约定的条目名被当成了委托声明的服务产品")
	}
}

// 显式清空的值不是一个产品；同名多条互相矛盾时一条都不认，读口不替客户挑。
func TestAnExplicitlyClearedOrDuplicatedRequestedServiceProductReadsAsUndeclared(t *testing.T) {
	cleared := domain.RequestedServiceProductOf([]domain.CanonicalContentEntry{
		contentEntry(t, domain.RequestedServiceProductEntryName, ""),
	})
	if cleared.Declared() {
		t.Fatal("显式清空的服务产品被读成了已声明")
	}
	duplicated := domain.RequestedServiceProductOf([]domain.CanonicalContentEntry{
		contentEntry(t, domain.RequestedServiceProductEntryName, "SYN-PROD-A"),
		contentEntry(t, domain.RequestedServiceProductEntryName, "SYN-PROD-B"),
	})
	if duplicated.Declared() {
		t.Fatalf("两条矛盾的声明读出了 %q", duplicated.String())
	}
}

// 规范化一次交回摘要与内容：声明的产品随内容交出，摘要与只算摘要的那条路逐字节相同。
func TestCanonicalizingASubmissionCarriesTheRequestedServiceProductWithoutChangingTheDigest(t *testing.T) {
	spec := payloadSpec(t)
	spec.Service = append(spec.Service, contentEntry(t, domain.RequestedServiceProductEntryName, "SYN-PROD-CN-SG-EXPRESS"))

	canonical, err := domain.CanonicalizeSubmission(spec)
	if err != nil {
		t.Fatalf("canonicalize submission: %v", err)
	}
	if got := canonical.RequestedServiceProduct(); got.String() != "SYN-PROD-CN-SG-EXPRESS" {
		t.Fatalf("requested product = %q, want SYN-PROD-CN-SG-EXPRESS", got.String())
	}
	if canonical.Digest() != canonicalize(t, spec) {
		t.Fatal("交出声明的产品之后摘要变了")
	}
}

// 每一版提交版本带自己的声明，不从上一版继承：新版本没报产品就是没报，继承会把「客户改了话」与「客户没说」混掉。
func TestEachSubmissionVersionCarriesItsOwnRequestedServiceProduct(t *testing.T) {
	spec := submitSpec(t, "parcel-1", "parcel-2")
	spec.RequestedProduct = requestedProduct(t, "SYN-PROD-CN-SG-EXPRESS")
	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("submit shipment request: %v", err)
	}
	first := request.CurrentSubmissionVersion()
	if got := first.RequestedServiceProduct(); got.String() != "SYN-PROD-CN-SG-EXPRESS" {
		t.Fatalf("first version product = %q", got.String())
	}

	superseded, err := request.FormNewSubmissionVersion(supersessionSpecFor(t, "parcel-1", "parcel-2"))
	if err != nil {
		t.Fatalf("form new submission version: %v", err)
	}
	if superseded.CurrentSubmissionVersion().RequestedServiceProduct().Declared() {
		t.Fatal("新版本没报产品却继承了上一版的声明")
	}

	found, ok := superseded.SubmissionVersionByID(first.VersionID())
	if !ok {
		t.Fatal("按版本号取不回历史版本")
	}
	if got := found.RequestedServiceProduct(); got.String() != "SYN-PROD-CN-SG-EXPRESS" {
		t.Fatalf("history version product = %q", got.String())
	}
	if _, ok := superseded.SubmissionVersionByID(mustValue(t, domain.NewSubmissionVersionID, "version-unknown")); ok {
		t.Fatal("不存在的版本号取回了一版")
	}
}

func TestRehydrationKeepsEachVersionsRequestedServiceProduct(t *testing.T) {
	snapshot := submittedSnapshot(t)
	snapshot.CurrentVersion.RequestedProduct = requestedProduct(t, "SYN-PROD-CN-SG-ECON")

	request, err := domain.RehydrateShipmentRequest(snapshot)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if got := request.CurrentSubmissionVersion().RequestedServiceProduct(); got.String() != "SYN-PROD-CN-SG-ECON" {
		t.Fatalf("rehydrated product = %q, want SYN-PROD-CN-SG-ECON", got.String())
	}
}

func requestedProduct(t *testing.T, value string) domain.DeclaredServiceProduct {
	t.Helper()
	product := domain.RequestedServiceProductOf([]domain.CanonicalContentEntry{
		contentEntry(t, domain.RequestedServiceProductEntryName, value),
	})
	if !product.Declared() {
		t.Fatalf("夹具产品 %q 没读出来", value)
	}
	return product
}
