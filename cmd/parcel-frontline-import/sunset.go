package main

import (
	"errors"
	"fmt"
	"time"
)

// structuralSunsetLiteral 是过渡导入口的结构性拆除期限（ADR-0089 决定③）。它写成源码
// 常量而不是配置或环境变量，正是为了让「延长」只能经 supersede ADR-0089 改源码发版——
// 一个能被运维改掉的期限不是拆除期限，是提醒。
const structuralSunsetLiteral = "2026-12-31T23:59:59+08:00"

// structuralSunset 在包初始化时解析。解析失败直接 panic：常量写坏是发版缺陷，不该
// 让进程带着一个不存在的期限跑起来。
var structuralSunset = func() time.Time {
	sunset, err := time.Parse(time.RFC3339, structuralSunsetLiteral)
	if err != nil {
		panic(fmt.Sprintf("parcel-frontline-import: 拆除期限常量不合法：%v", err))
	}
	return sunset
}()

// ErrStructuralSunsetReached 表示导入口已过拆除期限。它不是警告：调用方据此拒绝启动。
var ErrStructuralSunsetReached = errors.New("parcel-frontline-import: 过渡导入口已过结构性拆除期限")

// guardStructuralSunset 是 run 的第一道门。派工原话是「超过即启动拒绝」：期限时刻本身
// 还不算超过，晚于它一纳秒就算。
func guardStructuralSunset(now time.Time) error {
	if !now.After(structuralSunset) {
		return nil
	}
	return fmt.Errorf("%w（期限 %s，当前 %s）；延长只能经 supersede ADR-0089 改源码发版，没有环境变量或参数可绕过",
		ErrStructuralSunsetReached,
		structuralSunset.Format(time.RFC3339),
		now.Format(time.RFC3339))
}
