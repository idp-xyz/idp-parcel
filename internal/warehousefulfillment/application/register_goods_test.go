package application_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 本文件证商品登记用例（UC-WF-002 的商品修订，不涉及接入面）：判定次序、八个结果字面量、同一条修订序列、识别码与组成，以及并发下的定论。
// 册、仓与库存用内存替身，真库上的约束另在 adapters/postgres 的用例里证。租户、货主与商品都是隔离合成值，只记 `S`。

var errGoodsRead = errors.New("goods read failed")

// fakeGoodsRegistry 是商品登记册的内存替身。保存把判重、修订号接得上、识别码不撞串在锁里，与端口要求的次序一致：同一个键先按内容答，
// 接不上的修订号答基准过期，再是别的商品的当前修订占着的码。afterSubmissionLookup 与 beforeSave 让并发用例停在指定的窗口。
type fakeGoodsRegistry struct {
	mutex                 sync.Mutex
	rows                  []domain.GoodsRevision
	saves                 int
	loadSubmissionErr     error
	loadLatestErr         error
	findErr               error
	saveErr               error
	afterSubmissionLookup func()
	beforeSave            func()
}

func newFakeGoodsRegistry() *fakeGoodsRegistry {
	return &fakeGoodsRegistry{}
}

func (fake *fakeGoodsRegistry) LoadLatestGoods(
	_ context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
) (domain.GoodsRevision, bool, error) {
	return fake.loadLatest(tenant, owner, code)
}

