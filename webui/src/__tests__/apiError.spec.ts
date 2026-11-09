import { describe, expect, it } from 'vitest'
import { apiErrorMessage, apiErrorStatus, fieldErrors, NETWORK_MESSAGE } from '@/lib/apiError'
import { problem } from './helpers'

describe('apiError', () => {
    it('reads the detail, then the title', () => {
        expect(apiErrorMessage(problem(404, { detail: ' note x not found ' }))).toBe(
            'note x not found'
        )
        expect(apiErrorMessage(problem(500, { title: 'Internal' }))).toBe('Internal')
    })

    it('reports a request without a response as a network error', () => {
        expect(apiErrorMessage({ request: {}, message: 'Network Error' })).toBe(NETWORK_MESSAGE)
    })

    it('falls back to a generic message', () => {
        expect(apiErrorMessage(new Error('boom'))).toMatch(/went wrong/)
        expect(apiErrorMessage(undefined)).toMatch(/went wrong/)
    })

    it('exposes the status', () => {
        expect(apiErrorStatus(problem(401))).toBe(401)
        expect(apiErrorStatus(new Error('x'))).toBeUndefined()
    })

    it('keys field errors by field name', () => {
        const err = problem(422, { errors: [{ pointer: '/title', detail: 'must not be empty' }] })
        expect(fieldErrors(err)).toEqual({ title: 'must not be empty' })
        expect(fieldErrors(new Error('x'))).toEqual({})
    })
})
