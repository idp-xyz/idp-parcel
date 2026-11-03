// 法人身份层两格的呈现判读（票 legal-entity-profile/04「列表与详情上身份两格照答复原样示出」）。目录行、检查器、
// 对象页与修订历史读同一份：各写一份会在改号的写法时分叉。纯函数，node:test 钉着；页面只负责摆。
//
// **照答复原样**：本文件交回的格子里，国家码与号不译不补。两处例外都是呈现层的译名，不改答复本身：国家码是 ISO 公开标准里的
// 稳定码，页面显示时另经基础数据目录译成「中国（CN）」（refdata-names.ts，码仍在括号里）；注册号类型码由调用方把注册号
// 类型目录（GET /commercial-registration-number-types）折成的查法交进来，译成「统一社会信用代码」——运营与财务认的是这个名，
// 码进技术详情（spec 公司主体 UX「统一用语」）。不交查法、目录读不到、或目录里没有这个码（历史修订上登的旧码）时原样显码，
// 不报错：译不出只是这一次没译成，不是号错了。身份层没登记的修订交回 null，由调用方写 identityLayerAbsentNote，不拿空串顶格。

import type { IdentityLayerRecord, LifetimeRegistrationNumberRecord, RegistrationNumberTypeRecord } from './api';

export interface IdentityLayerCells {
  country: string;
  /** 一号一段「类型名（或类型码） 号」，多号之间用中文分号。 */
  numbers: string;
}

/** 按注册国家 / 地区与类型码查类型名；查不到交回 undefined，调用方原样显码。 */
export type RegistrationNumberTypeNameOf = (country: string, typeCode: string) => string | undefined;

function typeKey(country: string, typeCode: string): string {
  return `${country}\u0000${typeCode}`;
}

/**
 * 把注册号类型目录折成查法。目录按国家 / 地区分，同一个码在两国可以是两个名，所以键带国家。同一国同一码两层都登了时
 * 取身份层：终身注册号只会是身份层的类型（ADR-0145 决定一）。已停用的类型照样译——号是在它还在用时登的，名没有变。
 */
export function registrationNumberTypeNamesOf(types: readonly RegistrationNumberTypeRecord[]): RegistrationNumberTypeNameOf {
  const names = new Map<string, string>();
  for (const type of types) {
    const key = typeKey(type.countryCode, type.typeCode);
    if (type.layer === 'IDENTITY' || !names.has(key)) names.set(key, type.typeName);
  }
  return (country, typeCode) => names.get(typeKey(country, typeCode));
}

export function identityLayerCellsOf(
  record: IdentityLayerRecord,
  typeNameOf?: RegistrationNumberTypeNameOf,
): IdentityLayerCells | null {
  if (!record.identityLayerRegistered) return null;
  const country = record.registrationCountry ?? '';
  return {
    country,
    numbers: lifetimeNumbersText(
      record.lifetimeRegistrationNumbers ?? [],
      typeNameOf === undefined ? undefined : (typeCode) => typeNameOf(country, typeCode),
    ),
  };
}

export function lifetimeNumbersText(
  numbers: readonly LifetimeRegistrationNumberRecord[],
  typeNameOf?: (typeCode: string) => string | undefined,
): string {
  // 目录里名为空串的类型按码显：空名上屏等于把号裸露成「 91000…」，看不出是什么号。
  return numbers.map((entry) => `${typeNameOf?.(entry.typeCode) || entry.typeCode} ${entry.number}`).join('；');
}
