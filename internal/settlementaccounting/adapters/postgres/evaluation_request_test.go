package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉票 sa-cc/08 完成判据 1 的登记册半边：评价请求真库往返（按铸造 ID 与按自然键两口读回同一份）；自然键
// 唯一约束**在库上**——另一个铸造 ID、同一组成分的第二次 Save 答`已存在`、先到者不被覆盖、输家的 ID 不落
// 册；成分不同就是另一份请求（裁决 2）；写口无事务拒。夹具全是合成串。

var (
	evaluationRequestedAt = time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC)
	evaluationOccurredAt  = time.Date(2026, 9, 11, 8, 0, 0, 0, time.UTC)
)

func newEvaluationRequestRegistry(t *testing.T) (*adapter.EvaluationRequests, bentoapp.Transactor) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry, err := adapter.NewEvaluationRequests(db)
	if err != nil {
		t.Fatalf("构造评价请求登记册：%v", err)
	}
	return registry, db.Transactor()
}

// evaluationRequestRecord 按（租户、铸造 ID、费用项目）合成一份登记记录；其余成分固定，让「同成分不同 ID」
// 与「不同成分」两格只差一个参数。
func evaluationRequestRecord(t *testing.T, tenant, id, feeItem string) ports.EvaluationRequestRecord {
	t.Helper()
	occurrence, err := domain.NewTransportChargeOccurrence(
		saValue(t, domain.NewChargeOccurrenceID, "syn-occ-1"),
		saValue(t, domain.NewOccurrenceReasonReference, "BOOKING"),
		saValue(t, domain.NewOccurrenceVersion, "v1"),
		evaluationOccurredAt,
	)
	if err != nil {
		t.Fatalf("发生项引用：%v", err)
	}
	request, err := domain.SubmitEvaluationRequest(domain.EvaluationRequestSpec{
		ID:      saValue(t, domain.NewEvaluationRequestID, id),
		Scope:   saValue(t, domain.NewPrimaryScopeReference, "syn-scope-1"),
		Purpose: domain.BuySupplierCost,
		Sources: domain.EligibleSourceReferences{
			Occurrence: occurrence,
			FeeItem:    saValue(t, domain.NewFeeItemReference, feeItem),
			Agreement:  saValue(t, domain.NewSupplierAgreementReference, "syn-agreement-1"),
		},
		RequestedAt: evaluationRequestedAt,
		RequestedBy: saValue(t, domain.NewRequesterReference, "syn-settlement-job"),
	})
	if err != nil {
		t.Fatalf("形成评价请求：%v", err)
	}
	return ports.EvaluationRequestRecord{
		Key:        ports.EvaluationRequestKey{TenantID: saTenant(t, tenant), Request: request.ID()},
		Request:    request,
		RecordedAt: evaluationRequestedAt.Add(time.Second),
	}
}

func saveEvaluationRequest(
	t *testing.T,
	transactor bentoapp.Transactor,
	registry *adapter.EvaluationRequests,
	record ports.EvaluationRequestRecord,
) ports.EvaluationRequestSaveOutcome {
	t.Helper()
	var outcome ports.EvaluationRequestSaveOutcome
	saWithin(t, transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = registry.Save(txCtx, record)
		return err
	})
	return outcome
}

// Covers: 判据 1「评价请求登记册真库往返」——两口读回的是同一份：铸造 ID、主要范围、计算目的、三件引用
// （发生项连原因 / 版本 / 业务时间）、请求方、请求时刻与登记时刻一个都不变形。
func TestAnEvaluationRequestRoundTripsByIDAndByNaturalKey(t *testing.T) {
	registry, transactor := newEvaluationRequestRegistry(t)
	record := evaluationRequestRecord(t, "tenant-a", "EVREQ-SYN-1", "syn-fee-1")

	if outcome := saveEvaluationRequest(t, transactor, registry, record); outcome != ports.EvaluationRequestSaved {
		t.Fatalf("首登结果 = %q, want SAVED", outcome)
	}

	byID, found, err := registry.FindByID(t.Context(), record.Key)
	if err != nil || !found {
		t.Fatalf("按 ID 读回：found=%v err=%v", found, err)
	}
	assertEvaluationRequestEquals(t, byID, record)

	byNaturalKey, found, err := registry.FindByNaturalKey(t.Context(), record.Key.TenantID, record.Request.NaturalKey())
	if err != nil || !found {
		t.Fatalf("按自然键读回：found=%v err=%v", found, err)
	}
	assertEvaluationRequestEquals(t, byNaturalKey, record)

	// 租户条件在 SQL 里：另一个租户探同一个 ID 与真不存在长得一样。
	otherTenant := record.Key
	otherTenant.TenantID = saTenant(t, "tenant-b")
	if _, found, err := registry.FindByID(t.Context(), otherTenant); err != nil || found {
		t.Fatalf("另一租户按同一 ID 读到了行：found=%v err=%v", found, err)
	}
}

