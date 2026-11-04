import { invalidateWhenAnswered } from '../../query/answers';

// 参与方写口答到之后整域作废（ADR-0360 决定三）。一笔登记、结束、关版本或草稿提交 / 批准 / 发布，动到的不止本页那一册：
// 法人页的「关联」签读供应商协议与各类政策，表单的已知版本决定修订号建议，待发布清单随草稿走。逐键点名作废，漏一个就有
// 一页在新鲜期内显旧答案、给出错的修订号建议；整域作废只是让挂着的几页后台各重问一次，写口本来就少。

export const partyAnswersDomain = 'party';

export function afterPartyWrite<Answer extends { kind: string }>(request: Promise<Answer>): Promise<Answer> {
  return invalidateWhenAnswered([partyAnswersDomain], request);
}
