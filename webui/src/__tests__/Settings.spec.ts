import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import Toast from 'primevue/toast'
import AppearanceView from '@/views/settings/AppearanceView.vue'
import JobsView from '@/views/settings/JobsView.vue'
import type { Job, JobRun } from '@/lib/api/types'
import AboutView from '@/views/settings/AboutView.vue'
import SettingsSidebar from '@/components/settings/SettingsSidebar.vue'
import { useTheme } from '@/composables/useTheme'
import * as api from '@/lib/api/api'
import { mountWithApp, problem } from './helpers'

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

describe('Settings → Jobs', () => {
    const scanNotes: Job = {
        name: 'scan-notes',
        title: 'Scan notes',
        description: 'Goes over every note of the tenant.',
        schedules: []
    }
    const running: JobRun = {
        id: 'run-1',
        job: 'scan-notes',
        status: 'running',
        queuedAt: '2026-11-10T10:00:00Z',
        startedAt: '2026-11-10T10:00:01Z',
        progress: { done: 1, total: 4, stage: 'scanning notes' }
    }
    const done: JobRun = {
        id: 'run-0',
        job: 'scan-notes',
        status: 'complete',
        queuedAt: '2026-11-10T09:00:00Z',
        startedAt: '2026-11-10T09:00:01Z',
        endedAt: '2026-11-10T09:00:03Z'
    }

    // The page with a Toast beside it, so a toast's text lands in the document.
    const WithToast = defineComponent({ setup: () => () => [h(JobsView), h(Toast)] })

    beforeEach(() => {
        vi.resetAllMocks()
        document.body.innerHTML = ''
        vi.mocked(api.listJobs).mockResolvedValue([scanNotes])
        vi.mocked(api.listJobRuns).mockResolvedValue([])
        vi.mocked(api.getJobRunLog).mockResolvedValue('')
    })

    it('lists the jobs and runs one', async () => {
        vi.mocked(api.runJob).mockResolvedValue({ run: running, reused: false })
        const w = await mountWithApp(WithToast)
        expect(w.find('[data-test="job-scan-notes"]').text()).toContain('Scan notes')
        expect(w.find('[data-test="job-scan-notes"]').text()).toContain('Never run')
        expect(w.find('[data-test="schedule-summary-scan-notes"]').text()).toBe('Not scheduled')

        await w.find('[data-test="run-scan-notes"]').trigger('click')
        await flushPromises()

        expect(api.runJob).toHaveBeenCalledWith('scan-notes')
        expect(document.body.textContent).not.toContain('Already in progress')
    })

    it('says so when the job is already in progress', async () => {
        vi.mocked(api.runJob).mockResolvedValue({ run: running, reused: true })
        const w = await mountWithApp(WithToast)

        await w.find('[data-test="run-scan-notes"]').trigger('click')
        await flushPromises()

        expect(document.body.textContent).toContain('Already in progress')
    })

    it('shows a running job’s progress on its button', async () => {
        vi.mocked(api.listJobRuns).mockResolvedValue([running, done])
        const w = await mountWithApp(WithToast)
        const button = w.find('[data-test="run-scan-notes"]')
        expect(button.text()).toContain('25% complete')
        expect(button.attributes('disabled')).toBeDefined()
        expect(w.find('[data-test="stage-scan-notes"]').text()).toBe('scanning notes')
    })

    it('lists the runs and cancels a running one', async () => {
        vi.mocked(api.listJobRuns).mockResolvedValue([running, done])
        vi.mocked(api.cancelJobRun).mockResolvedValue()
        const w = await mountWithApp(WithToast)

        await w.find('[data-test="tab-runs"]').trigger('click')
        await flushPromises()

        expect(w.find('[data-test="runs-summary"]').text()).toContain('Total: 2')
        expect(w.find('[data-test="run-run-1"]').text()).toBe('Scan notes')
        expect(w.find('[data-test="cancel-run-0"]').exists()).toBe(false)

        await w.find('[data-test="cancel-run-1"]').trigger('click')
        await flushPromises()

        expect(api.cancelJobRun).toHaveBeenCalledWith('run-1')
    })

    it('opens a run’s log', async () => {
        vi.mocked(api.listJobRuns).mockResolvedValue([done])
        vi.mocked(api.getJobRunLog).mockResolvedValue('2026-11-10T09:00:01Z INFO task started\n')
        const w = await mountWithApp(WithToast)
        await w.find('[data-test="tab-runs"]').trigger('click')
        await flushPromises()

        await w.find('[data-test="log-run-0"]').trigger('click')
        await flushPromises()

        expect(api.getJobRunLog).toHaveBeenCalledWith('run-0')
        expect(document.body.querySelector('[data-test="run-log"]')?.textContent).toContain(
            'task started'
        )
    })

    it('adds a schedule from a preset', async () => {
        vi.mocked(api.createJobSchedule).mockResolvedValue({
            id: 's1',
            cron: '0 0 0 * * *',
            enabled: true,
            createdAt: '2026-11-10T10:00:00Z',
            updatedAt: '2026-11-10T10:00:00Z'
        })
        const w = await mountWithApp(WithToast)

        await w.find('[data-test="schedule-scan-notes"]').trigger('click')
        await flushPromises()
        click('[data-test="preset-Daily"]')
        click('[data-test="schedule-save"]')
        await flushPromises()

        expect(api.createJobSchedule).toHaveBeenCalledWith('scan-notes', {
            cron: '0 0 0 * * *',
            enabled: true
        })
        // a successful save refetches the jobs, whose schedules the dialog shows
        expect(api.listJobs).toHaveBeenCalledTimes(2)
    })

    it('shows the server’s message for a refused cron expression', async () => {
        vi.mocked(api.createJobSchedule).mockRejectedValue(
            problem(422, {
                detail: 'invalid cron',
                errors: [{ pointer: '/cron', detail: 'must fire at most once a minute' }]
            })
        )
        const w = await mountWithApp(WithToast)
        await w.find('[data-test="schedule-scan-notes"]').trigger('click')
        await flushPromises()

        const input = document.body.querySelector<HTMLInputElement>('[data-test="schedule-cron"]')!
        input.value = '* * * * * *'
        input.dispatchEvent(new Event('input'))
        await flushPromises()
        click('[data-test="schedule-save"]')
        await flushPromises()

        expect(
            document.body.querySelector('[data-test="schedule-cron-error"]')?.textContent
        ).toContain('must fire at most once a minute')
    })
})

/** Clicks an element the dialogs teleported into the document body. */
function click(selector: string) {
    const el = document.body.querySelector<HTMLElement>(selector)
    if (!el) throw new Error(`no element ${selector}`)
    el.click()
}
