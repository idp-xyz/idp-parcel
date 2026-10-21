package application

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// entryHandoverFinding 是「这条参与是凭哪一版`已交接`进的段」的答复。交付读数按它取 at，出发读数按它核班次的起讫；各自按自己的
// 停点作答，认法只在 entryHandoverOf 一处。
type entryHandoverFinding uint8

const (
	entryHandoverInvalid entryHandoverFinding = iota
	entryHandoverFound
	// entryByPickup：凭场外揽收或实际承运商首次有效收寄进的段，没有交接可认。
	entryByPickup
	// entryHandoverNotOnRegister：参与说凭`已交接`进段，入场依据却指不到交接登记册上的一版——依据拆不出版本，或那一版不在册、
	// 在册的业务时间对不上。段上的参与与那次交接由交接登记同一笔写下，这一格说的是库面不一致。
	entryHandoverNotOnRegister
	// entryHandoverAmbiguous：入场依据指名的那一版在册上对上不止一版（ADR-0221 之前只带版本的存量依据）。不猜其中一版。
	entryHandoverAmbiguous
	// entryHandoverUnreadable：交接登记册读不动。
	entryHandoverUnreadable
)

// entryHandoverOf 认一条参与进段的那一版`已交接`：按（租户、对象、入场依据里的版本）找回各版，由领域认哪一版是进段那一次
// （FulfillmentParticipation.EnteredByHandover）。
func entryHandoverOf(
	ctx context.Context,
	view ports.EntryHandoverView,
	tenant domain.TenantID,
	participation domain.FulfillmentParticipation,
) (domain.TransportHandover, entryHandoverFinding) {
	switch participation.EntryKind() {
	case domain.EnteredByOffsitePickup, domain.EnteredByCarrierFirstEffectivePickup:
		return domain.TransportHandover{}, entryByPickup
	}
	version, named := participation.EntryHandoverVersion()
	if !named {
		return domain.TransportHandover{}, entryHandoverNotOnRegister
	}
	handovers, err := view.FindHandedOverByVersion(ctx, tenant, participation.Object(), version)
	if err != nil {
		return domain.TransportHandover{}, entryHandoverUnreadable
	}
	var entry domain.TransportHandover
	matched := 0
	for _, handover := range handovers {
		if participation.EnteredByHandover(handover) {
			entry, matched = handover, matched+1
		}
	}
	switch {
	case matched == 0:
		return domain.TransportHandover{}, entryHandoverNotOnRegister
	case matched > 1:
		return domain.TransportHandover{}, entryHandoverAmbiguous
	}
	return entry, entryHandoverFound
}
