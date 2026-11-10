import { describe, expect, it } from 'vitest'
import type { Tag } from '@/lib/api/types'
import {
    ancestorIds,
    descendantCount,
    dropPosition,
    filterTags,
    flattenTags,
    lastChildId,
    moveTargets,
    resolveTagDrop,
    tagIdFromQuery,
    tagPath
} from '@/lib/tagTree'

const tag = (id: number, parentId: number, name: string, children: Tag[] = []): Tag => ({
    id,
    parentId,
    name,
    children
})

// work(reports(q1, q2), meetings), home
const tree: Tag[] = [
    tag(1, 0, 'work', [
        tag(2, 1, 'reports', [tag(3, 2, 'q1'), tag(4, 2, 'q2')]),
        tag(5, 1, 'meetings')
    ]),
    tag(6, 0, 'home')
]

describe('tagTree', () => {
    it('flattens in tree order with depth and path', () => {
        expect(flattenTags(tree).map((f) => [f.tag.id, f.depth, f.path])).toEqual([
            [1, 0, 'work'],
            [2, 1, 'work / reports'],
            [3, 2, 'work / reports / q1'],
            [4, 2, 'work / reports / q2'],
            [5, 1, 'work / meetings'],
            [6, 0, 'home']
        ])
    })

    it('filters by name, ignoring case', () => {
        expect(filterTags(tree, ' Q').map((f) => f.tag.id)).toEqual([3, 4])
        expect(filterTags(tree, '').length).toBe(6)
        expect(filterTags(tree, 'nothing')).toEqual([])
    })

    it('finds paths, ancestors, descendants and last children', () => {
        expect(tagPath(tree, 4)).toBe('work / reports / q2')
        expect(tagPath(tree, 99)).toBe('')
        expect(ancestorIds(tree, 3)).toEqual([1, 2])
        expect(ancestorIds(tree, 6)).toEqual([])
        expect(descendantCount(tree[0])).toBe(4)
        expect(lastChildId(tree, 0)).toBe(6)
        expect(lastChildId(tree, 2)).toBe(4)
        expect(lastChildId(tree, 6)).toBe(0)
    })

    it('reads the tag of the URL', () => {
        expect(tagIdFromQuery('12')).toBe(12)
        for (const bad of [undefined, '', '0', '-1', '1.5', 'abc', ['1'], '99999999999999999999']) {
            expect(tagIdFromQuery(bad)).toBeNull()
        }
    })

    it('classifies a drop by the pointer height', () => {
        const row = { top: 100, height: 40 }
        expect(dropPosition(row, 105)).toBe('before')
        expect(dropPosition(row, 120)).toBe('onto')
        expect(dropPosition(row, 135)).toBe('after')
    })

    it('nests a tag onto another, last among the children', () => {
        expect(resolveTagDrop(tree, 6, 2, 'onto')).toEqual({ parentId: 2 })
        expect(resolveTagDrop(tree, 3, 1, 'onto')).toEqual({ parentId: 1 })
    })

    it('places a tag before or after another: a reorder among siblings is no move', () => {
        expect(resolveTagDrop(tree, 6, 1, 'before')).toEqual({ afterId: 0 })
        expect(resolveTagDrop(tree, 5, 2, 'before')).toEqual({ afterId: 0 })
        expect(resolveTagDrop(tree, 3, 4, 'after')).toEqual({ afterId: 4 })
    })

    it('places a tag beside another of another parent, which moves it', () => {
        expect(resolveTagDrop(tree, 6, 3, 'after')).toEqual({ parentId: 2, afterId: 3 })
        expect(resolveTagDrop(tree, 3, 1, 'after')).toEqual({ parentId: 0, afterId: 1 })
    })

    it('lists where a tag can move without a drag', () => {
        // reports: anywhere but its parent and its own subtree
        expect(moveTargets(tree, 2).map((t) => [t.id, t.label])).toEqual([
            [0, 'Top level'],
            [5, 'work / meetings'],
            [6, 'home']
        ])
        // a tag at the top level cannot move to the top level
        expect(moveTargets(tree, 6).map((t) => t.id)).toEqual([1, 2, 3, 4, 5])
        expect(moveTargets(tree, 99)).toEqual([])
    })

    it('refuses drops that make a cycle or change nothing', () => {
        expect(resolveTagDrop(tree, 1, 1, 'onto')).toBeNull()
        expect(resolveTagDrop(tree, 1, 3, 'onto')).toBeNull() // into its own subtree
        expect(resolveTagDrop(tree, 1, 2, 'before')).toBeNull()
        expect(resolveTagDrop(tree, 3, 2, 'onto')).toBeNull() // already its parent
        expect(resolveTagDrop(tree, 4, 3, 'after')).toBeNull() // already right after
        expect(resolveTagDrop(tree, 3, 4, 'before')).toBeNull()
        expect(resolveTagDrop(tree, 99, 1, 'onto')).toBeNull()
    })
})
