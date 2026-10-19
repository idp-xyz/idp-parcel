# 56 预占先于班次出发到期时，分配的保障随之失效而没有任何一处会响

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [21](./21-load-assignment-allocates-against-capacity-reservations.md) 的完成记录。
Blocked by: [21](./21-load-assignment-allocates-against-capacity-reservations.md)
出处：[ADR-0203](../../../docs/adr/0203-capacity-assurance-requires-a-reservation-on-every-pool-of-the-schedule-and-reservations-are-operation-decisions.md) 文末补记（票 21）的越权风险点「分配的保障随预占到期失效」。

## 现象

票 21 判一行分配量时，只核分配时刻落在那份预占的有效区间里（判断时点之前答 `RESERVATION_NOT_YET_EFFECTIVE`，有效期之后答 `RESERVATION_EXPIRED`）。预占有效期早于班次出发也照样放行：到期那一刻，预占里未消耗的剩余随到期归还池（ADR-0203 决定四），已分配的那一段也在其中。分配仍在册、仍引用那份预占，池却已经把那段容量当成可用的了，下一份预占可以把它占走，直到出发都没有任何一格提示这份分配已不受保护。

## 证据（读码）

`domain.CapacityPool.AdmitAllocation` 只拿分配时刻比预占的判断时点与有效期；`CapacityReservation.occupiedAt` 过期之后只算已消耗。分配编排读到了班次（核在册），但不读它的出发时刻。

## 三类

判据是产品策略（内置一种）：要不要核「预占覆盖到班次出发」，覆盖到出发还是到达。规模 S。

## 最小的诚实修法

二选一，交 TF owner 定：

1. 分配时核预占有效期不早于班次出发时刻，早于答一个新格（续办是另占一份覆盖到出发的预占）；班次改版改了出发时刻的，由班次登记口提示受影响的分配（或另票）。
2. 不核，改在查阅面把「预占已过期而分配仍在」的那几份列成一种异常，交运营处置。

倾向 1：它把问题挡在形成那一刻，续办清楚；2 只是事后看得见。

## 要裁的

- 覆盖到出发还是到达（在途期间容量还算不算被占）。
- 班次改版改了出发时刻时，既有分配怎么处置。

## 既有票

none
