package ports

import "context"

// BookingAnswerHistory 交回一次订舱名下的全部承运答复，按答复时刻先后。一次订舱至多一份候补、至多一份定局（ADR-0202
// 补记「候补之后可接一份定局」）；没有答复交空切片，不交 error。
//
// 另立一口而不在 BookingAnswerStore 上加方法：那个口的 FindByKey 只交当前那一份，够读的一方用；判一份新答复是重放、
// 冲突还是候补的后继，要看见同一订舱名下的每一份——同一答复标识可能是早先那份候补，也可能是定局。
type BookingAnswerHistory interface {
	FindAnswers(ctx context.Context, key BookingKey) ([]BookingAnswerRecord, error)
}

// BookingAnswerLedger 是承运答复编排对答复册的全部依赖：看同一订舱名下的全部答复，并落新的一份。Save 撞上任何一条唯一
// 约束（同一答复标识、第二份候补、第二份定局）都答 BookingAlreadyAnswered，不覆盖先到者，由编排读回再判。
type BookingAnswerLedger interface {
	BookingAnswerHistory
	Save(ctx context.Context, record BookingAnswerRecord) (BookingAnswerSaveOutcome, error)
}
