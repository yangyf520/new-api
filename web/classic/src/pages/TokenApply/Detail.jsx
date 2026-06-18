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
import { Link, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  Descriptions,
  Empty,
  Spin,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconArrowLeft } from '@douyinfe/semi-icons';
import { API } from '../../helpers';
import { timestamp2string } from '../../helpers/utils';
import {
  formatApplicationBalanceAmount,
  formatCurrencyAmount,
  formatValueWithUnit,
  rowCurrency,
} from './format';

const { Title, Text } = Typography;

const API_BASE = '/api/token-apply';

const panelStyle = {
  borderColor: 'var(--semi-color-border)',
  background: 'var(--semi-color-fill-0)',
};

const tableBodyStyle = {
  borderColor: 'var(--semi-color-border)',
  background: 'var(--semi-color-bg-1)',
};

const ACTION_TAG = {
  issue: { color: 'green', labelKey: '发放' },
  increase: { color: 'blue', labelKey: '增额' },
  decrease: { color: 'orange', labelKey: '减额' },
  adjust: { color: 'grey', labelKey: '调整' },
};

async function fetchApplicationDetail(id) {
  const res = await API.get(`${API_BASE}/records/${id}`);
  return res.data?.data ?? null;
}

function displayValue(value) {
  if (value === null || value === undefined || value === '') {
    return '-';
  }
  return value;
}

const TokenApplyDetail = () => {
  const { t } = useTranslation();
  const { id } = useParams();
  const applicationId = Number(id);
  const [loading, setLoading] = useState(true);
  const [detail, setDetail] = useState(null);

  const loadDetail = useCallback(async () => {
    if (!Number.isFinite(applicationId) || applicationId <= 0) {
      setDetail(null);
      setLoading(false);
      return;
    }
    setLoading(true);
    try {
      setDetail(await fetchApplicationDetail(applicationId));
    } finally {
      setLoading(false);
    }
  }, [applicationId]);

  useEffect(() => {
    loadDetail();
  }, [loadDetail]);

  const currency = rowCurrency(detail);

  const basicInfo = useMemo(() => {
    if (!detail) return [];
    return [
      {
        key: t('所属组织'),
        value: displayValue(
          `${detail.org_code || ''} ${detail.org_name || ''}`.trim(),
        ),
      },
      { key: t('员工工号'), value: displayValue(detail.work_no) },
      {
        key: t('申请人员'),
        value: displayValue(detail.user_name || detail.user_email || detail.work_no),
      },
      {
        key: t('发放时间'),
        value: detail.issued_time ? timestamp2string(detail.issued_time) : '-',
      },
      { key: t('备注说明'), value: displayValue(detail.remark) },
    ];
  }, [detail, t]);

  const quotaInfo = useMemo(() => {
    if (!detail) return [];
    return [
      { key: t('令牌编号'), value: displayValue(detail.token_id) },
      { key: t('额度模式'), value: displayValue(detail.quota_mode) },
      {
        key: t('批准额度'),
        value: (
          <Text strong style={{ color: 'var(--semi-color-primary)' }}>
            {formatCurrencyAmount(detail.amount, currency)}
          </Text>
        ),
      },
      {
        key: t('已用额度'),
        value: (
          <Text strong style={{ color: 'var(--semi-color-text-1)' }}>
            {formatApplicationBalanceAmount(detail, 'used_amount', 'used_quota', currency)}
          </Text>
        ),
      },
      {
        key: t('剩余额度'),
        value: (
          <Text strong style={{ color: 'var(--semi-color-success)' }}>
            {formatApplicationBalanceAmount(detail, 'remain_amount', 'remain_quota', currency)}
          </Text>
        ),
      },
    ];
  }, [currency, detail, t]);

  const logColumns = useMemo(
    () => [
      {
        title: t('操作类型'),
        dataIndex: 'action',
        width: 88,
        render: (action) => {
          const config = ACTION_TAG[action] || { color: 'grey', labelKey: action };
          return <Tag color={config.color}>{t(config.labelKey)}</Tag>;
        },
      },
      {
        title: t('变更工单'),
        dataIndex: 'change_ticket_no',
        render: (value, row) =>
          displayValue(value || (row.action === 'issue' ? detail?.ticket_no : '')),
      },
      {
        title: t('额度变更'),
        dataIndex: 'amount_before',
        render: (_, row) => (
          <Text>
            {formatValueWithUnit(row.amount_before, currency)}
            {' → '}
            {formatValueWithUnit(row.amount_after, currency)}
          </Text>
        ),
      },
      {
        title: t('预算增量'),
        dataIndex: 'budget_delta',
        width: 120,
        render: (value) => {
          const num = Number(value);
          const prefix = num > 0 ? '+' : '';
          return formatCurrencyAmount(value, currency, { prefix });
        },
      },
      {
        title: t('变更时间'),
        dataIndex: 'created_at',
        width: 168,
        render: (value) => (value ? timestamp2string(value) : '-'),
      },
    ],
    [currency, detail?.ticket_no, t],
  );

  return (
    <div className='mt-[60px] px-2 pb-6'>
      <Card>
        <div className='mb-4 flex flex-wrap items-center justify-between gap-3'>
          <div className='flex min-w-0 items-center gap-3'>
            <Link to='/console/token-apply'>
              <Button icon={<IconArrowLeft />} theme='borderless' type='tertiary' />
            </Link>
            <div className='min-w-0'>
              <Title heading={4} style={{ margin: 0 }}>
                {t('申请详情')}
                {detail?.id ? ` #${detail.id}` : ''}
              </Title>
            </div>
          </div>
          <Link to='/console/token-apply'>
            <Button theme='light' type='tertiary'>
              {t('返回列表')}
            </Button>
          </Link>
        </div>

        <Spin spinning={loading}>
          {!loading && !detail ? (
            <Empty description={t('未找到')} style={{ padding: 48 }} />
          ) : detail ? (
            <div className='flex flex-col gap-4'>
              <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
                <div className='h-full rounded-lg border p-4' style={panelStyle}>
                  <Title heading={6} style={{ margin: '0 0 12px' }}>
                    {t('基本信息')}
                  </Title>
                  <div className='overflow-hidden rounded-md border p-3' style={tableBodyStyle}>
                    <Descriptions data={basicInfo} column={2} layout='vertical' />
                  </div>
                </div>
                <div className='h-full rounded-lg border p-4' style={panelStyle}>
                  <Title heading={6} style={{ margin: '0 0 12px' }}>
                    {t('额度与令牌')}
                  </Title>
                  <div className='overflow-hidden rounded-md border p-3' style={tableBodyStyle}>
                    <Descriptions data={quotaInfo} column={2} layout='vertical' />
                  </div>
                </div>
              </div>

              <div className='rounded-lg border p-4' style={panelStyle}>
                <Title heading={6} style={{ margin: '0 0 12px' }}>
                  {t('申请变更')}
                </Title>
                <div className='overflow-hidden rounded-md border' style={tableBodyStyle}>
                  <Table
                  columns={logColumns}
                  dataSource={detail.logs ?? []}
                  rowKey='id'
                  size='small'
                  pagination={false}
                  empty={
                    <Empty description={t('暂无审计记录')} style={{ padding: 32 }} />
                  }
                />
                </div>
              </div>
            </div>
          ) : null}
        </Spin>
      </Card>
    </div>
  );
};

export default TokenApplyDetail;
