// 答案的新鲜期（ADR-0360 决定二）：一份「答到了」的业务答案在这段时间里算新的，不为它再问一遍。工作区标签（离开满这段时间
// 再回来就按新开页重读，Layout 里 returnNeedsFreshMount 那一处）与取数层（query/client.ts 的 staleTime）都读这一个数——
// 两处各定一个，人就会碰到「这页刚刷过、那页回来却是半分钟前的」。
// 取值是产品策略（与 idp-prism 同值），不是租户取值。
//
// 本文件只放纯判读，node:test 钉着；接 TanStack Query 的在 client.ts 与 answers.ts。

export const answerFreshForMs = 30_000;

/** 读口答复的最小形：只看 `kind`。 */
export interface AnswerLike {
  kind: string;
}

/**
 * 一份答案新鲜多久：只有答到了（`outcome`）的算新鲜；未配置、拒绝、读不到等即刻过期，下次用到就再问——一次读不到
 * 不能被当成「没有」留满新鲜期。还没有答案时同样即刻过期。
 */
export function answerStaleTimeOf(answer: AnswerLike | undefined): number {
  return answer?.kind === 'outcome' ? answerFreshForMs : 0;
}

/**
 * 页面该画哪一份：重读在途时照旧显上一份 `outcome`（问的是「有没有更新」，不是「忘掉已知的」）；上一份不是 `outcome`
 * 的，在途时按加载画（null）——那几态下的「重试」若不换成加载，按的人看不出按没按到。新答案到了就替换，哪怕是读不到。
 */
export function answerToShow<Answer extends AnswerLike>(data: Answer | undefined, fetching: boolean): Answer | null {
  if (data === undefined) return null;
  if (fetching && data.kind !== 'outcome') return null;
  return data;
}
