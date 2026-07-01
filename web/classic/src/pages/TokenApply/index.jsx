/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Card, Empty, Input, Spin, TabPane, Tabs, Table, Tag, Typography } from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';
import { API } from '../../helpers';
import { timestamp2string } from '../../helpers/utils';
import { formatCurrencyAmount, formatApplicationBalanceAmount, rowCurrency } from './format';
import TokenModal from './TokenModal';
import { filterPoliciesWithAncestors, filterApplications, CLIENT_RECORD_SEARCH_LIMIT } from './search';

const { Title } = Typography;

const API_BASE = '/api/token-apply';

async function fetchApplications(page, pageSize, keyword) {
  const query = new URLSearchParams({
    p: String(page),
    page_size: String(pageSize),
  });
  if (keyword?.trim()) {
    query.set('keyword', keyword.trim());
  }
  const res = await API.get(`${API_BASE}/records?${query}`);
  return {
    items: res.data?.data?.items ?? [],
    total: res.data?.data?.total ?? 0,
  };
}

async function fetchPolicies(path) {
  const res = await API.get(`${API_BASE}/${path}`);
  return res.data?.data ?? [];
}

const SCOPE_TYPE_TAG = {
  org: { color: 'purple', labelKey: '组织' },
  team: { color: 'blue', labelKey: '部门' },
  project: { color: 'cyan', labelKey: '项目' },
  token: { color: 'grey', labelKey: '令牌' },
};

function compareByApplyIdDesc(a, b) {
  const aid = Number(a.token_apply_id) || 0;
  const bid = Number(b.token_apply_id) || 0;
  if (bid !== aid) return bid - aid;
  return b.id - a.id;
}

function sortPolicySiblings(nodes) {
  nodes.sort(compareByApplyIdDesc);
  for (const node of nodes) {
    if (node.children?.length) {
      sortPolicySiblings(node.children);
    }
  }
}

function buildPolicyTree(policies) {
  if (!policies?.length) return [];
  const sorted = [...policies].sort(compareByApplyIdDesc);
  const byId = new Map();
  for (const policy of sorted) {
    byId.set(policy.id, { ...policy, children: [] });
  }
  const roots = [];
  for (const policy of sorted) {
    const node = byId.get(policy.id);
    const parentId = policy.parent_id;
    if (parentId != null && byId.has(parentId)) {
      byId.get(parentId).children.push(node);
    } else {
      roots.push(node);
    }
  }
  const prune = (nodes) => {
    for (const node of nodes) {
      if (node.children.length === 0) {
        delete node.children;
      } else {
        prune(node.children);
      }
    }
  };
  prune(roots);
  sortPolicySiblings(roots);
  return roots;
}

