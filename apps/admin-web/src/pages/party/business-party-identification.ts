// 往来单位识别信息（ADR-0367）的呈现判读：对象页概要、右栏、列表检索与修订记录读同一份，各写一份会在号与编码的写法上分叉。
// 纯函数，node:test 钉着；页面只负责摆。照答复原样：号与编码不译不补，类型码由调用方交进来的目录查法译成类型名，译不出按码显
// （判据在 legal-entity-identity.ts 文件头）；用途是封闭三词，译成中文。

import type { BusinessPartyIdentificationRecord, ExternalReferenceRecord } from './api';
import { lifetimeNumberCells, type LifetimeNumberCell, type RegistrationNumberTypeNameOf } from './legal-entity-identity';
import { externalReferencePurposeLabels, labelOf } from './presentation';

export interface PartyIdentificationCells {
  /** 注册国家 / 地区码；这笔修订没带号即 null。 */
  country: string | null;
  numbers: LifetimeNumberCell[];
  references: ExternalReferenceCell[];
}

export interface ExternalReferenceCell {
  system: string;
  purpose: string;
  code: string;
}

export function externalReferenceCellOf(reference: ExternalReferenceRecord): ExternalReferenceCell {
  return { system: reference.system, purpose: labelOf(externalReferencePurposeLabels, reference.purpose), code: reference.code };
}

/** 一个外部编码的一段文本：「ERP 客户编码 C-0001」。 */
export function externalReferenceText(reference: ExternalReferenceRecord): string {
  const cell = externalReferenceCellOf(reference);
  return `${cell.system} ${cell.purpose} ${cell.code}`;
}

export function partyIdentificationCellsOf(
  record: BusinessPartyIdentificationRecord,
  typeNameOf?: RegistrationNumberTypeNameOf,
): PartyIdentificationCells {
  const registered = record.identityLayerRegistered === true;
  const country = registered ? (record.registrationCountry ?? '') : null;
  return {
    country,
    numbers: registered
      ? lifetimeNumberCells(
          record.lifetimeRegistrationNumbers ?? [],
          typeNameOf === undefined || country === null ? undefined : (typeCode) => typeNameOf(country, typeCode, 'IDENTITY'),
        )
      : [],
    references: (record.externalReferences ?? []).map(externalReferenceCellOf),
  };
}

/** 列表检索要命中的识别信息：每个号、每个外部编码（码本身，也带系统，便于按「ERP C-0001」搜）。 */
export function partyIdentificationSearchTerms(record: BusinessPartyIdentificationRecord): string[] {
  const numbers = record.identityLayerRegistered === true ? (record.lifetimeRegistrationNumbers ?? []).map((entry) => entry.number) : [];
  const references = (record.externalReferences ?? []).flatMap((entry) => [entry.code, `${entry.system} ${entry.code}`]);
  return [...numbers, ...references];
}

/**
 * 两笔修订之间识别信息改了什么，给修订记录逐条显：号的增删、外部编码的增删。只交事实，不判对错——改号有没有带更正依据由服务端
 * 在登记时判过了；这笔带了更正依据时另起一条说出来。
 */
export function identificationChangeLines(
  before: BusinessPartyIdentificationRecord | null,
  after: BusinessPartyIdentificationRecord,
): string[] {
  const numberKey = (record: BusinessPartyIdentificationRecord | null) =>
    record?.identityLayerRegistered === true
      ? (record.lifetimeRegistrationNumbers ?? []).map((entry) => `${record.registrationCountry ?? ''} ${entry.typeCode} ${entry.number}`)
      : [];
  const referenceKey = (record: BusinessPartyIdentificationRecord | null) => (record?.externalReferences ?? []).map(externalReferenceText);
  const lines: string[] = [];
  const diff = (label: string, was: string[], now: string[]) => {
    for (const entry of now) if (!was.includes(entry)) lines.push(`${label}新增 ${entry}`);
    for (const entry of was) if (!now.includes(entry)) lines.push(`${label}去掉 ${entry}`);
  };
  diff('注册号', numberKey(before), numberKey(after));
  diff('外部编码', referenceKey(before), referenceKey(after));
  if (after.identityCorrectionBasis) lines.push(`身份更正依据 ${after.identityCorrectionBasis}`);
  return lines;
}
