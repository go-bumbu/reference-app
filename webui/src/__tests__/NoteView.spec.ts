import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import ConfirmDialog from 'primevue/confirmdialog'
import NoteView from '@/views/NoteView.vue'
import * as api from '@/lib/api/api'
import { mountWithApp, problem } from './helpers'

vi.mock('@/lib/api/api')

const note = {
    id: 'n1',
    createdBy: 'u1',
    title: 'Plans',
    body: 'three little words',
    createdAt: '2026-01-02T03:04:05Z'
}

// The page with the shell's ConfirmDialog next to it, as App.vue mounts it.
const WithConfirm = defineComponent({
    setup: () => () => [h(NoteView, { id: 'n1' }), h(ConfirmDialog)]
})

describe('NoteView', () => {
    beforeEach(() => {
        vi.clearAllMocks()
        vi.mocked(api.getMe).mockResolvedValue({
            userId: 'u1',
            subject: 'alice',
            tenantId: 't1',
            tenant: 'acme'
        })
    })

    it('shows the note on the sheet and its details beside it', async () => {
        vi.mocked(api.getNote).mockResolvedValue(note)
        const w = await mountWithApp(NoteView, { id: 'n1' })
        expect(api.getNote).toHaveBeenCalledWith('n1')
        expect(w.find('h1').text()).toBe('Plans')
        expect(w.find('[data-test="detail-author"]').text()).toBe('You')
        expect(w.find('[data-test="detail-words"]').text()).toBe('3')
    })

    it('says a missing note does not exist', async () => {
        vi.mocked(api.getNote).mockRejectedValue(problem(404))
        const w = await mountWithApp(NoteView, { id: 'gone' })
        expect(w.find('[data-test="not-found"]').exists()).toBe(true)
        expect(w.find('[data-test="edit"]').exists()).toBe(false)
    })

    it('keeps Edit and Delete at the foot of the details card', async () => {
        vi.mocked(api.getNote).mockResolvedValue(note)
        const w = await mountWithApp(NoteView, { id: 'n1' })
        const details = w.find('[data-test="details"]')
        expect(details.find('[data-test="edit"]').exists()).toBe(true)
        expect(details.find('[data-test="delete"]').exists()).toBe(true)
        expect(details.element.lastElementChild?.getAttribute('data-test')).toBe('actions')
    })

    it('edits the note in the note dialog, not on the sheet', async () => {
        vi.mocked(api.getNote).mockResolvedValue(note)
        vi.mocked(api.updateNote).mockResolvedValue({ ...note, title: 'Better plans' })
        const w = await mountWithApp(NoteView, { id: 'n1' })

        await w.find('[data-test="edit"]').trigger('click')
        await flushPromises()
        expect(w.find('textarea').exists()).toBe(false)

        // PrimeVue's Dialog teleports to <body>
        const title = document.body.querySelector<HTMLInputElement>('[data-test="title"]')!
        expect(title.value).toBe('Plans')
        title.value = 'Better plans'
        title.dispatchEvent(new Event('input'))
        document.body
            .querySelector<HTMLFormElement>('[data-test="note-form"]')!
            .dispatchEvent(new Event('submit'))
        await flushPromises()

        expect(api.updateNote).toHaveBeenCalledWith('n1', {
            title: 'Better plans',
            body: 'three little words'
        })
        expect(w.find('h1').text()).toBe('Better plans')
    })

    it('deletes the note once confirmed and goes back to the list', async () => {
        vi.mocked(api.getNote).mockResolvedValue(note)
        vi.mocked(api.deleteNote).mockResolvedValue()
        const w = await mountWithApp(WithConfirm)
        await w.vm.$router.push('/notes/n1')
        await flushPromises()

        await w.find('[data-test="delete"]').trigger('click')
        await flushPromises()
        expect(api.deleteNote).not.toHaveBeenCalled()

        // ConfirmDialog teleports to <body>
        document.body.querySelector<HTMLButtonElement>('.p-confirmdialog-accept-button')!.click()
        await flushPromises()

        expect(api.deleteNote).toHaveBeenCalledWith('n1')
        expect(w.vm.$route.name).toBe('notes')
    })
})