func (fake *fakeGoodsRegistry) loadLatest(
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
) (domain.GoodsRevision, bool, error) {
	if fake.loadLatestErr != nil {
		return domain.GoodsRevision{}, false, fake.loadLatestErr
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	latest, has := fake.latestLocked(tenant, owner, code)
	return latest, has, nil
}

func (fake *fakeGoodsRegistry) latestLocked(
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
) (domain.GoodsRevision, bool) {
	var latest domain.GoodsRevision
	var has bool
	for _, row := range fake.rows {
		if row.Tenant() != tenant || row.Owner() != owner || row.Code() != code {
			continue
		}
		if !has || row.Revision() > latest.Revision() {
			latest, has = row, true
		}
	}
	return latest, has
}

func (fake *fakeGoodsRegistry) LoadGoodsRevisionBySubmission(
	_ context.Context,
	tenant domain.TenantID,
	submission domain.GoodsSubmission,
) (domain.GoodsRevision, bool, error) {
	if fake.loadSubmissionErr != nil {
		return domain.GoodsRevision{}, false, fake.loadSubmissionErr
	}
	fake.mutex.Lock()
	var found domain.GoodsRevision
	var has bool
	for _, row := range fake.rows {
		if row.Tenant() == tenant && row.Submission().SameSlotAs(submission) {
			found, has = row, true
		}
	}
	fake.mutex.Unlock()
	if fake.afterSubmissionLookup != nil {
		fake.afterSubmissionLookup()
	}
	return found, has, nil
}

func (fake *fakeGoodsRegistry) FindGoodsIdentifierConflict(
	_ context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
	claims []domain.IdentifierClaim,
) (domain.IdentifierCode, bool, error) {
	if fake.findErr != nil {
		return domain.IdentifierCode{}, false, fake.findErr
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	taken, collides := fake.conflictLocked(tenant, owner, code, claims)
	return taken, collides, nil
}

func (fake *fakeGoodsRegistry) conflictLocked(
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
	claims []domain.IdentifierClaim,
) (domain.IdentifierCode, bool) {
	for _, claim := range claims {
		if claim.Validity().Inverted() {
			continue
		}
		for _, row := range fake.rows {
			if row.Tenant() != tenant || row.Owner() != owner || row.Code() == code {
				continue
			}
			latest, has := fake.latestLocked(tenant, owner, row.Code())
			if !has || latest.Revision() != row.Revision() {
				continue
			}
			for _, held := range latest.IdentifierClaims() {
				if held.Validity().Inverted() {
					continue
				}
				if claim.Collides(held) {
					return claim.Code(), true
				}
			}
		}
	}
	return domain.IdentifierCode{}, false
}

func (fake *fakeGoodsRegistry) SaveGoodsRevision(
	_ context.Context,
	revision domain.GoodsRevision,
) (ports.GoodsSaveResult, error) {
	if fake.beforeSave != nil {
		fake.beforeSave()
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.saves++
	if fake.saveErr != nil {
		return ports.GoodsSaveResult{}, fake.saveErr
	}
	for _, row := range fake.rows {
		if row.Tenant() != revision.Tenant() || !row.Submission().SameSlotAs(revision.Submission()) {
			continue
		}
		if row.Request().SameAs(revision.Request()) {
			return ports.GoodsSaveResult{Outcome: ports.GoodsRevisionAlreadyRegistered}, nil
		}
		return ports.GoodsSaveResult{Outcome: ports.GoodsSourceConflict}, nil
	}
	latest, has := fake.latestLocked(revision.Tenant(), revision.Owner(), revision.Code())
	want := 1
	if has {
		want = latest.Revision() + 1
	}
	if revision.Revision() != want {
		return ports.GoodsSaveResult{Outcome: ports.GoodsBasisStale}, nil
	}
	taken, collides := fake.conflictLocked(revision.Tenant(), revision.Owner(), revision.Code(), revision.IdentifierClaims())
	if collides {
		return ports.GoodsSaveResult{Outcome: ports.GoodsIdentifierConflict, ConflictingCode: taken}, nil
	}
	fake.rows = append(fake.rows, revision)
	return ports.GoodsSaveResult{Outcome: ports.GoodsRevisionSaved}, nil
}

func (fake *fakeGoodsRegistry) seed(revision domain.GoodsRevision) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.rows = append(fake.rows, revision)
}

func (fake *fakeGoodsRegistry) revisions(owner, code string) []domain.GoodsRevision {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	var out []domain.GoodsRevision
	for _, row := range fake.rows {
		if row.Tenant().String() == "SYN-TENANT-01" && row.Owner().String() == owner && row.Code().String() == code {
			out = append(out, row)
		}
	}
	return out
}

func (fake *fakeGoodsRegistry) saveCount() int {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return fake.saves
}

// fakeGoodsStock 回答有没有在库库存。asked 在并发用例里会被两个 goroutine 同时加，用原子计数。
type fakeGoodsStock struct {
	onHand bool
	err    error
	asked  atomic.Int32
}

func (fake *fakeGoodsStock) HasOnHandStock(
	_ context.Context,
	_ domain.TenantID,
	_ domain.OwnerAccountID,
	_ domain.GoodsCode,
) (bool, error) {
	fake.asked.Add(1)
	if fake.err != nil {
		return false, fake.err
	}
	return fake.onHand, nil
}

func goodsLevel(t *testing.T, name string, factor int64) domain.PackagingLevel {
	t.Helper()
	return domain.NewPackagingLevel(valueOf(t, domain.NewGoodsUnitName, name), factor)
}

func goodsAlias(t *testing.T, code string, kind domain.GoodsAliasKind, platform string, from, to time.Time) domain.GoodsAlias {
	t.Helper()
	var named domain.PlatformName
	if platform != "" {
		named = valueOf(t, domain.NewPlatformName, platform)
	}
	return domain.NewGoodsAlias(valueOf(t, domain.NewGoodsAliasCode, code), kind, named, domain.NewAliasValidity(from, to))
}

func goodsContent(t *testing.T, edit func(*domain.GoodsContentSpec)) domain.GoodsContent {
	t.Helper()
	spec := domain.GoodsContentSpec{
		Name:     valueOf(t, domain.NewGoodsName, "合成商品"),
		BaseUnit: valueOf(t, domain.NewGoodsUnitName, "件"),
		PackagingLevels: []domain.PackagingLevel{
			goodsLevel(t, "件", 1), goodsLevel(t, "箱", 24),
		},
	}
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewGoodsContent(spec)
	if err != nil {
		t.Fatalf("NewGoodsContent: %v", err)
	}
	return content
}

func ownerSubmission(t *testing.T, key string) domain.GoodsSubmission {
	t.Helper()
	submission, err := domain.NewOwnerSubmission(
		valueOf(t, domain.NewSubmissionSource, "SYN-CHANNEL-01"), valueOf(t, domain.NewRequestKey, key))
	if err != nil {
		t.Fatalf("NewOwnerSubmission: %v", err)
	}
	return submission
}

func goodsCommand(t *testing.T, code, key string, basis domain.BasisRevision, edit func(*domain.GoodsContentSpec)) application.RegisterGoodsCommand {
	t.Helper()
	return application.RegisterGoodsCommand{
		Tenant:     valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		Owner:      valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		Code:       valueOf(t, domain.NewGoodsCode, code),
		Basis:      basis,
		Content:    goodsContent(t, edit),
		Submission: ownerSubmission(t, key),
		Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
	}
}

func goodsRevisionOf(t *testing.T, command application.RegisterGoodsCommand, number int, at time.Time) domain.GoodsRevision {
	t.Helper()
	revision, err := domain.NewGoodsRevision(
		command.Tenant, command.Owner, command.Code, number, command.Content, command.Submission, command.Registrant, at)
	if err != nil {
		t.Fatalf("NewGoodsRevision: %v", err)
	}
	return revision
}

func goodsStores() (*fakeGoodsRegistry, *fakeTenantWarehouses, *fakeGoodsStock) {
	return newFakeGoodsRegistry(), newFakeTenantWarehouses("SYN-TENANT-01"), &fakeGoodsStock{}
}

type fakeOwnerAccounts struct {
	registered bool
	err        error
	asked      atomic.Int32
}

func (fake *fakeOwnerAccounts) OwnerAccountRegistered(context.Context, domain.TenantID, domain.OwnerAccountID) (bool, error) {
	fake.asked.Add(1)
	if fake.err != nil {
		return false, fake.err
	}
	return fake.registered, nil
}

func registeredOwners() *fakeOwnerAccounts {
	return &fakeOwnerAccounts{registered: true}
}

func newGoodsHandler(
	registry *fakeGoodsRegistry,
	warehouses *fakeTenantWarehouses,
	stock *fakeGoodsStock,
) *application.RegisterGoodsHandler {
	return application.NewRegisterGoodsHandler(registry, warehouses, stock, registeredOwners(), &fixedClock{at: registrationInstant})
}

func mustRegisterGoods(
	t *testing.T,
	handler *application.RegisterGoodsHandler,
	command application.RegisterGoodsCommand,
) application.RegisterGoodsResult {
	t.Helper()
	result, err := handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("Register：业务格不该走 error：%v", err)
	}
	return result
}

func expectGoodsOutcome(t *testing.T, label string, result application.RegisterGoodsResult, want application.RegisterGoodsOutcome) {
	t.Helper()
	if result.Outcome() != want {
		t.Fatalf("%s = %s，want %s", label, result.Outcome(), want)
	}
}

func cellPaths(cells []domain.GoodsCell) []string {
	paths := make([]string, len(cells))
	for index, cell := range cells {
		paths[index] = cell.String()
	}
	return paths
}

// Covers: AT-WF-014（商品的核心）——首次登记落修订 1；同一请求换了登记责任方仍是已有结果，回显当初落册的那一笔；货主口与代录口写同一条修订序列。
func TestRegisterGoodsRecordsTheFirstRevisionAndReplaysIt(t *testing.T) {
	registry, warehouses, stock := goodsStores()
	handler := newGoodsHandler(registry, warehouses, stock)
	first := mustRegisterGoods(t, handler, goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil))
	expectGoodsOutcome(t, "首次", first, application.GoodsOutcomeRegistered)
	landed, has := first.Revision()
	if !has || landed.Revision() != 1 || landed.Registrant().String() != "operator-1" ||
		!landed.RegisteredAt().Equal(registrationInstant.Truncate(time.Microsecond)) {
		t.Fatalf("落册的修订 = %+v（%v）", landed, has)
	}

	replay := goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil)
	replay.Registrant = valueOf(t, domain.NewRegistrant, "operator-9")
	again := mustRegisterGoods(t, handler, replay)
	expectGoodsOutcome(t, "重投", again, application.GoodsOutcomeExistingResult)
	echoed, has := again.Revision()
	if !has || echoed.Registrant().String() != "operator-1" || echoed.Revision() != 1 {
		t.Fatalf("回显 = 修订 %d、责任方 %q（%v），want 当初那一笔", echoed.Revision(), echoed.Registrant(), has)
	}
	if len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-01")) != 1 {
		t.Fatalf("重投又落了一笔")
	}

	onBehalf, err := domain.NewOnBehalfSubmission(
		valueOf(t, domain.NewRequestKey, "DEC-1"), valueOf(t, domain.NewEvidenceReference, "AUTH-1"))
	if err != nil {
		t.Fatal(err)
	}
	second := goodsCommand(t, "SYN-GOODS-01", "REQ-2", domain.NewBasisRevision(1), func(spec *domain.GoodsContentSpec) {
		spec.Name = valueOf(t, domain.NewGoodsName, "改了名称")
	})
	second.Submission = onBehalf
	recorded := mustRegisterGoods(t, handler, second)
	expectGoodsOutcome(t, "代录下一笔", recorded, application.GoodsOutcomeRegistered)
	next, has := recorded.Revision()
	evidence, hasEvidence := next.Submission().EvidenceReference()
	if !has || next.Revision() != 2 || next.Submission().Path() != domain.SubmissionOnBehalf || !hasEvidence || evidence.String() != "AUTH-1" {
		t.Fatalf("代录修订 = %d、途径 %s、依据 %q（%v）", next.Revision(), next.Submission().Path(), evidence, hasEvidence)
	}
	if stock.asked.Load() != 0 {
		t.Fatalf("没改跟踪属性，不该问库存，问了 %d 次", stock.asked.Load())
	}
}

