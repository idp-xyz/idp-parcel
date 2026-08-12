package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// revalidatedAt 与 judgedAt 分开：重校验是另一次判断，交回同一个时刻就分不出结果是这一轮
// 确认的还是上一轮留下的。
var revalidatedAt = time.Date(2026, 6, 3, 14, 0, 0, 0, time.UTC)

// resolvedClosure 先走完第一阶段，交回它的结果与所用的权威视图，供提交前重校验接着用。
func resolvedClosure(t *testing.T, authority *authorityDouble, scope string) domain.CommercialClosure {
	t.Helper()
	resolved, err := application.NewResolveCommercialBasisHandler(authority, fixedClock{at: judgedAt}).
		Handle(context.Background(), application.ResolveCommercialBasisCommand{
			Key: closureKey(t, scope, domain.CustomerContractObject),
		})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Closure().Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want the first resolution to succeed", resolved.Closure().Outcome())
	}
	return resolved.Closure()
}

// Covers: UC-PC-002 步骤 8「业务决定提交前校验解析和关键结果仍相容」与 `AT-PC-026`「解析后合同
// 被当前修订替代，接受尚未提交 → 原解析失效并重解」。
//
// 同范围新增一个竞争候选时，已采用的那份合同一个字节都没动——逐对象去查会说它仍然有效，只有按
// 原查询重解才看得见解析已经不再唯一。这也是重校验必须拿原解析键、而不是拿调用方现给的键去跑
// 的原因：键跟着结果走，才不会用另一个查询去证明这一个仍然成立。
//
// 失效结果不带已采用依据：把它交回去等于引诱调用方在一份用例判定为不再成立的依据上继续提交。
func TestARevalidationSeesANewCandidateAndReportsTheResolutionStale(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	authority := &authorityDouble{registry: registry}
	prior := resolvedClosure(t, authority, "scope-a")

	effectiveIn(t, registry, domain.CustomerContractObject, "contract-2", "v1", "sha256:c2", "scope-a")

	store, caller, resolution := storedResolution(t, prior)
	revalidated, err := application.NewValidateCommercialBasisHandler(store, authority, fixedClock{at: revalidatedAt}).
		Handle(context.Background(), application.ValidateCommercialBasisCommand{Caller: caller, Resolution: resolution})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	if revalidated.Closure().Outcome() != domain.ResolutionStale {
		t.Fatalf("outcome = %q, want STALE; a second candidate in the same scope ended the uniqueness", revalidated.Closure().Outcome())
	}
	if revalidated.Closure().ResolutionID() != prior.ResolutionID() {
		t.Fatalf(
			"resolution id = %q, want the prior %q; the caller finds its stopped decision by that id",
			revalidated.Closure().ResolutionID(), prior.ResolutionID(),
		)
	}
	if revalidated.Closure().ContinuationReference().String() == "" {
		t.Fatal("失效没有续办引用，而用例要求`已失效`重新解析，调用方得能接上原来那次决定")
	}
	if len(revalidated.Closure().Adopted()) != 0 {
		t.Fatal("失效结果仍交回已采用依据，调用方会据以继续提交一个已经不成立的决定")
	}
	if !revalidated.JudgedAt().Equal(revalidatedAt) {
		t.Fatalf("judged at = %s, want this round's clock reading %s", revalidated.JudgedAt(), revalidatedAt)
	}
}

