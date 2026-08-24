package application_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// factsRegistryDouble 按(键+版本)记账的登记册替身。写入代数与真口同形:第一份
// Registered,同键同版本再来 AlreadyRegistered,内容之争留给编排比。
type factsRegistryDouble struct {
	records  map[string]ports.AutoRerouteFactsRecord
	saveErr  error
	lastSave ports.AutoRerouteFactsRecord
}

func newFactsRegistryDouble() *factsRegistryDouble {
	return &factsRegistryDouble{records: map[string]ports.AutoRerouteFactsRecord{}}
}

func factsVersionKey(key domain.InitialRouteJudgmentKey, version int) string {
	return key.TenantID.String() + "\x00" + key.DeclaredParcelID.String() + "\x00" +
		key.AcceptanceBaseline.String() + "\x00" + strconv.Itoa(version)
}

func (double *factsRegistryDouble) RegisterAutoRerouteFacts(
	_ context.Context,
	record ports.AutoRerouteFactsRecord,
) (ports.AutoRerouteFactsSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.AutoRerouteFactsSaveOutcomeInvalid, double.saveErr
	}
	double.lastSave = record
	id := factsVersionKey(record.Key, record.Version)
	if _, exists := double.records[id]; exists {
		return ports.AutoRerouteFactsAlreadyRegistered, nil
	}
	double.records[id] = record
	return ports.AutoRerouteFactsRegistered, nil
}

func (double *factsRegistryDouble) FindAutoRerouteFacts(
	_ context.Context,
	key domain.InitialRouteJudgmentKey,
	version int,
) (ports.AutoRerouteFactsRecord, bool, error) {
	record, found := double.records[factsVersionKey(key, version)]
	return record, found, nil
}

var factsRegisteredAt = time.Date(2026, 8, 24, 11, 0, 0, 0, time.UTC)

func newFactsHandler(registry *factsRegistryDouble) *application.RegisterAutoRerouteFactsHandler {
	return application.NewRegisterAutoRerouteFactsHandler(application.RegisterAutoRerouteFactsDeps{
		Registry: registry,
		Clock:    fixedClock{at: factsRegisteredAt},
	})
}

func factsJudgmentKey(t *testing.T) domain.InitialRouteJudgmentKey {
	t.Helper()
	return domain.InitialRouteJudgmentKey{
		TenantID:           scalarValue(t, domain.NewTenantID, "tenant-a"),
		CustomerAccountID:  scalarValue(t, domain.NewCustomerAccountID, "customer-a"),
		ShipmentRequestID:  scalarValue(t, domain.NewShipmentRequestID, "request-1"),
		AcceptanceBaseline: scalarValue(t, domain.NewAcceptanceBaselineReference, "submission-1"),
		DeclaredParcelID:   scalarValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		ServicePurpose:     scalarValue(t, domain.NewServicePurpose, "LAST_MILE_DELIVERY"),
	}
}

func scalarValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func validFactsCommand(t *testing.T) application.RegisterAutoRerouteFactsCommand {
	t.Helper()
	return application.RegisterAutoRerouteFactsCommand{
		Key:                         factsJudgmentKey(t),
		Version:                     1,
		PolicyAllowsAutomatic:       true,
		AtControlledNode:            true,
		OnlyUnexecutedAffected:      false,
		UnresolvedRestrictions:      []string{"restriction/customs-hold-1"},
		OutstandingResponsibilities: []string{"responsibility/booking-42"},
		StrategyBasis:               "route-strategy/syn-v1",
	}
}

// Covers: 票 05 登记口的受理门——目录内容属实例半边（PAR-NET-14），缺处逐格指名被
// 拒，一个默认值都不补。
func TestRegisterAutoRerouteFactsRefusesIncompleteStatements(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*application.RegisterAutoRerouteFactsCommand)
		want   application.AutoRerouteFactsRefusalReason
	}{
		{
			name: "键不完整",
			mutate: func(command *application.RegisterAutoRerouteFactsCommand) {
				command.Key.DeclaredParcelID = domain.DeclaredParcelID{}
			},
			want: application.AutoRerouteFactsKeyIncomplete,
		},
		{
			name: "版本缺席",
			mutate: func(command *application.RegisterAutoRerouteFactsCommand) {
				command.Version = 0
			},
			want: application.AutoRerouteFactsVersionMissing,
		},
		{
			name: "折算依据缺席",
			mutate: func(command *application.RegisterAutoRerouteFactsCommand) {
				command.StrategyBasis = ""
			},
			want: application.AutoRerouteFactsBasisMissing,
		},
		{
			name: "限制清单有空白元素",
			mutate: func(command *application.RegisterAutoRerouteFactsCommand) {
				command.UnresolvedRestrictions = []string{""}
			},
			want: application.AutoRerouteFactsRestrictionBlank,
		},
		{
			name: "责任清单有空白元素",
			mutate: func(command *application.RegisterAutoRerouteFactsCommand) {
				command.OutstandingResponsibilities = []string{"  "}
			},
			want: application.AutoRerouteFactsResponsibilityBlank,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			registry := newFactsRegistryDouble()
			command := validFactsCommand(t)
			testCase.mutate(&command)

			result, err := newFactsHandler(registry).Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.AutoRerouteFactsRefused {
				t.Fatalf("outcome = %q, want REFUSED", result.Outcome())
			}
			if result.RefusalReason() != testCase.want {
				t.Fatalf("refusal = %q, want %q", result.RefusalReason(), testCase.want)
			}
			if len(registry.records) != 0 {
				t.Fatal("被拒的陈述不得触写入口")
			}
		})
	}
}