// Covers: AT-WF-014（商品的核心）——同一键换内容答来源冲突，在册的不变；同一键换货主、换依据引用也是内容不同。
func TestRegisterGoodsRejectsADifferentPayloadOnTheSameKey(t *testing.T) {
	registry, warehouses, stock := goodsStores()
	handler := newGoodsHandler(registry, warehouses, stock)
	mustRegisterGoods(t, handler, goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil))

	renamed := goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, func(spec *domain.GoodsContentSpec) {
		spec.Name = valueOf(t, domain.NewGoodsName, "另一份")
	})
	conflict := mustRegisterGoods(t, handler, renamed)
	expectGoodsOutcome(t, "换名称", conflict, application.GoodsOutcomeSourceConflict)
	if _, has := conflict.Revision(); has || len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-01")) != 1 {
		t.Fatalf("来源冲突不该回显修订，册也不该变")
	}

	otherOwner := goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil)
	otherOwner.Owner = valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-02")
	expectGoodsOutcome(t, "换货主", mustRegisterGoods(t, handler, otherOwner), application.GoodsOutcomeSourceConflict)

	onBehalf, err := domain.NewOnBehalfSubmission(
		valueOf(t, domain.NewRequestKey, "DEC-1"), valueOf(t, domain.NewEvidenceReference, "AUTH-1"))
	if err != nil {
		t.Fatal(err)
	}
	behalf := goodsCommand(t, "SYN-GOODS-02", "DEC-1", domain.BasisRevision{}, nil)
	behalf.Submission = onBehalf
	mustRegisterGoods(t, handler, behalf)
	behalf.Submission, err = domain.NewOnBehalfSubmission(
		valueOf(t, domain.NewRequestKey, "DEC-1"), valueOf(t, domain.NewEvidenceReference, "AUTH-2"))
	if err != nil {
		t.Fatal(err)
	}
	expectGoodsOutcome(t, "换依据引用", mustRegisterGoods(t, handler, behalf), application.GoodsOutcomeSourceConflict)
	if len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-02")) != 1 {
		t.Fatalf("换依据引用不该再落一笔")
	}
}

// Covers: AT-WF-024（商品的核心）——一个仓都没登记答未配置，不读商品册、不落册。
func TestRegisterGoodsAnswersNotConfiguredBeforeTouchingTheRegister(t *testing.T) {
	registry := newFakeGoodsRegistry()
	registry.loadSubmissionErr = errGoodsRead
	handler := newGoodsHandler(registry, newFakeTenantWarehouses(), &fakeGoodsStock{})
	result := mustRegisterGoods(t, handler, goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil))
	expectGoodsOutcome(t, "没登记过仓", result, application.GoodsOutcomeNotConfigured)
	if registry.saveCount() != 0 || len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-01")) != 0 {
		t.Fatalf("未配置不该落册")
	}
}

// Covers: AT-WF-015（商品的核心）——基准不是当前修订、没带基准去改已登记的商品、带了基准去登记还没有的商品，都答基准过期；回显如实说当前修订号或没登记。
func TestRegisterGoodsRejectsAStaleBasisAndEchoesTheCurrentRevision(t *testing.T) {
	registry, warehouses, stock := goodsStores()
	handler := newGoodsHandler(registry, warehouses, stock)
	mustRegisterGoods(t, handler, goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil))
	mustRegisterGoods(t, handler, goodsCommand(t, "SYN-GOODS-01", "REQ-2", domain.NewBasisRevision(1), func(spec *domain.GoodsContentSpec) {
		spec.Name = valueOf(t, domain.NewGoodsName, "第二笔")
	}))

	for _, test := range []struct {
		name           string
		code           string
		key            string
		basis          domain.BasisRevision
		wantRegistered bool
		wantRevision   int
	}{
		{"基于旧修订", "SYN-GOODS-01", "REQ-3", domain.NewBasisRevision(1), true, 2},
		{"已登记却不带基准", "SYN-GOODS-01", "REQ-4", domain.BasisRevision{}, true, 2},
		{"没登记却带了基准", "SYN-GOODS-02", "REQ-5", domain.NewBasisRevision(1), false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := mustRegisterGoods(t, handler, goodsCommand(t, test.code, test.key, test.basis, func(spec *domain.GoodsContentSpec) {
				spec.Name = valueOf(t, domain.NewGoodsName, "不该落册")
			}))
			expectGoodsOutcome(t, test.name, result, application.GoodsOutcomeBasisStale)
			current := result.CurrentBasis()
			if current.Registered() != test.wantRegistered || current.Revision() != test.wantRevision {
				t.Fatalf("回显 = 已登记 %v、修订 %d，want 已登记 %v、修订 %d", current.Registered(), current.Revision(), test.wantRegistered, test.wantRevision)
			}
		})
	}
	if len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-01")) != 2 || len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-02")) != 0 {
		t.Fatalf("基准过期不该落册")
	}
}