// Covers: UC-PC-002「依赖调用超时与权威确认『无适用依据』是不同结果」在重校验一侧，以及
// `AT-PC-027`。
//
// 读不到权威时，原结果既不能被确认也不能被断言失效。判成`已失效`会让调用方去重解一份其实还
// 好好的解析，判成仍然成立则会让它在一份可能已经被推翻的依据上提交决定；两个方向都是拿一次
// 读取失败冒充一个商业事实。
func TestAnUnreadableAuthorityLeavesTheRevalidationPendingRatherThanStale(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	authority := &authorityDouble{registry: registry}
	prior := resolvedClosure(t, authority, "scope-a")

	authority.err = errors.New("authority view unavailable")

	store, caller, resolution := storedResolution(t, prior)
	revalidated, err := application.NewValidateCommercialBasisHandler(store, authority, fixedClock{at: revalidatedAt}).
		Handle(context.Background(), application.ValidateCommercialBasisCommand{Caller: caller, Resolution: resolution})
	if err != nil {
		t.Fatalf("读取失败被当成技术错误抛出，而用例要求它形成解析未决: %v", err)
	}

	if revalidated.Closure().Outcome() != domain.ResolutionPending {
		t.Fatalf("outcome = %q, want RESOLUTION_PENDING; an unreadable view neither confirms nor invalidates", revalidated.Closure().Outcome())
	}
	if revalidated.Closure().Reason() != domain.AuthorityUnreadable {
		t.Fatalf("reason = %q, want AUTHORITY_UNREADABLE", revalidated.Closure().Reason())
	}
	if revalidated.Closure().ContinuationReference().String() == "" {
		t.Fatal("未决无法续办，而用例要求保存缺口并安全续办")
	}
	if len(revalidated.Closure().Adopted()) != 0 {
		t.Fatal("未决结果携带了已采用依据")
	}
}

// Covers: UC-PC-002「相同解析输入与相同修订可以返回原结果」与 `AT-PC-024` — 视图没变时原解析
// 原样成立。
//
// 原先这条 Covers 还认领了 `AT-PC-025`，是错配（实测于 `f6196c6`）：那一条要的是「解析后合同
// **退役**、而委托接受**已提交** → 历史决定保留原快照，不追溯改写」，本用例不碰退役、不碰已提交
// 的接受，也不断快照不被改写。摘掉它而不是在这里补断言：`AT-PC-025` 的承重点是消费方持有的历史
// 决定，PC 侧只有一半，在两半拼起来之前先写一半，只会得到第二条「声称覆盖却只覆盖一半」。
//
// 关键是不产生新的解析标识：换一个标识意味着这是另一次解析，调用方按原标识存下的引用、快照与
// 审计就都对不上了，而它其实什么都没变。
func TestAnUnchangedViewLetsThePriorResolutionStandWithItsOriginalIdentity(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	authority := &authorityDouble{registry: registry}
	prior := resolvedClosure(t, authority, "scope-a")

	store, caller, resolution := storedResolution(t, prior)
	revalidated, err := application.NewValidateCommercialBasisHandler(store, authority, fixedClock{at: revalidatedAt}).
		Handle(context.Background(), application.ValidateCommercialBasisCommand{Caller: caller, Resolution: resolution})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	if revalidated.Closure().Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want the prior resolution to stand", revalidated.Closure().Outcome())
	}
	if revalidated.Closure().ResolutionID() != prior.ResolutionID() {
		t.Fatalf(
			"resolution id = %q, want the unchanged %q; a new id would orphan every reference the caller already stored",
			revalidated.Closure().ResolutionID(), prior.ResolutionID(),
		)
	}
	if len(revalidated.Closure().Adopted()) != len(prior.Adopted()) {
		t.Fatalf("adopted %d bases, want the prior %d", len(revalidated.Closure().Adopted()), len(prior.Adopted()))
	}
}

// Covers: UC-PC-002 步骤 8 与 ADR-0003 — 重校验按原解析键重解，因此它问的租户与范围必须是当初
// 形成这份解析的那一对，而不是调用方这一刻另给的一对。
//
// 键由调用方现给的话，一次「校验」就能拿另一个范围的视图去证明这份解析仍然成立，跨租户探测也
// 会因此变成一个合法调用。
func TestARevalidationAsksTheAuthorityForTheKeyThatFormedThePriorResolution(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	authority := &authorityDouble{registry: registry}
	prior := resolvedClosure(t, authority, "scope-a")
	key := closureKey(t, "scope-a", domain.CustomerContractObject)

	authority.loadCalled = 0
	store, caller, resolution := storedResolution(t, prior)
	if _, err := application.NewValidateCommercialBasisHandler(store, authority, fixedClock{at: revalidatedAt}).
		Handle(context.Background(), application.ValidateCommercialBasisCommand{Caller: caller, Resolution: resolution}); err != nil {
		t.Fatalf("validate: %v", err)
	}

	if authority.loadCalled != 1 {
		t.Fatalf("authority loaded %d times, want exactly one view per revalidation", authority.loadCalled)
	}
	if authority.askedTenant != key.TenantID {
		t.Fatalf("asked tenant = %q, want %q", authority.askedTenant, key.TenantID)
	}
	if authority.askedScope != key.Scope {
		t.Fatalf("asked scope = %q, want %q", authority.askedScope, key.Scope)
	}
}

