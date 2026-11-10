import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import TagsDialog from '@/components/tags/TagsDialog.vue'
import * as api from '@/lib/api/api'
import type { Note, Tag } from '@/lib/api/types'
import { dispatchDrag, fakeDataTransfer, mountWithApp, problem } from './helpers'

vi.mock('@/lib/api/api')

// PrimeVue's Dialog teleports to <body>, so its content is looked up there.
const $ = <T extends Element = HTMLElement>(sel: string) => document.body.querySelector<T>(sel)
const $$ = (sel: string) => Array.from(document.body.querySelectorAll<HTMLElement>(sel))

async function type(sel: string, value: string) {
    const el = $<HTMLInputElement>(sel)!
    el.value = value
    el.dispatchEvent(new Event('input'))
    await flushPromises()
}

async function click(sel: string) {
    $(sel)!.click()
    await flushPromises()
}

async function submitFormOf(sel: string) {
    $(sel)!.closest('form')!.dispatchEvent(new Event('submit'))
    await flushPromises()
}

const rowOf = (id: number) => $(`[data-test="tag-row"][data-tag-id="${id}"]`)!
const rowNames = () => $$('[data-test="tag-row"]').map((r) => r.textContent?.trim())

const tag = (id: number, parentId: number, name: string, children: Tag[] = []): Tag => ({
    id,
    parentId,
    name,
    children
})
// work(reports), home
const tree = [tag(1, 0, 'work', [tag(2, 1, 'reports')]), tag(3, 0, 'home')]

const note: Note = {
    id: 'n1',
    createdBy: 'u1',
    title: 'Plan',
    body: '',
    createdAt: '2026-01-02T03:04:05Z',
    tagIds: [2]
}