// 同一键的竞态窗口：查过键、还没核基准时，别人用同一个键先落了册。重放排在基准之前，这一笔按内容答已有结果或来源冲突，不答基准过期。
func TestRegisterGoodsRevisitsTheRequestKeyBeforeAnsweringBasisStale(t *testing.T) {
	for name, same := range map[string]bool{"内容相同": true, "内容不同": false} {
		t.Run(name, func(t *testing.T) {
			registry, warehouses, stock := goodsStores()
			command := goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil)
			competitor := command
			competitor.Registrant = valueOf(t, domain.NewRegistrant, "operator-2")
			if !same {
				competitor.Content = goodsContent(t, func(spec *domain.GoodsContentSpec) {
					spec.Name = valueOf(t, domain.NewGoodsName, "别人先落的")
				})
			}
			landed := false
			registry.afterSubmissionLookup = func() {
				if !landed {
					landed = true
					registry.seed(goodsRevisionOf(t, competitor, 1, registrationInstant.Add(-time.Minute)))
				}
			}
			result := mustRegisterGoods(t, newGoodsHandler(registry, warehouses, stock), command)
			want := application.GoodsOutcomeSourceConflict
			if same {
				want = application.GoodsOutcomeExistingResult
			}
			expectGoodsOutcome(t, name, result, want)
			if !landed || len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-01")) != 1 {
				t.Fatalf("竞争者落册 %v、册里 %d 笔", landed, len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-01")))
			}
			if same {
				echoed, has := result.Revision()
				if !has || echoed.Registrant().String() != "operator-2" {
					t.Fatalf("回显的登记责任方 = %q（%v），want operator-2", echoed.Registrant(), has)
				}
			}
		})
	}
}

// Covers: AT-WF-015（商品的核心）——两笔并发修订都基于同一修订（这里是都来做首次登记），至多一笔落册，另一笔答基准过期。
func TestRegisterGoodsKeepsOneLandingWhenTwoRevisionsShareABasis(t *testing.T) {
	registry, warehouses, stock := goodsStores()
	meeting := newRendezvous(2)
	registry.beforeSave = meeting.arrive
	handler := newGoodsHandler(registry, warehouses, stock)
	commands := []application.RegisterGoodsCommand{
		goodsCommand(t, "SYN-GOODS-01", "REQ-A", domain.BasisRevision{}, nil),
		goodsCommand(t, "SYN-GOODS-01", "REQ-B", domain.BasisRevision{}, func(spec *domain.GoodsContentSpec) {
			spec.Name = valueOf(t, domain.NewGoodsName, "并发的另一份")
		}),
	}
	results := make([]application.RegisterGoodsResult, 2)
	errs := make([]error, 2)
	var wait sync.WaitGroup
	for index := range commands {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index], errs[index] = handler.Register(t.Context(), commands[index])
		}()
	}
	wait.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("第 %d 笔：%v", index+1, err)
		}
	}
	if !meeting.met {
		t.Fatalf("两笔没有同时停在保存之前")
	}
	registered := 0
	for _, result := range results {
		switch result.Outcome() {
		case application.GoodsOutcomeRegistered:
			registered++
		case application.GoodsOutcomeBasisStale:
			if !result.CurrentBasis().Registered() || result.CurrentBasis().Revision() != 1 {
				t.Fatalf("基准过期回显 = 已登记 %v、修订 %d，want 修订 1", result.CurrentBasis().Registered(), result.CurrentBasis().Revision())
			}
		default:
			t.Fatalf("并发修订的结果 = %s", result.Outcome())
		}
	}
	if registered != 1 || len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-01")) != 1 {
		t.Fatalf("落册 %d 笔、结果里登记 %d 笔，want 各 1", len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-01")), registered)
	}
}

// 两个商品并发抢同一个码，至多一个落册，另一个答识别码冲突并指出那个码。
func TestRegisterGoodsKeepsOneLandingWhenTwoGoodsClaimOneCode(t *testing.T) {
	registry, warehouses, stock := goodsStores()
	meeting := newRendezvous(2)
	registry.beforeSave = meeting.arrive
	handler := newGoodsHandler(registry, warehouses, stock)
	alias := func(spec *domain.GoodsContentSpec) {
		spec.Aliases = []domain.GoodsAlias{goodsAlias(t, "SYN-SHARED", domain.GoodsAliasBarcode, "", time.Time{}, time.Time{})}
	}
	commands := []application.RegisterGoodsCommand{
		goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, alias),
		goodsCommand(t, "SYN-GOODS-02", "REQ-2", domain.BasisRevision{}, alias),
	}
	results := make([]application.RegisterGoodsResult, 2)
	errs := make([]error, 2)
	var wait sync.WaitGroup
	for index := range commands {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index], errs[index] = handler.Register(t.Context(), commands[index])
		}()
	}
	wait.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("第 %d 笔：%v", index+1, err)
		}
	}
	if !meeting.met {
		t.Fatalf("两笔没有同时停在保存之前")
	}
	registered := 0
	for _, result := range results {
		switch result.Outcome() {
		case application.GoodsOutcomeRegistered:
			registered++
		case application.GoodsOutcomeIdentifierConflict:
			code, has := result.ConflictingCode()
			if !has || code.String() != "SYN-SHARED" {
				t.Fatalf("指出的码 = %q（%v），want SYN-SHARED", code, has)
			}
		default:
			t.Fatalf("抢码的结果 = %s", result.Outcome())
		}
	}
	kept := len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-01")) + len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-02"))
	if registered != 1 || kept != 1 {
		t.Fatalf("落册 %d 笔、结果里登记 %d 笔，want 各 1", kept, registered)
	}
}

