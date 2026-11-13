import { describe, expect, it } from 'vitest'
import { areaLabel, defaultExpiry, keyState, scopeLabel, scopesFrom } from '@/lib/apiKeys'
import type { ApiKey } from '@/lib/api/types'

const key = (over: Partial<ApiKey> = {}): ApiKey => ({
    id: 'k1',
    name: 'ci',
    scopes: ['notes:read'],
    usable: true,
    createdAt: '2026-01-01T00:00:00Z',
    lastUsedAt: null,
    expiresAt: null,
    ...over
})

describe('apiKeys', () => {
    it('defaults the expiry to a year', () => {
        const now = new Date('2026-03-01T10:00:00Z')
        expect(defaultExpiry(now).getTime() - now.getTime()).toBe(365 * 24 * 3600 * 1000)
    })

    it('turns the area choices into sorted scopes, leaving out none', () => {
        expect(scopesFrom({ notes: 'write', jobs: 'read', tags: 'none' })).toEqual([
            'jobs:read',
            'notes:write'
        ])
        expect(scopesFrom({ notes: 'none' })).toEqual([])
    })

    it('labels areas and scopes for people', () => {
        expect(areaLabel('notes')).toBe('Notes')
        expect(areaLabel('job-runs')).toBe('Job runs')
        expect(scopeLabel('notes:write')).toBe('Notes: read & write')
        expect(scopeLabel('notes:read')).toBe('Notes: read')
    })

    it('tells active, expired and unusable keys apart', () => {
        const now = new Date('2026-06-01T00:00:00Z')
        expect(keyState(key(), now)).toBe('active')
        expect(keyState(key({ expiresAt: '2027-01-01T00:00:00Z' }), now)).toBe('active')
        expect(keyState(key({ expiresAt: '2026-01-01T00:00:00Z' }), now)).toBe('expired')
        expect(keyState(key({ usable: false, expiresAt: '2026-01-01T00:00:00Z' }), now)).toBe(
            'unusable'
        )
    })
})
