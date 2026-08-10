package domain

import (
	"fmt"
	"sort"
	"strings"
)

type RateTableFamily string

const (
	RateTableFamilyWeightZone    RateTableFamily = "WEIGHT_ZONE"
	RateTableFamilyFirstContinue RateTableFamily = "FIRST_CONTINUE"
	RateTableFamilyUnitPrice     RateTableFamily = "UNIT_PRICE"
)

func (family RateTableFamily) valid() bool {
	switch family {
	case RateTableFamilyWeightZone, RateTableFamilyFirstContinue, RateTableFamilyUnitPrice:
		return true
	default:
		return false
	}
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

// RateTableVersion 声明一个价表族，并且只携带该族的费率。不同族之间数据构成不同——
// 重量段阶梯、首重加续重、计费重乘单价各自存的东西不一样——所以它们不能合成一个靠标签
// 区分的切片。
type RateTableVersion struct {
	reference     VersionReference
	family        RateTableFamily
	currency      Currency
	unit          WeightUnit
	period        EffectivePeriod
	entries       []RateEntry
	firstContinue []FirstContinueRate
	unitPrice     []UnitPriceRate
}

func NewRateTableVersion(
	reference VersionReference,
	family RateTableFamily,
	currency Currency,
	unit WeightUnit,
	period EffectivePeriod,
	entries []RateEntry,
) (RateTableVersion, error) {
	if reference.kind != ArtifactRateTable || !reference.valid() || !currency.valid() || !unit.valid() || !period.valid() || len(entries) == 0 {
		return RateTableVersion{}, ErrInvalidRateTable
	}
	if family != RateTableFamilyWeightZone {
		return RateTableVersion{}, fmt.Errorf("%w: bracket entries only describe %s", ErrInvalidRateTable, RateTableFamilyWeightZone)
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

	// 即使调用方以不同顺序传入档位，也保持存储顺序稳定。这让评价与重放不依赖
	// 输入集合的顺序。
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
		family:    family,
		currency:  currency,
		unit:      unit,
		period:    period,
		entries:   copyOfEntries,
	}, nil
}

func (table RateTableVersion) Reference() VersionReference      { return table.reference }
func (table RateTableVersion) Family() RateTableFamily          { return table.family }
func (table RateTableVersion) Currency() Currency               { return table.currency }
func (table RateTableVersion) WeightUnit() WeightUnit           { return table.unit }
func (table RateTableVersion) EffectivePeriod() EffectivePeriod { return table.period }

func (table RateTableVersion) Entries() []RateEntry {
	return append([]RateEntry(nil), table.entries...)
}

func (table RateTableVersion) FirstContinueRates() []FirstContinueRate {
	return append([]FirstContinueRate(nil), table.firstContinue...)
}

func (table RateTableVersion) UnitPriceRates() []UnitPriceRate {
	return append([]UnitPriceRate(nil), table.unitPrice...)
}

func (table RateTableVersion) valid() bool {
	if table.reference.kind != ArtifactRateTable || !table.reference.valid() || !table.family.valid() || !table.currency.valid() || !table.unit.valid() || !table.period.valid() {
		return false
	}
	switch table.family {
	case RateTableFamilyWeightZone:
		if len(table.entries) == 0 || len(table.firstContinue) > 0 || len(table.unitPrice) > 0 {
			return false
		}
		_, err := NewRateTableVersion(table.reference, table.family, table.currency, table.unit, table.period, table.entries)
		return err == nil
	case RateTableFamilyFirstContinue:
		if len(table.firstContinue) == 0 || len(table.entries) > 0 || len(table.unitPrice) > 0 {
			return false
		}
		_, err := NewFirstContinueRateTable(table.reference, table.currency, table.unit, table.period, table.firstContinue)
		return err == nil
	case RateTableFamilyUnitPrice:
		if len(table.unitPrice) == 0 || len(table.entries) > 0 || len(table.firstContinue) > 0 {
			return false
		}
		_, err := NewUnitPriceRateTable(table.reference, table.currency, table.unit, table.period, table.unitPrice)
		return err == nil
	default:
		return false
	}
}

// NewFirstContinueRateTable 为每个分区声明一条首重加续重费率。这里没有区间需要检查
// 空档或重叠，因为单条费率就为其分区内的所有重量定价。
func NewFirstContinueRateTable(
	reference VersionReference,
	currency Currency,
	unit WeightUnit,
	period EffectivePeriod,
	rates []FirstContinueRate,
) (RateTableVersion, error) {
	if reference.kind != ArtifactRateTable || !reference.valid() || !currency.valid() || !unit.valid() || !period.valid() || len(rates) == 0 {
		return RateTableVersion{}, ErrInvalidRateTable
	}
	copyOfRates := append([]FirstContinueRate(nil), rates...)
	seenIDs := make(map[string]struct{}, len(copyOfRates))
	seenZones := make(map[string]struct{}, len(copyOfRates))
	for _, rate := range copyOfRates {
		if !rate.valid() || rate.firstWeight.unit != unit || rate.firstAmount.currency != currency {
			return RateTableVersion{}, ErrInvalidRateTable
		}
		if _, exists := seenIDs[rate.id.String()]; exists {
			return RateTableVersion{}, fmt.Errorf("%w: %s", ErrDuplicateRateEntry, rate.id.String())
		}
		seenIDs[rate.id.String()] = struct{}{}
		if _, exists := seenZones[rate.zone]; exists {
			return RateTableVersion{}, fmt.Errorf("%w: zone %s", ErrRateTableConflict, rate.zone)
		}
		seenZones[rate.zone] = struct{}{}
	}
	sort.SliceStable(copyOfRates, func(left, right int) bool {
		return copyOfRates[left].zone < copyOfRates[right].zone
	})
	return RateTableVersion{
		reference:     reference,
		family:        RateTableFamilyFirstContinue,
		currency:      currency,
		unit:          unit,
		period:        period,
		firstContinue: copyOfRates,
	}, nil
}

// NewUnitPriceRateTable 为每个分区声明一个单价。
func NewUnitPriceRateTable(
	reference VersionReference,
	currency Currency,
	unit WeightUnit,
	period EffectivePeriod,
	rates []UnitPriceRate,
) (RateTableVersion, error) {
	if reference.kind != ArtifactRateTable || !reference.valid() || !currency.valid() || !unit.valid() || !period.valid() || len(rates) == 0 {
		return RateTableVersion{}, ErrInvalidRateTable
	}
	copyOfRates := append([]UnitPriceRate(nil), rates...)
	seenIDs := make(map[string]struct{}, len(copyOfRates))
	seenZones := make(map[string]struct{}, len(copyOfRates))
	for _, rate := range copyOfRates {
		if !rate.valid() || rate.amountPerUnit.currency != currency {
			return RateTableVersion{}, ErrInvalidRateTable
		}
		if _, exists := seenIDs[rate.id.String()]; exists {
			return RateTableVersion{}, fmt.Errorf("%w: %s", ErrDuplicateRateEntry, rate.id.String())
		}
		seenIDs[rate.id.String()] = struct{}{}
		if _, exists := seenZones[rate.zone]; exists {
			return RateTableVersion{}, fmt.Errorf("%w: zone %s", ErrRateTableConflict, rate.zone)
		}
		seenZones[rate.zone] = struct{}{}
	}
	sort.SliceStable(copyOfRates, func(left, right int) bool {
		return copyOfRates[left].zone < copyOfRates[right].zone
	})
	return RateTableVersion{
		reference: reference,
		family:    RateTableFamilyUnitPrice,
		currency:  currency,
		unit:      unit,
		period:    period,
		unitPrice: copyOfRates,
	}, nil
}

func (table RateTableVersion) Lookup(zone string, weight Weight) (RateSelection, error) {
	if !table.valid() {
		return RateSelection{}, ErrInvalidRateTable
	}
	if strings.TrimSpace(zone) == "" || strings.TrimSpace(zone) != zone {
		return RateSelection{}, ErrNoMatchingRate
	}
	if !weight.valid() {
		return RateSelection{}, ErrInvalidWeight
	}
	if weight.unit != table.unit {
		return RateSelection{}, ErrWeightUnitMismatch
	}
	switch table.family {
	case RateTableFamilyWeightZone:
		return table.lookupBracket(zone, weight)
	case RateTableFamilyFirstContinue:
		for _, rate := range table.firstContinue {
			if rate.zone != zone {
				continue
			}
			amount, explanation, err := rate.price(weight)
			if err != nil {
				return RateSelection{}, err
			}
			return RateSelection{family: table.family, id: rate.id, zone: zone, amount: amount, explanation: explanation}, nil
		}
	case RateTableFamilyUnitPrice:
		for _, rate := range table.unitPrice {
			if rate.zone != zone {
				continue
			}
			amount, explanation, err := rate.price(weight)
			if err != nil {
				return RateSelection{}, err
			}
			return RateSelection{family: table.family, id: rate.id, zone: zone, amount: amount, explanation: explanation}, nil
		}
	}
	return RateSelection{}, fmt.Errorf("%w: zone %s, weight %s %s", ErrNoMatchingRate, zone, weight.value.String(), weight.unit)
}

func (table RateTableVersion) lookupBracket(zone string, weight Weight) (RateSelection, error) {
	var match *RateEntry
	for index := range table.entries {
		entry := &table.entries[index]
		if entry.zone != zone || !entry.contains(weight) {
			continue
		}
		if match != nil {
			return RateSelection{}, fmt.Errorf("%w: zone %s", ErrRateTableConflict, zone)
		}
		match = entry
	}
	if match == nil {
		return RateSelection{}, fmt.Errorf("%w: zone %s, weight %s %s", ErrNoMatchingRate, zone, weight.value.String(), weight.unit)
	}
	explanation := fmt.Sprintf("rate entry %s matched zone %s and interval [%s,%s) %s",
		match.id.String(), match.zone, match.minimum.value.String(), rateMaximumText(*match), match.minimum.unit)
	return RateSelection{family: table.family, id: match.id, zone: zone, amount: match.amount, explanation: explanation}, nil
}