// 同一键并发两笔，至多一笔落册，另一笔按内容答已有结果或来源冲突。
func TestRegisterGoodsKeepsOneLandingWhenTwoRequestsShareAKey(t *testing.T) {
	for name, same := range map[string]bool{"内容相同": true, "内容不同": false} {
		t.Run(name, func(t *testing.T) {
			registry, warehouses, stock := goodsStores()
			meeting := newRendezvous(2)
			registry.beforeSave = meeting.arrive
			handler := newGoodsHandler(registry, warehouses, stock)
			commands := []application.RegisterGoodsCommand{
				goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil),
				goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil),
			}
			commands[1].Registrant = valueOf(t, domain.NewRegistrant, "operator-2")
			if !same {
				commands[1].Content = goodsContent(t, func(spec *domain.GoodsContentSpec) {
					spec.Name = valueOf(t, domain.NewGoodsName, "并发的另一份")
				})
			}
			results := make([]application.RegisterGoodsResult, 2)
			errs := make([]error, 2)
			var wait sync.WaitGroup
			for index := range commands {
				wait.Add(1)
				go func() {
					defer wait.Done()
					results[index], errs[index] = handler.Register(t.Context(), commands[index])
				}()
			}
			wait.Wait()
			for index, err := range errs {
				if err != nil {
					t.Fatalf("第 %d 笔：%v", index+1, err)
				}
			}
			if !meeting.met {
				t.Fatalf("两笔没有同时停在保存之前")
			}
			wantLoser := application.GoodsOutcomeSourceConflict
			if same {
				wantLoser = application.GoodsOutcomeExistingResult
			}
			registered := 0
			for _, result := range results {
				switch result.Outcome() {
				case application.GoodsOutcomeRegistered:
					registered++
				case wantLoser:
					if same {
						echoed, has := result.Revision()
						if !has || (echoed.Registrant().String() != "operator-1" && echoed.Registrant().String() != "operator-2") {
							t.Fatalf("回显的登记责任方 = %q（%v），want 已落册那一笔", echoed.Registrant(), has)
						}
					}
				default:
					t.Fatalf("同一键的结果 = %s，want REGISTERED 或 %s", result.Outcome(), wantLoser)
				}
			}
			if registered != 1 || len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-01")) != 1 {
				t.Fatalf("落册 %d 笔、结果里登记 %d 笔，want 各 1", len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-01")), registered)
			}
		})
	}
}

// Covers: AT-WF-016（商品的核心）——结构上不可改的格指出那一格，册不变；追加包装层级照常登记。
func TestRegisterGoodsRefusesImmutableStructure(t *testing.T) {
	for name, test := range map[string]struct {
		edit      func(*domain.GoodsContentSpec)
		wantField domain.GoodsImmutableField
		wantLevel string
		register  bool
	}{
		"换基本单位": {func(spec *domain.GoodsContentSpec) {
			spec.BaseUnit = valueOf(t, domain.NewGoodsUnitName, "盒")
		}, domain.GoodsImmutableBaseUnit, "", false},
		"删掉一个层级": {func(spec *domain.GoodsContentSpec) {
			spec.PackagingLevels = []domain.PackagingLevel{goodsLevel(t, "件", 1)}
		}, domain.GoodsImmutablePackagingLevelRemoved, "箱", false},
		"改折合数": {func(spec *domain.GoodsContentSpec) {
			spec.PackagingLevels = []domain.PackagingLevel{goodsLevel(t, "件", 1), goodsLevel(t, "箱", 12)}
		}, domain.GoodsImmutablePackagingLevelFactor, "箱", false},
		"改成组合": {func(spec *domain.GoodsContentSpec) {
			spec.Composite = true
			spec.OutboundMode = domain.GoodsOutboundByComponent
			spec.Components = []domain.GoodsComponent{domain.NewGoodsComponent(valueOf(t, domain.NewGoodsCode, "SYN-PART"), 1)}
		}, domain.GoodsImmutableComposite, "", false},
		"追加一个层级": {func(spec *domain.GoodsContentSpec) {
			spec.PackagingLevels = append(spec.PackagingLevels, goodsLevel(t, "托", 480))
		}, 0, "", true},
	} {
		t.Run(name, func(t *testing.T) {
			registry, warehouses, stock := goodsStores()
			handler := newGoodsHandler(registry, warehouses, stock)
			mustRegisterGoods(t, handler, goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil))
			result := mustRegisterGoods(t, handler, goodsCommand(t, "SYN-GOODS-01", "REQ-2", domain.NewBasisRevision(1), test.edit))
			if test.register {
				expectGoodsOutcome(t, name, result, application.GoodsOutcomeRegistered)
				if len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-01")) != 2 {
					t.Fatalf("追加层级应落修订 2")
				}
				return
			}
			expectGoodsOutcome(t, name, result, application.GoodsOutcomeImmutableField)
			cell := result.ImmutableCell()
			level, named := cell.Level()
			if cell.Field() != test.wantField || (test.wantLevel == "" && named) || (test.wantLevel != "" && (!named || level.String() != test.wantLevel)) {
				t.Fatalf("指出的格 = %s、层 %q（%v）", cell.Field(), level, named)
			}
			if len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-01")) != 1 || stock.asked.Load() != 0 {
				t.Fatalf("结构格被拒仍落册，或问了库存 %d 次", stock.asked.Load())
			}
		})
	}
}

