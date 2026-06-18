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

export function buildPolicyTree<T extends { id: number; parent_id?: number | null }>(
  policies: T[]
): PolicyTreeNode<T>[] {
  if (!policies.length) return []
  const byId = new Map<number, PolicyTreeNode<T>>()
  for (const policy of policies) {
    byId.set(policy.id, { ...policy, children: [] })
  }
  const roots: PolicyTreeNode<T>[] = []
  for (const policy of policies) {
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
