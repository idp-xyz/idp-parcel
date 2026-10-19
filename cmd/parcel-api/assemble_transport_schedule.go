package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// transactionalTransportScheduleRegistrar 把周期安排与具体班次的每次登记各包进一笔事务，理由随
// transactionalCredentialRegistrar：登记册的写口按框架合同无事务即拒，事务边界归装配点；读当前版与落下一版同笔，
// 编排返回 error 时整笔回滚。
type transactionalTransportScheduleRegistrar struct {
	transactor bentoapp.Transactor
	inner      *tfapp.RegisterTransportScheduleHandler
}

var _ tfhttp.TransportScheduleRegistrar = transactionalTransportScheduleRegistrar{}

func (registrar transactionalTransportScheduleRegistrar) RegisterTemplate(
	ctx context.Context,
	command tfapp.RegisterScheduleTemplateCommand,
) (tfapp.ScheduleRegistrationResult, error) {
	var result tfapp.ScheduleRegistrationResult
	err := registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := registrar.inner.RegisterTemplate(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.ScheduleRegistrationResult{}, err
	}
	return result, nil
}

func (registrar transactionalTransportScheduleRegistrar) RegisterSchedule(
	ctx context.Context,
	command tfapp.RegisterTransportScheduleCommand,
) (tfapp.ScheduleRegistrationResult, error) {
	var result tfapp.ScheduleRegistrationResult
	err := registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := registrar.inner.RegisterSchedule(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.ScheduleRegistrationResult{}, err
	}
	return result, nil
}

// buildTransportScheduleRegistration 装配 `/transport-fulfillment-transport-schedule-template-registrations` 与
// `/transport-fulfillment-transport-schedule-registrations` 背后的真编排（ADR-0201 决定一）。缝只有三条——周期安排册、
// 班次册与时钟——全接真；没有意图交付：班次是本上下文自己的执行准备，准备、订舱与装载分配按需来读。受控 CLI
// parcel-transport-register 装的是同一个编排。
func buildTransportScheduleRegistration(db *bentopg.DB) (tfhttp.TransportScheduleRegistrar, error) {
	templates, err := tfpostgres.NewScheduleTemplates(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: schedule templates: %w", err)
	}
	schedules, err := tfpostgres.NewTransportSchedules(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: transport schedules: %w", err)
	}
	handler := tfapp.NewRegisterTransportScheduleHandler(tfapp.RegisterTransportScheduleDeps{
		Templates: templates,
		Schedules: schedules,
		Clock:     systemClock{},
	})
	return transactionalTransportScheduleRegistrar{transactor: db.Transactor(), inner: handler}, nil
}
