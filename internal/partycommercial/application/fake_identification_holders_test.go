package application_test

import (
	"context"
	"sort"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件给 fakePartyRegistry 补上查重读口（ports.IdentificationHolderLookup），口径照真适配器：只看各身份的最新修订，
// 在用指没停用或停用时点未到；真适配器在事务内取的咨询锁，内存替身里没有并发可防，不模拟。

func liveNow(lifecycle domain.IdentityLifecycle) bool {
	_, at, deactivated := lifecycle.Deactivation()
	return !deactivated || at.After(time.Now())
}

func (registry *fakePartyRegistry) FindRegistrationNumberHolders(
	_ context.Context, tenant domain.TenantID, identity domain.LegalEntityIdentityLayer,
) ([]ports.RegistrationNumberHolder, error) {
	holders := make([]ports.RegistrationNumberHolder, 0)
	for _, number := range identity.Numbers() {
		for key := range registry.parties {
			latest, _ := latestRevision(registry.parties, key, domain.BusinessPartyRegistration.Revision)
			layer, has := latest.IdentityLayer()
			if latest.Party().Tenant() != tenant || !has || !liveNow(latest.Lifecycle()) || layer.Country() != identity.Country() {
				continue
			}
			for _, held := range layer.Numbers() {
				if held == number {
					holders = append(holders, ports.RegistrationNumberHolder{
						Kind: ports.IdentificationHeldByBusinessParty, ID: latest.Party().ID().String(),
						Party: latest.Party().ID().String(), TypeCode: number.TypeCode().String(), Number: number.Number().String(),
					})
				}
			}
		}
		for key := range registry.entities {
			latest, _ := latestRevision(registry.entities, key, domain.LegalEntityRegistration.Revision)
			layer, has := latest.IdentityLayer()
			if latest.Entity().Tenant() != tenant || !has || !liveNow(latest.Lifecycle()) || layer.Country() != identity.Country() {
				continue
			}
			for _, held := range layer.Numbers() {
				if held == number {
					holders = append(holders, ports.RegistrationNumberHolder{
						Kind: ports.IdentificationHeldByLegalEntity, ID: latest.Entity().ID().String(),
						Party: latest.Entity().Party().String(), TypeCode: number.TypeCode().String(), Number: number.Number().String(),
					})
				}
			}
		}
	}
	sort.Slice(holders, func(i, j int) bool { return holders[i].ID < holders[j].ID })
	return holders, nil
}

func (registry *fakePartyRegistry) FindExternalReferenceHolders(
	_ context.Context, tenant domain.TenantID, references []domain.ExternalReference,
) ([]ports.ExternalReferenceHolder, error) {
	holders := make([]ports.ExternalReferenceHolder, 0)
	for _, reference := range references {
		for key := range registry.parties {
			latest, _ := latestRevision(registry.parties, key, domain.BusinessPartyRegistration.Revision)
			if latest.Party().Tenant() != tenant || !liveNow(latest.Lifecycle()) {
				continue
			}
			for _, held := range latest.ExternalReferences() {
				if held == reference {
					holders = append(holders, ports.ExternalReferenceHolder{
						Party: latest.Party().ID().String(), System: reference.System().String(),
						Purpose: reference.Purpose().String(), Code: reference.Code(),
					})
				}
			}
		}
	}
	sort.Slice(holders, func(i, j int) bool { return holders[i].Party < holders[j].Party })
	return holders, nil
}

func (registry *fakePartyRegistry) HasBusinessPartyProfile(
	_ context.Context, tenant domain.TenantID, party domain.PartyID,
) (bool, error) {
	return registry.profiled[registryKey(tenant.String(), party.String())], nil
}

func (registry *fakePartyRegistry) FindLegalEntitiesPinning(
	_ context.Context, tenant domain.TenantID, party domain.PartyID,
) ([]string, error) {
	entities := make([]string, 0)
	for key := range registry.entities {
		latest, _ := latestRevision(registry.entities, key, domain.LegalEntityRegistration.Revision)
		if latest.Entity().Tenant() == tenant && latest.Entity().Party() == party {
			entities = append(entities, latest.Entity().ID().String())
		}
	}
	sort.Strings(entities)
	return entities, nil
}
