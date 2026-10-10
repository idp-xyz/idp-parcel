package registrationjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/referenceconfig"
)

// 本文件是采用参考配置的那一半（ADR-0147 决定四）：带 adopt 的一行登记只给身份、修订号与生效时点，内容取自它点名
// 的那一版网络目录参考配置，依据格由这里写成引用串。采用行之后走的是与直接登记同一个登记用例、同一套受理门，内容格
// 的翻译也是同一段（各族的 *Content 类型），参考配置因此过的是与直接登记同一道门（ADR-0147 决定五）。不带 adopt，
// 参考配置就不进任何租户的目录。

// networkCatalogReferenceDirectory 是网络目录参考配置的标识前缀。标识末段（键）是一份网络的自然键：一份参考配置
// 装一张前后接得上的网，各族的行互相引用，拆开采用拼不成路。
const networkCatalogReferenceDirectory = "network-routing/network-catalog/"

var (
	// ErrAdoptedContentGiven 表示采用行自己写了内容或依据：那几格由参考配置与采用路径给，采用行只给身份、修订号与
	// 生效时点。
	ErrAdoptedContentGiven = errors.New("network registration: an adopt row gives identity, revision and effective time only")
	// ErrNotNetworkCatalogReference 表示 adopt 点名的不是网络目录的参考配置。
	ErrNotNetworkCatalogReference = errors.New("network registration: the adopted reference is not a network catalog reference configuration")
	// ErrNotInReference 表示点名的那一版参考配置里没有这一族的这个身份。
	ErrNotInReference = errors.New("network registration: the adopted reference has no entry for this identity")
)

// adoptionRow 是采用行的形状。租户在受控批量口取 tenant_id，在线口取操作者信封（批文带 tenant_id 先已拒）。
type adoptionRow struct {
	TenantID      string     `json:"tenant_id"`
	Code          string     `json:"code"`
	TargetKind    string     `json:"target_kind"`
	TargetCode    string     `json:"target_code"`
	Version       int32      `json:"version"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to"`
	Adopt         string     `json:"adopt"`
}

// 采用行允许出现的键。身份键按族不同：日历是适用对象类别加对象身份，其余各族是 code。
var (
	codeAdoptionKeys     = adoptionKeys("code")
	calendarAdoptionKeys = adoptionKeys("target_kind", "target_code")
)

func adoptionKeys(identity ...string) map[string]bool {
	keys := map[string]bool{"tenant_id": true, "version": true, "effective_from": true, "effective_to": true, "adopt": true}
	for _, key := range identity {
		keys[key] = true
	}
	return keys
}

// adoptionOf 认出一行是不是采用行，是就逐键核、按形状译出。不是 JSON 对象时答「不是采用行」，交给直接登记那一路的
// decodeStrict 去答形状错。
func adoptionOf(raw []byte, allowed map[string]bool) (adoptionRow, bool, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return adoptionRow{}, false, nil
	}
	if _, adopting := top["adopt"]; !adopting {
		return adoptionRow{}, false, nil
	}
	keys := make([]string, 0, len(top))
	for key := range top {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !allowed[key] {
			return adoptionRow{}, true, fmt.Errorf("%w: %s 由参考配置与采用路径给出，采用行不另写", ErrAdoptedContentGiven, key)
		}
	}
	var row adoptionRow
	if err := decodeStrict(raw, &row); err != nil {
		return adoptionRow{}, true, err
	}
	return row, true, nil
}

// networkCatalogReferenceDocument 是网络目录参考配置的原文形状。它只有判断方法与产品通用内容那几格：不放租户、
// 修订号与生效时点（ADR-0146 决定三），那几格由采用行给。
type networkCatalogReferenceDocument struct {
	Identifier       string                     `json:"identifier"`
	Version          int                        `json:"version"`
	Title            string                     `json:"title"`
	Source           string                     `json:"source"`
	Note             string                     `json:"note"`
	Nodes            []referenceNode            `json:"nodes"`
	Connections      []referenceConnection      `json:"connections"`
	Lines            []referenceLine            `json:"lines"`
	ServiceAreas     []referenceServiceArea     `json:"service_areas"`
	ServiceCalendars []referenceServiceCalendar `json:"service_calendars"`
	RouteStrategies  []referenceRouteStrategy   `json:"route_strategies"`
}

type referenceNode struct {
	Code string `json:"code"`
	nodeContent
}

type referenceConnection struct {
	Code string `json:"code"`
	connectionContent
}

type referenceLine struct {
	Code string `json:"code"`
	lineContent
}

type referenceServiceArea struct {
	Code string `json:"code"`
	areaContent
}

type referenceServiceCalendar struct {
	TargetKind string `json:"target_kind"`
	TargetCode string `json:"target_code"`
	calendarContent
}

type referenceRouteStrategy struct {
	Code string `json:"code"`
	strategyContent
}

// adoptedReference 打开 adopt 点名的那一版：形状对、在网络目录之下、已发布且原文与发布摘要一致，交回原文与写进
// 依据格的引用串。
func adoptedReference(text string) (networkCatalogReferenceDocument, domain.CatalogBasisReference, error) {
	none := networkCatalogReferenceDocument{}
	reference, err := referenceconfig.ParseReference(text)
	if err != nil {
		return none, domain.CatalogBasisReference{}, err
	}
	if !strings.HasPrefix(reference.Identifier(), networkCatalogReferenceDirectory) {
		return none, domain.CatalogBasisReference{}, fmt.Errorf("%w: %s", ErrNotNetworkCatalogReference, reference)
	}
	raw, err := referenceconfig.Open(reference)
	if err != nil {
		return none, domain.CatalogBasisReference{}, err
	}
	var document networkCatalogReferenceDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, domain.CatalogBasisReference{}, fmt.Errorf("参考配置 %s 不是网络目录的形状：%w", reference, err)
	}
	basis, err := domain.NewCatalogBasisReference(reference.Citation())
	if err != nil {
		return none, domain.CatalogBasisReference{}, err
	}
	return document, basis, nil
}

func notInReference(adopt, family, identity string) error {
	return fmt.Errorf("%w: %s 里没有%s %s", ErrNotInReference, adopt, family, identity)
}

func (document networkCatalogReferenceDocument) node(code string) (referenceNode, bool) {
	for _, entry := range document.Nodes {
		if entry.Code == code {
			return entry, true
		}
	}
	return referenceNode{}, false
}

func (document networkCatalogReferenceDocument) connection(code string) (referenceConnection, bool) {
	for _, entry := range document.Connections {
		if entry.Code == code {
			return entry, true
		}
	}
	return referenceConnection{}, false
}

func (document networkCatalogReferenceDocument) line(code string) (referenceLine, bool) {
	for _, entry := range document.Lines {
		if entry.Code == code {
			return entry, true
		}
	}
	return referenceLine{}, false
}

func (document networkCatalogReferenceDocument) serviceArea(code string) (referenceServiceArea, bool) {
	for _, entry := range document.ServiceAreas {
		if entry.Code == code {
			return entry, true
		}
	}
	return referenceServiceArea{}, false
}

func (document networkCatalogReferenceDocument) serviceCalendar(kind, code string) (referenceServiceCalendar, bool) {
	for _, entry := range document.ServiceCalendars {
		if entry.TargetKind == kind && entry.TargetCode == code {
			return entry, true
		}
	}
	return referenceServiceCalendar{}, false
}

func (document networkCatalogReferenceDocument) routeStrategy(code string) (referenceRouteStrategy, bool) {
	for _, entry := range document.RouteStrategies {
		if entry.Code == code {
			return entry, true
		}
	}
	return referenceRouteStrategy{}, false
}
