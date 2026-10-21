package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrChargeOccurrenceChainBroken 说明发生项的有效性版本链在库里断了形：后一版没有回指前一版，或链长过了上限。版本号由前一版派生、
// 一版至多被修订一次（迁移 TF 0049 的唯一索引），合格的链走不到这里；走到了是库里的数据违约，重试不会变。
var ErrChargeOccurrenceChainBroken = errors.New("transport fulfillment: charge occurrence validity chain is broken")

// maxOccurrenceChainLength 是沿版本链往后走的步数上限：一项发生项一年也修订不了几回，过了这个数只可能是链成了环或被灌了数据。
const maxOccurrenceChainLength = 64

// chargeOccurrenceKeyReader 是沿链读版本要的那一口：按键整条读回。
type chargeOccurrenceKeyReader interface {
	FindByKey(ctx context.Context, key ports.ChargeOccurrenceKey) (ports.ChargeOccurrenceRecord, bool, error)
}

// occurrenceChainHead 从首版 V1 起沿派生的版本号往后走，交回链头——此刻这一项是什么样子。没有首版答 found=false。
//
// 只用按键读这一口，不另开「按引用读链头」：版本号由前一版派生（domain.NextOccurrenceValidity），下一版在哪个键上是算得出来的，
// 多开一口就多一处对「哪一版是当前」的回答。
func occurrenceChainHead(
	ctx context.Context,
	registry chargeOccurrenceKeyReader,
	tenant domain.TenantID,
	occurrence domain.ChargeOccurrenceReference,
) (ports.ChargeOccurrenceRecord, bool, error) {
	key := ports.ChargeOccurrenceKey{TenantID: tenant, Occurrence: occurrence, Validity: domain.FirstOccurrenceValidity()}
	head, found, err := registry.FindByKey(ctx, key)
	if err != nil || !found {
		return ports.ChargeOccurrenceRecord{}, false, err
	}
	for step := 0; step < maxOccurrenceChainLength; step++ {
		next, err := domain.NextOccurrenceValidity(head.Key.Validity)
		if err != nil {
			return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("%w: %s after %s", ErrChargeOccurrenceChainBroken, occurrence, head.Key.Validity)
		}
		successor, found, err := registry.FindByKey(ctx, ports.ChargeOccurrenceKey{TenantID: tenant, Occurrence: occurrence, Validity: next})
		if err != nil {
			return ports.ChargeOccurrenceRecord{}, false, err
		}
		if !found {
			return head, true, nil
		}
		if corrects, revised := successor.Occurrence.Corrects(); !revised || corrects != head.Key.Validity {
			return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("%w: %s %s does not correct %s",
				ErrChargeOccurrenceChainBroken, occurrence, next, head.Key.Validity)
		}
		head = successor
	}
	return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("%w: %s is longer than %d versions",
		ErrChargeOccurrenceChainBroken, occurrence, maxOccurrenceChainLength)
}
