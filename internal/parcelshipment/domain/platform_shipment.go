package domain

import (
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/platform/storable"
)

// 本文件是 PS CONTEXT「外部标识关联」里平台三种号在委托声明上的那一半（ADR-0374 决定四）：客户交来的平台货件号、平台
// 参考号与每箱的平台箱标号，随提交版本进摘要。分配方是销售平台，号由客户交来，本产品不向平台核对。号码在关联册里
// 形成关联、按号核对，随它们的来源与消费方另做；这里只立声明。

// ErrInvalidPlatformIdentifier 说明平台号的声明立不起来。客户输入的问题，接入口答请求形状不对。
var ErrInvalidPlatformIdentifier = errors.New("parcel shipment: invalid platform identifier")

// PlatformShipmentDeclaration 是委托上的平台货件：一个平台货件一份委托（ADR-0374 决定二）。零值即没报。
type PlatformShipmentDeclaration struct {
	platform  requiredValue
	shipment  ExternalIdentifierNumber
	reference ExternalIdentifierNumber
}

// DeclarePlatformShipment 立一份平台货件声明。销售平台是开放取值、原样保存；参考号可缺，给了就与货件号过同一道入册门。
func DeclarePlatformShipment(platform string, shipment ExternalIdentifierNumber, reference string) (PlatformShipmentDeclaration, error) {
	named, err := newRequiredValue("sales platform", platform)
	if err != nil {
		return PlatformShipmentDeclaration{}, fmt.Errorf("%w: %v", ErrInvalidPlatformIdentifier, err)
	}
	// 销售平台是这几类号的适用范围，日后随号进关联册的索引；与号码同一道字节界，同一条「库存得下」。
	if !storable.Text(platform) || len(platform) > externalIdentifierNumberMaxBytes {
		return PlatformShipmentDeclaration{}, fmt.Errorf("%w: the sales platform is not storable text within %d bytes",
			ErrInvalidPlatformIdentifier, externalIdentifierNumberMaxBytes)
	}
	if err := passesRegisterGate(shipment); err != nil {
		return PlatformShipmentDeclaration{}, fmt.Errorf("%w: shipment: %v", ErrInvalidPlatformIdentifier, err)
	}
	declaration := PlatformShipmentDeclaration{platform: named, shipment: shipment}
	if reference != "" {
		number, err := NewExternalIdentifierNumber(reference)
		if err != nil {
			return PlatformShipmentDeclaration{}, fmt.Errorf("%w: reference: %v", ErrInvalidPlatformIdentifier, err)
		}
		declaration.reference = number
	}
	return declaration, nil
}

func (declaration PlatformShipmentDeclaration) Platform() string {
	return declaration.platform.String()
}

func (declaration PlatformShipmentDeclaration) ShipmentNumber() ExternalIdentifierNumber {
	return declaration.shipment
}

func (declaration PlatformShipmentDeclaration) ReferenceNumber() (ExternalIdentifierNumber, bool) {
	return declaration.reference, declaration.reference.valid()
}

// Declared 报告委托报没报平台货件。
func (declaration PlatformShipmentDeclaration) Declared() bool {
	return declaration.platform.valid() && declaration.shipment.valid()
}

// PlatformBoxLabelDeclaration 是委托上的一组（声明包裹，平台箱标号）：亚马逊一类平台要求每一箱有唯一的箱标。它的分配方
// 是这份委托所报平台货件的销售平台，所以报箱标号就要报平台货件。
type PlatformBoxLabelDeclaration struct {
	parcel DeclaredParcelID
	number ExternalIdentifierNumber
}

func DeclarePlatformBoxLabel(parcel DeclaredParcelID, number ExternalIdentifierNumber) (PlatformBoxLabelDeclaration, error) {
	if !parcel.valid() {
		return PlatformBoxLabelDeclaration{}, ErrInvalidPlatformIdentifier
	}
	if err := passesRegisterGate(number); err != nil {
		return PlatformBoxLabelDeclaration{}, fmt.Errorf("%w: %v", ErrInvalidPlatformIdentifier, err)
	}
	return PlatformBoxLabelDeclaration{parcel: parcel, number: number}, nil
}

// passesRegisterGate 让交进来的号再过一遍入册门（NewExternalIdentifierNumber）：仓储重建的号（RehydrateExternalIdentifierNumber）
// 只拒全空白，拿它当新声明交进来就会绕过 NUL 与字节界，日后写关联册时整笔回滚。
func passesRegisterGate(number ExternalIdentifierNumber) error {
	_, err := NewExternalIdentifierNumber(number.String())
	return err
}

func (declaration PlatformBoxLabelDeclaration) Parcel() DeclaredParcelID {
	return declaration.parcel
}

func (declaration PlatformBoxLabelDeclaration) Number() ExternalIdentifierNumber {
	return declaration.number
}

// platformBoxLabelsByParcel 按成员引用交回箱标号：配对规则与客户包裹件号共用 pairsFitMembers，另要有平台货件。
func platformBoxLabelsByParcel(
	labels []PlatformBoxLabelDeclaration,
	shipment PlatformShipmentDeclaration,
	declared map[DeclaredParcelID]struct{},
) (map[string]string, error) {
	if len(labels) == 0 {
		return nil, nil
	}
	if !shipment.Declared() {
		return nil, fmt.Errorf("%w: platform box labels need a platform shipment", ErrInvalidPlatformIdentifier)
	}
	fits := pairsFitMembers(len(labels), func(index int) (DeclaredParcelID, ExternalIdentifierNumber) {
		return labels[index].parcel, labels[index].number
	}, declared)
	if !fits {
		return nil, fmt.Errorf("%w: box labels must name parcels of the request, at most one each", ErrInvalidPlatformIdentifier)
	}
	// 两箱同号拒：平台给每箱唯一的箱标，同一委托里重号只能是录错（ADR-0374 决定四）。这与客户包裹件号不同——那边重号
	// 留给节点答身份冲突（pairsFitMembers 头注）。
	byParcel := make(map[string]string, len(labels))
	numbers := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		if _, twice := numbers[label.number.String()]; twice {
			return nil, fmt.Errorf("%w: two parcels carry the same box label", ErrInvalidPlatformIdentifier)
		}
		numbers[label.number.String()] = struct{}{}
		byParcel[label.parcel.String()] = label.number.String()
	}
	return byParcel, nil
}
