/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import React from 'react';
import { Typography } from '@douyinfe/semi-ui';
import { quotaToApplicationAmount } from '../../helpers/quota';

const { Text } = Typography;

function normalizeApplicationCurrency(currency = 'CNY') {
  const code = String(currency || 'CNY').trim().toUpperCase();
  if (code === 'RMB' || code === '') return 'CNY';
  return code;
}

function applicationCurrencyFractionDigits(currency = 'CNY') {
  switch (normalizeApplicationCurrency(currency)) {
    case 'CNY':
    case 'USD':
      return 2;
    default:
      return 2;
  }
}

export function formatMoneyNumber(value, currency = 'CNY') {
  const n = Number(value);
  if (!Number.isFinite(n)) {
    return String(value);
  }
  const digits = applicationCurrencyFractionDigits(currency);
  const factor = 10 ** digits;
  const rounded = Math.round(n * factor) / factor;
  return rounded.toLocaleString(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: digits,
  });
}

export function formatValueWithUnit(value, unit, { formatNumber = false, prefix = '' } = {}) {
  if (value == null || value === '') {
    return '-';
  }
  const display = formatNumber
    ? Number(value).toLocaleString()
    : `${prefix}${formatMoneyNumber(value, unit)}`;
  if (!unit) {
    return display;
  }
  return (
    <>
      {display}
      <Text type='tertiary' size='small'>
        {' '}
        ({unit})
      </Text>
    </>
  );
}

export function formatCurrencyAmount(value, currency = 'CNY', options = {}) {
  return formatValueWithUnit(value, currency || 'CNY', options);
}

export function rowCurrency(row, fallback = 'CNY') {
  return row?.currency || fallback;
}

/** 优先使用 API 返回的 remain_amount / used_amount（后端 decimal 换算） */
export function formatApplicationBalanceAmount(row, amountField, quotaField, currency = 'CNY') {
  const amount = row?.[amountField];
  if (amount != null && amount !== '') {
    return formatCurrencyAmount(amount, currency);
  }
  return formatApplicationQuota(row?.[quotaField], currency);
}

/** 将 tokens.remain_quota / used_quota 换算为申请币种金额（与后端 QuotaToAmount 一致） */
export function formatApplicationQuota(value, currency = 'CNY') {
  if (value == null || value === '') {
    return '-';
  }
  const amount = quotaToApplicationAmount(value, currency);
  return formatCurrencyAmount(amount, currency || 'CNY');
}
