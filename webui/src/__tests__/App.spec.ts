import { describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import App from '@/App.vue'
import * as api from '@/lib/api/api'
import { mountWithApp, problem } from './helpers'

vi.mock('@/lib/api/api')

describe('App', () => {
    it('shows who is signed in', async () => {
        vi.mocked(api.getMe).mockResolvedValue({
            userId: 'u1',
            subject: 'alice',
            tenantId: 't1',
            tenant: 'acme'
        })
        vi.mocked(api.getInfo).mockResolvedValue({
            version: 'v1.0.0',
            buildTime: '',
            commit: '',
            goVersion: ''
        })
        const w = await mountWithApp(App)
        expect(w.find('[data-test="profile"]').text()).toBe('A')

        // The profile menu is a popup, teleported to the body.
        await w.find('[data-test="profile"]').trigger('click')
        await flushPromises()
        const identity = document.body.querySelector('[data-test="identity"]')
        expect(identity?.textContent).toContain('alice')
        expect(identity?.textContent).toContain('acme')
        expect(w.text()).toContain('server v1.0.0')
    })

    it('shows Back off the notes list, and it falls back to the list', async () => {
        vi.mocked(api.getMe).mockResolvedValue({
            userId: 'u1',
            subject: 'alice',
            tenantId: 't1',
            tenant: 'acme'
        })
        vi.mocked(api.getInfo).mockResolvedValue({
            version: 'v1.0.0',
            buildTime: '',
            commit: '',
            goVersion: ''
        })
        const w = await mountWithApp(App)
        await w.vm.$router.push('/')
        await flushPromises()
        expect(w.find('[data-test="back"]').exists()).toBe(false)

        // Opened directly: no earlier entry to go back to, so Back goes to the list.
        await w.vm.$router.replace('/notes/n1')
        await flushPromises()
        await w.find('[data-test="back"]').trigger('click')
        await flushPromises()
        expect(w.vm.$route.name).toBe('notes')
    })

    it('puts the search into the URL and takes a note page back to the list', async () => {
        vi.mocked(api.getMe).mockResolvedValue({
            userId: 'u1',
            subject: 'alice',
            tenantId: 't1',
            tenant: 'acme'
        })
        vi.mocked(api.getInfo).mockResolvedValue({
            version: 'v1.0.0',
            buildTime: '',
            commit: '',
            goVersion: ''
        })
        const w = await mountWithApp(App)
        await w.vm.$router.push('/notes/n1')
        await flushPromises()

        await w.find('[data-test="search-input"]').setValue('milk')
        await flushPromises()
        expect(w.vm.$route.name).toBe('notes')
        expect(w.vm.$route.query.q).toBe('milk')

        await w.find('[data-test="search-clear"]').trigger('click')
        await flushPromises()
        expect(w.vm.$route.query.q).toBeUndefined()
    })

    it('opens the create dialog from the top bar', async () => {
        vi.mocked(api.getMe).mockResolvedValue({
            userId: 'u1',
            subject: 'alice',
            tenantId: 't1',
            tenant: 'acme'
        })
        vi.mocked(api.getInfo).mockResolvedValue({
            version: 'v1.0.0',
            buildTime: '',
            commit: '',
            goVersion: ''
        })
        const w = await mountWithApp(App)
        expect(document.body.querySelector('[data-test="note-form"]')).toBeNull()

        await w.find('[data-test="add-note"]').trigger('click')
        await flushPromises()
        expect(document.body.querySelector('[data-test="note-form"]')).not.toBeNull()
    })

    it('opens the settings from the profile menu, with no tags rail or search', async () => {
        vi.mocked(api.getMe).mockResolvedValue({
            userId: 'u1',
            subject: 'alice',
            tenantId: 't1',
            tenant: 'acme'
        })
        vi.mocked(api.getInfo).mockResolvedValue({
            version: 'v1.0.0',
            buildTime: '',
            commit: '',
            goVersion: ''
        })
        const w = await mountWithApp(App)
        expect(w.find('aside.sidebar').exists()).toBe(true)
        expect(w.find('[data-test="search-input"]').exists()).toBe(true)

        await w.find('[data-test="profile"]').trigger('click')
        await flushPromises()
        // The last one: earlier tests' popups may still sit in the body.
        const links = document.body.querySelectorAll<HTMLElement>('[data-test="open-settings"]')
        links[links.length - 1]?.click()
        await flushPromises()

        expect(w.vm.$route.name).toBe('settings-appearance')
        expect(w.find('[data-test="search-input"]').exists()).toBe(false)
        expect(w.find('aside.sidebar').exists()).toBe(false)
        expect(w.find('[data-test="sidebar-toggle"]').exists()).toBe(false)
    })

    it('collapses the sidebar to an icon rail', async () => {
        vi.mocked(api.getMe).mockResolvedValue({
            userId: 'u1',
            subject: 'alice',
            tenantId: 't1',
            tenant: 'acme'
        })
        vi.mocked(api.getInfo).mockResolvedValue({
            version: 'v1.0.0',
            buildTime: '',
            commit: '',
            goVersion: ''
        })
        const w = await mountWithApp(App)
        expect(w.findAll('[data-test="sidebar-tag"]').length).toBeGreaterThan(0)
        expect(w.find('aside.sidebar').classes()).not.toContain('collapsed')

        await w.find('[data-test="sidebar-toggle"]').trigger('click')
        expect(w.find('aside.sidebar').classes()).toContain('collapsed')
        await w.find('[data-test="sidebar-toggle"]').trigger('click')
        expect(w.find('aside.sidebar').classes()).not.toContain('collapsed')
    })

    it('explains a 401 instead of rendering the views', async () => {
        vi.mocked(api.getMe).mockRejectedValue(problem(401))
        vi.mocked(api.getInfo).mockResolvedValue({
            version: 'v1.0.0',
            buildTime: '',
            commit: '',
            goVersion: ''
        })
        const w = await mountWithApp(App)
        expect(w.find('[data-test="unauthenticated"]').exists()).toBe(true)
    })
})