// Covers: UC-PC-002 `AT-PC-028`「其他客户账户探测合同 → 范围拒绝且不泄露候选」在第三阶段一侧
// （ADR-0027：解析标识不是能力凭证）。本用例压的是「拒绝」那半——越权者读不到依据，本上下文也
// 不去读权威；「不泄露」那半由下面那条不可区分用例压。
//
// 第二阶段已有同名守卫，这一支单独压：两处各自取回、各自比对，漏掉任何一处，拿到标识的人都能
// 从那一处读走另一个客户的商业依据。提交前重校验尤其要紧——它交回的是一份仍然成立的解析。
func TestARevalidationNamedByAnotherCustomerIsNotAccepted(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	authority := &authorityDouble{registry: registry}
	prior := resolvedClosure(t, authority, "scope-a")

	store, owner, resolution := storedResolution(t, prior)
	intruder := owner
	intruder.CustomerAccountID = value(t, domain.NewCustomerAccountID, "customer-elsewhere")

	authority.loadCalled = 0
	revalidated, err := application.NewValidateCommercialBasisHandler(store, authority, fixedClock{at: revalidatedAt}).
		Handle(context.Background(), application.ValidateCommercialBasisCommand{Caller: intruder, Resolution: resolution})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	// 取值是`依据未解析`而不是`输入未受理`：越权与「标识从未签发」的恢复动作同为回第一阶段
	// 重解，因此共用一格（ADR-0029）。`输入未受理`此后只留给查询发生前就短路的那一支。
	if revalidated.Closure().Outcome() != domain.BasisNotResolved {
		t.Fatalf("outcome = %q, want BASIS_NOT_RESOLVED——另一个客户凭标识重校验了这份解析", revalidated.Closure().Outcome())
	}
	if len(revalidated.Closure().Adopted()) != 0 {
		t.Fatal("范围不符却仍交回了已采用依据")
	}
	if authority.loadCalled != 0 {
		t.Fatal("范围不符却仍去读了权威视图——那次读取本身就回答了这个范围里有没有对象")
	}
}

// Covers: UC-PC-002 `AT-PC-028`「……按标识回指时，答案必须与探测一个从未签发的标识**完全一致**，
// 仅拒绝不足以满足本项」。上面那条压的是「拒绝」，这一条压的是「完全一致」。
//
// 引的是新文本。旧文本「输入未受理**或**范围拒绝」允许两个答案，而那个「或」正是 `3affd01`
// 认定的泄露源——本用例要证伪的恰好就是它。
//
// 拒绝一次越权重校验并不等于没泄露。若`这个标识不存在`与`这个标识存在、只是不属于你`给出两个
// 不同的答案，同租户下任何人都能拿一串标识挨个问，凭答案的差别把别人的解析枚举出来——被拒绝
// 的那一次同样告诉了他这份解析是真的。
//
// 两次探测因此只差一个变量：同一个入侵者、同一个标识，只换取回端口里有没有那份解析。调用方
// 看得见的每一处都必须一致。一致成哪一个取值不由本用例决定——那要动 `ResolutionOutcome` 的
// 取值集，属改领域语言；本用例只压两者不可区分。
func TestAProbeCannotTellAMissingResolutionFromOneOwnedByAnotherCustomer(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	authority := &authorityDouble{registry: registry}
	prior := resolvedClosure(t, authority, "scope-a")

	store, owner, resolution := storedResolution(t, prior)
	intruder := owner
	intruder.CustomerAccountID = value(t, domain.NewCustomerAccountID, "customer-elsewhere")
	probe := application.ValidateCommercialBasisCommand{Caller: intruder, Resolution: resolution}

	existing, err := application.NewValidateCommercialBasisHandler(store, authority, fixedClock{at: revalidatedAt}).
		Handle(context.Background(), probe)
	if err != nil {
		t.Fatalf("探测一份真实存在的解析: %v", err)
	}

	// 空的取回端口交回「没找到」，代表这个标识从未签发过。
	absent, err := application.NewValidateCommercialBasisHandler(
		&resolutionStoreDouble{}, authority, fixedClock{at: revalidatedAt},
	).Handle(context.Background(), probe)
	if err != nil {
		t.Fatalf("探测一个从未签发的标识: %v", err)
	}

	if existing.Closure().Outcome() != absent.Closure().Outcome() {
		t.Fatalf(
			"存在但不属于你 = %q，从未签发 = %q；两者可分即可枚举同租户下其他客户账户的解析",
			existing.Closure().Outcome(), absent.Closure().Outcome(),
		)
	}
	if existing.Closure().Reason() != absent.Closure().Reason() {
		t.Fatalf(
			"原因分别是 %q 与 %q；取值相同而原因不同，一样把存在与否说了出去",
			existing.Closure().Reason(), absent.Closure().Reason(),
		)
	}
}

