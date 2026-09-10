// Package parcelshipment 是 transport-fulfillment 消费 parcel-shipment 事实的适配器（ADR-0025 消费方侧；只有本包
// 可以同时导入两个上下文；只翻译不判断，翻译必须是全函数）。
//
// 首发只有一条缝：末端派送任务七件里的**地点**（ADR-0114 决定三；票 tf-segment-lifecycle-closure/12）。PS 交出的是
// 「收件地点引用」——租户、委托、收件资料范围、资料版本锚四段的不透明串（ADR-0130 决定一），落进任务的 Place 也是
// 这个串；地址本体留在 PS，本包不读、不落、不解析它（ADR-0075 要防的那一半在这里守住）。
package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUntranslatableAnswer 与其余跨上下文适配器的同名哨兵同义：某一侧交出了词汇表之外的内容，是编程错误不是业务
// 答案，不折成「没有」——折了会让一次适配器故障变成「所有者说这个对象没有收件地点」，任务永远待形成而原因不可见。
var ErrUntranslatableAnswer = errors.New("transport fulfillment parcelshipment adapter: untranslatable answer")

// DeliveryPlaceReferenceLookup 是本适配器向 parcel-shipment 取收件地点引用的窄口。真实装配交给 PS 的
// adapters/postgres.ShipmentRequests（它兼实现 PS ports.DeliveryPlaceReferenceView）；这里只声明读的那一个方法，
// 不 import PS 的 application，理由同 ps-port-remainder/05 的 ParcelDeclarationFactsLookup——测试替身不必背上提供方
// 的整个端口面，消费侧拿到的也不该是 PS 的写口。
type DeliveryPlaceReferenceLookup interface {
	LoadDeliveryPlaceReference(
		ctx context.Context,
		tenant psdomain.TenantID,
		parcel psdomain.DeclaredParcelID,
	) (psdomain.DeliveryPlaceResolution, error)
}

// DeliveryPlaceSource 实现 tfports.DeliveryPlaceSource：把 PS 读口的封闭四格译成端口的两格。
//
// **全函数翻译，四格逐格有落点**（票面做法「适配器」那一步）：
//   - 基线锚引用 / 已采用版本锚引用 → RequirementResolved + 引用的 String() **逐字**——不重拼、不改一字，TF 拿新旧两个
//     串比「地址变了没有」比的正是这个字面（ADR-0130 决定三）。
//   - 没有收件地点 → RequirementMissing：所有者说这个对象没有收件地点，业务答案不是错误；集运单元引用在 PS 那一侧本就
//     不属任何已接受委托的成员集合，落的也是这一格（票面「所有者」节：集运单元不属本缝）。不补默认、不拿目的地节点顶替。
//   - 收件地点未定 → **取甲**（票面做法「先定`待复核`在 TF 端口上的落法」那一步的默认；ADR-0130 越权风险点 1）：
//     译成 RequirementMissing。**所有者说的是「未定」，不是「没有」**——PS 此刻说不出该送哪一版（两条修订分叉等人来并），
//     端口今天没有「未定」那一格，任务同样保持待形成；拍到分叉并掉之后同一拍重跑就拿到引用。要给端口加「未定」那一格得先
//     证明时间窗与条件两条缝也用得上（同一步写的默认判据），届时改这里一格与 TF CONTEXT「派送要求」那句。
//   - 读口 error 原样上抛，让执行器落它既有的「读不到」格（DELIVERY_PLACE_SOURCE_UNAVAILABLE）。
//   - 四格之外的取值上抛 ErrUntranslatableAnswer。
//
// 载运对象引用 → PS 声明包裹身份的翻译在这里做：两个上下文里是同一串字面（与 CC / NO 消费适配器把关联引用直接当
// 包裹标识用同一条约定），PS 不认识 TF 的词。
type DeliveryPlaceSource struct {
	references DeliveryPlaceReferenceLookup
}

func NewDeliveryPlaceSource(references DeliveryPlaceReferenceLookup) (*DeliveryPlaceSource, error) {
	if references == nil {
		return nil, fmt.Errorf("transport fulfillment parcelshipment adapter: delivery place reference lookup is nil")
	}
	return &DeliveryPlaceSource{references: references}, nil
}

var _ tfports.DeliveryPlaceSource = (*DeliveryPlaceSource)(nil)

// LoadDeliveryPlace 按（租户，载运对象）向 PS 取收件地点引用并译成端口两格。
func (source *DeliveryPlaceSource) LoadDeliveryPlace(
	ctx context.Context,
	tenant tfdomain.TenantID,
	object tfdomain.CarriedObjectReference,
) (string, tfports.RequirementResolution, error) {
	psTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return "", tfports.RequirementResolutionInvalid, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	parcel, err := psdomain.NewDeclaredParcelID(object.String())
	if err != nil {
		return "", tfports.RequirementResolutionInvalid, fmt.Errorf("%w: carried object: %v", ErrUntranslatableAnswer, err)
	}
	resolution, err := source.references.LoadDeliveryPlaceReference(ctx, psTenant, parcel)
	if err != nil {
		return "", tfports.RequirementResolutionInvalid, fmt.Errorf("load delivery place: %w", err)
	}

	switch resolution.Outcome() {
	case psdomain.DeliveryPlaceAnchoredOnBaseline, psdomain.DeliveryPlaceAnchoredOnAdoptedVersion:
		reference, present := resolution.Reference()
		if !present {
			// 带引用的两格却没带引用是 PS 读口违约，不是「没有」。
			return "", tfports.RequirementResolutionInvalid, fmt.Errorf("%w: outcome %q carries no reference",
				ErrUntranslatableAnswer, resolution.Outcome())
		}
		return reference.String(), tfports.RequirementResolved, nil
	case psdomain.DeliveryPlaceUndetermined:
		return "", tfports.RequirementMissing, nil
	case psdomain.NoDeliveryPlace:
		return "", tfports.RequirementMissing, nil
	}
	return "", tfports.RequirementResolutionInvalid, fmt.Errorf("%w: delivery place outcome %q",
		ErrUntranslatableAnswer, resolution.Outcome())
}
