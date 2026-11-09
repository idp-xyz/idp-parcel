import { useEffect, useState } from 'react';
import { listPortsPaths, registryConditions } from '../customs/api';
import { everyPage } from '../customs/every-page';
import { listNetworkCatalog, networkCatalogConditions } from './api';
import { placeNameIndexOf, type NamedVersion, type PlaceNameIndex } from './place-names';

// 网点与口岸的名称一处取、各页共用（照 use-customer-names 的做法）：网点读网络目录的网点册，口岸读合规候选口岸册，两册都分页
// 走到末页，各折成「编码 → 此刻生效那一版的名称」。模块级缓存：一个标签页里只读一次；读不到（未配置、拒绝、依赖故障）不缓存，
// 下次挂载再问，读不到时各页显编码，不把一次读不到当成「没有名称」。

const emptyIndex: PlaceNameIndex = new Map();

async function loadNodeVersions(): Promise<NamedVersion[] | null> {
  const versions: NamedVersion[] = [];
  let after: string | null = null;
  do {
    const answer = await listNetworkCatalog(networkCatalogConditions('node'), after);
    if (answer.kind !== 'outcome') return null;
    for (const record of answer.body.versions) {
      if (record.code) versions.push({ code: record.code, displayName: record.displayName, from: record.effectiveFrom, to: record.effectiveTo });
    }
    after = answer.body.page.next;
  } while (after !== null);
  return versions;
}

async function loadPortVersions(): Promise<NamedVersion[] | null> {
  const answer = await everyPage((after) => listPortsPaths(registryConditions('candidate-port'), after), (body) => body.ports);
  if (answer.kind !== 'outcome') return null;
  return answer.body.map((record) => ({ code: record.port, displayName: record.displayName, from: record.appliesFrom, to: record.appliesUntil }));
}

let pending: Promise<PlaceNameIndex> | null = null;

function loadPlaceNames(): Promise<PlaceNameIndex> {
  if (pending === null) {
    pending = Promise.all([loadNodeVersions(), loadPortVersions()]).then(([nodes, ports]) => {
      // 一半读不到也照用另一半；两半都读不到才不缓存，下次挂载再问。
      if (nodes === null || ports === null) pending = null;
      if (nodes === null && ports === null) return emptyIndex;
      return placeNameIndexOf([...(nodes ?? []), ...(ports ?? [])], new Date());
    });
  }
  return pending;
}

/** 网点与口岸编码 → 名称（两册并成一张表，同一编码两册都登了名称时网点那一份在先）。读回之前与读不到时是空索引，页面按编码显示。 */
export function usePlaceNames(): PlaceNameIndex {
  const [names, setNames] = useState<PlaceNameIndex>(emptyIndex);
  useEffect(() => {
    let cancelled = false;
    void loadPlaceNames().then((index) => {
      if (!cancelled) setNames(index);
    });
    return () => {
      cancelled = true;
    };
  }, []);
  return names;
}
