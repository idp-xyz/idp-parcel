package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
)

// ChannelExecution 是受控通道一次执行的留痕（票 12 身份双轨的第①轨，经票 15 沿用）。
// OSUser 与 Hostname 由登记入口从运行环境自取；RecordReference 指名这次执行送达的
// 登记（区间型目录用租户+版本，键型目录用租户+键身份），Outcome 是登记册当时的答案。
// 它不是目录内容——内容轨（第②轨，approved_by）在各目录表自己的列上，这里一个内容
// 字段都没有。
type ChannelExecution struct {
	Command         string
	RecordReference string
	OSUser          string
	Hostname        string
	Outcome         string
	ExecutedAt      time.Time
}

func (execution ChannelExecution) complete() bool {
	return strings.TrimSpace(execution.Command) != "" &&
		strings.TrimSpace(execution.RecordReference) != "" &&
		strings.TrimSpace(execution.OSUser) != "" &&
		strings.TrimSpace(execution.Hostname) != "" &&
		strings.TrimSpace(execution.Outcome) != "" &&
		!execution.ExecutedAt.IsZero()
}

// ChannelExecutions 是留痕库。只追加、每次执行各一行、无幂等约束——同一登记被执行
// 过几次本身就是要留的事实；写入走 RequireExecutor，与它所留痕的登记同一笔事务成败
// （登记回滚则痕消，痕不声称一笔没落库的登记）。
type ChannelExecutions struct {
	db *bentopg.DB
}

func NewChannelExecutions(db *bentopg.DB) (*ChannelExecutions, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &ChannelExecutions{db: db}, nil
}

// Append 落一条执行留痕。留白拒绝：通道技术身份是入口自取的事实，取不出就不许落
// 一行装作取到了。
func (repository *ChannelExecutions) Append(ctx context.Context, execution ChannelExecution) error {
	if !execution.complete() {
		return fmt.Errorf("visibility exception postgres: channel execution trace is incomplete")
	}
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("append channel execution: %w", err)
	}

	_, err = executor.Exec(ctx,
		`INSERT INTO visibility_exception.channel_execution
			(command, record_reference, os_user, hostname, outcome, executed_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		execution.Command,
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
