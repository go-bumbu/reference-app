import { mount, flushPromises } from '@vue/test-utils'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import ConfirmationService from 'primevue/confirmationservice'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { Component } from 'vue'

/** An axios-shaped error carrying a problem+json response. */
export function problem(status: number, body: Record<string, unknown> = {}) {
    return Object.assign(new Error(`status ${status}`), {
        response: { status, data: { status, title: 'Problem', ...body } }
    })
}

/** Mounts a component with the app's plugins: PrimeVue, a fresh query client and a router. */
export async function mountWithApp(component: Component, props: Record<string, unknown> = {}) {
    const router = createRouter({
        history: createMemoryHistory(),
        routes: [
            { path: '/', name: 'notes', component: { template: '<div />' } },
            { path: '/notes/:id', name: 'note', component: { template: '<div />' } },
            { path: '/settings', name: 'settings', redirect: '/settings/appearance' },
            {
                path: '/settings/appearance',
                name: 'settings-appearance',
                component: { template: '<div />' }
            },
            { path: '/settings/jobs', name: 'settings-jobs', component: { template: '<div />' } },
            {
                path: '/settings/api-keys',
                name: 'settings-api-keys',
                component: { template: '<div />' }
            },
            { path: '/settings/about', name: 'settings-about', component: { template: '<div />' } }
        ]
    })
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const wrapper = mount(component, {
        props,
        global: {
            plugins: [
                PrimeVue,
                ToastService,
                ConfirmationService,
                router,
                [VueQueryPlugin, { queryClient }]
            ]
        }
    })
    await flushPromises()
    return wrapper
}

/** A fake DataTransfer: jsdom has none. `data` holds what the drag carries, by type. */
export function fakeDataTransfer(data: Record<string, string> = {}) {
    return {
        data,
        get types() {
            return Object.keys(this.data)
        },
        dropEffect: 'none',
        effectAllowed: 'all',
        setData(type: string, value: string) {
            this.data[type] = value
        },
        getData(type: string) {
            return this.data[type] ?? ''
        }
    }
}

/**
 * Dispatches a drag event on el: jsdom has no DragEvent, so it is a plain
 * cancelable Event carrying a dataTransfer and the pointer's clientY.
 * Returns the event, to check whether a handler took the drop (defaultPrevented).
 */
export function dispatchDrag(
    el: Element,
    type: string,
    dataTransfer: ReturnType<typeof fakeDataTransfer>,
    clientY = 0
) {
    const event = new Event(type, { bubbles: true, cancelable: true })
    Object.defineProperty(event, 'dataTransfer', { value: dataTransfer })
    Object.defineProperty(event, 'clientY', { value: clientY })
    el.dispatchEvent(event)
    return event
}
