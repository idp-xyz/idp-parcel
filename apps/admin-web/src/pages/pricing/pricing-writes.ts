import { invalidateWhenAnswered } from '../../query/answers';

// 定价写口答到之后整域作废（ADR-0360 决定三），与 party-writes.ts 同一做法。价卡目录不止价卡页在读：价格政策与供应商协议
// 两张表单的选单也读它，键都在本域名下，发布一版草稿或直接登记一张之后几处一起过期——键若落在参与方域下，定价的写口就
// 作废不到它，回到「全部价卡」签看不见刚发布的那张（评审 2026-11-03 指出）。参考序列的读今天还没迁到取数层，写口照样包着：
// 作废找不到键就什么都不做，迁的那天不用回头补。

export const pricingAnswersDomain = 'pricing';

export function afterPricingWrite<Answer extends { kind: string }>(request: Promise<Answer>): Promise<Answer> {
  return invalidateWhenAnswered([pricingAnswersDomain], request);
}
