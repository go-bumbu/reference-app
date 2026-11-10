import type { Tag, TagPatch } from '@/lib/api/types'

// Pure helpers over the tag tree as the API returns it (GET /tags): the tags at
// the root, each with its children in order. Kept free of Vue and the DOM so
// the rules of the tree editor are unit-tested on their own.

/** A tag with where it sits: its depth (0 at the root) and the names on its way down. */
export interface FlatTag {
    tag: Tag
    depth: number
    // "work / reports": the names of its ancestors and its own
    path: string
}

/** Every tag of the tree in tree order (a tag before its children). */
export function flattenTags(tree: Tag[]): FlatTag[] {
    const out: FlatTag[] = []
    const walk = (tags: Tag[], depth: number, prefix: string) => {
        for (const tag of tags) {
            const path = prefix ? `${prefix} / ${tag.name}` : tag.name
            out.push({ tag, depth, path })
            walk(tag.children, depth + 1, path)
        }
    }
    walk(tree, 0, '')
    return out
}

/** The tags whose name holds the query, ignoring case, in tree order. */
export function filterTags(tree: Tag[], query: string): FlatTag[] {
    const q = query.trim().toLowerCase()
    const all = flattenTags(tree)
    return q ? all.filter((f) => f.tag.name.toLowerCase().includes(q)) : all
}

/** The tag with the id, at any depth. */
export function findTag(tree: Tag[], id: number): Tag | undefined {
    for (const tag of tree) {
        if (tag.id === id) return tag
        const found = findTag(tag.children, id)
        if (found) return found
    }
    return undefined
}

/** The path of the tag ("work / reports"), or '' when it is not in the tree. */
export function tagPath(tree: Tag[], id: number): string {
    return flattenTags(tree).find((f) => f.tag.id === id)?.path ?? ''
}

/** The ids of the tag and of every tag below it. */
export function subtreeIds(tag: Tag, out: Set<number> = new Set()): Set<number> {
    out.add(tag.id)
    for (const child of tag.children) subtreeIds(child, out)
    return out
}

/** How many tags are below the tag, at any depth. */
export function descendantCount(tag: Tag): number {
    return subtreeIds(tag).size - 1
}

/** The children of parentId, 0 being the root. */
export function childrenOf(tree: Tag[], parentId: number): Tag[] {
    return parentId === 0 ? tree : (findTag(tree, parentId)?.children ?? [])
}

/** The last child of parentId (0 for the root), to place a new tag after: 0 when it has none. */
export function lastChildId(tree: Tag[], parentId: number): number {
    const children = childrenOf(tree, parentId)
    return children[children.length - 1]?.id ?? 0
}

/** Where a drop lands on a row: before or after it among its siblings, or onto it, nested. */
export type DropPos = 'before' | 'onto' | 'after'

/**
 * Classifies a drop by the pointer's height within the row: the top and bottom
 * bands (`edgeRatio` of the height each) place the tag beside the row, the
 * middle nests it under the row.
 */
export function dropPosition(
    rect: { top: number; height: number },
    clientY: number,
    edgeRatio = 0.25
): DropPos {
    const offset = clientY - rect.top
    if (offset < rect.height * edgeRatio) return 'before'
    if (offset > rect.height * (1 - edgeRatio)) return 'after'
    return 'onto'
}

/**
 * The PATCH body that drops `draggedId` on `targetId` at `pos`, or null when
 * the drop is not allowed (on the tag itself or into its own subtree, which
 * would make a cycle) or changes nothing (the tag is already there).
 *
 * Onto a tag nests the dragged one under it, last among the children (a move
 * without afterId). Before or after places it among the target's siblings:
 * afterId follows the API's convention, 0 meaning first, and parentId is sent
 * only when the tag changes parent, so a reorder is no move.
 */
export function resolveTagDrop(
    tree: Tag[],
    draggedId: number,
    targetId: number,
    pos: DropPos
): TagPatch | null {
    const dragged = findTag(tree, draggedId)
    const target = findTag(tree, targetId)
    if (!dragged || !target || subtreeIds(dragged).has(targetId)) return null

    if (pos === 'onto') {
        return dragged.parentId === targetId ? null : { parentId: targetId }
    }
    const siblings = childrenOf(tree, target.parentId)
    const at = siblings.findIndex((t) => t.id === targetId)
    const afterId = pos === 'after' ? targetId : (siblings[at - 1]?.id ?? 0)
    if (afterId === draggedId) return null
    if (dragged.parentId === target.parentId) {
        // already right after afterId: nothing moves
        const current = siblings.findIndex((t) => t.id === draggedId)
        const afterAt = afterId === 0 ? -1 : siblings.findIndex((t) => t.id === afterId)
        if (current === afterAt + 1) return null
        return { afterId }
    }
    return { parentId: target.parentId, afterId }
}

/** The ids of the tags above the tag, root first; empty when it is at the root or not in the tree. */
export function ancestorIds(tree: Tag[], id: number): number[] {
    const walk = (tags: Tag[], above: number[]): number[] | null => {
        for (const tag of tags) {
            if (tag.id === id) return above
            const found = walk(tag.children, [...above, tag.id])
            if (found) return found
        }
        return null
    }
    return walk(tree, []) ?? []
}

/** The tag id of the notes list's `?tag=`: a positive integer, or null for every note. */
export function tagIdFromQuery(value: unknown): number | null {
    if (typeof value !== 'string' || !/^[1-9][0-9]*$/.test(value)) return null
    const id = Number(value)
    return Number.isSafeInteger(id) ? id : null
}

/** A parent a tag can move to: the top level (id 0) or another tag, by path. */
export interface MoveTarget {
    id: number
    label: string
}

/**
 * Where the tag can move, for a move without a drag: the top level and every
 * tag but its current parent, itself and the tags below it, in tree order.
 */
export function moveTargets(tree: Tag[], id: number): MoveTarget[] {
    const tag = findTag(tree, id)
    if (!tag) return []
    const own = subtreeIds(tag)
    const out: MoveTarget[] = tag.parentId === 0 ? [] : [{ id: 0, label: 'Top level' }]
    for (const f of flattenTags(tree)) {
        if (!own.has(f.tag.id) && f.tag.id !== tag.parentId)
            out.push({ id: f.tag.id, label: f.path })
    }
    return out
}
