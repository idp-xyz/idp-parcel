package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证渠道候选装配（票 `label-channel/12`）。装配自己不判优劣也不定价——它只回答
// 「这一票此刻有哪些渠道可选」，交给票 13 那条链去评价与择优。
//
// 收窄规则本身不在这个缝上证：`CandidatesAllowedBy` 只收窄不扩张已由 party-commercial
// 自己的 TestCustomerConstraintOnlyNarrowsTheCandidateRange 守着。这里要证的是**装配调用
// 了它，而不是自己又写了一遍**，以及调用之前那几道门。

// 装配器就是择优编排要的那个口。断言写在测试里而不只靠编排测试的替身：编排测试用替身
// 一路绿，装配器却可以一直接不进 Deps——那正是接线后曾经的状态。
var _ psports.ChannelCandidateAssembly = (*adapter.ChannelCandidateAssembler)(nil)

const (
	assemblyTenant  = "tenant-1"
	assemblyScope   = "scope-a"
	assemblyProduct = "product-label-channel"
	assemblyMapping = "mapping-1"
)

func assemblyAt(t testing.TB) time.Time {
	t.Helper()
	return time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
}

func assemblyValue[T any](t testing.TB, constructor func(string) (T, error), value string) T {
	t.Helper()
	result, err := constructor(value)
	if err != nil {
		t.Fatalf("构造 %q：%v", value, err)
	}
	return result
}

// effectiveLabelChannelProduct 造一份**已生效**的面单渠道服务产品版本，并登记进册。
//
// 走完整的草稿→发布→生效三步而不是直接拼一个版本值：NewServiceProduct 只收已生效版本，
// 那条不变量正是装配要依赖的（退役产品不得再产候选），绕过它造夹具等于把待证的那一半
// 先替实现假设掉。
func effectiveLabelChannelProduct(t testing.TB, registry *pcdomain.CommercialRegistry) pcdomain.ServiceProduct {
	t.Helper()

	interval, err := pcdomain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("构造生效区间：%v", err)
	}
	draft, err := pcdomain.NewCommercialDraft(pcdomain.CommercialVersionSpec{
		TenantID:      assemblyValue(t, pcdomain.NewTenantID, assemblyTenant),
		Kind:          pcdomain.ServiceProductObject,
		ObjectID:      assemblyValue(t, pcdomain.NewCommercialObjectID, assemblyProduct),
		Version:       assemblyValue(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         assemblyValue(t, pcdomain.NewCommercialScopeReference, assemblyScope),
		ContentDigest: assemblyValue(t, pcdomain.NewCommercialContentDigest, "sha256:syn-label-channel-v1"),
		Effective:     interval,
	})
	if err != nil {
		t.Fatalf("构造草稿：%v", err)
	}
	basis, err := pcdomain.NewApprovalBasis(
		assemblyValue(t, pcdomain.NewApprovalReference, "approval-label-channel-v1"),
		assemblyValue(t, pcdomain.NewCommercialSourceReference, "syn-source"),
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("构造批准依据：%v", err)
	}
	published, err := draft.Publish(basis, pcdomain.ApprovalRoleConfirmed, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatalf("发布：%v", err)
	}
	live, err := published.TakeEffect(time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("生效：%v", err)
	}
	product, err := pcdomain.NewServiceProduct(live, pcdomain.LabelChannelServiceForm)
	if err != nil {
		t.Fatalf("构造服务产品：%v", err)
	}
	if _, err := registry.Register(live); err != nil {
		t.Fatalf("登记版本：%v", err)
	}
	registry.RegisterServiceProduct(product)
	return product
}

// twoChannelRegistration 造一笔登记两个渠道的产品—渠道映射登记。两个而不是一个：一个
// 渠道时「全部放行」与「恰好只剩它」在结果上分不开。
func twoChannelRegistration(t testing.TB) pcdomain.ProductChannelMappingRegistration {
	t.Helper()

	return twoChannelRegistrationFor(t, "v1")
}

