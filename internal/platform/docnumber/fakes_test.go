package docnumber

import (
	"context"
	"errors"
	"sort"
	"time"

	"go.idp.xyz/idp-platform-go/numbering"
)

// memoryBook 是规则册的内存替身：与 docnumberpg.Store 同一组语义（同键不落、生效时刻不晚于 at 的最晚一版）。
type memoryBook struct {
	rules []RuleVersion
	zones []TimeZoneVersion
}

func (book *memoryBook) AppendRuleVersion(_ context.Context, version RuleVersion) (RuleVersion, bool, error) {
	for _, stored := range book.rules {
		if stored.Tenant == version.Tenant && stored.Type == version.Type && stored.EffectiveFrom.Equal(version.EffectiveFrom) {
			return stored, false, nil
		}
	}
	book.rules = append(book.rules, version)
	return version, true, nil
}

func (book *memoryBook) AppendTimeZoneVersion(_ context.Context, version TimeZoneVersion) (TimeZoneVersion, bool, error) {
	for _, stored := range book.zones {
		if stored.Tenant == version.Tenant && stored.EffectiveFrom.Equal(version.EffectiveFrom) {
			return stored, false, nil
		}
	}
	book.zones = append(book.zones, version)
	return version, true, nil
}

func (book *memoryBook) EffectiveRuleVersion(_ context.Context, tenant string, code TypeCode, at time.Time) (RuleVersion, bool, error) {
	var found *RuleVersion
	for index, stored := range book.rules {
		if stored.Tenant != tenant || stored.Type != code || stored.EffectiveFrom.After(at) {
			continue
		}
		if found == nil || stored.EffectiveFrom.After(found.EffectiveFrom) {
			found = &book.rules[index]
		}
	}
	if found == nil {
		return RuleVersion{}, false, nil
	}
	return *found, true, nil
}

func (book *memoryBook) EffectiveTimeZoneVersion(_ context.Context, tenant string, at time.Time) (TimeZoneVersion, bool, error) {
	var found *TimeZoneVersion
	for index, stored := range book.zones {
		if stored.Tenant != tenant || stored.EffectiveFrom.After(at) {
			continue
		}
		if found == nil || stored.EffectiveFrom.After(found.EffectiveFrom) {
			found = &book.zones[index]
		}
	}
	if found == nil {
		return TimeZoneVersion{}, false, nil
	}
	return *found, true, nil
}

func (book *memoryBook) RuleVersions(_ context.Context, tenant string) ([]RuleVersion, error) {
	var versions []RuleVersion
	for _, stored := range book.rules {
		if stored.Tenant == tenant {
			versions = append(versions, stored)
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].Type != versions[j].Type {
			return versions[i].Type < versions[j].Type
		}
		return versions[i].EffectiveFrom.Before(versions[j].EffectiveFrom)
	})
	return versions, nil
}

func (book *memoryBook) TimeZoneVersions(_ context.Context, tenant string) ([]TimeZoneVersion, error) {
	var versions []TimeZoneVersion
	for _, stored := range book.zones {
		if stored.Tenant == tenant {
			versions = append(versions, stored)
		}
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].EffectiveFrom.Before(versions[j].EffectiveFrom) })
	return versions, nil
}

// memorySequences 是序列的内存替身，记下每次取值用的键。
type memorySequences struct {
	next map[numbering.SequenceKey]int64
	keys []numbering.SequenceKey
}

func (sequences *memorySequences) Next(_ context.Context, key numbering.SequenceKey) (int64, error) {
	if sequences.next == nil {
		sequences.next = map[numbering.SequenceKey]int64{}
	}
	sequences.next[key]++
	sequences.keys = append(sequences.keys, key)
	return sequences.next[key], nil
}

// transactionScope 是事务归属的替身：inside 为假时答不在事务里。
type transactionScope struct{ inside bool }

var errNoTransaction = errors.New("no transaction")

func (scope transactionScope) RequireTransaction(context.Context) error {
	if !scope.inside {
		return errNoTransaction
	}
	return nil
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

// testDeclarations 是测试用的一组声明：照 ADR-0355 决定五的两类典型——按日重置且开立方没有业务时区的，与按月重置
// 且开立方有业务时区的。
var testDeclarations = []TypeDeclaration{
	{Code: "SHIPMENT", DefaultTemplate: "P{yyyy}{mm}{dd}{seq:6}", DateTimeZone: DateTimeZoneFromTenant},
	{Code: "DELIVERY_PRE_ADVICE", DefaultTemplate: "PA-{yyyy}{mm}{dd}-{seq:5}", DateTimeZone: DateTimeZoneFromIssuer},
	{Code: "CUSTOMER_STATEMENT", DefaultTemplate: "STMT-{yyyy}{mm}-{seq:5}", DateTimeZone: DateTimeZoneFromIssuer},
}

func testCatalogue() *Catalogue {
	catalogue, err := NewCatalogue(testDeclarations)
	if err != nil {
		panic(err)
	}
	return catalogue
}

const testTenant = "SYN-TENANT-01"

// testSecret 是测试用的加扰密钥，够最短长度。
const testSecret = "syn-scramble-secret-0123456789abcdef"
