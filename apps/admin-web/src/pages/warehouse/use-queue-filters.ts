import { useState } from 'react';

/** 一张队列式列表的筛选态：检索词、状态签、货主、仓库与排序（null 即按页面自己的缺省排）。 */
export interface QueueFilters {
  keyword: string;
  status: string;
  owner: string;
  warehouse: string;
  sort: string | null;
  setKeyword: (value: string) => void;
  setStatus: (value: string) => void;
  setOwner: (value: string) => void;
  setWarehouse: (value: string) => void;
  setSort: (value: string | null) => void;
}

/**
 * 筛选态由页签的外面持有：盘点与调整页的三个页签切走时那一张列表会卸载，状态放在列表里就丢了，换回来又得重选。
 * 单据页（别的标签）与这张列表标签无关，不经过这里。
 */
export function useQueueFilters(initialStatus: string): QueueFilters {
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState(initialStatus);
  const [owner, setOwner] = useState('');
  const [warehouse, setWarehouse] = useState('');
  const [sort, setSort] = useState<string | null>(null);
  return { keyword, status, owner, warehouse, sort, setKeyword, setStatus, setOwner, setWarehouse, setSort };
}
