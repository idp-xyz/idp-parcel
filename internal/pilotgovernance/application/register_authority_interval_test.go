package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/application"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

// 本文件证权威区间的独立登记编排（票 12 首批第一类——OWNERSHIP_UNRESOLVED 的恢复
// 动作落点）：冲突预检先于任何落库、全等重放答已在册、形状拒绝与依赖故障各归各格。
// 判据在领域（DetectAuthorityConflicts），这里只证编排把答案接对。

var intervalRegisterAt = time.Date(2026, 8, 24, 2, 0, 0, 0, time.UTC)

// registrationIntervalStoreDouble 自带两处故障注入。不复用评审用例那份替身：这里要
// 让 ListCurrent 也能坏，而共享替身加格会让别人的用例多出没人证的分支。
type registrationIntervalStoreDouble struct {
	intervals []domain.AuthorityInterval
	listErr   error
	appendErr error
	appends   int
}

func (double *registrationIntervalStoreDouble) ListCurrent(_ context.Context) ([]domain.AuthorityInterval, error) {
	if double.listErr != nil {
		return nil, double.listErr
	}
	return append([]domain.AuthorityInterval(nil), double.intervals...), nil
}

func (double *registrationIntervalStoreDouble) Append(_ context.Context, interval domain.AuthorityInterval) error {
	if double.appendErr != nil {
		return double.appendErr
	}
	double.appends++
	double.intervals = append(double.intervals, interval)
	return nil
}

func registrationInterval(scope string) domain.AuthorityInterval {
	return domain.AuthorityInterval{
		ObjectScope: scope,
		Capability:  "shipment-intake",
		FactKind:    "production-ownership",
		Authority:   "parcel-product",
		From:        intervalRegisterAt,
	}
}

func newIntervalRegistration(store *registrationIntervalStoreDouble) *application.RegisterAuthorityIntervalHandler {
	return application.NewRegisterAuthorityIntervalHandler(application.RegisterAuthorityIntervalDeps{
		Intervals: store,
	})
}

// TestRegisterAuthorityIntervalAppendsToAnEmptyRegister 证绿路径：空册上登记一条
// 合法区间，答`已登记`且区间在册。
func TestRegisterAuthorityIntervalAppendsToAnEmptyRegister(t *testing.T) {
	store := &registrationIntervalStoreDouble{}
	handler := newIntervalRegistration(store)

	result, err := handler.Handle(context.Background(), registrationInterval("pilot-scope/v1"))
	if err != nil {
		t.Fatalf("登记权威区间：%v", err)
	}
	if result.Outcome() != application.IntervalRegistered {
		t.Fatalf("结果 = %s，要 %s", result.Outcome(), application.IntervalRegistered)
	}
	if store.appends != 1 {
		t.Fatalf("追加次数 = %d，要 1", store.appends)
	}
}

// TestRegisterAuthorityIntervalReplayAnswersAlreadyRegistered 证幂等：同一区间第二次
// 登记答`已在册`，不追加第二行——全等区间与自己必然重叠，重放不得被冲突预检误伤。
func TestRegisterAuthorityIntervalReplayAnswersAlreadyRegistered(t *testing.T) {
	store := &registrationIntervalStoreDouble{}
	handler := newIntervalRegistration(store)
	ctx := context.Background()

	if _, err := handler.Handle(ctx, registrationInterval("pilot-scope/v1")); err != nil {
		t.Fatalf("首次登记：%v", err)
	}
	result, err := handler.Handle(ctx, registrationInterval("pilot-scope/v1"))
	if err != nil {
		t.Fatalf("重放登记：%v", err)
	}
	if result.Outcome() != application.IntervalAlreadyRegistered {
		t.Fatalf("结果 = %s，要 %s", result.Outcome(), application.IntervalAlreadyRegistered)
	}
	if store.appends != 1 {
		t.Fatalf("追加次数 = %d，要 1（重放不长第二行）", store.appends)
	}
}

// TestRegisterAuthorityIntervalOverlapIsBlockedWithConflictPairs 证冲突预检：同维
// 不同权威方的重叠区间被阻断、冲突对全数交回、一行都不落——「双写后人工对账」是
// 交接点名的错误结果。
func TestRegisterAuthorityIntervalOverlapIsBlockedWithConflictPairs(t *testing.T) {
	store := &registrationIntervalStoreDouble{}
	handler := newIntervalRegistration(store)
	ctx := context.Background()

	if _, err := handler.Handle(ctx, registrationInterval("pilot-scope/v1")); err != nil {
		t.Fatalf("首次登记：%v", err)
	}
	overlapping := registrationInterval("pilot-scope/v1")
	overlapping.Authority = "legacy-system"
	overlapping.From = intervalRegisterAt.Add(time.Hour)

	result, err := handler.Handle(ctx, overlapping)
	if err != nil {
		t.Fatalf("登记重叠区间：%v", err)
	}
	if result.Outcome() != application.IntervalConflictBlocked {
		t.Fatalf("结果 = %s，要 %s", result.Outcome(), application.IntervalConflictBlocked)
	}
	if len(result.Conflicts()) != 1 {
		t.Fatalf("冲突对 = %d，要 1", len(result.Conflicts()))
	}
	if store.appends != 1 {
		t.Fatalf("追加次数 = %d，要 1（冲突不落库）", store.appends)
	}
}

// TestRegisterAuthorityIntervalRejectsAnInvalidShape 证形状门在编排最前：空能力维
// 答`未受理`，不问库也不落库。
func TestRegisterAuthorityIntervalRejectsAnInvalidShape(t *testing.T) {
	store := &registrationIntervalStoreDouble{listErr: errors.New("must not be asked")}
	handler := newIntervalRegistration(store)

	invalid := registrationInterval("pilot-scope/v1")
	invalid.Capability = "  "

	result, err := handler.Handle(context.Background(), invalid)
	if err != nil {
		t.Fatalf("登记非法区间：%v", err)
	}
	if result.Outcome() != application.IntervalNotAccepted {
		t.Fatalf("结果 = %s，要 %s", result.Outcome(), application.IntervalNotAccepted)
	}
}

// TestRegisterAuthorityIntervalStoreFailuresAnswerUndecided 证依赖故障如实答`未决`：
// 读册坏与追加坏都不许折成拒绝或成功。
func TestRegisterAuthorityIntervalStoreFailuresAnswerUndecided(t *testing.T) {
	t.Run("读册故障", func(t *testing.T) {
		store := &registrationIntervalStoreDouble{listErr: errors.New("register unavailable")}
		handler := newIntervalRegistration(store)

		result, err := handler.Handle(context.Background(), registrationInterval("pilot-scope/v1"))
		if err != nil {
			t.Fatalf("登记：%v", err)
		}
		if result.Outcome() != application.IntervalUndecided {
			t.Fatalf("结果 = %s，要 %s", result.Outcome(), application.IntervalUndecided)
		}
	})

	t.Run("追加故障", func(t *testing.T) {
		store := &registrationIntervalStoreDouble{appendErr: errors.New("append failed")}
		handler := newIntervalRegistration(store)

		result, err := handler.Handle(context.Background(), registrationInterval("pilot-scope/v1"))
		if err != nil {
			t.Fatalf("登记：%v", err)
		}
		if result.Outcome() != application.IntervalUndecided {
			t.Fatalf("结果 = %s，要 %s", result.Outcome(), application.IntervalUndecided)
		}
	})
}
