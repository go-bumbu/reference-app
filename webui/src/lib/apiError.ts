// Turning a thrown value into text a person can read.
//
// Every /api/v0 failure answers an RFC 9457 problem+json body
// ({type, title, status, detail, instance, errors?}). This module is the one
// place the UI reads that shape, so no view re-derives
// `err?.response?.data?.detail ?? err.message`.

const GENERIC_MESSAGE = 'Something went wrong. Please try again.'
export const NETWORK_MESSAGE = 'The server could not be reached. Check your connection.'

/** One failing field of a 422 problem: a JSON Pointer (e.g. "/title") and its message. */
export interface FieldError {
    pointer: string
    detail: string
}

export interface Problem {
    type?: string
    title?: string
    status?: number
    detail?: string
    errors?: FieldError[]
}

interface AxiosLike {
    response?: { status?: number; data?: unknown }
    request?: unknown
    message?: string
}

function asAxios(err: unknown): AxiosLike | undefined {
    return typeof err === 'object' && err !== null ? (err as AxiosLike) : undefined
}

function problemOf(err: unknown): Problem | undefined {
    const data = asAxios(err)?.response?.data
    return typeof data === 'object' && data !== null ? (data as Problem) : undefined
}

/** The HTTP status of a failed request, if there was a response at all. */
export function apiErrorStatus(err: unknown): number | undefined {
    return asAxios(err)?.response?.status
}

/** A sentence for the user: the problem's detail, else its title, else a generic one. */
export function apiErrorMessage(err: unknown): string {
    const ax = asAxios(err)
    if (ax && !ax.response && ax.request) return NETWORK_MESSAGE
    const p = problemOf(err)
    if (p?.detail?.trim()) return p.detail.trim()
    if (p?.title?.trim()) return p.title.trim()
    return GENERIC_MESSAGE
}

/** The messages of a 422 problem keyed by field name ("/title" -> "title"). */
export function fieldErrors(err: unknown): Record<string, string> {
    const out: Record<string, string> = {}
    for (const fe of problemOf(err)?.errors ?? []) {
        out[fe.pointer.replace(/^\//, '')] = fe.detail
    }
    return out
}
