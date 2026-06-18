/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Descriptions,
  Empty,
  Modal,
  Spin,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { IconCopy, IconKey } from '@douyinfe/semi-icons';
import { API, copy, showError, showSuccess } from '../../helpers';
import {
  formatApplicationBalanceAmount,
  formatCurrencyAmount,
  rowCurrency,
} from './format';

const { Text, Title } = Typography;
const API_BASE = '/api/token-apply';

const panelStyle = {
  borderColor: 'var(--semi-color-border)',
  background: 'var(--semi-color-fill-0)',
};

async function fetchApplicationToken(id) {
  const res = await API.get(`${API_BASE}/records/${id}/token`);
  const { success, message, data } = res.data ?? {};
  if (!success) {
    throw new Error(message || 'failed');
  }
  return data ?? null;
}

function QuotaStat({ label, value, accent }) {
  return (
    <div
      className='flex min-w-0 flex-col gap-1 rounded-lg border px-4 py-3'
      style={panelStyle}
    >
      <Text type='tertiary' size='small'>
        {label}
      </Text>
      <Text
        strong
        style={{
          color: accent,
          fontSize: 15,
          lineHeight: '22px',
          wordBreak: 'break-word',
        }}
      >
        {value}
      </Text>
    </div>
  );
}

export default function TokenModal({ application, visible, onClose }) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [token, setToken] = useState(null);

  const loadToken = useCallback(async () => {
    if (!application?.id) {
      setToken(null);
      return;
    }
    setLoading(true);
    try {
      const data = await fetchApplicationToken(application.id);
      setToken(data);
    } catch (error) {
      setToken(null);
      showError(error?.message || t('加载失败'));
    } finally {
      setLoading(false);
    }
  }, [application?.id, t]);

  useEffect(() => {
    if (visible) {
      loadToken();
    } else {
      setToken(null);
    }
  }, [visible, loadToken]);

  const handleCopy = async () => {
    if (!token?.token_key) return;
    if (await copy(token.token_key)) {
      showSuccess(t('已复制到剪贴板！'));
    }
  };

  const currency = rowCurrency(token || application || {});

  const tokenInfo = useMemo(() => {
    if (!token) return [];
    return [
      { key: t('流程单号'), value: application?.ticket_no || '-' },
      { key: t('令牌名称'), value: token.token_name || '-' },
      { key: t('令牌编号'), value: token.token_id ?? '-' },
    ];
  }, [application?.ticket_no, t, token]);

  const remainAmount = token
    ? formatApplicationBalanceAmount(token, 'remain_amount', 'remain_quota', currency)
    : '-';
  const usedAmount = token
    ? formatApplicationBalanceAmount(token, 'used_amount', 'used_quota', currency)
    : '-';
  const approvedAmount = formatCurrencyAmount(application?.amount, currency);

  return (
    <Modal
      title={t('申请令牌')}
      visible={visible}
      onCancel={onClose}
      footer={null}
      width={720}
      closeOnEsc
      bodyStyle={{ paddingTop: 8, paddingBottom: 16 }}
    >
      <Spin spinning={loading}>
        {!loading && !token ? (
          <Empty description={t('未找到')} style={{ padding: '32px 0' }} />
        ) : token ? (
          <div className='flex flex-col gap-4'>
            <div className='rounded-lg border p-4' style={panelStyle}>
              <div className='mb-3 flex items-center gap-2'>
                <IconKey style={{ color: 'var(--semi-color-primary)' }} />
                <Title heading={6} style={{ margin: 0 }}>
                  {t('令牌 Key')}
                </Title>
              </div>
              <div className='flex items-center gap-2 px-1 py-1'>
                {token.token_key ? (
                  <Tooltip content={t('复制')}>
                    <Button
                      icon={<IconCopy />}
                      size='small'
                      theme='borderless'
                      type='tertiary'
                      aria-label={t('复制')}
                      onClick={handleCopy}
                    />
                  </Tooltip>
                ) : null}
                <span className='min-w-0 flex-1 overflow-x-auto'>
                  <Text
                    type='secondary'
                    style={{ whiteSpace: 'nowrap', fontFamily: 'inherit' }}
                  >
                    {token.token_key || '-'}
                  </Text>
                </span>
              </div>
            </div>

            <div className='rounded-lg border p-4' style={panelStyle}>
              <Title heading={6} style={{ margin: '0 0 12px' }}>
                {t('令牌信息')}
              </Title>
              <Descriptions data={tokenInfo} column={2} layout='vertical' />
            </div>

            <div className='rounded-lg border p-4' style={panelStyle}>
              <Title heading={6} style={{ margin: '0 0 12px' }}>
                {t('额度信息')}
              </Title>
              <div className='grid grid-cols-3 gap-3'>
                <QuotaStat
                  label={t('剩余额度')}
                  value={remainAmount}
                  accent='var(--semi-color-success)'
                />
                <QuotaStat
                  label={t('已用额度')}
                  value={usedAmount}
                  accent='var(--semi-color-text-1)'
                />
                <QuotaStat
                  label={t('批准额度')}
                  value={approvedAmount}
                  accent='var(--semi-color-primary)'
                />
              </div>
            </div>
          </div>
        ) : null}
      </Spin>
    </Modal>
  );
}
