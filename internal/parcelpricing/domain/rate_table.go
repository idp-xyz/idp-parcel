package domain

import (
	"fmt"
	"sort"
	"strings"
)

type RateTableKind string

const RateTableKindWeightZone RateTableKind = "WEIGHT_ZONE"

func (kind RateTableKind) valid() bool {
	return kind == RateTableKindWeightZone
}

type RateEntry struct {
	id         RateEntryID
	zone       string
	minimum    Weight
	maximum    Weight
	hasMaximum bool
	amount     Money
}

func NewRateEntry(
	id RateEntryID,
	zone string,
	minimum Weight,
	maximum Weight,
	amount Money,
) (RateEntry, error) {
	if !id.valid() || strings.TrimSpace(zone) == "" || strings.TrimSpace(zone) != zone || !minimum.valid() || !maximum.valid() || !amount.valid() {
		return RateEntry{}, ErrInvalidRateEntry
	}
	if minimum.unit != maximum.unit {
		return RateEntry{}, ErrWeightUnitMismatch
	}
	if maximum.value.Cmp(minimum.value) <= 0 {
		return RateEntry{}, fmt.Errorf("%w: maximum must be greater than minimum", ErrInvalidRateEntry)
	}
	return RateEntry{id: id, zone: zone, minimum: minimum, maximum: maximum, hasMaximum: true, amount: amount}, nil
}

func NewOpenEndedRateEntry(id RateEntryID, zone string, minimum Weight, amount Money) (RateEntry, error) {
	if !id.valid() || strings.TrimSpace(zone) == "" || strings.TrimSpace(zone) != zone || !minimum.valid() || !amount.valid() {
		return RateEntry{}, ErrInvalidRateEntry
	}
	return RateEntry{id: id, zone: zone, minimum: minimum, amount: amount}, nil
}

func (entry RateEntry) ID() RateEntryID { return entry.id }
func (entry RateEntry) Zone() string    { return entry.zone }
func (entry RateEntry) Minimum() Weight { return entry.minimum }
func (entry RateEntry) Maximum() (Weight, bool) {
	return entry.maximum, entry.hasMaximum
}
func (entry RateEntry) Amount() Money { return entry.amount }
func (entry RateEntry) valid() bool {
	if !entry.id.valid() || strings.TrimSpace(entry.zone) == "" || strings.TrimSpace(entry.zone) != entry.zone || !entry.minimum.valid() || !entry.amount.valid() {
		return false
	}
	if !entry.hasMaximum {
		return true
	}
	return entry.maximum.valid() && entry.minimum.unit == entry.maximum.unit && entry.maximum.value.Cmp(entry.minimum.value) > 0
}

func (entry RateEntry) contains(weight Weight) bool {
	if entry.minimum.unit != weight.unit || entry.minimum.value.Cmp(weight.value) > 0 {
		return false
	}
	return !entry.hasMaximum || weight.value.Cmp(entry.maximum.value) < 0
}

type RateTableVersion struct {
	reference VersionReference
	kind      RateTableKind
	currency  Currency
	unit      WeightUnit
	period    EffectivePeriod
	entries   []RateEntry
}