func twoChannelRegistrationFor(t testing.TB, productVersion string) pcdomain.ProductChannelMappingRegistration {
	t.Helper()

	binding, err := pcdomain.NewConfiguredChannelBinding([]pcdomain.ChannelProductReference{
		assemblyValue(t, pcdomain.NewChannelProductReference, "channel-a"),
		assemblyValue(t, pcdomain.NewChannelProductReference, "channel-b"),
	})
	if err != nil {
		t.Fatalf("构造渠道绑定：%v", err)
	}
	interval, err := pcdomain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("构造映射生效区间：%v", err)
	}
	registration, err := pcdomain.NewProductChannelMappingRegistration(
		assemblyValue(t, pcdomain.NewTenantID, assemblyTenant),
		assemblyValue(t, pcdomain.NewProductChannelMappingID, assemblyMapping),
		1,
		pcdomain.ProductChannelMappingSpec{
			Product:        assemblyValue(t, pcdomain.NewCommercialObjectID, assemblyProduct),
			ProductVersion: assemblyValue(t, pcdomain.NewCommercialVersionLabel, productVersion),
			Binding:        binding,
			Effective:      interval,
			Basis:          assemblyValue(t, pcdomain.NewMappingBasisReference, "syn-mapping-basis"),
		},
	)
	if err != nil {
		t.Fatalf("构造映射登记：%v", err)
	}
	return registration
}

// stubMappings 与 stubPublication 是提供方两个读口的替身。它们如实作答，不模拟失败——
// 本文件此刻要证的门都在装配这一侧。
type stubMappings struct {
	registration pcdomain.ProductChannelMappingRegistration
	found        bool
}

func (stub stubMappings) LoadLatestMapping(
	_ context.Context,
	_ pcdomain.TenantID,
	_ pcdomain.ProductChannelMappingID,
) (pcdomain.ProductChannelMappingRegistration, bool, error) {
	return stub.registration, stub.found, nil
}

type stubPublication struct {
	registry *pcdomain.CommercialRegistry
}

func (stub stubPublication) LoadForScope(
	_ context.Context,
	_ pcdomain.TenantID,
	_ pcdomain.CommercialScopeReference,
) (*pcdomain.CommercialRegistry, error) {
	return stub.registry, nil
}

// stubConstraints 按三态作答。零值 = 未配置，与真实装配面对的缺席同形。
type stubConstraints struct {
	constraint adapter.ChannelConstraint
}

func (stub stubConstraints) ChannelConstraintFor(
	_ context.Context,
	_ psports.ChannelSelectionQuery,
) (adapter.ChannelConstraint, error) {
	return stub.constraint, nil
}

// recordingMappings 与 recordingPublication 记下提供方读口收到了什么键。它们存在只为证
// 翻译：择优侧给的是自己的引用，到提供方读口时必须是提供方的键且取值原样。
type recordingMappings struct {
	stubMappings
	tenant  string
	mapping string
	calls   int
}

func (stub *recordingMappings) LoadLatestMapping(
	ctx context.Context,
	tenant pcdomain.TenantID,
	mapping pcdomain.ProductChannelMappingID,
) (pcdomain.ProductChannelMappingRegistration, bool, error) {
	stub.calls++
	stub.tenant = tenant.String()
	stub.mapping = mapping.String()
	return stub.stubMappings.LoadLatestMapping(ctx, tenant, mapping)
}

type recordingPublication struct {
	stubPublication
	tenant string
	scope  string
	calls  int
}

func (stub *recordingPublication) LoadForScope(
	ctx context.Context,
	tenant pcdomain.TenantID,
	scope pcdomain.CommercialScopeReference,
) (*pcdomain.CommercialRegistry, error) {
	stub.calls++
	stub.tenant = tenant.String()
	stub.scope = scope.String()
	return stub.stubPublication.LoadForScope(ctx, tenant, scope)
}

type recordingConstraints struct {
	stubConstraints
	calls int
}

func (stub *recordingConstraints) ChannelConstraintFor(
	ctx context.Context,
	query psports.ChannelSelectionQuery,
) (adapter.ChannelConstraint, error) {
	stub.calls++
	return stub.stubConstraints.ChannelConstraintFor(ctx, query)
}

func assemblerWith(t testing.TB, constraint adapter.ChannelConstraint) *adapter.ChannelCandidateAssembler {
	t.Helper()

	return assemblerFor(t, constraint, twoChannelRegistration(t))
}

