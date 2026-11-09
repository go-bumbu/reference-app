import { describe, expect, it, vi } from 'vitest'
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
        expect(w.find('[data-test="identity"]').text()).toContain('alice')
        expect(w.find('[data-test="identity"]').text()).toContain('acme')
        expect(w.text()).toContain('server v1.0.0')
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
