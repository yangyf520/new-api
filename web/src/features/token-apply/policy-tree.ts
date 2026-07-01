/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

export type PolicyTreeNode<T extends { id: number; parent_id?: number | null }> =
  T & {
    children?: PolicyTreeNode<T>[]
  }

function compareByApplyIdDesc<
  T extends { id: number; token_apply_id?: number | null },
>(a: T, b: T): number {
  const aid = Number(a.token_apply_id) || 0
  const bid = Number(b.token_apply_id) || 0
  if (bid !== aid) return bid - aid
  return b.id - a.id
}

function sortPolicySiblings<
  T extends { id: number; token_apply_id?: number | null; parent_id?: number | null },
>(nodes: Array<PolicyTreeNode<T>>) {
  nodes.sort(compareByApplyIdDesc)
  for (const node of nodes) {
    if (node.children?.length) {
      sortPolicySiblings(node.children)
    }
  }
}

export function buildPolicyTree<
  T extends { id: number; parent_id?: number | null; token_apply_id?: number | null },
>(policies: T[]): PolicyTreeNode<T>[] {
  if (!policies.length) return []
  const sorted = [...policies].sort(compareByApplyIdDesc)
  const byId = new Map<number, PolicyTreeNode<T>>()
  for (const policy of sorted) {
    byId.set(policy.id, { ...policy, children: [] })
  }
  const roots: PolicyTreeNode<T>[] = []
  for (const policy of sorted) {
    const node = byId.get(policy.id)!
    const parentId = policy.parent_id
    if (parentId != null && byId.has(parentId)) {
      byId.get(parentId)!.children!.push(node)
    } else {
      roots.push(node)
    }
  }
  const prune = (nodes: PolicyTreeNode<T>[]) => {
    for (const node of nodes) {
      if (!node.children?.length) {
        delete node.children
      } else {
        prune(node.children)
      }
    }
  }
  prune(roots)
  sortPolicySiblings(roots)
  return roots
}

export function flattenPolicyTree<
  T extends { id: number; parent_id?: number | null },
>(
  nodes: Array<PolicyTreeNode<T>>,
  depth = 0
): Array<T & { depth: number }> {
  const rows: Array<T & { depth: number }> = []
  for (const node of nodes) {
    const { children, ...rest } = node
    rows.push({ ...rest, depth })
    if (children?.length) {
      rows.push(...flattenPolicyTree(children, depth + 1))
    }
  }
  return rows
}