// Covers: 裁决 2「自然键唯一约束在库上，重复提交答`已存在`并交回原 ID」——第二份带**另一个铸造 ID**、
// 同一组成分：Save 撞的是自然键那条唯一约束而不是主键；先到者原样在册、输家的 ID 不落册。
func TestASecondRequestWithTheSameNaturalKeyAnswersAlreadyRequestedAndKeepsTheWinner(t *testing.T) {
	registry, transactor := newEvaluationRequestRegistry(t)
	winner := evaluationRequestRecord(t, "tenant-a", "EVREQ-SYN-1", "syn-fee-1")
	loser := evaluationRequestRecord(t, "tenant-a", "EVREQ-SYN-2", "syn-fee-1")
	if winner.Request.NaturalKey() != loser.Request.NaturalKey() {
		t.Fatal("夹具错了：两份的自然键必须相同，本例测的正是这一格")
	}

	if outcome := saveEvaluationRequest(t, transactor, registry, winner); outcome != ports.EvaluationRequestSaved {
		t.Fatalf("先到者结果 = %q, want SAVED", outcome)
	}
	if outcome := saveEvaluationRequest(t, transactor, registry, loser); outcome != ports.EvaluationRequestAlreadyRequested {
		t.Fatalf("同自然键第二份结果 = %q, want ALREADY_REQUESTED", outcome)
	}

	found, exists, err := registry.FindByNaturalKey(t.Context(), winner.Key.TenantID, winner.Request.NaturalKey())
	if err != nil || !exists {
		t.Fatalf("按自然键读回先到者：exists=%v err=%v", exists, err)
	}
	if found.Key.Request != winner.Key.Request {
		t.Fatalf("自然键指向的 ID = %q, want 先到者 %q", found.Key.Request.String(), winner.Key.Request.String())
	}
	if _, exists, err := registry.FindByID(t.Context(), loser.Key); err != nil || exists {
		t.Fatalf("输家的 ID 落了册：exists=%v err=%v", exists, err)
	}

	// 同一个 ID 再登一次（重放同一份）撞的是主键，同样答`已存在`不覆盖。
	if outcome := saveEvaluationRequest(t, transactor, registry, winner); outcome != ports.EvaluationRequestAlreadyRequested {
		t.Fatalf("同 ID 重放结果 = %q, want ALREADY_REQUESTED", outcome)
	}
}

// Covers: 裁决 2「成分不同就是另一份请求、另一个 ID」——换一个费用项目，自然键不同，两份并存。
func TestADifferentSourceSetIsAnotherRequestInTheRegistry(t *testing.T) {
	registry, transactor := newEvaluationRequestRegistry(t)
	first := evaluationRequestRecord(t, "tenant-a", "EVREQ-SYN-1", "syn-fee-1")
	second := evaluationRequestRecord(t, "tenant-a", "EVREQ-SYN-2", "syn-fee-2")

	if outcome := saveEvaluationRequest(t, transactor, registry, first); outcome != ports.EvaluationRequestSaved {
		t.Fatalf("第一份结果 = %q", outcome)
	}
	if outcome := saveEvaluationRequest(t, transactor, registry, second); outcome != ports.EvaluationRequestSaved {
		t.Fatalf("成分不同的第二份结果 = %q, want SAVED", outcome)
	}
	for _, record := range []ports.EvaluationRequestRecord{first, second} {
		found, exists, err := registry.FindByID(t.Context(), record.Key)
		if err != nil || !exists {
			t.Fatalf("%s 读回：exists=%v err=%v", record.Key.Request.String(), exists, err)
		}
		assertEvaluationRequestEquals(t, found, record)
	}
}

