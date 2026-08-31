package postgres

import "time"

// 目录读面共用的两个取值助手（票 admin-skeleton-closure-batch/04）。它们只在读面用：
// 写侧的 optionalRef 走的是反方向（域值 → 可空列），两者不合并——合并出来的东西会同时
// 承担「域值转库列」与「库列转转写值」两件事，而这两件事的缺席语义并不相同。

// catalogueText 把可空文本列转成转写用的字串，NULL 转空串。
//
// 这么折不丢信息：库上每个引用列都带 btrim(...) <> ” 的非空白门，所以非空即真值，
// 空串只可能来自 NULL——「这一格没有登记」。可选时刻不能照此办理，见 catalogueInstant。
func catalogueText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// catalogueInstant 把可空时刻列转成转写用的可空时刻，并统一到 UTC。
//
// 保留指针而不折成零时刻：零时刻是一个合法时间值，用它兼表「没发生过」会让两态在
// 类型上分不开，而这两态要人做的事相反（未确认要人去确认，确认于零时要人去查数据）。
func catalogueInstant(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	instant := value.UTC()
	return &instant
}
