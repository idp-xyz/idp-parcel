package postgres_test

import (
	"context"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证权威视图读口：已发布版本经本口取回、租户与范围隔离、
// 视图修订可派生，以及装配缺陷响亮报错。

func newAuthority(t *testing.T) (*adapter.CommercialAuthority, *adapter.CommercialPublications, func(domain.CommercialVersion)) {
	t.Helper()
	publications, transactor, _ := newPublications(t)
	authority, err := adapter.NewCommercialAuthority(publications)
	if err != nil {
		t.Fatalf("构造权威视图：%v", err)
	}
	ctx := t.Context()
	publish := func(version domain.CommercialVersion) {
		mustSaveVersion(t, transactor, ctx, publications, version)
	}
	return authority, publications, publish
}

func TestTheAuthorityViewCarriesThePublishedRegister(t *testing.T) {
	authority, _, publish := newAuthority(t)
	publish(effectiveContract(t, "contract-1", "v1", "digest-1"))

	registry, err := authority.LoadScope(t.Context(), pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("读权威视图：%v", err)
	}
	if registry == nil || registry.Count() != 1 {
		t.Fatalf("已发布版本没进权威视图：%+v", registry)
	}
	if _, exists := registry.Lookup(
		pcTenant(t, "tenant-1"), domain.CustomerContractObject,
		pcValue(t, domain.NewCommercialObjectID, "contract-1"),
		pcValue(t, domain.NewCommercialVersionLabel, "v1")); !exists {
		t.Fatal("权威视图里查不到那份合同版本")
	}
	// 解析要用视图修订证明「当时那个视图是不是还是同一个」，派生不出就无从证明。
	if registry.ViewRevision(pcTenant(t, "tenant-1"), pcScope(t)).String() == "" {
		t.Fatal("权威视图派生不出视图修订")
	}
}

// 同范围新增一个竞争候选时，先前采用的对象一个字节都没变，但视图修订必须跟着变——
// 这正是提交前失效要抓的东西，而它只有在按范围整册取回时才看得见。
func TestASecondCandidateInTheSameScopeMovesTheViewRevision(t *testing.T) {
	authority, _, publish := newAuthority(t)
	publish(effectiveContract(t, "contract-1", "v1", "digest-1"))

	before, err := authority.LoadScope(t.Context(), pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("读权威视图：%v", err)
	}
	firstRevision := before.ViewRevision(pcTenant(t, "tenant-1"), pcScope(t))

	publish(effectiveContract(t, "contract-2", "v1", "digest-2"))
	after, err := authority.LoadScope(t.Context(), pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("再读权威视图：%v", err)
	}
	if after.Count() != 2 {
		t.Fatalf("第二个候选没进视图：count=%d", after.Count())
	}
	if after.ViewRevision(pcTenant(t, "tenant-1"), pcScope(t)) == firstRevision {
		t.Fatal("同范围多了一个候选，视图修订却没动")
	}
}

// 空视图与「读不到」是两个结果：本口只在真的没有已发布版本时交回空册，且不是 error。
func TestAnEmptyScopeIsAnEmptyRegisterRatherThanAnError(t *testing.T) {
	authority, _, _ := newAuthority(t)

	registry, err := authority.LoadScope(t.Context(), pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("空范围不该是错误：%v", err)
	}
	if registry == nil || registry.Count() != 0 {
		t.Fatalf("空范围读出了内容：%+v", registry)
	}
}

func TestTheAuthorityViewIsIsolatedByTenantAndScope(t *testing.T) {
	authority, _, publish := newAuthority(t)
	publish(effectiveContract(t, "contract-1", "v1", "digest-1"))

	otherTenant, err := authority.LoadScope(t.Context(), pcTenant(t, "tenant-b"), pcScope(t))
	if err != nil {
		t.Fatalf("他租户读视图：%v", err)
	}
	if otherTenant.Count() != 0 {
		t.Error("他租户读到了本租户的权威视图")
	}

	otherScope, err := authority.LoadScope(t.Context(),
		pcTenant(t, "tenant-1"),
		pcValue(t, domain.NewCommercialScopeReference, "scope-2"))
	if err != nil {
		t.Fatalf("他范围读视图：%v", err)
	}
	if otherScope.Count() != 0 {
		t.Error("另一个范围读到了本范围的候选")
	}
}

// 没接登记册的权威视图会把每个范围都答成空册，而空册在解析里是一个合法答案
// （「无适用依据」）——那会让一处装配遗漏表现为业务结论，构造期拦住才看得见。
func TestAnUnwiredAuthorityViewRefusesToBeBuilt(t *testing.T) {
	if _, err := adapter.NewCommercialAuthority(nil); err == nil {
		t.Fatal("没有登记册也构造出了权威视图")
	}
}

// readOnlyPublicationView 只实现 LoadForScope，没有 Save。能交给 NewCommercialAuthority
// 就证明权威视图不再要求写侧方法——类型形状本身是证据，不靠反射。
type readOnlyPublicationView struct{}

func (readOnlyPublicationView) LoadForScope(
	context.Context,
	domain.TenantID,
	domain.CommercialScopeReference,
) (*domain.CommercialRegistry, error) {
	return domain.NewCommercialRegistry(), nil
}

var _ ports.CommercialPublicationView = readOnlyPublicationView{}

func TestAReadOnlyPublicationViewCanWireTheAuthority(t *testing.T) {
	authority, err := adapter.NewCommercialAuthority(readOnlyPublicationView{})
	if err != nil {
		t.Fatalf("只读替身应当能构造权威视图：%v", err)
	}
	registry, err := authority.LoadScope(t.Context(), pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("只读替身读范围：%v", err)
	}
	if registry == nil || registry.Count() != 0 {
		t.Fatalf("只读替身应交回空册：%+v", registry)
	}
}
