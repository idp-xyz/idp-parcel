package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证绑定登记用例只做受理与答案翻译：零值绑定不到达登记册；登记册三格答案逐格译出；
// 依赖故障不吞。

type bindingRegisterDouble struct {
	outcome ports.SourceConnectorBindingOutcome
	err     error
	calls   int
}

func (double *bindingRegisterDouble) Register(context.Context, domain.SourceConnectorBinding) (ports.SourceConnectorBindingOutcome, error) {
	double.calls++
	return double.outcome, double.err
}

func TestRegisterSourceConnectorBindingTranslatesTheRegisterAlgebra(t *testing.T) {
	binding := feedBinding(t, domain.ReviewExemptionUndeclared, "SYN-PRC-SERIES-REGISTRAR")
	translations := map[ports.SourceConnectorBindingOutcome]application.RegisterSourceConnectorBindingOutcome{
		ports.SourceConnectorBindingRegistered:        application.SourceConnectorBindingRecorded,
		ports.SourceConnectorBindingAlreadyRegistered: application.SourceConnectorBindingAlreadyOnRegister,
		ports.SourceConnectorBindingConflict:          application.SourceConnectorBindingRegistrationConflict,
	}
	for saved, want := range translations {
		register := &bindingRegisterDouble{outcome: saved}
		handler := application.NewRegisterSourceConnectorBindingHandler(application.RegisterSourceConnectorBindingDeps{Bindings: register})
		got, err := handler.Handle(t.Context(), application.RegisterSourceConnectorBindingCommand{Binding: binding})
		if err != nil || got != want || register.calls != 1 {
			t.Fatalf("%d：got %s err %v calls %d，想要 %s", saved, got, err, register.calls, want)
		}
		if got.String() == "" {
			t.Fatalf("%s 没有名字", want)
		}
	}

	untouched := &bindingRegisterDouble{outcome: ports.SourceConnectorBindingRegistered}
	handler := application.NewRegisterSourceConnectorBindingHandler(application.RegisterSourceConnectorBindingDeps{Bindings: untouched})
	if got, err := handler.Handle(t.Context(), application.RegisterSourceConnectorBindingCommand{}); err != nil || got != application.SourceConnectorBindingNotAccepted || untouched.calls != 0 {
		t.Fatalf("零值绑定：got %s err %v calls %d", got, err, untouched.calls)
	}

	boom := errors.New("bindings are down")
	handler = application.NewRegisterSourceConnectorBindingHandler(application.RegisterSourceConnectorBindingDeps{Bindings: &bindingRegisterDouble{err: boom}})
	if got, err := handler.Handle(t.Context(), application.RegisterSourceConnectorBindingCommand{Binding: binding}); got != application.SourceConnectorBindingUndecided || !errors.Is(err, boom) {
		t.Fatalf("依赖故障：got %s err %v", got, err)
	}

	handler = application.NewRegisterSourceConnectorBindingHandler(application.RegisterSourceConnectorBindingDeps{Bindings: &bindingRegisterDouble{outcome: ports.SourceConnectorBindingOutcome(99)}})
	if _, err := handler.Handle(t.Context(), application.RegisterSourceConnectorBindingCommand{Binding: binding}); !errors.Is(err, application.ErrUnexpectedSourceConnectorBindingOutcome) {
		t.Fatalf("代数外答案：err = %v", err)
	}
}
