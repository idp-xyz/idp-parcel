package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// 身份冲突处置（NO CONTEXT「身份冲突处置」；ADR-0210 决定二至四）：持授予的人在一件判身份冲突的作业实物的处置候选里指定一件，
// 或确认不是其中任何一件。它不是销毁、退回、移交一类改变实物去向的处置决定，只定实物与哪一件正式包裹关联。

// ErrInvalidIdentityDisposition 说明一份身份冲突处置立不起来：结果与指定的包裹、被排除的那一组、连带实物对不上，或缺了必备的
// 格。重发同样的内容不会变好。
var ErrInvalidIdentityDisposition = errors.New("node operations: invalid identity conflict disposition")

// IdentityDispositionResult 是处置的两种结果（ADR-0210 决定二）。
type IdentityDispositionResult uint8

const (
	IdentityDispositionResultInvalid IdentityDispositionResult = iota
	// DispositionDesignatesParcel：关联到处置候选中的一件（甲）。
	DispositionDesignatesParcel
	// DispositionNoneOfCandidates：确认不是其中任何一件（乙）。
	DispositionNoneOfCandidates
)

func (result IdentityDispositionResult) String() string {
	switch result {
	case DispositionDesignatesParcel:
		return "DESIGNATED"
	case DispositionNoneOfCandidates:
		return "NONE_OF_CANDIDATES"
	default:
		return ""
	}
}

// ParseIdentityDispositionResult 按线上名认结果，大小写不折叠。
func ParseIdentityDispositionResult(raw string) (IdentityDispositionResult, error) {
	for _, result := range []IdentityDispositionResult{DispositionDesignatesParcel, DispositionNoneOfCandidates} {
		if result.String() == raw {
			return result, nil
		}
	}
	return IdentityDispositionResultInvalid, fmt.Errorf("%w: result %q", ErrInvalidIdentityDisposition, raw)
}

// CoDisposedUnit 是同一次处置一并确认「不是指定的那件」的一件作业实物，带操作者看到的那一版当前有效版本（ADR-0210 决定四）。
type CoDisposedUnit struct {
	Unit    HandlingUnitID
	Version IntakeResultVersion
}

// IdentityDisposition 是身份冲突处置文档：同一次处置追加的每一版都带同一份（ADR-0210 决定三）。哪一版排除了什么由
// ExcludedFor 按这一版的作业实物给出，不另存——同一次处置的各版因此逐字同一份文档，读的人不必拼几份才知道这次处置了什么。
//
// 零值即「不是处置版」：处置版之外的各版判断不带这份文档。
type IdentityDisposition struct {
	// Disposed 是调用方指名的所处置的版本，也就是重放身份；Unit 是主实物。
	Disposed IntakeResultVersion
	Unit     HandlingUnitID
	Result   IdentityDispositionResult
	// Parcel 只在指定一件时在场。
	Parcel ParcelAssociationReference
	// Excluded 只在确认都不是时在场，是主实物的处置候选。
	Excluded []ParcelAssociationReference
	// CoDisposed 只在指定一件时可能非空，按作业实物定序。
	CoDisposed []CoDisposedUnit
	// Basis 是处置依据引用，去两端空白后逐字保存，不解析、不判真伪（与 ADR-0187 请求证据同一待遇）。
	Basis string
	// Operator 是提交操作者，写作 `OPERATOR/<发行方>#<sub>`（与 ADR-0187 操作证据同一写法）。
	Operator string
	// DisposedAt 是服务端的形成时刻；载荷不带时间。
	DisposedAt time.Time
}

// Recorded 报告这一版带不带处置文档。
func (disposition IdentityDisposition) Recorded() bool {
	return disposition.Result != IdentityDispositionResultInvalid
}

// ExcludedFor 交回这次处置在 unit 那一版上排除的那一组包裹：确认都不是时是处置候选；指定一件时，连带实物排除的只有指定的那件，
// 主实物什么也不排除。处置之后的重核据此不把实物关联回被排除的包裹（ADR-0210 决定九）。
func (disposition IdentityDisposition) ExcludedFor(unit HandlingUnitID) []ParcelAssociationReference {
	switch {
	case disposition.Result == DispositionNoneOfCandidates:
		return slices.Clone(disposition.Excluded)
	case disposition.Result == DispositionDesignatesParcel && unit != disposition.Unit:
		return []ParcelAssociationReference{disposition.Parcel}
	default:
		return nil
	}
}

// CoDisposedUnits 交回连带实物的作业实物标识，按标识定序。
func (disposition IdentityDisposition) CoDisposedUnits() []HandlingUnitID {
	units := make([]HandlingUnitID, 0, len(disposition.CoDisposed))
	for _, co := range disposition.CoDisposed {
		units = append(units, co.Unit)
	}
	return units
}

