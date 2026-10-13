package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件钉受理前估价采用登记交信封的判据（票 product-strategy-boundary/25 第 3 条；ADR-0094 决定四）：按落点交，不按「这次调用有没有
// 成功」交。真库那一层钉不住「冲突不交」，理由同结算账户登记那份用例的头注。

type adoptionRegisterDouble struct {
	effect ports.PreAcceptanceEstimateAdoptionEffect
	err    error
	at     time.Time
}

func (double *adoptionRegisterDouble) SaveAdoption(
	_ context.Context,
	_ domain.TenantID,
	_ domain.PreAcceptanceEstimateAdoption,
	at time.Time,
) (ports.PreAcceptanceEstimateAdoptionEffect, error) {
	double.at = at
	return double.effect, double.err
}

type adoptionHandoffDouble struct {
	intents []ports.PreAcceptanceEstimateAdoptionRegisteredIntent
	err     error
}

func (double *adoptionHandoffDouble) HandOffPreAcceptanceEstimateAdoptionRegistered(
	_ context.Context,
	intent ports.PreAcceptanceEstimateAdoptionRegisteredIntent,
) error {
	double.intents = append(double.intents, intent)
	return double.err
}

func adoptionCommand(t *testing.T) application.RegisterPreAcceptanceEstimateAdoptionCommand {
	t.Helper()
	adoption, err := domain.NewPreAcceptanceEstimateAdoption(
		mustAccountValue(t, domain.NewSettlementAccountID, "ACCT-1"),
		domain.PerParcelSellTrialSum,
		mustAccountValue(t, domain.NewEstimatePricingScope, "SCOPE-SELL-1"),
	)
	if err != nil {
		t.Fatalf("adoption: %v", err)
	}
	return application.RegisterPreAcceptanceEstimateAdoptionCommand{
		Tenant:   mustAccountValue(t, domain.NewTenantID, "tenant-1"),
		Adoption: adoption,
	}
}

func TestAnAdoptionRegistrationHandsOffByItsEffectAndAConflictHandsOffNothing(t *testing.T) {
	for name, tc := range map[string]struct {
		effect  ports.PreAcceptanceEstimateAdoptionEffect
		handoff int
	}{
		"已登记": {effect: ports.PreAcceptanceEstimateAdoptionRegistered, handoff: 1},
		"已存在": {effect: ports.PreAcceptanceEstimateAdoptionReplay, handoff: 1},
		"冲突":  {effect: ports.PreAcceptanceEstimateAdoptionConflict, handoff: 0},
	} {
		t.Run(name, func(t *testing.T) {
			register := &adoptionRegisterDouble{effect: tc.effect}
			handoff := &adoptionHandoffDouble{}
			handler, err := application.NewRegisterPreAcceptanceEstimateAdoptionHandler(register, handoff, accountClock{at: accountRegisteredAt})
			if err != nil {
				t.Fatalf("handler: %v", err)
			}
			effect, err := handler.Register(context.Background(), adoptionCommand(t))
			if err != nil || effect != tc.effect {
				t.Fatalf("effect = %d err = %v, 想要 %d", effect, err, tc.effect)
			}
			if len(handoff.intents) != tc.handoff {
				t.Fatalf("交接口被调 %d 次, 想要 %d", len(handoff.intents), tc.handoff)
			}
			if !register.at.Equal(accountRegisteredAt) {
				t.Fatalf("落库时刻 = %s, 想要时钟给的 %s", register.at, accountRegisteredAt)
			}
			if tc.handoff == 1 {
				intent := handoff.intents[0]
				if intent.Tenant.String() != "tenant-1" || intent.Account.String() != "ACCT-1" || !intent.RegisteredAt.Equal(accountRegisteredAt) {
					t.Fatalf("intent = %+v, 想要租户、账户与登记时刻", intent)
				}
			}
		})
	}
}

func TestAnAdoptionRegistrationSurfacesItsFailuresAndRefusesMissingParts(t *testing.T) {
	register := &adoptionRegisterDouble{err: domain.ErrCatalogueTargetMissing}
	handoff := &adoptionHandoffDouble{}
	handler, _ := application.NewRegisterPreAcceptanceEstimateAdoptionHandler(register, handoff, accountClock{at: accountRegisteredAt})
	if _, err := handler.Register(context.Background(), adoptionCommand(t)); !errors.Is(err, domain.ErrCatalogueTargetMissing) || len(handoff.intents) != 0 {
		t.Fatalf("err = %v intents = %d, 想要账户不在册上原样上抛、不交信封", err, len(handoff.intents))
	}

	failing := &adoptionHandoffDouble{err: ports.ErrPreAcceptanceEstimateAdoptionHandoffRejected}
	handler, _ = application.NewRegisterPreAcceptanceEstimateAdoptionHandler(
		&adoptionRegisterDouble{effect: ports.PreAcceptanceEstimateAdoptionRegistered}, failing, accountClock{at: accountRegisteredAt})
	if _, err := handler.Register(context.Background(), adoptionCommand(t)); !errors.Is(err, ports.ErrPreAcceptanceEstimateAdoptionHandoffRejected) {
		t.Fatalf("err = %v, 想要交接失败上抛，信封与采用行同生共死", err)
	}

	if _, err := handler.Register(context.Background(), application.RegisterPreAcceptanceEstimateAdoptionCommand{}); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("空命令 err = %v, 想要拒", err)
	}
	for name, build := range map[string]func() error{
		"缺采用册": func() error {
			_, err := application.NewRegisterPreAcceptanceEstimateAdoptionHandler(nil, handoff, accountClock{})
			return err
		},
		"缺交接口": func() error {
			_, err := application.NewRegisterPreAcceptanceEstimateAdoptionHandler(register, nil, accountClock{})
			return err
		},
		"缺时钟": func() error {
			_, err := application.NewRegisterPreAcceptanceEstimateAdoptionHandler(register, handoff, nil)
			return err
		},
	} {
		if err := build(); !errors.Is(err, application.ErrNilDependency) {
			t.Fatalf("%s err = %v, 想要构造期拒", name, err)
		}
	}
}