// assemblerFor 装出一个协作方齐备的装配器：发布册里只有 v1 这一份已生效的面单渠道服务
// 产品，登记册交回给定的那一笔映射。
func assemblerFor(
	t testing.TB,
	constraint adapter.ChannelConstraint,
	registration pcdomain.ProductChannelMappingRegistration,
) *adapter.ChannelCandidateAssembler {
	t.Helper()

	registry := pcdomain.NewCommercialRegistry()
	effectiveLabelChannelProduct(t, registry)
	return adapter.NewChannelCandidateAssembler(adapter.ChannelCandidateAssemblerDeps{
		Mappings:    stubMappings{registration: registration, found: true},
		Publication: stubPublication{registry: registry},
		Constraints: stubConstraints{constraint: constraint},
	})
}

// assemblyQuery 用**择优侧**的引用造查询——编排住在 parcel-shipment 的应用层，手上只有
// 这一套；提供方的键由装配器译出来，不由调用方先译好再给。
func assemblyQuery(t testing.TB) psports.ChannelSelectionQuery {
	t.Helper()

	return psports.ChannelSelectionQuery{
		Tenant:  assemblyValue(t, psdomain.NewTenantID, assemblyTenant),
		Scope:   assemblyValue(t, psdomain.NewCommercialScopeReference, assemblyScope),
		Mapping: assemblyValue(t, psdomain.NewProductChannelMappingReference, assemblyMapping),
		At:      assemblyAt(t),
	}
}

