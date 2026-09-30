package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// RegisterAmountGrammarHandler 登记限额、比例、免赔三项取值。落库时刻不是载荷的一格。
type RegisterAmountGrammarHandler struct {
	grammars ports.AmountGrammarRegister
	clock    ports.Clock
}

func NewRegisterAmountGrammarHandler(
	grammars ports.AmountGrammarRegister,
	clock ports.Clock,
) (*RegisterAmountGrammarHandler, error) {
	if grammars == nil || clock == nil {
		return nil, fmt.Errorf("%w: amount grammar", ErrNilDependency)
	}
	return &RegisterAmountGrammarHandler{grammars: grammars, clock: clock}, nil
}

func (handler *RegisterAmountGrammarHandler) RegisterAmountGrammar(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.AmountGrammarRegistration,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" {
		return 0, fmt.Errorf("%w: tenant", domain.ErrBlankValue)
	}
	return handler.grammars.SaveAmountGrammar(ctx, tenant, registration, handler.clock.Now())
}

type grammarLookup uint8

const (
	grammarReady grammarLookup = iota
	grammarViewUnavailable
	grammarUnconfigured
	grammarNotFormed
	grammarRejected
)

// composeRegisteredAmount 用已登记的三项取值把主张收成展开。读口没有、或这一行没登记，
// 都停在调用方自己的未决格，不用主张金额顶上。主张不合法是提交矛盾。算出 0 是不形成金额。
func composeRegisteredAmount(
	ctx context.Context,
	view ports.AmountGrammarView,
	tenant domain.TenantID,
	subject domain.AmountGrammarSubject,
	ref string,
	assertedMinor int64,
) (domain.AmountComposition, grammarLookup, error) {
	if view == nil {
		return domain.AmountComposition{}, grammarViewUnavailable, nil
	}
	grammar, found, err := view.LoadAmountGrammar(ctx, tenant, subject, ref)
	if err != nil {
		return domain.AmountComposition{}, grammarViewUnavailable, nil
	}
	if !found {
		return domain.AmountComposition{}, grammarUnconfigured, nil
	}
	composition, err := grammar.Compose(assertedMinor)
	if errors.Is(err, domain.ErrAmountGrammarOverflow) {
		return domain.AmountComposition{}, grammarReady, err
	}
	if err != nil {
		return domain.AmountComposition{}, grammarRejected, nil
	}
	if !composition.FormsAmount() {
		return composition, grammarNotFormed, nil
	}
	return composition, grammarReady, nil
}
