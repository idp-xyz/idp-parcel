# 02 CC 关案编排无装配

Category: enhancement
Status: in-progress——2026-10-28 会话 tops-20（idp-parcel-91）立并认领（用户「现在就接线」），分支 claude-lw-cc
Blocked by: 无

## 做什么

把 `CloseCustomsCaseHandler` 接成生产入口（UC-CC-010 定触发方：运营命令口或派发消费者，照 UC 选），带 code/03 的冲突项阻止关闭；生产装配真库用例。授予与种子同 01 的写法。

## 完成判据

- 生产路径能关案；有未解决或冲突项时答被阻并列出各项（真库用例）。
