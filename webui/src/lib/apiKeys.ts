// What the API keys page works out from the keys, apart from the components so
// it is tested on its own.
import type { ApiKey, ApiKeyLevel } from '@/lib/api/types'

/** A key's level in one area, as the create form holds it; 'none' grants nothing. */
export type AreaChoice = ApiKeyLevel | 'none'

/** How long a key lives unless its creator picks another date: the server's default. */
export const DEFAULT_KEY_DAYS = 365

/** The expiry the create form starts with: DEFAULT_KEY_DAYS from now. */
export function defaultExpiry(now: Date = new Date()): Date {
    const d = new Date(now)
    d.setDate(d.getDate() + DEFAULT_KEY_DAYS)
    return d
}

/** The scopes of the create form's choices, sorted, without the areas left at 'none'. */
export function scopesFrom(choices: Record<string, AreaChoice>): string[] {
    return Object.entries(choices)
        .filter(([, level]) => level !== 'none')
        .map(([area, level]) => `${area}:${level}`)
        .sort()
}

/** An area's name for people: "notes" → "Notes", "job-runs" → "Job runs". */
export function areaLabel(area: string): string {
    const words = area.replace(/-/g, ' ')
    return words.charAt(0).toUpperCase() + words.slice(1)
}

/** A scope for people: "notes:write" → "Notes: read & write". */
export function scopeLabel(scope: string): string {
    const [area, level] = scope.split(':')
    const what = level === 'write' ? 'read & write' : level === 'read' ? 'read' : level
    return `${areaLabel(area ?? '')}: ${what}`
}

export type KeyState = 'active' | 'expired' | 'unusable'

/** Whether a key still opens anything: unusable (scopes the server no longer knows), expired, or active. */
export function keyState(key: ApiKey, now: Date = new Date()): KeyState {
    if (!key.usable) return 'unusable'
    if (key.expiresAt && new Date(key.expiresAt).getTime() <= now.getTime()) return 'expired'
    return 'active'
}
