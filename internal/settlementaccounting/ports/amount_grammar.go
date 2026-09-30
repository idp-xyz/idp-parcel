package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// AmountGrammarRegister 写下限额、比例、免赔三项取值。读口是 AmountGrammarView。
// 金额规则版本册只存版本引用，这三项不写回去。
type AmountGrammarRegister interface {
	SaveAmountGrammar(
		ctx context.Context,
		tenant domain.TenantID,
		registration domain.AmountGrammarRegistration,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}

// AmountGrammarView 按已采用的引用取三项取值。found=false 表示没有这一行：
// 不形成金额，也不用主张金额顶上。
type AmountGrammarView interface {
	LoadAmountGrammar(
		ctx context.Context,
		tenant domain.TenantID,
		subject domain.AmountGrammarSubject,
		ref string,
	) (domain.AmountGrammar, bool, error)
}