func NewRateTableVersion(
	reference VersionReference,
	kind RateTableKind,
	currency Currency,
	unit WeightUnit,
	period EffectivePeriod,
	entries []RateEntry,
) (RateTableVersion, error) {
	if reference.kind != ArtifactRateTable || !reference.valid() || !kind.valid() || !currency.valid() || !unit.valid() || !period.valid() || len(entries) == 0 {
		return RateTableVersion{}, ErrInvalidRateTable
	}
	copyOfEntries := append([]RateEntry(nil), entries...)
	seenIDs := make(map[string]struct{}, len(copyOfEntries))
	byZone := make(map[string][]RateEntry)
	for _, entry := range copyOfEntries {
		if !entry.valid() || entry.minimum.unit != unit || (entry.hasMaximum && entry.maximum.unit != unit) || entry.amount.currency != currency {
			return RateTableVersion{}, ErrInvalidRateTable
		}
		if _, exists := seenIDs[entry.id.String()]; exists {
			return RateTableVersion{}, fmt.Errorf("%w: %s", ErrDuplicateRateEntry, entry.id.String())
		}
		seenIDs[entry.id.String()] = struct{}{}
		byZone[entry.zone] = append(byZone[entry.zone], entry)
	}
	zones := make([]string, 0, len(byZone))
	for zone := range byZone {
		zones = append(zones, zone)
	}
	sort.Strings(zones)
	for _, zone := range zones {
		zoneEntries := byZone[zone]
		sort.SliceStable(zoneEntries, func(left, right int) bool {
			comparison := zoneEntries[left].minimum.value.Cmp(zoneEntries[right].minimum.value)
			if comparison != 0 {
				return comparison < 0
			}
			return zoneEntries[left].id.String() < zoneEntries[right].id.String()
		})
		for index := 1; index < len(zoneEntries); index++ {
			previous := zoneEntries[index-1]
			if !previous.hasMaximum || previous.maximum.value.Cmp(zoneEntries[index].minimum.value) > 0 {
				return RateTableVersion{}, fmt.Errorf("%w: zone %s", ErrRateIntervalOverlap, zone)
			}
		}
	}

	// Keep the stored order stable even if callers provide entries in a
	// different order. This makes evaluation and replay independent of input
	// collection order.
	sort.SliceStable(copyOfEntries, func(left, right int) bool {
		if copyOfEntries[left].zone != copyOfEntries[right].zone {
			return copyOfEntries[left].zone < copyOfEntries[right].zone
		}
		comparison := copyOfEntries[left].minimum.value.Cmp(copyOfEntries[right].minimum.value)
		if comparison != 0 {
			return comparison < 0
		}
		return copyOfEntries[left].id.String() < copyOfEntries[right].id.String()
	})
	return RateTableVersion{
		reference: reference,
		kind:      kind,
		currency:  currency,
		unit:      unit,
		period:    period,
		entries:   copyOfEntries,
	}, nil
}

func (table RateTableVersion) Reference() VersionReference      { return table.reference }
func (table RateTableVersion) Kind() RateTableKind              { return table.kind }
func (table RateTableVersion) Currency() Currency               { return table.currency }
func (table RateTableVersion) WeightUnit() WeightUnit           { return table.unit }
func (table RateTableVersion) EffectivePeriod() EffectivePeriod { return table.period }

func (table RateTableVersion) Entries() []RateEntry {
	return append([]RateEntry(nil), table.entries...)
}

func (table RateTableVersion) valid() bool {
	if table.reference.kind != ArtifactRateTable || !table.reference.valid() || !table.kind.valid() || !table.currency.valid() || !table.unit.valid() || !table.period.valid() || len(table.entries) == 0 {
		return false
	}
	_, err := NewRateTableVersion(table.reference, table.kind, table.currency, table.unit, table.period, table.entries)
	return err == nil
}

func (table RateTableVersion) Lookup(zone string, weight Weight) (RateEntry, error) {
	if !table.valid() {
		return RateEntry{}, ErrInvalidRateTable
	}
	if strings.TrimSpace(zone) == "" || strings.TrimSpace(zone) != zone {
		return RateEntry{}, ErrNoMatchingRate
	}
	if !weight.valid() {
		return RateEntry{}, ErrInvalidWeight
	}
	if weight.unit != table.unit {
		return RateEntry{}, ErrWeightUnitMismatch
	}
	var match *RateEntry
	for index := range table.entries {
		entry := &table.entries[index]
		if entry.zone != zone || !entry.contains(weight) {
			continue
		}
		if match != nil {
			return RateEntry{}, fmt.Errorf("%w: zone %s", ErrRateTableConflict, zone)
		}
		match = entry
	}
	if match == nil {
		return RateEntry{}, fmt.Errorf("%w: zone %s, weight %s %s", ErrNoMatchingRate, zone, weight.value.String(), weight.unit)
	}
	return *match, nil
}
