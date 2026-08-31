# 复核决定面：完成人工复核的应用编排与命令端点

Category: feature
Status: superseded——并入 admin-skeleton-closure-batch/09（MCP-5 按频道 5 用户指示
承接命令面：复核完成 + 主动拒绝两端点，Intake 照红线未配置即拒起步）。本票「为什么
现在不做」段里的 BD-PS-002 授权缺参与续办触发归属两点仍成立，留此供 09 参考。
Blocked by: BD-PS-002（复核角色目录未确认）、PAR-INT-01（接入渠道未提供）

## 要做什么（届时）

1. 应用编排 `CompleteManualReview`：读回委托 → 域转移 `CompleteManualReview`（三项
   引用必填、越过提交边界拒补录、同版本只收一次——域上已锁）→ 落库 → 触发接受判断
   续办（重跑 Decide 装配当轮校验；续办入口与 UC-PS-001 的判断编排如何衔接是本票
   最大的未决设计点，parcel-dispatch 的 advance 链是现成候选）。
2. 命令端点 `POST /acceptance-review-completions`（名待定）：Intake 未配置即拒起步，
   词表照域错误封闭集合（已完成、已越界、重复完成各成一格）。
3. 页 `acceptance-review` 决定区解禁：复核通过/不通过映射到完成留痕的证据引用语义
   ——注意 CONTEXT 明写「复核完成本身不形成决定」，按钮词不得演成「接受/拒绝」。

## 为什么现在不做

- 授权判定缺参：复核该由谁做是 party-commercial 的授权规则（BD-PS-002 未确认），
  域上只存引用不判资格；命令面今天上线等于「谁都能签」。
- 复核后的续办触发牵动接受判断编排链（parcel-dispatch），不是 parcel-api 单侧加个
  端点的事。
- 接入渠道墙（PAR-INT-01）未破：命令端点挂上也只能未配置即拒。

## 重启条件

BD-PS-002 落定（或治理侧显式裁「先记引用不判资格」）且续办触发的归属定下（进
parcel-dispatch 链还是独立编排）——两样齐了本票转 ready。

## Comments
