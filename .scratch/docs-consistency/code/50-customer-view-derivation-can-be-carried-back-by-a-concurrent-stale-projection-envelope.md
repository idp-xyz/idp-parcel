# 50 VE 客户视图派生在 READ COMMITTED 下会被并发的旧投影信封带回旧依据或旧内容

Category: bug
Status: needs-triage——建议 R2（上线后尽快）：答错且静默，但要两封同一包裹的投影派生信封并发消费、交错落在一个窄窗口里才出现。2026-10-21 文档自洽 code/05 隔离评审立（用户「你作为业务和系统专家自决」授权），分支 claude-code05。分级由协调方定稿。
Blocked by: 无（建在 [code/05](./05-customer-tracking-view-overwrites-published-versions.md) 的版本表与确认依据之上；code/05 未进 main 时以其分支为准）
出处：code/05 隔离评审 N4。code/05 只修存储形状与换代判据，不修这个竞态。

## 现象

同一包裹先后形成投影 P1、P2，两封派生信封被两个消费事务并发处理。处理 P1 的事务 A 先读到当前投影仍是 P1，于是认定自己的信封不旧；处理 P2 的事务 B 随后提交了 P2 的视图（换代为 V2，或把 V1 的确认依据前移到 P2）。A 接着读当前视图、按 P1 重新得出四维再写：

- 四维与当前版不同：A 以 P1 的内容替代 V2，已经不再有效的内容重新成为当前事实；
- 四维相同：A 把确认依据从 P2 拉回 P1，下一封 P2 重投时又被当成新投影处理。

两种情形下乐观锁都会放行：A 读到的当前版就是 B 刚写的那一版，`Save` 与 `Reaffirm` 的条件对它成立。

## 证据

- 文档：VE CONTEXT「客户可见性与通知」：「原已发布版本保留，但不再有效的内容不得继续显示为当前事实。」同节：「客户全程追踪视图只基于当前有效的全程追踪投影……形成」。
- 代码：
  - `adapters/parcelshipment` 的 `DeriveCustomerViewOnProjectionAdapter.HandleDerivedTrackingProjection`：用 `projections.FindCurrent` 读当前投影，比 `current.Version() != announced` 判旧信封；这一读不加锁，也不在写视图之前再核一次。
  - `adapters/postgres` 的 `Projections.FindCurrent`：普通 `SELECT`，不对 `tracking_projection_current` 那一行加锁。
  - `application.DeriveCustomerViewHandler.Handle`：读当前视图、算四维、写，乐观锁只看视图版本与确认依据，不看投影当前版。
  - 消费事务在本仓未见另设隔离级别，按库默认（READ COMMITTED）：同一事务里两次读之间看得见别人刚提交的行。
- 归属确立补派生 `DeriveCustomerViewOnAcceptanceAdapter` 也按 `projections.FindCurrent` 读当前投影再派生，同形。

## 三类

机制。无产品策略、无租户取值。

## 规模

S–M。

## 最小的诚实修法

二选一，作者裁：

1. 消费事务里读当前投影时，对 `tracking_projection_current` 那一行加 `FOR SHARE`：P2 的投影派生要改这一行就得等 A 提交，A 判新旧时看到的当前版在它的事务里不会变。读口要分出「锁读」一支，只给消费链用；运营查阅读面不加锁。
2. 写视图之前再核一次投影当前版：仍是信封宣告的那一版才写，否则按旧信封入账收工。

两条都要补真库并发用例：两个事务交错处理同一包裹的 P1 与 P2，断言最终当前视图依据 P2、内容是 P2 的。

## 要裁的

- 两种修法取哪一种。前者把投影派生与视图派生串行化到同一行锁上，后者多一次读、窗口缩到最后一读与写之间（还要配合视图写入条件才算闭合）。
