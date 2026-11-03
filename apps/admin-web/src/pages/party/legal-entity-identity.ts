// 法人身份层两格的呈现判读（票 legal-entity-profile/04「列表与详情上身份两格照答复原样示出」）。目录行、检查器、
// 对象页与修订历史读同一份：各写一份会在改号的写法时分叉。纯函数，node:test 钉着；页面只负责摆。
//
// **照答复原样**：本文件交回的格子里，国家码与号不译不补。两处例外都是呈现层的译名，不改答复本身：国家码是 ISO 公开标准里的
// 稳定码，页面显示时另经基础数据目录译成「中国（CN）」（refdata-names.ts，码仍在括号里）；注册号类型码由调用方把注册号
// 类型目录（GET /commercial-registration-number-types）折成的查法交进来，译成「统一社会信用代码」——运营与财务认的是这个名，
// 码进技术详情（spec 公司主体 UX「统一用语」）。不交查法、目录读不到、或目录里没有这个码（历史修订上登的旧码）时原样显码，
// 不报错：译不出只是这一次没译成，不是号错了。身份层没登记的修订交回 null，由调用方写 identityLayerAbsentNote，不拿空串顶格。

import type { IdentityLayerRecord, LifetimeRegistrationNumberRecord, RegistrationNumberTypeRecord } from './api';

export interface LifetimeNumberCell {
  /** 类型名；译不出（没交查法、目录没有这个码、名为空串）就是类型码。 */
  typeName: string;
  number: string;
}

export interface IdentityLayerCells {
  country: string;
  /** 一号一段「类型名（或类型码） 号」，多号之间用中文分号——目录行的一格文本。 */
  numbers: string;
  /**
   * 同一批号逐个拆开，给头部用：胶囊一号一枚、只显号，类型名进 title（票 company-subject-ux/07）。拼成一段文本放进
   * 会截断的胶囊时，类型名长（演示目录「模拟注册号（仅演示，非真实证照格式）」）挤掉的是号，而号才是要认、要复制的。
   */
  numberCells: LifetimeNumberCell[];
}

/** 目录的两层（ADR-0145 决定一）：身份层登终身注册号的类型，资料层登监管登记号的类型。 */
export type RegistrationNumberTypeLayer = 'IDENTITY' | 'PROFILE';

/**
 * 按注册国家 / 地区与类型码查类型名；查不到交回 undefined，调用方原样显码。`layer` 说的是调用方手里的号属哪一层——
 * 同一国同一码两层都登了时各取各的名；不交按身份层优先（终身注册号的老调用方）。
 */
export type RegistrationNumberTypeNameOf = (country: string, typeCode: string, layer?: RegistrationNumberTypeLayer) => string | undefined;

function typeKey(country: string, typeCode: string, layer: string): string {
  return `${country}\u0000${typeCode}\u0000${layer}`;
}

/**
 * 把注册号类型目录折成查法。目录按国家 / 地区分，同一个码在两国可以是两个名，所以键带国家；两层各自登名，所以键也带层
 * （票 company-subject-ux/06 第 4 条：此前同码两层都登时拿身份层的名顶资料层的号）。只登了一层的码不论问哪一层都译——
 * 查法不替调用方判断该不该译，它只答目录里叫什么。已停用的类型照样译——号是在它还在用时登的，名没有变。
 */
export function registrationNumberTypeNamesOf(types: readonly RegistrationNumberTypeRecord[]): RegistrationNumberTypeNameOf {
  const names = new Map<string, string>();
  for (const type of types) {
    const key = typeKey(type.countryCode, type.typeCode, type.layer);
    if (!names.has(key)) names.set(key, type.typeName);
  }
  return (country, typeCode, layer) => {
    const order: RegistrationNumberTypeLayer[] = layer === 'PROFILE' ? ['PROFILE', 'IDENTITY'] : ['IDENTITY', 'PROFILE'];
    for (const candidate of order) {
      const name = names.get(typeKey(country, typeCode, candidate));
      if (name !== undefined) return name;
    }
    return undefined;
  };
}

export function identityLayerCellsOf(
  record: IdentityLayerRecord,
  typeNameOf?: RegistrationNumberTypeNameOf,
): IdentityLayerCells | null {
  if (!record.identityLayerRegistered) return null;
  const country = record.registrationCountry ?? '';
  const numberCells = lifetimeNumberCells(
    record.lifetimeRegistrationNumbers ?? [],
    typeNameOf === undefined ? undefined : (typeCode) => typeNameOf(country, typeCode, 'IDENTITY'),
  );
  return { country, numbers: cellsText(numberCells), numberCells };
}

export function lifetimeNumberCells(
  numbers: readonly LifetimeRegistrationNumberRecord[],
  typeNameOf?: (typeCode: string) => string | undefined,
): LifetimeNumberCell[] {
  // 目录里名为空串的类型按码显：空名上屏等于把号裸露成「 91000…」，看不出是什么号。
  return numbers.map((entry) => ({ typeName: typeNameOf?.(entry.typeCode) || entry.typeCode, number: entry.number }));
}

function cellsText(cells: readonly LifetimeNumberCell[]): string {
  return cells.map((cell) => `${cell.typeName} ${cell.number}`).join('；');
}

export function lifetimeNumbersText(
  numbers: readonly LifetimeRegistrationNumberRecord[],
  typeNameOf?: (typeCode: string) => string | undefined,
): string {
  return cellsText(lifetimeNumberCells(numbers, typeNameOf));
}
