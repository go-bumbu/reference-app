import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import Toast from 'primevue/toast'
import AppNav from '@/components/AppNav.vue'
import * as api from '@/lib/api/api'
import type { Tag } from '@/lib/api/types'
import { NOTE_DRAG_TYPE } from '@/lib/noteDrag'
import { dispatchDrag, fakeDataTransfer, mountWithApp, problem } from './helpers'

vi.mock('@/lib/api/api')

const tag = (id: number, parentId: number, name: string, children: Tag[] = []): Tag => ({
    id,
    parentId,
    name,
    children
})
// work(reports), home
const tree = [tag(1, 0, 'work', [tag(2, 1, 'reports')]), tag(3, 0, 'home')]

// The nav with a Toast beside it, to read what a drop says.
const WithToast = defineComponent({
    setup: () => () => h('div', [h(AppNav), h(Toast)])
})

const rows = (w: Awaited<ReturnType<typeof mountWithApp>>) =>
    w.findAll('[data-test="sidebar-tag"]').map((r) => r.text())

describe('AppNav', () => {
    beforeEach(() => {
        vi.clearAllMocks()
        localStorage.clear()
        vi.mocked(api.listTags).mockResolvedValue(tree)
    })

    it('shows the tree, a tag opening the tags below it', async () => {
        const w = await mountWithApp(AppNav)
        expect(rows(w)).toEqual(['work', 'home'])

        await w.find('[data-test="tag-caret"]').trigger('click')
        expect(rows(w)).toEqual(['work', 'reports', 'home'])
    })

    it('links each tag to the notes under it, keeping the search', async () => {
        const w = await mountWithApp(AppNav)
        await w.vm.$router.push('/?q=milk')
        await flushPromises()
        const link = w.find('[data-tag-id="3"] a')
        expect(link.attributes('href')).toBe('/?q=milk&tag=3')

        await w.vm.$router.push('/?tag=2')
        await flushPromises()
        // the active tag shows, its ancestors open
        expect(w.find('[data-tag-id="2"]').classes()).toContain('active')
        // only the active tag is the current page
        expect(w.findAll('[aria-current="page"]').map((a) => a.text())).toEqual(['reports'])

        // the caret of an ancestor of the active tag still closes it
        await w.find('[data-tag-id="1"] [data-test="tag-caret"]').trigger('click')
        expect(rows(w)).toEqual(['work', 'home'])
        expect(
            w.find('[data-tag-id="1"] [data-test="tag-caret"]').attributes('aria-expanded')
        ).toBe('false')
    })

    it('releases the filter on a second click of the active tag', async () => {
        const w = await mountWithApp(AppNav)
        await w.vm.$router.push('/?q=milk&tag=3')
        await flushPromises()
        expect(w.find('[data-tag-id="3"] a').attributes('href')).toBe('/?q=milk')
        expect(w.find('[data-tag-id="1"] a').attributes('href')).toBe('/?q=milk&tag=1')

        await w.find('[data-tag-id="3"] a').trigger('click')
        await flushPromises()
        expect(w.vm.$route.query).toEqual({ q: 'milk' })
        expect(w.find('[data-tag-id="3"]').classes()).not.toContain('active')
    })

    it('offers a clear button in the heading while a tag is active', async () => {
        const w = await mountWithApp(AppNav)
        expect(w.find('[data-test="clear-tag-filter"]').exists()).toBe(false)

        await w.vm.$router.push('/?q=milk&tag=3')
        await flushPromises()
        const clear = w.find('[data-test="clear-tag-filter"]')
        expect(clear.attributes('href')).toBe('/?q=milk')

        await clear.trigger('click')
        await flushPromises()
        expect(w.vm.$route.query).toEqual({ q: 'milk' })
        expect(w.find('[data-test="clear-tag-filter"]').exists()).toBe(false)
    })

    it('offers the clear button on the collapsed rail while a tag is active', async () => {
        const w = await mountWithApp(AppNav, { collapsed: true })
        expect(w.find('[data-test="rail-clear-tag-filter"]').exists()).toBe(false)
        await w.vm.$router.push('/?tag=1')
        await flushPromises()
        expect(w.find('[data-test="rail-clear-tag-filter"]').attributes('href')).toBe('/')
        expect(w.findAll('[data-test="rail-tag"]')[0].attributes('href')).toBe('/')
    })

    it('remembers which tags are open', async () => {
        const first = await mountWithApp(AppNav)
        await first.find('[data-test="tag-caret"]').trigger('click')
        expect(JSON.parse(localStorage.getItem('refapp-tags-expanded') ?? '[]')).toEqual([1])
        first.unmount()

        const second = await mountWithApp(AppNav)
        expect(rows(second)).toEqual(['work', 'reports', 'home'])
    })

    it('tags a note dropped on a tag', async () => {
        vi.mocked(api.addNoteTag).mockResolvedValue({
            id: 'n1',
            createdBy: 'u1',
            title: 'Plan',
            body: '',
            createdAt: '2026-01-02T03:04:05Z',
            tagIds: [3]
        })
        const w = await mountWithApp(WithToast)
        const row = w.find('[data-tag-id="3"]').element
        const drag = fakeDataTransfer({
            [NOTE_DRAG_TYPE]: JSON.stringify({ id: 'n1', title: 'Plan', tagIds: [] })
        })

        expect(dispatchDrag(row, 'dragover', drag).defaultPrevented).toBe(true)
        await flushPromises()
        expect(w.find('[data-tag-id="3"]').classes()).toContain('drop-target')

        dispatchDrag(row, 'drop', drag)
        await flushPromises()
        expect(api.addNoteTag).toHaveBeenCalledWith('n1', 3)
        expect(document.body.textContent).toContain('Tagged “Plan” with home')
        expect(w.find('[data-tag-id="3"]').classes()).not.toContain('drop-target')
    })

    it('says so when the dropped note has the tag already', async () => {
        const w = await mountWithApp(WithToast)
        const row = w.find('[data-tag-id="3"]').element
        const drag = fakeDataTransfer({
            [NOTE_DRAG_TYPE]: JSON.stringify({ id: 'n1', title: 'Plan', tagIds: [3] })
        })
        dispatchDrag(row, 'drop', drag)
        await flushPromises()
        expect(api.addNoteTag).not.toHaveBeenCalled()
        expect(document.body.textContent).toContain('“Plan” is tagged home already')
    })

    it('says what went wrong when a drop fails', async () => {
        vi.mocked(api.addNoteTag).mockRejectedValue(problem(404, { detail: 'note n1 not found' }))
        const w = await mountWithApp(WithToast)
        const drag = fakeDataTransfer({
            [NOTE_DRAG_TYPE]: JSON.stringify({ id: 'n1', title: 'Gone', tagIds: [] })
        })
        dispatchDrag(w.find('[data-tag-id="1"]').element, 'drop', drag)
        await flushPromises()
        expect(document.body.textContent).toContain('note n1 not found')
    })

    it('takes a dropped note on a tag of the collapsed rail', async () => {
        vi.mocked(api.addNoteTag).mockResolvedValue({
            id: 'n1',
            createdBy: 'u1',
            title: 'Plan',
            body: '',
            createdAt: '2026-01-02T03:04:05Z',
            tagIds: [1]
        })
        const w = await mountWithApp(AppNav, { collapsed: true })
        const drag = fakeDataTransfer({
            [NOTE_DRAG_TYPE]: JSON.stringify({ id: 'n1', title: 'Plan', tagIds: [] })
        })
        const rail = w.findAll('[data-test="rail-tag"]')[0].element
        expect(dispatchDrag(rail, 'dragover', drag).defaultPrevented).toBe(true)
        dispatchDrag(rail, 'drop', drag)
        await flushPromises()
        expect(api.addNoteTag).toHaveBeenCalledWith('n1', 1)
    })

    it('ignores a drag that carries no note', async () => {
        const w = await mountWithApp(AppNav)
        const row = w.find('[data-tag-id="3"]').element
        const drag = fakeDataTransfer({ 'text/uri-list': 'https://example.com' })

        expect(dispatchDrag(row, 'dragover', drag).defaultPrevented).toBe(false)
        dispatchDrag(row, 'drop', drag)
        await flushPromises()
        expect(api.addNoteTag).not.toHaveBeenCalled()
    })

    it('opens the tag editor from the pencil', async () => {
        const w = await mountWithApp(AppNav)
        expect(document.body.querySelector('[data-test="tag-filter"]')).toBeNull()
        await w.find('[data-test="edit-tags"]').trigger('click')
        await flushPromises()
        expect(document.body.querySelector('[data-test="tag-filter"]')).not.toBeNull()
    })

    it('says when there are no tags yet', async () => {
        vi.mocked(api.listTags).mockResolvedValue([])
        const w = await mountWithApp(AppNav)
        expect(w.find('[data-test="tags-empty"]').exists()).toBe(true)
    })

    it('keeps the tags at the root as icons when collapsed', async () => {
        const w = await mountWithApp(AppNav, { collapsed: true })
        const rail = w.findAll('[data-test="rail-tag"]')
        expect(rail.map((r) => r.attributes('aria-label'))).toEqual(['work', 'home'])
    })
})
