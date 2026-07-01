/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import {
  filterItemsWithAncestors,
  isKeywordEmpty,
  matchSearchValues,
  pushSearchValue,
} from '../../helpers/keyword-search';
import { quotaToApplicationAmount } from '../../helpers/quota';
import { timestamp2string } from '../../helpers/utils';
import { formatMoneyNumber, rowCurrency } from './format';

const CLIENT_RECORD_SEARCH_LIMIT = 1000;

function pushFormattedAmount(parts, value, currency, fractionDigits) {
  if (value == null || value === '') {
    return;
  }
  pushSearchValue(parts, value);
  pushSearchValue(parts, formatMoneyNumber(value, currency, fractionDigits));
}

function pushApplicationRemainAmount(parts, row, currency) {
  if (row.remain_amount != null && row.remain_amount !== '') {
    pushFormattedAmount(parts, row.remain_amount, currency, 2);
    return;
  }
  if (row.remain_quota != null && row.remain_quota !== '') {
    pushFormattedAmount(parts, quotaToApplicationAmount(row.remain_quota, currency), currency, 2);
  }
}

export function buildApplicationSearchValues(row) {
  const currency = rowCurrency(row);
  const parts = [];
  pushSearchValue(parts, row.id);
  pushSearchValue(parts, row.ticket_no);
  pushSearchValue(parts, row.user_name);
  pushSearchValue(parts, row.user_email);
  pushSearchValue(parts, row.work_no);
  pushSearchValue(parts, row.org_code);
  pushSearchValue(parts, row.org_name);
  pushFormattedAmount(parts, row.amount, currency, 2);
  pushApplicationRemainAmount(parts, row, currency);
  pushSearchValue(parts, row.issued_time ? timestamp2string(row.issued_time) : '');
  return parts;
}

export function applicationMatchesKeyword(row, keyword) {
  return matchSearchValues(buildApplicationSearchValues(row), keyword);
}

export function filterApplications(applications, keyword) {
  if (isKeywordEmpty(keyword)) {
    return applications;
  }
  return applications.filter((row) => applicationMatchesKeyword(row, keyword));
}

export { CLIENT_RECORD_SEARCH_LIMIT };

export function buildPolicySearchValues(policy, t, scopeTypeTag) {
  const scopeLabel = scopeTypeTag[policy.scope_type]?.labelKey;
  const currency = rowCurrency(policy);
  const parts = [];
  pushSearchValue(parts, policy.id);
  pushSearchValue(parts, policy.token_apply_id);
  pushSearchValue(parts, policy.scope_code);
  pushSearchValue(parts, policy.scope_type);
  pushSearchValue(parts, scopeLabel ? t(scopeLabel) : '');
  pushSearchValue(parts, policy.token_type);
  pushSearchValue(parts, policy.period_type);
  pushSearchValue(parts, policy.period_key);
  pushSearchValue(parts, policy.enabled ? t('是') : t('否'));
  pushSearchValue(parts, policy.currency);
  pushFormattedAmount(parts, policy.total_amount, currency, 2);
  pushFormattedAmount(parts, policy.approved_amount, currency, 2);
  pushFormattedAmount(parts, policy.remaining_amount, currency, 2);
  pushFormattedAmount(parts, policy.cap_amount, currency, 4);
  pushFormattedAmount(parts, policy.used_amount, currency, 4);
  return parts;
}

export function policyMatchesKeyword(policy, keyword, t, scopeTypeTag) {
  return matchSearchValues(buildPolicySearchValues(policy, t, scopeTypeTag), keyword);
}

export function filterPoliciesWithAncestors(policies, keyword, t, scopeTypeTag) {
  return filterItemsWithAncestors(
    policies,
    keyword,
    (policy, value) => policyMatchesKeyword(policy, value, t, scopeTypeTag),
    (policy) => policy.parent_id,
    (policy) => policy.id,
  );
}