// Covers: AT-WF-016（商品的核心）——有在库库存时不能改是否跟踪批号；没有时可以，并落新修订。
func TestRegisterGoodsAllowsTrackingChangesOnlyWithoutOnHandStock(t *testing.T) {
	registry, warehouses, stock := goodsStores()
	stock.onHand = true
	handler := newGoodsHandler(registry, warehouses, stock)
	mustRegisterGoods(t, handler, goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil))
	blocked := mustRegisterGoods(t, handler, goodsCommand(t, "SYN-GOODS-01", "REQ-2", domain.NewBasisRevision(1), func(spec *domain.GoodsContentSpec) {
		spec.TrackBatch = true
	}))
	expectGoodsOutcome(t, "有库存", blocked, application.GoodsOutcomeImmutableField)
	if blocked.ImmutableCell().Field() != domain.GoodsImmutableTrackBatch || stock.asked.Load() != 1 {
		t.Fatalf("指出 %s，问库存 %d 次", blocked.ImmutableCell().Field(), stock.asked.Load())
	}
	if len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-01")) != 1 {
		t.Fatalf("有库存时不该落新修订")
	}

	stock.onHand = false
	allowed := mustRegisterGoods(t, handler, goodsCommand(t, "SYN-GOODS-01", "REQ-3", domain.NewBasisRevision(1), func(spec *domain.GoodsContentSpec) {
		spec.TrackExpiry = true
	}))
	expectGoodsOutcome(t, "没有库存", allowed, application.GoodsOutcomeRegistered)
	landed, has := allowed.Revision()
	if !has || landed.Revision() != 2 || !landed.Content().TrackExpiry() || stock.asked.Load() != 2 {
		t.Fatalf("没有库存应落修订 2 并再问一次库存，实得修订 %d、问了 %d 次", landed.Revision(), stock.asked.Load())
	}
}

// Covers: AT-WF-017（商品的核心）——别名撞上本货主另一个商品的编码或此刻有效的别名则指出那个码；区间不交叠、别的货主用同一个码，都登记。
func TestRegisterGoodsRefusesIdentifierCollisionsInsideOneOwner(t *testing.T) {
	registry, warehouses, stock := goodsStores()
	handler := newGoodsHandler(registry, warehouses, stock)
	mustRegisterGoods(t, handler, goodsCommand(t, "SYN-OTHER", "REQ-OTHER", domain.BasisRevision{}, func(spec *domain.GoodsContentSpec) {
		spec.Aliases = []domain.GoodsAlias{goodsAlias(t, "SYN-LIVE", domain.GoodsAliasBarcode, "", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))}
	}))

	held := mustRegisterGoods(t, handler, goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, func(spec *domain.GoodsContentSpec) {
		spec.Aliases = []domain.GoodsAlias{goodsAlias(t, "SYN-OTHER", domain.GoodsAliasBarcode, "", time.Time{}, time.Time{})}
	}))
	expectGoodsOutcome(t, "撞上商品编码", held, application.GoodsOutcomeIdentifierConflict)
	if code, has := held.ConflictingCode(); !has || code.String() != "SYN-OTHER" {
		t.Fatalf("指出的码 = %q（%v）", code, has)
	}

	live := mustRegisterGoods(t, handler, goodsCommand(t, "SYN-GOODS-01", "REQ-2", domain.BasisRevision{}, func(spec *domain.GoodsContentSpec) {
		spec.Aliases = []domain.GoodsAlias{goodsAlias(t, "SYN-LIVE", domain.GoodsAliasBarcode, "", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), time.Time{})}
	}))
	expectGoodsOutcome(t, "撞上有效别名", live, application.GoodsOutcomeIdentifierConflict)

	ended := mustRegisterGoods(t, handler, goodsCommand(t, "SYN-GOODS-01", "REQ-3", domain.BasisRevision{}, func(spec *domain.GoodsContentSpec) {
		spec.Aliases = []domain.GoodsAlias{goodsAlias(t, "SYN-LIVE", domain.GoodsAliasBarcode, "", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), time.Time{})}
	}))
	expectGoodsOutcome(t, "区间不交叠", ended, application.GoodsOutcomeRegistered)

	repeated := mustRegisterGoods(t, handler, goodsCommand(t, "SYN-GOODS-02", "REQ-4", domain.BasisRevision{}, func(spec *domain.GoodsContentSpec) {
		spec.Aliases = []domain.GoodsAlias{goodsAlias(t, "SYN-GOODS-02", domain.GoodsAliasBarcode, "", time.Time{}, time.Time{})}
	}))
	expectGoodsOutcome(t, "别名等于自己的编码", repeated, application.GoodsOutcomeIdentifierConflict)
	if len(registry.revisions("SYN-OWNER-01", "SYN-GOODS-02")) != 0 {
		t.Fatalf("同一份资料里的重复码不该落册")
	}

	otherOwner := goodsCommand(t, "SYN-OTHER", "REQ-5", domain.BasisRevision{}, nil)
	otherOwner.Owner = valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-02")
	expectGoodsOutcome(t, "别的货主", mustRegisterGoods(t, handler, otherOwner), application.GoodsOutcomeRegistered)
}

