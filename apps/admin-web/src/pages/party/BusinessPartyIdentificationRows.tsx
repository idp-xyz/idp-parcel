import { useMemo } from 'react';
import { listRegistrationNumberTypes, type BusinessPartyIdentificationRecord } from './api';
import { partyIdentificationCellsOf, type PartyIdentificationCells } from './business-party-identification';
import { registrationNumberTypeNamesOf } from './legal-entity-identity';
import { CountryLabel } from '../refdata-labels';
import type { DetailMeta } from '../../templates';
import { RefText } from './copy-affordance';
import { Fact } from './detail-primitives';
import { useLoaded } from './PublicationFormFields';

/**
 * 识别信息的读出格，类型码经注册号类型目录译成目录译名；目录读不到就按码显。目录走取数层的共享缓存（useLoaded），
 * 对象页与右栏各调一次也只读一遍。
 */
function usePartyIdentificationCells(record: BusinessPartyIdentificationRecord): PartyIdentificationCells {
  const typeCatalogue = useLoaded(listRegistrationNumberTypes);
  const typeNameOf = useMemo(
    () => registrationNumberTypeNamesOf(typeCatalogue?.kind === 'outcome' ? typeCatalogue.body.registrationNumberTypes : []),
    [typeCatalogue],
  );
  return partyIdentificationCellsOf(record, typeNameOf);
}

/**
 * 右栏头区 meta 行上的识别信息（照公司主体右栏的 meta 行）：注册地、每个注册号、每个外部编码各一格，号与码点击即复制。
 * 没登的格不出现——往来单位的识别信息本来可缺，头区不为空格占位。
 */
export function BusinessPartyIdentificationFacts({ record }: { record: BusinessPartyIdentificationRecord }) {
  const cells = usePartyIdentificationCells(record);
  return (
    <>
      {cells.country !== null ? (
        <Fact inline label="注册地">
          <CountryLabel code={cells.country} />
        </Fact>
      ) : null}
      {cells.numbers.map((cell, index) => (
        <Fact inline label="注册号" key={`number-${cell.number}#${index}`}>
          <span className="text-idpxyz-breadcrumb">{cell.typeName}</span> <RefText value={cell.number} name={cell.typeName} />
        </Fact>
      ))}
      {cells.references.map((cell) => (
        <Fact inline label={cell.purpose} key={`reference-${cell.system}-${cell.purpose}`}>
          <span className="text-idpxyz-breadcrumb">{cell.system}</span> <RefText value={cell.code} name={`${cell.system} ${cell.purpose}`} />
        </Fact>
      ))}
    </>
  );
}

/**
 * 对象页 meta 行上的识别信息，与右栏头区（BusinessPartyIdentificationFacts）同一份判读：注册地、注册号、外部编码，没登的格不出现
 * （票 business-party-ux/03）。模板拿标签作 key，所以同名的格并成一项——一家可以登好几个注册号，也可以有两套系统的同一用途编码。
 */
export function useBusinessPartyIdentificationMeta(record: BusinessPartyIdentificationRecord | null): DetailMeta[] {
  const typeCatalogue = useLoaded(listRegistrationNumberTypes);
  const typeNameOf = useMemo(
    () => registrationNumberTypeNamesOf(typeCatalogue?.kind === 'outcome' ? typeCatalogue.body.registrationNumberTypes : []),
    [typeCatalogue],
  );
  if (record === null) return [];
  const cells = partyIdentificationCellsOf(record, typeNameOf);
  const joined = (parts: { key: string; head: string; value: string; name: string }[]) => (
    <span className="inline-flex flex-wrap gap-x-3">
      {parts.map((part) => (
        <span key={part.key}>
          <span className="text-idpxyz-breadcrumb">{part.head}</span> <RefText value={part.value} name={part.name} />
        </span>
      ))}
    </span>
  );
  const purposes = [...new Set(cells.references.map((cell) => cell.purpose))];
  return [
    ...(cells.country !== null ? [{ label: '注册地', value: <CountryLabel code={cells.country} /> }] : []),
    ...(cells.numbers.length
      ? [
          {
            label: '注册号',
            value: joined(cells.numbers.map((cell, index) => ({ key: `${cell.number}#${index}`, head: cell.typeName, value: cell.number, name: cell.typeName }))),
          },
        ]
      : []),
    ...purposes.map((purpose) => ({
      label: purpose,
      value: joined(
        cells.references
          .filter((cell) => cell.purpose === purpose)
          .map((cell) => ({ key: cell.system, head: cell.system, value: cell.code, name: `${cell.system} ${cell.purpose}` })),
      ),
    })),
  ];
}
