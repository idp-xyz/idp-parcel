package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// PriceCardClosureSaveOutcome 是收尾事实落库的结果代数。`已登记`是同一后继、同一依据的那一次已在册——编排在锁之后读回的
// 那一版里本该已认出重放，走到这一格说明锁与读之间的约定被打破了，仍按库答；`已凭别的退役`只有退役有：一版只退一次。
type PriceCardClosureSaveOutcome uint8

const (
	PriceCardClosureSaveOutcomeInvalid PriceCardClosureSaveOutcome = iota
	PriceCardClosureSaved
	PriceCardClosureAlreadyRegistered
	PriceCardRetiredOtherwise
)

// PriceCardClosureRegistry 是价卡收尾（ADR-0234）的写侧存取口：先锁旧版的版本行，再读回两版此刻在册上的样子，判过之后写
// 截止或退役，全在调用方的同一笔事务里。
//
// 先锁再读是判据的要求，不是性能取舍：收尾判的是「旧版此刻的选用」与「后继接不接得住」，读的是册上的整段收尾史。只靠
// 写入那一刻的行锁，两笔对同一旧版的收尾会各自读到未收尾的样子、各自判过，后提交的那条照样落下——例如两次截止各让位
// 给不同的后继，截止点更晚的那一条后提交，就成了最新一条，把旧版的选用放宽回先到那一条已经截掉的那一段，两版在那一段
// 重新同时适用。锁住旧版行，后到者停在锁上，先到者提交之后它读到的就是已收尾的样子。
type PriceCardClosureRegistry interface {
	// LockPriceCardVersionForClosure 锁住本租户这一版的版本行，直到调用方事务结束。found=false 是这一版不在册，不是故障。
	LockPriceCardVersionForClosure(ctx context.Context, tenant domain.TenantID, plan domain.VersionReference) (bool, error)
	// LoadPriceCardStanding 读回这一版此刻在册上的样子：发布的那一版（经领域整图重验）、按登记顺序的全部截止与退役。
	LoadPriceCardStanding(ctx context.Context, tenant domain.TenantID, plan domain.VersionReference) (domain.PriceCardStanding, bool, error)
	SavePriceCardCut(ctx context.Context, cut domain.PriceCardCut) (PriceCardClosureSaveOutcome, error)
	SavePriceCardRetirement(ctx context.Context, retirement domain.PriceCardRetirement) (PriceCardClosureSaveOutcome, error)
}
