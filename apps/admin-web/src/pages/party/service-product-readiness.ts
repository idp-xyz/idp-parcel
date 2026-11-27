// 服务产品详情上「这一版」的读法：只说这一行自己已经知道的三格，空着写后果。
// 渠道映射和客户合同是别的册来引用这一版，这里不数它们有几条。

export const serviceProductFormAbsentNote = '还没有服务形态，这一版还不能按网络服务或面单渠道被选中。';

export const serviceProductDeliveryAbsentNote = '这一版不约定末端交付，以后的合同没有可收紧的条件。';

/** 形态键缺席才是未登记。空串与缺席同一件事：目录不补一个默认形态。 */
export function serviceProductFormAbsent(form: string | undefined): boolean {
  return form === undefined || form.trim() === '';
}

/**
 * 接着去的两册。hash 与导航里的页面 id 同名（渠道产品、客户与合同）。
 * 一句只说那一册怎么用到这一版，不报条数。
 */
export const serviceProductFollowOn = [
  { hash: '#/channel-product-catalog', label: '渠道产品', note: '能走哪些渠道在那里映射。' },
  { hash: '#/party-contracts', label: '客户与合同', note: '合同引用已经发布的这一版。' },
] as const;