// 登记面写入必须在事务里：登记与信封同事务是裁决 1 的前提，写口在事务外被调是装配缺陷，按框架合同拒。
func TestSavingAnEvaluationRequestRequiresATransaction(t *testing.T) {
	registry, _ := newEvaluationRequestRegistry(t)
	record := evaluationRequestRecord(t, "tenant-a", "EVREQ-SYN-1", "syn-fee-1")

	if _, err := registry.Save(t.Context(), record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
	if _, found, err := registry.FindByID(t.Context(), record.Key); err != nil || found {
		t.Fatalf("事务外的 Save 不该留下行：found=%v err=%v", found, err)
	}
}

// Covers: 票 sa-cc/16 做法 2——键与对象说的不是同一件事（键指 EVREQ-SYN-1、对象的 ID 是 EVREQ-SYN-2）→
// 拒存并报错，不发 INSERT：两个 ID 都不该有行。零值键同一道门。库上没有约束能拦这一格，只能在写口拦。
func TestSavingAnEvaluationRequestWhoseKeyDisagreesWithItsRequestIsRefused(t *testing.T) {
	registry, transactor := newEvaluationRequestRegistry(t)
	claimed := evaluationRequestRecord(t, "tenant-a", "EVREQ-SYN-1", "syn-fee-1")
	mismatched := evaluationRequestRecord(t, "tenant-a", "EVREQ-SYN-2", "syn-fee-1")
	mismatched.Key = claimed.Key

	var outcome ports.EvaluationRequestSaveOutcome
	var saveErr error
	saWithin(t, transactor, t.Context(), func(txCtx context.Context) error {
		outcome, saveErr = registry.Save(txCtx, mismatched)
		return nil
	})
	if saveErr == nil || outcome != ports.EvaluationRequestSaveOutcomeInvalid {
		t.Fatalf("键指 %s、对象是 %s 的记录被存下了：outcome=%q err=%v",
			claimed.Key.Request.String(), mismatched.Request.ID().String(), outcome, saveErr)
	}
	for _, id := range []string{"EVREQ-SYN-1", "EVREQ-SYN-2"} {
		key := ports.EvaluationRequestKey{TenantID: saTenant(t, "tenant-a"), Request: saValue(t, domain.NewEvaluationRequestID, id)}
		if _, found, err := registry.FindByID(t.Context(), key); err != nil || found {
			t.Fatalf("%s 落了行：found=%v err=%v", id, found, err)
		}
	}

	blank := evaluationRequestRecord(t, "tenant-a", "EVREQ-SYN-1", "syn-fee-1")
	blank.Key = ports.EvaluationRequestKey{}
	saWithin(t, transactor, t.Context(), func(txCtx context.Context) error {
		outcome, saveErr = registry.Save(txCtx, blank)
		return nil
	})
	if saveErr == nil || outcome != ports.EvaluationRequestSaveOutcomeInvalid {
		t.Fatalf("零值键被存下了：outcome=%q err=%v", outcome, saveErr)
	}
}

func assertEvaluationRequestEquals(t *testing.T, got, want ports.EvaluationRequestRecord) {
	t.Helper()
	if got.Key != want.Key {
		t.Fatalf("键 = %+v, want %+v", got.Key, want.Key)
	}
	if got.Request.ID() != want.Request.ID() || got.Request.Scope() != want.Request.Scope() ||
		got.Request.Purpose() != want.Request.Purpose() || got.Request.RequestedBy() != want.Request.RequestedBy() {
		t.Fatalf("请求身份 / 范围 / 目的 / 请求方变形：got %+v", got.Request)
	}
	if got.Request.NaturalKey() != want.Request.NaturalKey() {
		t.Fatalf("自然键 = %+v, want %+v", got.Request.NaturalKey(), want.Request.NaturalKey())
	}
	gotSources, wantSources := got.Request.Sources(), want.Request.Sources()
	if gotSources.FeeItem != wantSources.FeeItem || gotSources.Agreement != wantSources.Agreement ||
		gotSources.Occurrence.ID() != wantSources.Occurrence.ID() ||
		gotSources.Occurrence.Reason() != wantSources.Occurrence.Reason() ||
		gotSources.Occurrence.Version() != wantSources.Occurrence.Version() ||
		!gotSources.Occurrence.OccurredAt().Equal(wantSources.Occurrence.OccurredAt()) {
		t.Fatalf("三件引用变形：got %+v, want %+v", gotSources, wantSources)
	}
	if !got.Request.RequestedAt().Equal(want.Request.RequestedAt()) || !got.RecordedAt.Equal(want.RecordedAt) {
		t.Fatalf("时刻变形：requestedAt %v / recordedAt %v", got.Request.RequestedAt(), got.RecordedAt)
	}
}
