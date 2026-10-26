// 发布表单里「引用他物」那几格的候选（票 party-commercial-uiux/13）。
//
// 引用对象本身的格（接单规则包正文的服务产品与客户合同、客户服务规则的适用对象）要的是对象标识，读口答的是一版一行，
// 候选从版本里去重出对象，顺带写出它有哪几版、哪一版在用，帮人认是哪一个。要连版本一起选的格（合同层交付条件所收紧的
// 产品版本）一版一项。不按状态过滤：表单不裁，状态写在选项里给人看，在不在册、能不能引仍由服务端判。
// 纯函数，node:test 直接跑。

import type { CustomerContractListResponseBody, ServiceProductListResponseBody } from './api';
import { commercialStatusLabels, labelOf } from './presentation';
import type { PickerOption } from './PublicationFormFields';

interface VersionRow {
  objectId: string;
  version: string;
  status: string;
}

const versionOrder = new Intl.Collator('en', { numeric: true, sensitivity: 'base' });

/** 一版一行 → 一对象一项：「标识 · 各版 · 在用的那版」，按对象标识排。 */
export function objectOptionsOf(rows: readonly VersionRow[]): PickerOption[] {
  const byObject = new Map<string, VersionRow[]>();
  for (const row of rows) byObject.set(row.objectId, [...(byObject.get(row.objectId) ?? []), row]);
  return [...byObject.entries()]
    .sort(([a], [b]) => versionOrder.compare(a, b))
    .map(([objectId, versions]) => {
      const sorted = [...versions].sort((a, b) => versionOrder.compare(b.version, a.version));
      const inUse = sorted.filter((row) => row.status === 'EFFECTIVE').map((row) => row.version);
      const versionsText = sorted.map((row) => row.version).join('、');
      return {
        value: objectId,
        label: `${objectId} · ${versionsText}${inUse.length > 0 ? ` · 在用 ${inUse.join('、')}` : ''}`,
      };
    });
}

export function serviceProductObjectOptions(body: ServiceProductListResponseBody): PickerOption[] {
  return objectOptionsOf(body.products);
}

export function customerContractObjectOptions(body: CustomerContractListResponseBody): PickerOption[] {
  return objectOptionsOf(body.contracts);
}

/** 对象 + 版本一起选时的一项：两格同时落，不让人手拼版本号。 */
export interface VersionChoice {
  objectId: string;
  version: string;
  label: string;
}

export function serviceProductVersionChoices(body: ServiceProductListResponseBody): VersionChoice[] {
  return [...body.products]
    .sort((a, b) => versionOrder.compare(a.objectId, b.objectId) || versionOrder.compare(b.version, a.version))
    .map((product) => ({
      objectId: product.objectId,
      version: product.version,
      label: `${product.objectId} · ${product.version} · ${labelOf(commercialStatusLabels, product.status)}`,
    }));
}

/** 选单里一项的键：对象与版本拼成一串只给 `<option value>` 用，不上屏、不进载荷。 */
export function versionChoiceKey(choice: { objectId: string; version: string }): string {
  return JSON.stringify([choice.objectId, choice.version]);
}
