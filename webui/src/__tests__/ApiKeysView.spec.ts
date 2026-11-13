import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import ConfirmDialog from 'primevue/confirmdialog'
import Toast from 'primevue/toast'
import ApiKeysView from '@/views/settings/ApiKeysView.vue'
import SettingsSidebar from '@/components/settings/SettingsSidebar.vue'
import type { ApiKey } from '@/lib/api/types'
import * as api from '@/lib/api/api'
import { mountWithApp, problem } from './helpers'

vi.mock('@/lib/api/api')

// The page with the shell's ConfirmDialog and Toast beside it, as App.vue mounts them.
const Page = defineComponent({
    setup: () => () => [h(ApiKeysView), h(ConfirmDialog), h(Toast)]
})

const reader: ApiKey = {
    id: 'k1',
    name: 'backup script',
    scopes: ['notes:read'],
    usable: true,
    createdAt: '2026-11-01T10:00:00Z',
    lastUsedAt: null,
    expiresAt: '2099-11-01T10:00:00Z'
}
const stale: ApiKey = {
    id: 'k2',
    name: 'old tool',
    scopes: [],
    usable: false,
    createdAt: '2026-01-01T10:00:00Z',
    lastUsedAt: '2026-02-01T10:00:00Z',
    expiresAt: null
}

function body<T extends HTMLElement = HTMLElement>(selector: string): T {
    const el = document.body.querySelector<T>(selector)
    if (!el) throw new Error(`no element ${selector}`)
    return el
}

function type(selector: string, value: string) {
    const input = body<HTMLInputElement>(selector)
    input.value = value
    input.dispatchEvent(new Event('input'))
}

// Picks an area's level in the create form: the SelectButton's option with the label.
function pickLevel(area: string, label: string) {
    const options = Array.from(
        body(`[data-test="apikey-area-${area}"]`).querySelectorAll<HTMLElement>('button')
    )
    const option = options.find((b) => b.textContent?.trim() === label)
    if (!option) throw new Error(`no option ${label} for ${area}`)
    option.click()
}

describe('Settings → API keys', () => {
    beforeEach(() => {
        vi.resetAllMocks()
        document.body.innerHTML = ''
        vi.mocked(api.listApiKeyScopes).mockResolvedValue([
            { area: 'notes', levels: ['read', 'write'] }
        ])
        vi.mocked(api.listApiKeys).mockResolvedValue([reader, stale])
    })

    it('lists the keys with what they grant', async () => {
        const w = await mountWithApp(Page)
        const text = w.find('[data-test="apikeys-table"]').text()
        expect(w.find('[data-test="apikey-k1"]').text()).toBe('backup script')
        expect(text).toContain('Notes: read')
        expect(text).toContain('Unusable')
        expect(w.find('[data-test="apikeys-off"]').exists()).toBe(false)
    })

    it('says so when the server offers no keys', async () => {
        vi.mocked(api.listApiKeyScopes).mockRejectedValue(problem(404))
        const w = await mountWithApp(Page)
        expect(w.find('[data-test="apikeys-off"]').exists()).toBe(true)
        expect(w.find('[data-test="apikey-new"]').exists()).toBe(false)
        expect(api.listApiKeys).not.toHaveBeenCalled()
    })

    it('creates a key and shows its token once', async () => {
        vi.mocked(api.createApiKey).mockResolvedValue({
            ...reader,
            id: 'k3',
            name: 'ci',
            scopes: ['notes:write'],
            token: 'refapp_k3_secret'
        })
        const w = await mountWithApp(Page)

        await w.find('[data-test="apikey-new"]').trigger('click')
        await flushPromises()
        type('[data-test="apikey-name"]', ' ci ')
        pickLevel('notes', 'Read & write')
        await flushPromises()
        body('[data-test="apikey-submit"]').click()
        await flushPromises()

        expect(api.createApiKey).toHaveBeenCalledTimes(1)
        const input = vi.mocked(api.createApiKey).mock.calls[0][0]
        expect(input.name).toBe('ci')
        expect(input.scopes).toEqual(['notes:write'])
        const days = (new Date(input.expiresAt!).getTime() - Date.now()) / 86_400_000
        expect(days).toBeGreaterThan(364)
        expect(days).toBeLessThan(366)

        expect(body<HTMLInputElement>('[data-test="apikey-token"]').value).toBe('refapp_k3_secret')
        // the list is refetched with the new key
        expect(api.listApiKeys).toHaveBeenCalledTimes(2)
    })

    it('asks for a name and an area before sending', async () => {
        const w = await mountWithApp(Page)
        await w.find('[data-test="apikey-new"]').trigger('click')
        await flushPromises()
        body('[data-test="apikey-submit"]').click()
        await flushPromises()

        expect(api.createApiKey).not.toHaveBeenCalled()
        expect(document.body.textContent).toContain('A name is required')
        expect(document.body.textContent).toContain('Grant at least one area')
    })

    it('shows the server’s field errors', async () => {
        vi.mocked(api.createApiKey).mockRejectedValue(
            problem(422, { errors: [{ pointer: '/scopes', detail: 'unknown scope "notes:read"' }] })
        )
        const w = await mountWithApp(Page)
        await w.find('[data-test="apikey-new"]').trigger('click')
        await flushPromises()
        type('[data-test="apikey-name"]', 'ci')
        pickLevel('notes', 'Read')
        await flushPromises()
        body('[data-test="apikey-submit"]').click()
        await flushPromises()

        expect(document.body.textContent).toContain('unknown scope "notes:read"')
        expect(document.body.querySelector('[data-test="apikey-token"]')).toBeNull()
    })

    it('revokes a key after confirmation', async () => {
        vi.mocked(api.revokeApiKey).mockResolvedValue()
        const w = await mountWithApp(Page)

        await w.find('[data-test="apikey-revoke-k1"]').trigger('click')
        await flushPromises()
        expect(api.revokeApiKey).not.toHaveBeenCalled()

        body('.p-confirmdialog-accept-button').click()
        await flushPromises()

        expect(api.revokeApiKey).toHaveBeenCalledWith('k1')
        expect(document.body.textContent).toContain('API key revoked')
    })
})

describe('Settings sidebar → API keys', () => {
    beforeEach(() => vi.resetAllMocks())

    it('lists the page when the server offers keys', async () => {
        vi.mocked(api.listApiKeyScopes).mockResolvedValue([
            { area: 'notes', levels: ['read', 'write'] }
        ])
        const w = await mountWithApp(SettingsSidebar)
        expect(w.find('[data-test="nav-settings-api-keys"]').exists()).toBe(true)
    })

    it('hides the page when the server offers none', async () => {
        vi.mocked(api.listApiKeyScopes).mockRejectedValue(problem(404))
        const w = await mountWithApp(SettingsSidebar)
        expect(w.find('[data-test="nav-settings-api-keys"]').exists()).toBe(false)
        expect(w.find('[data-test="nav-settings-jobs"]').exists()).toBe(true)
    })
})