// FormIdentityDisposition 立一份处置文档并验它的形状：被排除的那一组与连带实物按标识定序，同一份处置落成同一个样子。坏载荷
// 的各格（ADR-0210 决定六）在入口就拦了，这里再验一遍是为了让绕过入口的写入方也立不起一份自相矛盾的文档。
func FormIdentityDisposition(disposition IdentityDisposition) (IdentityDisposition, error) {
	formed := disposition
	formed.Basis = strings.TrimSpace(disposition.Basis)
	formed.DisposedAt = disposition.DisposedAt.UTC()
	formed.Excluded = slices.Clone(disposition.Excluded)
	slices.SortFunc(formed.Excluded, func(left, right ParcelAssociationReference) int {
		return strings.Compare(left.String(), right.String())
	})
	formed.CoDisposed = slices.Clone(disposition.CoDisposed)
	slices.SortFunc(formed.CoDisposed, func(left, right CoDisposedUnit) int {
		return strings.Compare(left.Unit.String(), right.Unit.String())
	})
	if err := formed.validate(); err != nil {
		return IdentityDisposition{}, err
	}
	return formed, nil
}

func (disposition IdentityDisposition) validate() error {
	fail := func(reason string) error { return fmt.Errorf("%w: %s", ErrInvalidIdentityDisposition, reason) }
	switch {
	case !disposition.Disposed.valid() || !disposition.Unit.valid():
		return fail("disposed version and unit are required")
	case disposition.Basis == "" || strings.ContainsRune(disposition.Basis, 0):
		return fail("basis reference is blank or carries NUL")
	case strings.TrimSpace(disposition.Operator) == "":
		return fail("operator is required")
	case disposition.DisposedAt.IsZero():
		return fail("disposed-at is required")
	}
	switch disposition.Result {
	case DispositionDesignatesParcel:
		if !disposition.Parcel.valid() || len(disposition.Excluded) > 0 {
			return fail("a designation names exactly the designated parcel")
		}
		for index, co := range disposition.CoDisposed {
			if !co.Unit.valid() || !co.Version.valid() || co.Unit == disposition.Unit {
				return fail("a co-disposed unit needs its version and is not the disposed unit")
			}
			if index > 0 && disposition.CoDisposed[index-1].Unit == co.Unit {
				return fail("a co-disposed unit is listed twice")
			}
		}
	case DispositionNoneOfCandidates:
		if disposition.Parcel.valid() || len(disposition.CoDisposed) > 0 || len(disposition.Excluded) == 0 {
			return fail("none-of-candidates excludes the candidates and names no parcel or co-disposed unit")
		}
		for index, parcel := range disposition.Excluded {
			if !parcel.valid() || (index > 0 && disposition.Excluded[index-1] == parcel) {
				return fail("an excluded parcel is blank or listed twice")
			}
		}
	default:
		return fail("result is neither a designation nor none-of-candidates")
	}
	return nil
}

// 处置文档的形状版本（ADR-0014）。与 NOC-1 是两份文档、各自换版：NOC-1 只收重放比对的那几格（结果、指定的包裹、连带实物的
// 集合），处置文档另收依据引用、操作者与形成时刻，它们不进比对（ADR-0210 决定六）。
const identityDispositionShapeVersion = "NID-1"

// identityDispositionDocument 不带 omitempty，理由同 deliverySourceDocument：缺席格写空值、不省键。
type identityDispositionDocument struct {
	Shape           string                   `json:"shape"`
	Unit            string                   `json:"unit"`
	DisposedVersion string                   `json:"disposed_version"`
	Result          string                   `json:"result"`
	Parcel          string                   `json:"parcel"`
	Excluded        []string                 `json:"excluded"`
	CoDisposed      []coDisposedUnitDocument `json:"co_disposed"`
	Basis           string                   `json:"basis"`
	Operator        string                   `json:"operator"`
	DisposedAt      string                   `json:"disposed_at"`
}

type coDisposedUnitDocument struct {
	Unit    string `json:"unit"`
	Version string `json:"version"`
}

