package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

// ChannelExecution 是受控通道一次执行的留痕（票 12 身份双轨的第①轨）。OSUser 与
// Hostname 由登记入口从运行环境自取；RecordReference 指名这次执行送达的登记
// （暂停/恢复用暂停标识，权威区间用四维身份加生效起点），Outcome 是登记册当时的
// 答案。它不是治理记录——治理记录的内容轨（第②轨）在各自表上，这里一个内容字段
// 都没有。
//
// Command 取领域封闭集而不是自由文本：集合归产品定，写入方换一个也造不出集合外的行
// （票 pilot-governance-context-gaps/01）。Outcome 仍是文本——它的取值来自应用层各结果枚举的
// String()，那个集合在应用层已封闭，且每加一种答案都不该牵动留痕表。
type ChannelExecution struct {
	Command         domain.ChannelCommand
	RecordReference string
	OSUser          string
	Hostname        string
	Outcome         string
	ExecutedAt      time.Time
}

func (execution ChannelExecution) complete() bool {
	return execution.Command.String() != "" &&
		strings.TrimSpace(execution.RecordReference) != "" &&
		strings.TrimSpace(execution.OSUser) != "" &&
		strings.TrimSpace(execution.Hostname) != "" &&
		strings.TrimSpace(execution.Outcome) != "" &&
		!execution.ExecutedAt.IsZero()
}

// ChannelExecutions 是留痕库。只追加、每次执行各一行、无幂等约束——同一登记被
// 执行过几次本身就是要留的事实；写入走 RequireExecutor，与它所留痕的登记同一笔
// 事务成败（登记回滚则痕消，痕不声称一笔没落库的登记）。
type ChannelExecutions struct {
	db *bentopg.DB
}

func NewChannelExecutions(db *bentopg.DB) (*ChannelExecutions, error) {
	if db == nil {
		return nil, fmt.Errorf("pilot governance postgres: db is nil")
	}
	return &ChannelExecutions{db: db}, nil
}

// Append 落一条执行留痕。留白拒绝：通道技术身份是入口自取的事实，取不出就不许
// 落一行装作取到了。
func (repository *ChannelExecutions) Append(ctx context.Context, execution ChannelExecution) error {
	if !execution.complete() {
		return fmt.Errorf("pilot governance postgres: channel execution trace is incomplete")
	}
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("append channel execution: %w", err)
	}

	_, err = executor.Exec(ctx,
		`INSERT INTO pilot_governance.channel_execution
			(command, record_reference, os_user, hostname, outcome, executed_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		execution.Command.String(),
		execution.RecordReference,
		execution.OSUser,
		execution.Hostname,
		execution.Outcome,
		execution.ExecutedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("append channel execution: %w", err)
	}
	return nil
}