// Covers: `AT-PC-014` 应用半边（回指/重校验）：错租户只切换 Caller.TenantID，与从未签发同形。
// 对象半边（登记册键含 TenantID）见 domain `TestCrossTenantSameObjectVersionNeitherReplaysNorLeaks`。
// 机制与 AT-PC-028 同支（TenantID 与 CustomerAccountID 一并比对）。
func TestARevalidationNamedByAnotherTenantIsNotAccepted(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	authority := &authorityDouble{registry: registry}
	prior := resolvedClosure(t, authority, "scope-a")

	store, owner, resolution := storedResolution(t, prior)
	intruder := owner
	intruder.TenantID = value(t, domain.NewTenantID, "tenant-elsewhere")

	authority.loadCalled = 0
	revalidated, err := application.NewValidateCommercialBasisHandler(store, authority, fixedClock{at: revalidatedAt}).
		Handle(context.Background(), application.ValidateCommercialBasisCommand{Caller: intruder, Resolution: resolution})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if revalidated.Closure().Outcome() != domain.BasisNotResolved {
		t.Fatalf("outcome = %q, want BASIS_NOT_RESOLVED——另一个租户凭标识重校验了这份解析", revalidated.Closure().Outcome())
	}
	if len(revalidated.Closure().Adopted()) != 0 {
		t.Fatal("错租户却仍交回了已采用依据")
	}
	if authority.loadCalled != 0 {
		t.Fatal("错租户却仍去读了权威视图")
	}
}

func TestAProbeCannotTellAMissingResolutionFromOneOwnedByAnotherTenant(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	authority := &authorityDouble{registry: registry}
	prior := resolvedClosure(t, authority, "scope-a")

	store, owner, resolution := storedResolution(t, prior)
	intruder := owner
	intruder.TenantID = value(t, domain.NewTenantID, "tenant-elsewhere")
	probe := application.ValidateCommercialBasisCommand{Caller: intruder, Resolution: resolution}

	existing, err := application.NewValidateCommercialBasisHandler(store, authority, fixedClock{at: revalidatedAt}).
		Handle(context.Background(), probe)
	if err != nil {
		t.Fatalf("探测一份真实存在的解析: %v", err)
	}
	absent, err := application.NewValidateCommercialBasisHandler(
		&resolutionStoreDouble{}, authority, fixedClock{at: revalidatedAt},
	).Handle(context.Background(), probe)
	if err != nil {
		t.Fatalf("探测一个从未签发的标识: %v", err)
	}
	if existing.Closure().Outcome() != absent.Closure().Outcome() {
		t.Fatalf(
			"存在但不属于你的租户 = %q，从未签发 = %q；两者可分即可跨租户枚举解析",
			existing.Closure().Outcome(), absent.Closure().Outcome(),
		)
	}
	if existing.Closure().Reason() != absent.Closure().Reason() {
		t.Fatalf(
			"原因分别是 %q 与 %q；取值相同而原因不同，一样把存在与否说了出去",
			existing.Closure().Reason(), absent.Closure().Reason(),
		)
	}
}
