// 往来单位编辑态的两样纯逻辑（票 business-party-redesign/11）：「更新资料」表单分哪几段，以及从名录右栏进编辑态时记的那一笔。
// 不引 React，node:test 直接钉。

/**
 * 「更新资料」表单的分段，次序即表单里的先后：这次更新（来源单据、生效时间、版本号）在最前——这几格说的是这一次，不沿用上一版；
 * 其余与只读时右栏概要的卡片同名同序。锚点列、各段的 id、卡头「编辑」落到哪一段，都认这一份。
 */
export const profileEditSections = [
  { id: 'this-update', label: '这次更新' },
  { id: 'names-addresses', label: '名称与地址' },
  { id: 'invoicing', label: '税务与开票' },
  { id: 'bank-accounts', label: '收款账户' },
  { id: 'contacts', label: '联系人' },
] as const;

export type ProfileEditSectionId = (typeof profileEditSections)[number]['id'];

/** 「更正登记信息」表单的分段：这次更正（来源单据、生效时间、更新序号）在前，同上一份的道理；后面是名称与识别信息。 */
export const identityEditSections = [
  { id: 'this-correction', label: '这次更正' },
  { id: 'identity', label: '单位身份' },
] as const;

/** 表单里一段的 DOM id；锚点列与进来时落到哪一段都按它找。 */
export function editSectionDomId(section: string): string {
  return `party-edit-${section}`;
}

export interface EditOrigin {
  partyId: string;
  /** 进来时要落到哪一段（卡头的「编辑」）；页头的「更新资料」不指定，落在表单开头。 */
  section: ProfileEditSectionId | null;
}

// 编辑态住在对象页的地址上，对象页在壳层是另一张标签；地址键不带「从哪儿来」（本票不改地址键）。右栏在改地址之前把这一笔
// 记在这里，对象页那张标签开始编辑时取走。模块级的一格而不进地址：它只在同一次页面生命里、紧接着的那一次进入有意义，刷新
// 之后本来就该按地址走。
let pending: EditOrigin | null = null;

/** 名录右栏要进某一家的编辑态：记下是哪一家、落到哪一段。后一次覆盖前一次——人只会停在最后点的那一家上。 */
export function markEditFromRoster(partyId: string, section: ProfileEditSectionId | null = null): void {
  pending = { partyId, section };
}

/**
 * 看一眼记着的那一笔，不取走。渲染时用它（表单挂上的那一下就要知道落到哪一段），取走放在提交之后——渲染可能被重做，
 * 在渲染里取走，重做的那一次就拿不到了。对不上这一家答 null。
 */
export function peekEditOrigin(partyId: string): EditOrigin | null {
  return pending !== null && pending.partyId === partyId ? pending : null;
}

/**
 * 对象页开始编辑时取走这一笔。对得上这一家才交出、交出即清：一次进入只认一次，之后在对象页自己再点「更新资料」不算从名录来。
 * 对不上（记的是别家）不交也不清——那一笔是留给那一家的标签的。没有这一笔即刷新或深链进来的：退出照旧停在对象页。
 */
export function takeEditOrigin(partyId: string): EditOrigin | null {
  const origin = peekEditOrigin(partyId);
  if (origin !== null) pending = null;
  return origin;
}