const TokenApply = () => {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState('records');
  const [loading, setLoading] = useState(false);
  const [applications, setApplications] = useState([]);
  const [appTotal, setAppTotal] = useState(0);
  const [appPage, setAppPage] = useState(1);
  const [appPageSize, setAppPageSize] = useState(20);
  const [applicationSource, setApplicationSource] = useState([]);
  const [budgetPolicies, setBudgetPolicies] = useState([]);
  const [consumptionPolicies, setConsumptionPolicies] = useState([]);
  const [tokenModalApp, setTokenModalApp] = useState(null);
  const [searchInput, setSearchInput] = useState('');
  const [keyword, setKeyword] = useState('');

  useEffect(() => {
    const timer = window.setTimeout(() => setKeyword(searchInput), 300);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  useEffect(() => {
    setAppPage(1);
  }, [keyword]);

  const loadApplicationSearchSource = useCallback(async () => {
    setLoading(true);
    try {
      const { items } = await fetchApplications(1, CLIENT_RECORD_SEARCH_LIMIT, '');
      setApplicationSource([...items].sort((a, b) => b.id - a.id));
    } finally {
      setLoading(false);
    }
  }, []);

  const loadApplicationPage = useCallback(async () => {
    setLoading(true);
    try {
      const { items, total } = await fetchApplications(appPage, appPageSize, '');
      const rows = [...items].sort((a, b) => b.id - a.id);
      setApplicationSource(rows);
      setApplications(rows);
      setAppTotal(total);
    } finally {
      setLoading(false);
    }
  }, [appPage, appPageSize]);

  useEffect(() => {
    if (!keyword?.trim()) {
      return;
    }
    const filtered = filterApplications(applicationSource, keyword);
    setAppTotal(filtered.length);
    const start = (appPage - 1) * appPageSize;
    setApplications(filtered.slice(start, start + appPageSize));
  }, [applicationSource, keyword, appPage, appPageSize]);

  const loadPolicies = useCallback(async (tab) => {
    setLoading(true);
    try {
      if (tab === 'budget') {
        setBudgetPolicies(await fetchPolicies('budget'));
      } else if (tab === 'consumption') {
        setConsumptionPolicies(await fetchPolicies('consumption'));
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (activeTab !== 'records') {
      return;
    }
    if (keyword?.trim()) {
      loadApplicationSearchSource();
      return;
    }
    loadApplicationPage();
  }, [activeTab, keyword, loadApplicationSearchSource, loadApplicationPage]);

  useEffect(() => {
    if (activeTab === 'budget') {
      loadPolicies('budget');
    } else if (activeTab === 'consumption') {
      loadPolicies('consumption');
    }
  }, [activeTab, loadPolicies]);

  const applicationIdCol = useMemo(
    () => ({
      title: t('申请ID'),
      dataIndex: 'token_apply_id',
      width: 108,
      render: (value, row) => {
        const applyId = value ?? row.id;
        if (!applyId || applyId <= 0) {
          return '-';
        }
        return (
          <Link
            to={`/console/token-apply/${applyId}`}
            className='text-semi-color-primary font-mono'
          >
            {applyId}
          </Link>
        );
      },
    }),
    [t],
  );

  const applicationColumns = useMemo(
    () => [
      {
        ...applicationIdCol,
        dataIndex: 'id',
      },
      {
        title: t('流程单号'),
        dataIndex: 'ticket_no',
        render: (value, row) =>
          row.token_id > 0 ? (
            <span
              role='button'
              tabIndex={0}
              className='text-semi-color-primary cursor-pointer hover:underline'
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                setTokenModalApp(row);
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  setTokenModalApp(row);
                }
              }}
            >
              {value}
            </span>
          ) : (
            value
          ),
      },
      {
        title: t('申请人员'),
        dataIndex: 'user_name',
        render: (_, row) => row.user_name || row.user_email || '-',
      },
      {
        title: t('员工工号'),
        dataIndex: 'work_no',
        render: (value) => value || '-',
      },
      {
        title: t('所属组织'),
        dataIndex: 'org_code',
        render: (_, row) => `${row.org_code || ''} ${row.org_name || ''}`.trim() || '-',
      },
      {
        title: t('批准额度'),
        dataIndex: 'amount',
        render: (amount, row) => formatCurrencyAmount(amount, rowCurrency(row)),
      },
      {
        title: t('剩余额度'),
        dataIndex: 'remain_quota',
        render: (_, row) =>
          formatApplicationBalanceAmount(row, 'remain_amount', 'remain_quota', rowCurrency(row)),
      },
      {
        title: t('发放时间'),
        dataIndex: 'issued_time',
        render: (value) => (value ? timestamp2string(value) : '-'),
      },
      {
        title: t('操作'),
        dataIndex: 'id',
        width: 100,
        render: (value) => (
          <Link to={`/console/token-apply/${value}`} className='text-semi-color-primary'>
            {t('查看变更')}
          </Link>
        ),
      },
    ],
    [applicationIdCol, t],
  );

  const policyTreeBaseColumns = useMemo(
    () => [
      {
        title: t('策略范围'),
        dataIndex: 'scope_code',
        render: (scopeCode, row) => {
          const config = SCOPE_TYPE_TAG[row.scope_type] || {
            color: 'grey',
            labelKey: row.scope_type,
          };
          return (
            <span className='inline-flex items-center gap-2'>
              <Tag color={config.color} size='small'>
                {t(config.labelKey)}
              </Tag>
              <span>{scopeCode}</span>
            </span>
          );
        },
      },
      { title: t('令牌类型'), dataIndex: 'token_type' },
      { title: t('周期'), dataIndex: 'period_type' },
      { title: t('周期键'), dataIndex: 'period_key' },
      {
        title: t('启用'),
        dataIndex: 'enabled',
        width: 72,
        render: (enabled) => (enabled ? t('是') : t('否')),
      },
    ],
    [t],
  );

  const currencyAmountCol = (titleKey, dataIndex, fractionDigits) => ({
    title: t(titleKey),
    dataIndex,
    render: (value, row) =>
      formatCurrencyAmount(value, rowCurrency(row), { fractionDigits }),
  });

  const budgetColumns = useMemo(
    () => [
      applicationIdCol,
      ...policyTreeBaseColumns,
      currencyAmountCol('上限额度', 'total_amount'),
      currencyAmountCol('已批额度', 'approved_amount'),
      currencyAmountCol('剩余额度', 'remaining_amount'),
    ],
    [policyTreeBaseColumns, applicationIdCol, t],
  );

  const consumptionColumns = useMemo(
    () => [
      applicationIdCol,
      ...policyTreeBaseColumns,
      currencyAmountCol('封顶额度', 'cap_amount', 4),
      currencyAmountCol('已用额度', 'used_amount', 4),
      currencyAmountCol('剩余额度', 'remaining_amount', 4),
    ],
    [policyTreeBaseColumns, applicationIdCol, t],
  );

  const budgetPolicyTree = useMemo(
    () => buildPolicyTree(filterPoliciesWithAncestors(budgetPolicies, keyword, t, SCOPE_TYPE_TAG)),
    [budgetPolicies, keyword, t],
  );

  const consumptionPolicyTree = useMemo(
    () => buildPolicyTree(filterPoliciesWithAncestors(consumptionPolicies, keyword, t, SCOPE_TYPE_TAG)),
    [consumptionPolicies, keyword, t],
  );

  const renderTable = (columns, dataSource, emptyText, pagination, tree = false) => (
    <Table
      columns={columns}
      dataSource={dataSource}
      rowKey='id'
      pagination={pagination}
      defaultExpandAllRows={tree || undefined}
      empty={
        <Empty
          description={emptyText}
          style={{ padding: 24 }}
        />
      }
    />
  );

  return (
    <div className='mt-[60px] px-2'>
      <Card>
        <Title heading={4} style={{ marginBottom: 0 }}>
          {t('部门额度')}
        </Title>
        <Spin spinning={loading}>
          <Tabs
            type='line'
            activeKey={activeTab}
            onChange={setActiveTab}
            style={{ marginTop: 0 }}
            tabBarExtraContent={
              <Input
                size='small'
                value={searchInput}
                onChange={setSearchInput}
                prefix={<IconSearch />}
                placeholder={t('搜索流程单号、组织、申请人…')}
                showClear
                className='w-64'
              />
            }
          >
            <TabPane
              tab={t('令牌申请')}
              itemKey='records'
            >
              {renderTable(
                applicationColumns,
                applications,
                t('暂无令牌申请'),
                {
                  currentPage: appPage,
                  pageSize: appPageSize,
                  total: appTotal,
                  onPageChange: setAppPage,
                  onPageSizeChange: setAppPageSize,
                },
              )}
            </TabPane>
            <TabPane
              tab={t('预算策略')}
              itemKey='budget'
            >
              {renderTable(
                budgetColumns,
                budgetPolicyTree,
                t('暂无预算策略'),
                false,
                true,
              )}
            </TabPane>
            <TabPane
              tab={t('消耗策略')}
              itemKey='consumption'
            >
              {renderTable(
                consumptionColumns,
                consumptionPolicyTree,
                t('暂无消耗策略'),
                false,
                true,
              )}
            </TabPane>
          </Tabs>
        </Spin>
      </Card>
      <TokenModal
        application={tokenModalApp}
        visible={Boolean(tokenModalApp)}
        onClose={() => setTokenModalApp(null)}
      />
    </div>
  );
};

export default TokenApply;