// Covers: 装配器答的是 ports.ChannelCandidateAssembly 这道口，且择优侧引用的取值**原样**
// 到达提供方的两个读口——租户与映射标识到登记册，租户与商业范围到发布册。
//
// 这一格是接线那笔留下的缝：编排经端口传的是本上下文的引用，装配器原先只收提供方的键，
// 两边各自绿而真装配器交不进编排的 Deps。只断言「实现了接口」不够——译错一个字段照样
// 满足接口，所以让两个读口把收到的键记下来对。
func TestTheAssemblerAnswersTheSelectionPortWithTheQueryTranslatedIntoProviderKeys(t *testing.T) {
	t.Parallel()

	registry := pcdomain.NewCommercialRegistry()
	effectiveLabelChannelProduct(t, registry)
	mappings := &recordingMappings{stubMappings: stubMappings{registration: twoChannelRegistration(t), found: true}}
	publication := &recordingPublication{stubPublication: stubPublication{registry: registry}}
	var assembly psports.ChannelCandidateAssembly = adapter.NewChannelCandidateAssembler(
		adapter.ChannelCandidateAssemblerDeps{
			Mappings:    mappings,
			Publication: publication,
			Constraints: stubConstraints{constraint: adapter.UnconstrainedChannels()},
		},
	)

	candidates, err := assembly.AssembleChannelCandidates(context.Background(), assemblyQuery(t))
	if err != nil {
		t.Fatalf("经端口装配：%v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("候选数 = %d，want 2", len(candidates))
	}
	if mappings.tenant != assemblyTenant || mappings.mapping != assemblyMapping {
		t.Fatalf("登记册收到的键 = (%q, %q)，want (%q, %q)——择优侧的引用没有原样译到提供方",
			mappings.tenant, mappings.mapping, assemblyTenant, assemblyMapping)
	}
	if publication.tenant != assemblyTenant || publication.scope != assemblyScope {
		t.Fatalf("发布册收到的键 = (%q, %q)，want (%q, %q)",
			publication.tenant, publication.scope, assemblyTenant, assemblyScope)
	}
}

// Covers: 译不成提供方键的查询在问**任何**协作方之前就被拒，且落在 ErrUntranslatableQuery
// 这一格而不是混进「读取失败」。
//
// 零值查询是唯一能到这里的形状（两侧构造门都拒空白）。三个协作方都记调用次数：约束口是
// 消费方自己的实例半边、可能是一次远程读，为一个立不住的查询去问它，答回来也没处用；
// 而若先问了再拒，一次「拒」在约束口那侧看起来就是一次正常读取。
func TestAQueryThatCannotBeTranslatedIsRefusedBeforeAnyCollaboratorIsAsked(t *testing.T) {
	t.Parallel()

	registry := pcdomain.NewCommercialRegistry()
	effectiveLabelChannelProduct(t, registry)
	mappings := &recordingMappings{stubMappings: stubMappings{registration: twoChannelRegistration(t), found: true}}
	publication := &recordingPublication{stubPublication: stubPublication{registry: registry}}
	constraints := &recordingConstraints{stubConstraints: stubConstraints{constraint: adapter.UnconstrainedChannels()}}
	assembler := adapter.NewChannelCandidateAssembler(adapter.ChannelCandidateAssemblerDeps{
		Mappings:    mappings,
		Publication: publication,
		Constraints: constraints,
	})

	candidates, err := assembler.AssembleChannelCandidates(
		context.Background(),
		psports.ChannelSelectionQuery{At: assemblyAt(t)},
	)
	if !errors.Is(err, adapter.ErrUntranslatableQuery) {
		t.Fatalf("装配 err = %v，want %v", err, adapter.ErrUntranslatableQuery)
	}
	if len(candidates) != 0 {
		t.Fatalf("被拒的装配仍交回了 %d 个候选", len(candidates))
	}
	if constraints.calls != 0 || mappings.calls != 0 || publication.calls != 0 {
		t.Fatalf("协作方被问了 (约束 %d, 登记册 %d, 发布册 %d) 次，want 全 0——译不过去的查询不该出门",
			constraints.calls, mappings.calls, publication.calls)
	}
}

// Covers: 票 12 红线「不填任何候选内容、渠道账号、映射取值（实例半边）」在读取失败一侧的
// 落法——**渠道约束读不回来时装配停下，不得当作「客户没提约束」**。
//
// 这一格是本票最贵的一格，理由在 CandidatesAllowedBy 自己的实现里：它在 allowed 为空时
// 交回**全部**可用渠道。对它自己这是对的（客户没提约束本就等于不收窄），但装配若把「未
// 配置」也折成空切片传进去，一次读取失败就静默变成一次**范围放大**——放大出来的渠道客户
// 可能恰好禁止过，而下游只看到一个多出来的候选，看不出它是怎么进来的。
//
// 夹具刻意让映射真的有两个渠道可放：映射为空时「停下」与「没有候选」在结果上分不开，
// 那样的绿是假的。
func TestAnUnconfiguredChannelConstraintStopsTheAssemblyRatherThanAllowingEveryChannel(t *testing.T) {
	t.Parallel()

	assembler := assemblerWith(t, adapter.ChannelConstraint{})

	candidates, err := assembler.AssembleChannelCandidates(context.Background(), assemblyQuery(t))
	if !errors.Is(err, adapter.ErrChannelConstraintNotConfigured) {
		t.Fatalf("装配 err = %v，want %v", err, adapter.ErrChannelConstraintNotConfigured)
	}
	if len(candidates) != 0 {
		t.Fatalf("停下的装配仍交回了 %d 个候选——读取失败被当成了「无约束」", len(candidates))
	}
}

// Covers: 装配交回的是**映射给出、再经客户约束收窄**的那一批，且译成择优侧的候选标识。
//
// 约束里刻意多点一个映射并未提供的 channel-z：客户点名不等于该渠道就成为候选，运营企业
// 只能在商业上可用的范围内选择（PC CONTEXT）。它与被收窄掉的 channel-a 一起，把两个方向
// 的错都钉住——只做交集的一半会漏掉其中一个。
func TestAssemblyReturnsTheMappedChannelsNarrowedByTheCustomerConstraint(t *testing.T) {
	t.Parallel()

	constraint, err := adapter.ConstrainedToChannels(
		assemblyValue(t, pcdomain.NewChannelProductReference, "channel-b"),
		assemblyValue(t, pcdomain.NewChannelProductReference, "channel-z"),
	)
	if err != nil {
		t.Fatalf("构造渠道约束：%v", err)
	}

	candidates, err := assemblerWith(t, constraint).
		AssembleChannelCandidates(context.Background(), assemblyQuery(t))
	if err != nil {
		t.Fatalf("装配：%v", err)
	}

	got := make([]string, len(candidates))
	for index, candidate := range candidates {
		got[index] = candidate.String()
	}
	if len(got) != 1 || got[0] != "channel-b" {
		t.Fatalf("候选 = %v，want [channel-b]——channel-a 该被约束收窄掉，channel-z 客户点了名但映射没提供", got)
	}
}

// Covers: 映射指名的服务产品版本此刻不在册（或不已生效）时装配停下，不产候选。
//
// 这道门不是仪式。ProductChannelMapping 的候选逻辑以「属于一份已生效产品」为前提——那正是
// NewProductChannelMapping 收 ServiceProduct 而 ServiceProduct 只收已生效版本的原因。跳过它
// 就等于让一份草稿、已到期或已退役的产品继续产出新候选，而候选下游是一笔真实的供应商采购：
// 一个已经收尾的商业决定会因此重新参与新的采购。
//
// 摆法取「映射指着 v2，而册上生效的是 v1」：这是版本推进时的常见错位，且它与「映射根本
// 没登记」不同——那一格由 ErrProductChannelMappingNotRegistered 表达，续办也不同。
func TestAssemblyStopsWhenTheMappedProductVersionIsNotEffective(t *testing.T) {
	t.Parallel()

	constraint := adapter.UnconstrainedChannels()
	assembler := assemblerFor(t, constraint, twoChannelRegistrationFor(t, "v2"))

	candidates, err := assembler.AssembleChannelCandidates(context.Background(), assemblyQuery(t))
	if !errors.Is(err, adapter.ErrMappedServiceProductNotEffective) {
		t.Fatalf("装配 err = %v，want %v", err, adapter.ErrMappedServiceProductNotEffective)
	}
	if len(candidates) != 0 {
		t.Fatalf("停下的装配仍交回了 %d 个候选", len(candidates))
	}
}

// Covers: 这笔映射从未登记时停下，且与「登记了但此刻没有候选」分成两格。
//
// 两者都交回零个候选，压成一格就分不出续办：从未登记要去登记映射，而到期或被约束收窄到
// 空是映射如实作过答，续办是改约束或换产品版本。票 14 的落选留痕要答的正是这一类问题。
func TestAnUnregisteredMappingStopsTheAssembly(t *testing.T) {
	t.Parallel()

	registry := pcdomain.NewCommercialRegistry()
	effectiveLabelChannelProduct(t, registry)
	assembler := adapter.NewChannelCandidateAssembler(adapter.ChannelCandidateAssemblerDeps{
		Mappings:    stubMappings{found: false},
		Publication: stubPublication{registry: registry},
		Constraints: stubConstraints{constraint: adapter.UnconstrainedChannels()},
	})

	candidates, err := assembler.AssembleChannelCandidates(context.Background(), assemblyQuery(t))
	if !errors.Is(err, adapter.ErrProductChannelMappingNotRegistered) {
		t.Fatalf("装配 err = %v，want %v", err, adapter.ErrProductChannelMappingNotRegistered)
	}
	if len(candidates) != 0 {
		t.Fatalf("停下的装配仍交回了 %d 个候选", len(candidates))
	}
}

// Covers: 时点落在映射有效期之外时交回**零个候选而不是错误**。
//
// 这一格的要害是它**不是**失败：到期正是映射把渠道排除在新决定之外的方式，而它对这个
// 时点如实作过答。折成错误会让调用方把「这个时点没有可用渠道」与「装配没能进行」混为
// 一谈，前者的续办是换时点或换产品版本，后者是修装配。
//
// 同一份夹具在区间内交回候选、区间外交回零个，两次对照摆在一个用例里：只断言区间外为空
// 时，一个恒返回空的实现也能过。
func TestATimeOutsideTheMappingIntervalYieldsNoCandidatesRatherThanAnError(t *testing.T) {
	t.Parallel()

	assembler := assemblerWith(t, adapter.UnconstrainedChannels())

	inside, err := assembler.AssembleChannelCandidates(context.Background(), assemblyQuery(t))
	if err != nil {
		t.Fatalf("区间内装配：%v", err)
	}
	if len(inside) != 2 {
		t.Fatalf("区间内候选数 = %d，want 2——对照的那一半没立住，区间外为空就说明不了问题", len(inside))
	}

	expired := assemblyQuery(t)
	expired.At = time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	outside, err := assembler.AssembleChannelCandidates(context.Background(), expired)
	if err != nil {
		t.Fatalf("区间外装配报了错，而到期是映射如实作过的答：%v", err)
	}
	if len(outside) != 0 {
		t.Fatalf("区间外候选 = %d 个，want 0——到期的映射仍在为新决定提供渠道", len(outside))
	}
}
