# 70 SA 冻结与暴露账本按作用域整册回写，并发下丢失更新，已释放的钱被静默占回去

Category: bug
Status: ready-for-agent——R1（上线前必修）：碰钱、静默。同一结算作用域下两份委托的控制或释放并发时，后提交的那一笔把它读到的整册快照写回，先提交那一笔刚落的释放（或冻结）被压回去；客户可用余额或额度被吃掉，不报任何错。问题早于 code/20；code/20 让接受链对被取代版本每一轮都请求一次释放，整册回写的次数变多了。2026-10-21 协调方据 code/20 隔离评审 N1 立。
Blocked by: 无
出处：code/20 隔离评审 N1；code/20 完成记录「评审记录」。

## 现象

`ApplyPreAcceptanceControlHandler` 与 `ReleasePreAcceptanceControlHandler` 都是「`LoadForScope` 读整本作用域账本 → 领域改一条 → `Save` 把整本写回」。`sapostgres.FreezeLedgers.Save`（暴露那本同形）对账本里每一条都做 `INSERT … ON CONFLICT DO UPDATE SET status, released_at`。

并发两笔落在同一（租户，作用域）上：
- A 读到账本，此时委托 X 那一笔还是「已冻结」；
- B 释放 X 那一笔并提交；
- A 冻结自己那一笔，`Save` 整册写回，X 那一行被改回「已冻结」、`released_at` 清空。

X 的钱又被占住，没有任何一方报错。两笔都是释放时同理：后写的那一笔把对方的释放压回去。

## 证据

- `internal/settlementaccounting/adapters/postgres/pre_acceptance_control.go`：`FreezeLedgers.Save` 遍历 `ledger.Entries()` 逐条 upsert，`DO UPDATE` 无条件覆盖 `status` 与 `released_at`；暴露账本的 `Save` 同形。
- `internal/settlementaccounting/application/apply_pre_acceptance_control.go` 与 `release_pre_acceptance_control.go`：读与写之间没有作用域级的锁，事务隔离级别是默认的读已提交。
- SA CONTEXT：「重复请求、查询和补偿不得形成重复冻结或重复释放」；冻结金额由账本当场求和（迁移 `settlement_accounting/0010` 头注），一行被改回「已冻结」即重复占用。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：改两本账的写入与一条读写路径，补真库并发用例（两连接、屏障）。

## 最小的诚实修法

两种取一：
1. **作用域级串行**：施加与释放在 `LoadForScope` 之前按（租户，作用域）取 `pg_advisory_xact_lock`，锁与写入在同一笔事务里（先例：code/14 的分户账锁、code/39 的来源身份锁）。改动小，同一作用域的控制排队。
2. **只写变化的条目**：`Save` 只写本次领域改动的那几条，`DO UPDATE` 加条件（例如只允许「已冻结 → 已释放」、已释放的行不再被改回）。不排队，但领域账本要能交出「改了哪几条」。

两种都要补真库用例：两路并发、一路释放 X、一路冻结 Y，提交后 X 仍是已释放、Y 已冻结、可用余额等于两者算出来的数。

附注：code/20 已先止血一格——`ReleasePreAcceptanceControlHandler` 遇到已是「已释放」的冻结或暴露时跳过 `Save`，重复释放不再整册回写。首次释放与施加仍整册写，本票要收的是这两格。

## 要裁的

无。两种修法属机制，落地时按代价选一种，选法写进完成记录。
