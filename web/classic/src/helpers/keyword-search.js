/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

/** @returns {string} */
export function normalizeKeyword(keyword) {
  return String(keyword ?? '').trim().toLowerCase();
}

/** @returns {boolean} */
export function isKeywordEmpty(keyword) {
  return normalizeKeyword(keyword) === '';
}

/**
 * Fuzzy match keyword against one or more display/search values.
 * @param {string} keyword
 * @param {...(string|number|null|undefined)} values
 * @returns {boolean}
 */
export function fuzzyMatchKeyword(keyword, ...values) {
  const normalized = normalizeKeyword(keyword);
  if (!normalized) {
    return true;
  }
  return values.some((value) => {
    if (value == null || value === '') {
      return false;
    }
    return String(value).toLowerCase().includes(normalized);
  });
}

/** @param {unknown[]} parts */
export function pushSearchValue(parts, value) {
  if (value == null || value === '') {
    return;
  }
  parts.push(value);
}

/**
 * @param {unknown[]} values
 * @param {string} keyword
 */
export function matchSearchValues(values, keyword) {
  return fuzzyMatchKeyword(keyword, ...values);
}

/**
 * Filter flat items by keyword, keeping ancestor rows for tree display.
 * @template T
 * @param {T[]} items
 * @param {string} keyword
 * @param {(item: T, keyword: string) => boolean} matchItem
 * @param {(item: T) => number|null|undefined} getParentId
 * @param {(item: T) => number} getId
 */
export function filterItemsWithAncestors(items, keyword, matchItem, getParentId, getId) {
  if (isKeywordEmpty(keyword)) {
    return items;
  }
  const byId = new Map(items.map((item) => [getId(item), item]));
  const matchedIds = new Set();
  for (const item of items) {
    if (!matchItem(item, keyword)) {
      continue;
    }
    matchedIds.add(getId(item));
    let parentId = getParentId(item);
    while (parentId != null && byId.has(parentId)) {
      matchedIds.add(parentId);
      parentId = getParentId(byId.get(parentId));
    }
  }
  return items.filter((item) => matchedIds.has(getId(item)));
}
