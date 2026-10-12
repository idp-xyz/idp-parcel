// 接入渠道的三格答复（ADR-0100 决定四；票 operator-channel/07）：各 fetch 出口按这里判、各页按这里的词呈现。
//
// 三格按恢复动作分（ADR-0029），不并成一句「接入渠道未配置」：
//   unconfigured  —— 403 ACCESS_CHANNEL_NOT_CONFIGURED：这一口没接上渠道或部署没设发行方参数，要配部署的人去配；
//   loginRequired —— 401 OPERATOR_CREDENTIAL_REJECTED：令牌缺失、过期或校验不过，持令牌的人重新登录；
//   notGranted    —— 403 OPERATOR_NOT_GRANTED：令牌认下了，但操作者不在册、没绑这个租户或缺这一面的授予，要管授予的人
//                    去登记。服务端不说是哪一半（ADR-0055 决定四），页面也不猜。
// 三格都是「重试同一个请求不会好」，所以都不给重试；只有 loginRequired 有页面能给的出路（重新登录）。
//
// 判据是状态码与错误码原名一起对上，与 unconfigured 一向的判法相同：别的 401 / 403 不冒充这三格，照旧按调用方问题交回
// 状态与码。这里不放 React，node:test 直接钉。

export type OperatorRefusal = { kind: 'loginRequired' } | { kind: 'notGranted' };

export type AccessRefusal = { kind: 'unconfigured' } | OperatorRefusal;

export type AccessRefusalKind = AccessRefusal['kind'];

export type OperatorRefusalKind = OperatorRefusal['kind'];

export function accessRefusalOf(status: number, code: string): AccessRefusal | null {
  for (const kind of ['unconfigured', 'loginRequired', 'notGranted'] as const) {
    if (status === statuses[kind] && code === codes[kind]) return { kind };
  }
  return null;
}

/** 两格操作者答复（需要登录 / 没有授予）。未配置不在其中：各页对它有自己的那段说明。 */
export function isOperatorRefusal<Answer extends { kind: string }>(
  answer: Answer,
): answer is Extract<Answer, OperatorRefusal> {
  return answer.kind === 'loginRequired' || answer.kind === 'notGranted';
}

const titles: Record<AccessRefusalKind, string> = {
  unconfigured: '接入渠道未配置',
  loginRequired: '需要登录',
  notGranted: '没有授予',
};

const statuses: Record<AccessRefusalKind, number> = {
  unconfigured: 403,
  loginRequired: 401,
  notGranted: 403,
};

const codes: Record<AccessRefusalKind, string> = {
  unconfigured: 'ACCESS_CHANNEL_NOT_CONFIGURED',
  loginRequired: 'OPERATOR_CREDENTIAL_REJECTED',
  notGranted: 'OPERATOR_NOT_GRANTED',
};

export function accessRefusalTitle(kind: AccessRefusalKind): string {
  return titles[kind];
}

export function accessRefusalStatus(kind: AccessRefusalKind): number {
  return statuses[kind];
}

/** 错误码原名，便于按码检索，不翻译。 */
export function accessRefusalCode(kind: AccessRefusalKind): string {
  return codes[kind];
}

/** 「需要登录（401 OPERATOR_CREDENTIAL_REJECTED）」这一句。未配置那一格与各页已在用的句子逐字相同。 */
export function accessRefusalHeadline(kind: AccessRefusalKind): string {
  return `${titles[kind]}（${statuses[kind]} ${codes[kind]}）`;
}

/** 行内整句：「{what}：需要登录（401 …）。续办」。各页答案区与纯逻辑层的命令答复共用这一句。 */
export function operatorRefusalSentence(kind: OperatorRefusalKind, what?: string): string {
  return `${what ? `${what}：` : ''}${accessRefusalHeadline(kind)}。${operatorRefusalRecovery[kind]}`;
}

/**
 * 两格操作者答复的续办：找谁、做什么、重试会不会好。与哪一页无关——授予与登录都不随页面变——所以各页共用这一句，
 * 不各写一份。未配置的续办随口而异（这一口在等什么），留在各页。
 */
export const operatorRefusalRecovery: Record<OperatorRefusalKind, string> = {
  loginRequired:
    '服务端没有认下这次请求带的登录凭据（缺失、过期或校验不过）。重新登录后再试；不换凭据，重试不会改变结果。',
  notGranted:
    '登录凭据认下了，但当前操作者在本租户没有这一族能力的授予（不在册、没绑本租户或缺这一面的授予，服务端不说是哪一半）。' +
    '要管授予的人去登记授予；重试或重新登录都不会改变结果。',
};
