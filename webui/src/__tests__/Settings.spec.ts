import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import Toast from 'primevue/toast'
import AppearanceView from '@/views/settings/AppearanceView.vue'
import JobsView from '@/views/settings/JobsView.vue'
import AboutView from '@/views/settings/AboutView.vue'
import SettingsSidebar from '@/components/settings/SettingsSidebar.vue'
import { useTheme } from '@/composables/useTheme'
import * as api from '@/lib/api/api'
import { mountWithApp } from './helpers'

vi.mock('@/lib/api/api')

describe('Settings', () => {
    beforeEach(() => {
        localStorage.clear()
        useTheme().setMode('auto')
        useTheme().setTheme('teal')
    })

    it('picks a theme and keeps it on the device', async () => {
        const w = await mountWithApp(AppearanceView)
        expect(w.find('[data-test="theme-teal"]').attributes('aria-checked')).toBe('true')

        await w.find('[data-test="theme-amber"]').trigger('click')

        expect(w.find('[data-test="theme-amber"]').attributes('aria-checked')).toBe('true')
        expect(w.find('[data-test="theme-teal"]').attributes('aria-checked')).toBe('false')
        expect(localStorage.getItem('refapp-theme')).toBe('amber')
    })

    it('sets dark mode and keeps it on the device', async () => {
        const w = await mountWithApp(AppearanceView)

        await w.find('[data-test="mode-dark"]').trigger('click')
        expect(document.documentElement.classList.contains('dark-mode')).toBe(true)
        expect(localStorage.getItem('refapp-mode')).toBe('dark')

        await w.find('[data-test="mode-light"]').trigger('click')
        expect(document.documentElement.classList.contains('dark-mode')).toBe(false)
        expect(w.find('[data-test="mode-light"]').attributes('aria-checked')).toBe('true')
    })

    it('says a placeholder job does not run yet', async () => {
        const WithToast = defineComponent({ setup: () => () => [h(JobsView), h(Toast)] })
        const w = await mountWithApp(WithToast)
        expect(w.findAll('[data-test^="job-"]').length).toBeGreaterThan(0)

        await w.find('[data-test="run-export-notes"]').trigger('click')
        await flushPromises()

        expect(document.body.textContent).toContain('jobs are not implemented yet')
    })

    it('shows who is signed in and the server version', async () => {
        vi.mocked(api.getMe).mockResolvedValue({
            userId: 'u1',
            subject: 'alice',
            tenantId: 't1',
            tenant: 'acme'
        })
        vi.mocked(api.getInfo).mockResolvedValue({
            version: 'v1.0.0',
            buildTime: '',
            commit: 'abc123',
            goVersion: ''
        })
        const w = await mountWithApp(AboutView)
        expect(w.find('[data-test="about-user"]').text()).toBe('alice')
        expect(w.find('[data-test="about-version"]').text()).toBe('v1.0.0')
    })

    it('marks the open page in the sidebar', async () => {
        const w = await mountWithApp(SettingsSidebar)
        await w.vm.$router.push('/settings/jobs')
        await flushPromises()
        expect(w.find('[data-test="nav-settings-jobs"]').classes()).toContain('active')
        expect(w.find('[data-test="nav-settings-appearance"]').classes()).not.toContain('active')
    })
})