// Covers: 票 05 登记口的受理路——五件事实与折算依据全须落进记录,登记时刻由时钟给
// 出（不由登记方带入,防止倒填历史）。
func TestRegisterAutoRerouteFactsAcceptsACompleteStatement(t *testing.T) {
	registry := newFactsRegistryDouble()

	result, err := newFactsHandler(registry).Handle(context.Background(), validFactsCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AutoRerouteFactsAccepted {
		t.Fatalf("outcome = %q, want REGISTERED", result.Outcome())
	}
	saved := registry.lastSave
	if !saved.Facts.PolicyAllowsAutomatic || !saved.Facts.AtControlledNode ||
		saved.Facts.OnlyUnexecutedAffected {
		t.Fatalf("三个折算结论没有原样落进记录：%+v", saved.Facts)
	}
	if len(saved.Facts.UnresolvedRestrictions) != 1 ||
		saved.Facts.UnresolvedRestrictions[0].String() != "restriction/customs-hold-1" {
		t.Fatalf("限制清单没有译成领域引用：%+v", saved.Facts.UnresolvedRestrictions)
	}
	if len(saved.Facts.OutstandingResponsibilities) != 1 ||
		saved.Facts.OutstandingResponsibilities[0].String() != "responsibility/booking-42" {
		t.Fatalf("责任清单没有译成领域引用：%+v", saved.Facts.OutstandingResponsibilities)
	}
	if saved.StrategyBasis != "route-strategy/syn-v1" {
		t.Fatalf("折算依据 = %q", saved.StrategyBasis)
	}
	if !saved.RegisteredAt.Equal(factsRegisteredAt) {
		t.Fatalf("登记时刻 = %v, want 时钟时刻 %v", saved.RegisteredAt, factsRegisteredAt)
	}
}

// Covers: 不可覆盖纪律的幂等半边——同键同版本重放同一份内容答`已存在`,不是错误也
// 不是第二份。
func TestRegisterAutoRerouteFactsReplayAnswersAlreadyExists(t *testing.T) {
	registry := newFactsRegistryDouble()
	handler := newFactsHandler(registry)
	if _, err := handler.Handle(context.Background(), validFactsCommand(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}

	result, err := handler.Handle(context.Background(), validFactsCommand(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if result.Outcome() != application.AutoRerouteFactsAlreadyExists {
		t.Fatalf("outcome = %q, want ALREADY_EXISTS", result.Outcome())
	}
}

// Covers: 不可覆盖纪律的冲突半边——同键同版本换了内容答`内容冲突`,绝不顶替已在册
// 的陈述（同键同版本的内容之争没有「后到为准」）。
func TestRegisterAutoRerouteFactsContentChangeAnswersConflict(t *testing.T) {
	registry := newFactsRegistryDouble()
	handler := newFactsHandler(registry)
	if _, err := handler.Handle(context.Background(), validFactsCommand(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}

	changed := validFactsCommand(t)
	changed.PolicyAllowsAutomatic = false
	result, err := handler.Handle(context.Background(), changed)
	if err != nil {
		t.Fatalf("conflict handle: %v", err)
	}
	if result.Outcome() != application.AutoRerouteFactsConflict {
		t.Fatalf("outcome = %q, want CONTENT_CONFLICT", result.Outcome())
	}
	kept := registry.records[factsVersionKey(changed.Key, changed.Version)]
	if !kept.Facts.PolicyAllowsAutomatic {
		t.Fatal("冲突把已在册的陈述顶掉了")
	}
}

// Covers: 登记是管理动作没有`未决`格——依赖调不通交回错误由调用方重试,不折成业务
// 答案。
func TestRegisterAutoRerouteFactsDependencyFailureSurfacesAsError(t *testing.T) {
	registry := newFactsRegistryDouble()
	registry.saveErr = errors.New("registry down")

	_, err := newFactsHandler(registry).Handle(context.Background(), validFactsCommand(t))
	if err == nil {
		t.Fatal("写入口故障必须上抛错误")
	}
}

// Covers: 写口答已在册却读不回那一版——仓储不变量已破,响亮报错,不得折成幂等或冲突。
func TestRegisterAutoRerouteFactsMissingWinnerIsLoud(t *testing.T) {
	handler := application.NewRegisterAutoRerouteFactsHandler(application.RegisterAutoRerouteFactsDeps{
		Registry: &missingWinnerRegistry{},
		Clock:    fixedClock{at: factsRegisteredAt},
	})
	if _, err := handler.Handle(context.Background(), validFactsCommand(t)); err == nil {
		t.Fatal("已在册却读不回必须报错")
	}
}

// missingWinnerRegistry 恒答已在册但永远读不回——专演仓储不变量已破的那一格。
type missingWinnerRegistry struct{}

func (*missingWinnerRegistry) RegisterAutoRerouteFacts(
	context.Context, ports.AutoRerouteFactsRecord,
) (ports.AutoRerouteFactsSaveOutcome, error) {
	return ports.AutoRerouteFactsAlreadyRegistered, nil
}

func (*missingWinnerRegistry) FindAutoRerouteFacts(
	context.Context, domain.InitialRouteJudgmentKey, int,
) (ports.AutoRerouteFactsRecord, bool, error) {
	return ports.AutoRerouteFactsRecord{}, false, nil
}