// Covers: AT-WF-018（商品的核心）——组件须是本货主已登记的非组合商品、数量为正；之后改组成与出库方式落新修订。
func TestRegisterGoodsChecksCompositionAgainstCurrentRevisions(t *testing.T) {
	registry, warehouses, stock := goodsStores()
	handler := newGoodsHandler(registry, warehouses, stock)
	mustRegisterGoods(t, handler, goodsCommand(t, "SYN-PART", "REQ-PART", domain.BasisRevision{}, nil))

	kit := goodsCommand(t, "SYN-KIT", "REQ-KIT", domain.BasisRevision{}, func(spec *domain.GoodsContentSpec) {
		spec.Composite = true
		spec.OutboundMode = domain.GoodsOutboundByComponent
		spec.Components = []domain.GoodsComponent{domain.NewGoodsComponent(valueOf(t, domain.NewGoodsCode, "SYN-PART"), 2)}
	})
	expectGoodsOutcome(t, "组件成立", mustRegisterGoods(t, handler, kit), application.GoodsOutcomeRegistered)

	revised := goodsCommand(t, "SYN-KIT", "REQ-KIT-2", domain.NewBasisRevision(1), func(spec *domain.GoodsContentSpec) {
		spec.Composite = true
		spec.OutboundMode = domain.GoodsOutboundAsFinishedGoods
		spec.Components = []domain.GoodsComponent{domain.NewGoodsComponent(valueOf(t, domain.NewGoodsCode, "SYN-PART"), 3)}
	})
	next := mustRegisterGoods(t, handler, revised)
	expectGoodsOutcome(t, "改组成", next, application.GoodsOutcomeRegistered)
	if landed, has := next.Revision(); !has || landed.Revision() != 2 || len(registry.revisions("SYN-OWNER-01", "SYN-KIT")) != 2 {
		t.Fatalf("改组成应留下两笔修订")
	}

	for name, test := range map[string]struct {
		edit func(*domain.GoodsContentSpec)
		want string
	}{
		"组件是组合商品": {func(spec *domain.GoodsContentSpec) {
			spec.Composite = true
			spec.OutboundMode = domain.GoodsOutboundByComponent
			spec.Components = []domain.GoodsComponent{domain.NewGoodsComponent(valueOf(t, domain.NewGoodsCode, "SYN-KIT"), 1)}
		}, "composition.components[0].code"},
		"组件没登记": {func(spec *domain.GoodsContentSpec) {
			spec.Composite = true
			spec.OutboundMode = domain.GoodsOutboundByComponent
			spec.Components = []domain.GoodsComponent{domain.NewGoodsComponent(valueOf(t, domain.NewGoodsCode, "SYN-MISSING"), 1)}
		}, "composition.components[0].code"},
		"数量为零": {func(spec *domain.GoodsContentSpec) {
			spec.Composite = true
			spec.OutboundMode = domain.GoodsOutboundByComponent
			spec.Components = []domain.GoodsComponent{domain.NewGoodsComponent(valueOf(t, domain.NewGoodsCode, "SYN-PART"), 0)}
		}, "composition.components[0].quantity"},
		"非组合带了组成": {func(spec *domain.GoodsContentSpec) {
			spec.Components = []domain.GoodsComponent{domain.NewGoodsComponent(valueOf(t, domain.NewGoodsCode, "SYN-PART"), 1)}
		}, "composition.components"},
	} {
		t.Run(name, func(t *testing.T) {
			result := mustRegisterGoods(t, handler, goodsCommand(t, "SYN-BAD", "REQ-"+name, domain.BasisRevision{}, test.edit))
			expectGoodsOutcome(t, name, result, application.GoodsOutcomeContentInvalid)
			if got := cellPaths(result.InvalidCells()); !slices.Contains(got, test.want) {
				t.Fatalf("指出的格 = %v，want 含 %s", got, test.want)
			}
		})
	}
	if len(registry.revisions("SYN-OWNER-01", "SYN-BAD")) != 0 {
		t.Fatalf("组成不成立不该落册")
	}
}

// Covers: AT-WF-019（商品的核心）——资料内容不合并逐格指出，不落册。几处一起不合时全部列出，次序固定。
func TestRegisterGoodsListsEveryInvalidCell(t *testing.T) {
	registry, warehouses, stock := goodsStores()
	handler := newGoodsHandler(registry, warehouses, stock)

	for name, test := range map[string]struct {
		edit func(*domain.GoodsContentSpec)
		want []string
	}{
		"折合数不是正整数且层名重复": {func(spec *domain.GoodsContentSpec) {
			spec.PackagingLevels = []domain.PackagingLevel{goodsLevel(t, "件", 1), goodsLevel(t, "件", 0)}
		}, []string{"packagingLevels[1].name", "packagingLevels[1].factor"}},
		"基本单位不是折合为一的那一层": {func(spec *domain.GoodsContentSpec) {
			spec.PackagingLevels = []domain.PackagingLevel{goodsLevel(t, "件", 2), goodsLevel(t, "箱", 1)}
		}, []string{"baseUnit"}},
		"原产国与币种不在目录": {func(spec *domain.GoodsContentSpec) {
			declaration, err := domain.NewGoodsDeclaration(domain.GoodsDeclarationSpec{
				OriginCountry: valueOf(t, domain.NewCountryCode, "XX"),
				UnitPrice:     measureOf(t, "5.5"),
				Currency:      valueOf(t, domain.NewCurrencyCode, "ZZZ"),
			})
			if err != nil {
				t.Fatal(err)
			}
			spec.Declaration = declaration
		}, []string{"declaration.originCountry", "declaration.currency"}},
		"重量为零且单位类别不对": {func(spec *domain.GoodsContentSpec) {
			weight, err := domain.NewDeclaredWeight(measureOf(t, "0"), valueOf(t, domain.NewUnitCode, "CM"))
			if err != nil {
				t.Fatal(err)
			}
			measurement, err := domain.NewDeclaredMeasurement(domain.DeclaredMeasurementSpec{
				Level: valueOf(t, domain.NewGoodsUnitName, "件"), Weight: &weight,
			})
			if err != nil {
				t.Fatal(err)
			}
			spec.DeclaredMeasurements = []domain.DeclaredMeasurement{measurement}
		}, []string{"declaredMeasurements[0].weight.value", "declaredMeasurements[0].weight.unit"}},
		"别名区间止不晚于起且条码带了平台名": {func(spec *domain.GoodsContentSpec) {
			spec.Aliases = []domain.GoodsAlias{goodsAlias(t, "SYN-BAR", domain.GoodsAliasBarcode, "SYN-PLATFORM",
				time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))}
		}, []string{"aliases[0].platform", "aliases[0].validity"}},
	} {
		t.Run(name, func(t *testing.T) {
			result := mustRegisterGoods(t, handler, goodsCommand(t, "SYN-BAD", "REQ-"+name, domain.BasisRevision{}, test.edit))
			expectGoodsOutcome(t, name, result, application.GoodsOutcomeContentInvalid)
			if got := cellPaths(result.InvalidCells()); !slices.Equal(got, test.want) {
				t.Fatalf("指出的格 = %v，want %v", got, test.want)
			}
		})
	}
	if registry.saveCount() != 0 {
		t.Fatalf("资料不合不该保存，保存了 %d 次", registry.saveCount())
	}
}

