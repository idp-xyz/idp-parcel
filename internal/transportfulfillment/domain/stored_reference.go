package domain

import (
	"errors"
	"fmt"
)

// ErrValueTooLongToStore 说明一个引用超过了它进册所需的字节界。它是确定的：同一个值再交多少次都一样，所以编排答`未受理`、
// 不答依赖故障。
var ErrValueTooLongToStore = errors.New("transport fulfillment: value is too long to store")

// storedReferenceMaxBytes 是订舱标识与承运答复标识「存得下、索引得了」的机制界，不是业务口径：两者进订舱册、答复册与承运答复
// 挡回册的主键，btree 索引行有长度上限，不设界就会在写入那一刻整笔回滚、答不出业务结论——承运方照「稍后再交」重交一万次也
// 一样。界远低于那条上限、远高于承运系统与运营给出的标识；按 UTF-8 字节计，因为库里的上限按字节算。先例是 parcel-shipment
// 外部标识号码的字节界。只给进键的引用设界：不进键的格（例如答复依据）撞不到索引上限，设了界只是让一格可能装自由文本的值早早
// 被拒。
//
// 承运答复口的两条挡回路还靠它守住「同一副答复」（ADR-0239 决定三）：挡回册的依据定宽，两条路写的键等长，界让这个键永远在索引
// 上限之下，于是两条路都写得进；超界的标识在查库之前就答`未受理`，两条路也一样。
const storedReferenceMaxBytes = 512

// newStoredReference 在必备值的那道门之后再过字节界。
func newStoredReference(name, value string) (requiredValue, error) {
	required, err := newRequiredValue(name, value)
	if err != nil {
		return requiredValue{}, err
	}
	if len(value) > storedReferenceMaxBytes {
		return requiredValue{}, fmt.Errorf("%w: %s is %d bytes, over the %d-byte limit",
			ErrValueTooLongToStore, name, len(value), storedReferenceMaxBytes)
	}
	return required, nil
}