describe('TagsDialog', () => {
    beforeEach(() => {
        vi.clearAllMocks()
        vi.mocked(api.listTags).mockResolvedValue(tree)
    })

    it('lists the whole tree, and the filter narrows it by name', async () => {
        await mountWithApp(TagsDialog, { visible: true })
        expect(rowNames()).toEqual(['work', 'reports', 'home'])

        await type('[data-test="tag-filter"]', 'REP')
        expect(rowNames()).toEqual(['work / reports'])

        await type('[data-test="tag-filter"]', 'garden')
        expect($('[data-test="tags-no-match"]')).not.toBeNull()
    })

    it('adds a tag at the top level, last among the roots', async () => {
        vi.mocked(api.createTag).mockResolvedValue(tag(4, 0, 'ideas'))
        await mountWithApp(TagsDialog, { visible: true })

        await type('[data-test="new-root-tag"]', ' ideas ')
        await submitFormOf('[data-test="new-root-tag"]')

        expect(api.createTag).toHaveBeenCalledWith({ name: 'ideas', parentId: 0, afterId: 3 })
        expect($<HTMLInputElement>('[data-test="new-root-tag"]')!.value).toBe('')
    })

    it('adds a tag below another', async () => {
        vi.mocked(api.createTag).mockResolvedValue(tag(4, 1, 'meetings'))
        await mountWithApp(TagsDialog, { visible: true })

        rowOf(1).querySelector<HTMLElement>('[data-test="tag-add-child"]')!.click()
        await flushPromises()
        await type('[data-test="add-child-input"]', 'meetings')
        await submitFormOf('[data-test="add-child-input"]')

        expect(api.createTag).toHaveBeenCalledWith({ name: 'meetings', parentId: 1, afterId: 2 })
        expect($('[data-test="add-child-input"]')).toBeNull()
    })

    it('makes a tag of the filter text', async () => {
        vi.mocked(api.createTag).mockResolvedValue(tag(4, 0, 'garden'))
        await mountWithApp(TagsDialog, { visible: true })

        await type('[data-test="tag-filter"]', 'garden')
        await click('[data-test="create-from-filter"]')

        expect(api.createTag).toHaveBeenCalledWith({ name: 'garden', parentId: 0, afterId: 3 })
    })

    it('renames a tag in place, and says when a sibling has the name', async () => {
        vi.mocked(api.updateTag).mockRejectedValueOnce(problem(409, { detail: 'taken' }))
        vi.mocked(api.updateTag).mockResolvedValueOnce(tag(3, 0, 'house'))
        await mountWithApp(TagsDialog, { visible: true })

        rowOf(3).querySelector<HTMLElement>('[data-test="tag-rename"]')!.click()
        await flushPromises()
        await type('[data-test="rename-input"]', 'work')
        await submitFormOf('[data-test="rename-input"]')
        expect(api.updateTag).toHaveBeenCalledWith(3, { name: 'work' })
        expect($('[data-test="tags-failure"]')?.textContent).toContain('already has that name')
        // the field stays open to correct the name
        expect($('[data-test="rename-input"]')).not.toBeNull()

        await type('[data-test="rename-input"]', 'house')
        await submitFormOf('[data-test="rename-input"]')
        expect(api.updateTag).toHaveBeenLastCalledWith(3, { name: 'house' })
        expect($('[data-test="rename-input"]')).toBeNull()
        expect($('[data-test="tags-failure"]')).toBeNull()
    })

    it('deletes a tag after a confirmation that counts what goes with it', async () => {
        vi.mocked(api.deleteTag).mockResolvedValue()
        await mountWithApp(TagsDialog, { visible: true })

        rowOf(1).querySelector<HTMLElement>('[data-test="tag-delete"]')!.click()
        await flushPromises()
        expect(rowOf(1).textContent).toContain('Delete “work” and the tag below it?')
        expect(api.deleteTag).not.toHaveBeenCalled()

        await click('[data-test="confirm-delete"]')
        expect(api.deleteTag).toHaveBeenCalledWith(1)
    })

    it('moves a tag by drag: onto a row nests it, an edge places it beside', async () => {
        vi.mocked(api.updateTag).mockResolvedValue(tag(3, 1, 'home'))
        await mountWithApp(TagsDialog, { visible: true })
        const home = rowOf(3)
        const drag = fakeDataTransfer()
        dispatchDrag(home, 'dragstart', drag)
        await flushPromises()

        // jsdom lays nothing out: give the target row a height to aim at
        const work = rowOf(1)
        work.getBoundingClientRect = () => ({ top: 0, height: 40 }) as DOMRect
        expect(dispatchDrag(work, 'dragover', drag, 20).defaultPrevented).toBe(true)
        await flushPromises()
        expect(work.classList).toContain('drop-onto')
        dispatchDrag(work, 'drop', drag, 20)
        await flushPromises()
        expect(api.updateTag).toHaveBeenCalledWith(3, { parentId: 1 })

        dispatchDrag(home, 'dragstart', drag)
        await flushPromises()
        dispatchDrag(work, 'drop', drag, 2)
        await flushPromises()
        // home and work are siblings: a reorder, no move
        expect(api.updateTag).toHaveBeenLastCalledWith(3, { afterId: 0 })
    })

    it('takes its mark off a row the drag leaves', async () => {
        await mountWithApp(TagsDialog, { visible: true })
        const drag = fakeDataTransfer()
        dispatchDrag(rowOf(3), 'dragstart', drag)
        await flushPromises()
        const work = rowOf(1)
        work.getBoundingClientRect = () => ({ top: 0, height: 40 }) as DOMRect
        dispatchDrag(work, 'dragover', drag, 20)
        await flushPromises()
        expect(work.classList).toContain('drop-onto')

        dispatchDrag(work, 'dragleave', drag)
        await flushPromises()
        expect(work.classList).not.toContain('drop-onto')

        // and a cancelled drag (dragend) leaves nothing behind
        dispatchDrag(rowOf(3), 'dragend', drag)
        await flushPromises()
        expect($('[data-test="root-drop"]')).toBeNull()
    })

    it('takes a dragged tag back to the top level at the strip', async () => {
        vi.mocked(api.updateTag).mockResolvedValue(tag(2, 0, 'reports'))
        await mountWithApp(TagsDialog, { visible: true })
        const drag = fakeDataTransfer()
        dispatchDrag(rowOf(2), 'dragstart', drag)
        await flushPromises()

        const strip = $('[data-test="root-drop"]')!
        expect(dispatchDrag(strip, 'dragover', drag).defaultPrevented).toBe(true)
        dispatchDrag(strip, 'drop', drag)
        await flushPromises()
        expect(api.updateTag).toHaveBeenCalledWith(2, { parentId: 0 })
    })

    it('moves a tag without a drag, from its row', async () => {
        vi.mocked(api.updateTag).mockResolvedValue(tag(2, 3, 'reports'))
        await mountWithApp(TagsDialog, { visible: true })

        rowOf(2).querySelector<HTMLElement>('[data-test="tag-move"]')!.click()
        await flushPromises()
        const select = $<HTMLSelectElement>('[data-test="move-select"]')!
        // the top level and home, not work (its parent) nor itself
        expect(Array.from(select.options).map((o) => o.text.trim())).toEqual([
            'Move under…',
            'Top level',
            'home'
        ])
        select.value = '3'
        select.dispatchEvent(new Event('change'))
        await flushPromises()
        await submitFormOf('[data-test="move-select"]')

        expect(api.updateTag).toHaveBeenCalledWith(2, { parentId: 3 })
        expect($('[data-test="move-select"]')).toBeNull()
    })

    it('says when a new tag takes the name of a sibling', async () => {
        vi.mocked(api.createTag).mockRejectedValue(problem(409, { detail: 'taken' }))
        await mountWithApp(TagsDialog, { visible: true })

        await type('[data-test="new-root-tag"]', 'work')
        await submitFormOf('[data-test="new-root-tag"]')

        expect($('[data-test="tags-failure"]')?.textContent).toContain('already has that name')
        // the name stays, to correct it
        expect($<HTMLInputElement>('[data-test="new-root-tag"]')!.value).toBe('work')
    })

    it('refuses to drop a tag into its own subtree', async () => {
        await mountWithApp(TagsDialog, { visible: true })
        const drag = fakeDataTransfer()
        dispatchDrag(rowOf(1), 'dragstart', drag)
        await flushPromises()
        const reports = rowOf(2)
        reports.getBoundingClientRect = () => ({ top: 0, height: 40 }) as DOMRect

        expect(dispatchDrag(reports, 'dragover', drag, 20).defaultPrevented).toBe(false)
        dispatchDrag(reports, 'drop', drag, 20)
        await flushPromises()
        expect(api.updateTag).not.toHaveBeenCalled()
    })

    it('given a note, picks its tags', async () => {
        vi.mocked(api.addNoteTag).mockResolvedValue({ ...note, tagIds: [2, 3] })
        vi.mocked(api.removeNoteTag).mockResolvedValue({ ...note, tagIds: [] })
        await mountWithApp(TagsDialog, { visible: true, note })
        expect(document.body.textContent).toContain('Tags of “Plan”')

        const picks = () => $$('[data-test="tag-pick"]')
        expect(picks().map((p) => p.getAttribute('aria-checked'))).toEqual([
            'false',
            'true',
            'false'
        ])

        rowOf(3).querySelector<HTMLElement>('[data-test="tag-pick"]')!.click()
        await flushPromises()
        expect(api.addNoteTag).toHaveBeenCalledWith('n1', 3)

        rowOf(2).querySelector<HTMLElement>('[data-test="tag-pick"]')!.click()
        await flushPromises()
        expect(api.removeNoteTag).toHaveBeenCalledWith('n1', 2)
    })

    it('given a note, waits for one pick before the next', async () => {
        let resolve: (n: Note) => void = () => {}
        vi.mocked(api.addNoteTag).mockReturnValue(new Promise((r) => (resolve = r)))
        await mountWithApp(TagsDialog, { visible: true, note })

        const pick = () => rowOf(3).querySelector<HTMLButtonElement>('[data-test="tag-pick"]')!
        pick().click()
        await flushPromises()
        expect(pick().disabled).toBe(true)
        expect(pick().getAttribute('aria-label')).toBe('home')

        resolve({ ...note, tagIds: [2, 3] })
        await flushPromises()
        expect(pick().disabled).toBe(false)
    })

    it('given a note, adds a tag made here to the note', async () => {
        vi.mocked(api.createTag).mockResolvedValue(tag(4, 0, 'ideas'))
        vi.mocked(api.addNoteTag).mockResolvedValue({ ...note, tagIds: [2, 4] })
        await mountWithApp(TagsDialog, { visible: true, note })

        await type('[data-test="new-root-tag"]', 'ideas')
        await submitFormOf('[data-test="new-root-tag"]')

        expect(api.addNoteTag).toHaveBeenCalledWith('n1', 4)
    })
})
