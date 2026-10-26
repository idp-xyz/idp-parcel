// 「基于此版本发新版本」的公共部分（票 party-commercial-uiux/14）。
//
// 改动靠发新版本，本区的主要任务就是它；此前每次都要从空白整张重填，差一个字就成了另一份内容。这里是从目录里的一版带一份
// 预填草稿去发布表单：草稿由各册 *-form.ts 的 `*DraftFromRecord` 从读口记录还原，**只抄读面上确有的**——读面没有的格
// （信用政策、结算政策没有版本的适用范围，合同没有货主客户账户，各册没有批准依据等）留空，由人填。
//
// 版本号与生效区间一律留空：版本号要人起，新旧两版区间重叠解析时答「适用冲突」，把上一版的区间抄过来就是替人造这个冲突。
// 纯类型与小函数，node:test 直接跑。

/** 带去发布表单的起点：还原好的草稿，加一句「基于哪一版」给表单顶上的提示用。 */
export interface NewVersionStart<Draft> {
  draft: Draft;
  /** 例：「信用政策 SYN-CREDIT-01 v1」 */
  from: string;
  /**
   * 这一册预填特有的警示：读面上还原不了、而「留空即未声明」会让新版本悄悄丢掉的东西。接单规则包的读口不带时点策略、接受
   * 内容、面单有效期、资料修订四节声明，上一版声明过的，新版本不补就等于没声明——这种不是「一格没填」，要单独提醒。
   */
  caution?: string;
}

export function newVersionFrom(kindLabel: string, objectId: string, version: string): string {
  return `${kindLabel} ${objectId} ${version}`;
}

/**
 * 一次「基于此版本」的发起。`token` 每发起一次加一，页面拿它做表单的 key：同一版连点两次、或换一版再点，表单都按新的预填
 * 重建，不留上一次的预览与草稿。
 */
export interface NewVersionRequest<Source> {
  token: number;
  source: Source;
}

export function nextRequest<Source>(previous: NewVersionRequest<Source> | null, source: Source): NewVersionRequest<Source> {
  return { token: (previous?.token ?? 0) + 1, source };
}
