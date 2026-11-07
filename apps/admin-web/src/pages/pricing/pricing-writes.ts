import { invalidateWhenAnswered } from '../../query/answers';

// 定价写口答到之后整域作废（ADR-0360 决定三），与 party-writes.ts 同一做法。价格政策与供应商协议两张表单的选单经取数层读
// 价卡目录，键在本域名下，发布一版草稿或直接登记一张之后跟着过期——键若落在参与方域下，定价的写口就作废不到它，选单上
// 看不见刚发布的那张。价卡页的「全部价卡」签与参考序列页按游标翻页（ADR-0144），不经取数层：签切走时卸载、回来重取第一页，
// 参考序列页落册后自己重取当前这一页。作废找不到键就什么都不做，写口照样包着。

export const pricingAnswersDomain = 'pricing';

export function afterPricingWrite<Answer extends { kind: string }>(request: Promise<Answer>): Promise<Answer> {
  return invalidateWhenAnswered([pricingAnswersDomain], request);
}