// CanonicalizeIdentityDisposition 把处置文档写成 NID-1 形状，时刻按 UTC。
func CanonicalizeIdentityDisposition(disposition IdentityDisposition) ([]byte, error) {
	if err := disposition.validate(); err != nil {
		return nil, err
	}
	document := identityDispositionDocument{
		Shape:           identityDispositionShapeVersion,
		Unit:            disposition.Unit.String(),
		DisposedVersion: disposition.Disposed.String(),
		Result:          disposition.Result.String(),
		Parcel:          disposition.Parcel.String(),
		Excluded:        []string{},
		CoDisposed:      []coDisposedUnitDocument{},
		Basis:           disposition.Basis,
		Operator:        disposition.Operator,
		DisposedAt:      disposition.DisposedAt.UTC().Format(time.RFC3339Nano),
	}
	for _, parcel := range disposition.Excluded {
		document.Excluded = append(document.Excluded, parcel.String())
	}
	for _, co := range disposition.CoDisposed {
		document.CoDisposed = append(document.CoDisposed, coDisposedUnitDocument{Unit: co.Unit.String(), Version: co.Version.String()})
	}
	return json.Marshal(document)
}

// ParseIdentityDisposition 读回一份处置文档。形状版本认不出就拒，理由同 ParseDeliverySource；读回的值逐个过构造器，再验一遍形状，
// 一份坏文档在这里响亮，而不是变成一次看起来合法的处置。
func ParseIdentityDisposition(raw []byte) (IdentityDisposition, error) {
	var document identityDispositionDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return IdentityDisposition{}, fmt.Errorf("%w: %v", ErrInvalidIdentityDisposition, err)
	}
	if document.Shape != identityDispositionShapeVersion {
		return IdentityDisposition{}, fmt.Errorf("%w: shape %q", ErrInvalidIdentityDisposition, document.Shape)
	}
	wrap := func(err error) error { return fmt.Errorf("%w: %v", ErrInvalidIdentityDisposition, err) }
	unit, err := NewHandlingUnitID(document.Unit)
	if err != nil {
		return IdentityDisposition{}, wrap(err)
	}
	disposed, err := NewIntakeResultVersion(document.DisposedVersion)
	if err != nil {
		return IdentityDisposition{}, wrap(err)
	}
	result, err := ParseIdentityDispositionResult(document.Result)
	if err != nil {
		return IdentityDisposition{}, err
	}
	disposedAt, err := time.Parse(time.RFC3339Nano, document.DisposedAt)
	if err != nil {
		return IdentityDisposition{}, wrap(err)
	}
	disposition := IdentityDisposition{
		Disposed:   disposed,
		Unit:       unit,
		Result:     result,
		Basis:      document.Basis,
		Operator:   document.Operator,
		DisposedAt: disposedAt.UTC(),
	}
	if document.Parcel != "" {
		if disposition.Parcel, err = NewParcelAssociationReference(document.Parcel); err != nil {
			return IdentityDisposition{}, wrap(err)
		}
	}
	for _, raw := range document.Excluded {
		parcel, err := NewParcelAssociationReference(raw)
		if err != nil {
			return IdentityDisposition{}, wrap(err)
		}
		disposition.Excluded = append(disposition.Excluded, parcel)
	}
	for _, co := range document.CoDisposed {
		coUnit, err := NewHandlingUnitID(co.Unit)
		if err != nil {
			return IdentityDisposition{}, wrap(err)
		}
		version, err := NewIntakeResultVersion(co.Version)
		if err != nil {
			return IdentityDisposition{}, wrap(err)
		}
		disposition.CoDisposed = append(disposition.CoDisposed, CoDisposedUnit{Unit: coUnit, Version: version})
	}
	if err := disposition.validate(); err != nil {
		return IdentityDisposition{}, err
	}
	return disposition, nil
}

type identityDispositionComparisonDocument struct {
	Canonicalization string   `json:"canonicalization"`
	Face             string   `json:"face"`
	Result           string   `json:"result"`
	Parcel           string   `json:"parcel"`
	CoDisposedUnits  []string `json:"co_disposed_units"`
}

// CanonicalizeIdentityDispositionComparison 定身份冲突处置口比对格的 NOC-1 形状（ADR-0210 决定六）：结果、指定的包裹、连带实物的
// 集合（只比实物）。处置依据引用与操作者不进来（ADR-0187 决定四）；所处置的版本是重放身份，也不进来。同一次处置追加的各版
// 内容指纹都是这一个摘要。
func CanonicalizeIdentityDispositionComparison(
	result IdentityDispositionResult,
	parcel ParcelAssociationReference,
	coDisposed []HandlingUnitID,
) ([]byte, string, error) {
	units := make([]string, 0, len(coDisposed))
	for _, unit := range coDisposed {
		units = append(units, unit.String())
	}
	slices.Sort(units)
	return marshalPayload(identityDispositionComparisonDocument{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "DISPOSE_IDENTITY_CONFLICT",
		Result:           result.String(),
		Parcel:           parcel.String(),
		CoDisposedUnits:  units,
	})
}