// 册或库存读不动是技术失败，不折成业务答复。装配缺了租户同样走 error。
func TestRegisterGoodsPropagatesReadFailures(t *testing.T) {
	command := goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil)
	command.Tenant = domain.TenantID{}
	if _, err := newGoodsHandler(newFakeGoodsRegistry(), newFakeTenantWarehouses("SYN-TENANT-01"), &fakeGoodsStock{}).Register(t.Context(), command); err == nil {
		t.Fatal("缺租户应走 error")
	}

	for name, install := range map[string]func(*fakeGoodsRegistry, *fakeGoodsStock){
		"认重放读不动":   func(registry *fakeGoodsRegistry, _ *fakeGoodsStock) { registry.loadSubmissionErr = errGoodsRead },
		"读当前修订读不动": func(registry *fakeGoodsRegistry, _ *fakeGoodsStock) { registry.loadLatestErr = errGoodsRead },
		"保存读不动":    func(registry *fakeGoodsRegistry, _ *fakeGoodsStock) { registry.saveErr = errGoodsRead },
	} {
		t.Run(name, func(t *testing.T) {
			registry, warehouses, stock := goodsStores()
			install(registry, stock)
			_, err := newGoodsHandler(registry, warehouses, stock).Register(t.Context(), goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil))
			if !errors.Is(err, errGoodsRead) {
				t.Fatalf("err = %v，want 包住读失败", err)
			}
		})
	}

	registry, warehouses, stock := goodsStores()
	handler := newGoodsHandler(registry, warehouses, stock)
	mustRegisterGoods(t, handler, goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil))
	stock.err = errGoodsRead
	_, err := handler.Register(t.Context(), goodsCommand(t, "SYN-GOODS-01", "REQ-2", domain.NewBasisRevision(1), func(spec *domain.GoodsContentSpec) {
		spec.TrackBatch = true
	}))
	if !errors.Is(err, errGoodsRead) {
		t.Fatalf("库存读不动 err = %v", err)
	}
}

// Covers: AT-WF-020（货主核对半边）——代录先问这个货主客户账户在本租户有没有登记，排在「有没有仓」之前。
// 查不到是已形成的答案，不读仓册、不落册；读不动走 error，不折成查不到；货主口不问这一口。
func TestRegisterGoodsOnBehalfChecksTheOwnerBeforeReadingTheWarehouse(t *testing.T) {
	onBehalf := func(t *testing.T) application.RegisterGoodsCommand {
		t.Helper()
		command := goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil)
		submission, err := domain.NewOnBehalfSubmission(
			valueOf(t, domain.NewRequestKey, "DEC-1"), valueOf(t, domain.NewEvidenceReference, "AUTH-1"))
		if err != nil {
			t.Fatal(err)
		}
		command.Submission = submission
		return command
	}

	t.Run("不在本租户", func(t *testing.T) {
		registry, warehouses, stock := goodsStores()
		owners := &fakeOwnerAccounts{}
		handler := application.NewRegisterGoodsHandler(registry, warehouses, stock, owners, &fixedClock{at: registrationInstant})
		_, err := handler.Register(t.Context(), onBehalf(t))
		if !errors.Is(err, application.ErrOwnerAccountNotInTenant) {
			t.Fatalf("err = %v，want 货主不在本租户", err)
		}
		if owners.asked.Load() != 1 || warehouses.asked.Load() != 0 || registry.saveCount() != 0 {
			t.Fatalf("问了货主 %d 次、仓 %d 次、保存 %d 次", owners.asked.Load(), warehouses.asked.Load(), registry.saveCount())
		}
	})

	t.Run("货主册读不动", func(t *testing.T) {
		registry, warehouses, stock := goodsStores()
		owners := &fakeOwnerAccounts{err: errGoodsRead}
		handler := application.NewRegisterGoodsHandler(registry, warehouses, stock, owners, &fixedClock{at: registrationInstant})
		_, err := handler.Register(t.Context(), onBehalf(t))
		if err == nil || errors.Is(err, application.ErrOwnerAccountNotInTenant) || !errors.Is(err, errGoodsRead) {
			t.Fatalf("err = %v，want 包住读失败，而不是查不到", err)
		}
		if warehouses.asked.Load() != 0 || registry.saveCount() != 0 {
			t.Fatal("读不动不该再读仓或落册")
		}
	})

	t.Run("货主口不问", func(t *testing.T) {
		registry, warehouses, stock := goodsStores()
		owners := &fakeOwnerAccounts{}
		handler := application.NewRegisterGoodsHandler(registry, warehouses, stock, owners, &fixedClock{at: registrationInstant})
		result := mustRegisterGoods(t, handler, goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil))
		expectGoodsOutcome(t, "货主口", result, application.GoodsOutcomeRegistered)
		if owners.asked.Load() != 0 {
			t.Fatalf("货主口问了货主核对 %d 次", owners.asked.Load())
		}
	})

	t.Run("在册才继续登记", func(t *testing.T) {
		registry, warehouses, stock := goodsStores()
		owners := registeredOwners()
		handler := application.NewRegisterGoodsHandler(registry, warehouses, stock, owners, &fixedClock{at: registrationInstant})
		result := mustRegisterGoods(t, handler, onBehalf(t))
		expectGoodsOutcome(t, "代录", result, application.GoodsOutcomeRegistered)
		if owners.asked.Load() != 1 {
			t.Fatalf("代录问了货主核对 %d 次", owners.asked.Load())
		}
	})
}
