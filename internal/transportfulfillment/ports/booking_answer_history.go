package ports

import "context"

// BookingAnswerLedger 是承运答复编排对答复册的全部依赖：锁住一次订舱、看它名下的全部答复，并落新的一份。
//
// 另立一口而不在 BookingAnswerStore 上加方法：那个口的 FindByKey 只交当前那一份，够读的一方用；判一份新答复是重放、
// 冲突还是候补的后继，要看见同一订舱名下的每一份——同一答复标识可能是早先那份候补，也可能是定局。
type BookingAnswerLedger interface {
	// FindAnswersForUpdate 在调用方的事务里先取这次订舱的锁，再交回名下全部承运答复，按答复时刻先后；没有答复交空
	// 切片，不交 error。锁随事务释放，不在事务里调用答 error。
	//
	// 「定局之后不再有后继」「定局不早于候补」是跨行的条件（ADR-0202 补记「候补之后可接一份定局」），库面两条部分唯一
	// 索引各钉一种答复，够不着。不先锁，同一订舱的一份候补与一份接受并发到达，会各自读到空册、各自落库。
	FindAnswersForUpdate(ctx context.Context, key BookingKey) ([]BookingAnswerRecord, error)
	// Save 撞上任何一条唯一约束（同一答复标识、第二份候补、第二份定局）都答 BookingAlreadyAnswered，不覆盖先到者，由
	// 编排读回再判。
	Save(ctx context.Context, record BookingAnswerRecord) (BookingAnswerSaveOutcome, error)
}
