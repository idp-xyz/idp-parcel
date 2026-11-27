package accessidentity

import (
	"errors"
	"slices"
	"strings"
	"time"
)

// ErrDataVisibilityRequired 表示「数据可见范围」这一格没带种类与引用。不带的一笔无法登记：
// 空不是「全部」，没授这一类就是这一类什么都看不见（ADR-0385 决定一、二）。
var ErrDataVisibilityRequired = errors.New("access identity: data visibility grant needs a visibility scope")

// ErrDataVisibilityKindUnknown 表示给出的可见范围种类不在封闭集里。
var ErrDataVisibilityKindUnknown = errors.New("access identity: data visibility kind is unknown")

// ErrInvalidDataVisibility 表示可见范围缺引用。全部也是一个引用，不靠留空表达。
var ErrInvalidDataVisibility = errors.New("access identity: data visibility needs a reference")

// DataVisibilityAll 是「这一类全部」的引用。必须写在授予上；省略引用不是全部。
const DataVisibilityAll = "*"

// DataVisibilityKind 是数据可见范围的种类：客户账户，或仓。种类是册的列；某个租户有哪些客户、哪些仓，是租户取值。
type DataVisibilityKind string

const (
	DataVisibilityCustomerAccount DataVisibilityKind = "CUSTOMER_ACCOUNT"
	DataVisibilityWarehouse       DataVisibilityKind = "WAREHOUSE"
)

// DataVisibilityKinds 交出全部可见范围种类，顺序固定：管理台据它摆选项，不在前端抄第二份。
func DataVisibilityKinds() []DataVisibilityKind {
	return []DataVisibilityKind{DataVisibilityCustomerAccount, DataVisibilityWarehouse}
}

// ParseDataVisibilityKind 按字面取值认种类，大小写不折叠。
func ParseDataVisibilityKind(value string) (DataVisibilityKind, error) {
	switch kind := DataVisibilityKind(value); kind {
	case DataVisibilityCustomerAccount, DataVisibilityWarehouse:
		return kind, nil
	}
	return "", ErrDataVisibilityKindUnknown
}

func (kind DataVisibilityKind) String() string { return string(kind) }

// DataVisibility 是一笔数据可见范围：种类，加上该租户的客户账户标识、仓编码，或 DataVisibilityAll。
type DataVisibility struct {
	kind      DataVisibilityKind
	reference string
}

func NewDataVisibility(kind DataVisibilityKind, reference string) (DataVisibility, error) {
	if _, err := ParseDataVisibilityKind(string(kind)); err != nil {
		return DataVisibility{}, err
	}
	trimmed := strings.TrimSpace(reference)
	if trimmed == "" {
		return DataVisibility{}, ErrInvalidDataVisibility
	}
	return DataVisibility{kind: kind, reference: trimmed}, nil
}

func (visibility DataVisibility) Kind() DataVisibilityKind { return visibility.kind }
func (visibility DataVisibility) Reference() string        { return visibility.reference }

// All 答这一笔是不是「这一类全部」。只有引用恰好是 DataVisibilityAll 才是；别的引用都是点名。
func (visibility DataVisibility) All() bool { return visibility.reference == DataVisibilityAll }

func (visibility DataVisibility) valid() bool {
	_, err := ParseDataVisibilityKind(string(visibility.kind))
	return err == nil && strings.TrimSpace(visibility.reference) != ""
}

// NewDataVisibilityGrant 登一笔数据可见范围授予。能力面固定为 CapabilityDataVisibility，种类与引用必填。
func NewDataVisibilityGrant(
	tenantID string,
	grantID string,
	subject OperatorSubject,
	visibility DataVisibility,
	interval EffectiveInterval,
	basis string,
) (OperatorGrant, error) {
	if !visibility.valid() {
		return OperatorGrant{}, ErrDataVisibilityRequired
	}
	tenant := strings.TrimSpace(tenantID)
	id := strings.TrimSpace(grantID)
	reference := strings.TrimSpace(basis)
	if tenant == "" || id == "" || subject.issuer == "" || interval.startsAt.IsZero() || reference == "" {
		return OperatorGrant{}, ErrIncompleteOperatorRegistration
	}
	return OperatorGrant{
		tenantID:       tenant,
		grantID:        id,
		subject:        subject,
		face:           CapabilityDataVisibility,
		dataVisibility: visibility,
		interval:       interval,
		basis:          reference,
	}, nil
}

// DataVisibility 交回这一笔的可见范围；别的能力面上为零值。
func (grant OperatorGrant) DataVisibility() DataVisibility { return grant.dataVisibility }

// CustomerVisibility 答铸造时生效的客户可见范围。all 为真是显式的全部客户；为假时 ids 是看得见的账户，
// 空切片是什么都看不见。没铸出来的零值信封同样是什么都看不见，不是全部。
func (envelope OperatorEnvelope) CustomerVisibility() (all bool, ids []string) {
	if envelope.allCustomers {
		return true, nil
	}
	return false, append([]string(nil), envelope.customerAccounts...)
}

// WarehouseVisibility 答铸造时生效的仓可见范围。形状同 CustomerVisibility。
func (envelope OperatorEnvelope) WarehouseVisibility() (all bool, ids []string) {
	if envelope.allWarehouses {
		return true, nil
	}
	return false, append([]string(nil), envelope.warehouseCodes...)
}

// foldDataVisibility 把此刻生效的数据可见范围收成两份集合。同一类里有「全部」又有点名时，全部覆盖点名。
// 这一格不进能力面持有集：它不是端点门（ADR-0385 决定一）。
func foldDataVisibility(grants []RecordedGrant, at time.Time) (allCustomers bool, customers []string, allWarehouses bool, warehouses []string) {
	for _, recorded := range grants {
		if recorded.grant.face != CapabilityDataVisibility || !recorded.EffectiveAt(at) {
			continue
		}
		visibility := recorded.grant.dataVisibility
		if !visibility.valid() {
			continue
		}
		switch visibility.kind {
		case DataVisibilityCustomerAccount:
			if visibility.All() {
				allCustomers = true
				continue
			}
			if !slices.Contains(customers, visibility.reference) {
				customers = append(customers, visibility.reference)
			}
		case DataVisibilityWarehouse:
			if visibility.All() {
				allWarehouses = true
				continue
			}
			if !slices.Contains(warehouses, visibility.reference) {
				warehouses = append(warehouses, visibility.reference)
			}
		}
	}
	return allCustomers, customers, allWarehouses, warehouses
}
