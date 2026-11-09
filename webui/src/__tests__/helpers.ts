import { mount, flushPromises } from '@vue/test-utils'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
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
            { path: '/notes/:id', name: 'note', component: { template: '<div />' } }
        ]
    })
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const wrapper = mount(component, {
        props,
        global: { plugins: [PrimeVue, ToastService, router, [VueQueryPlugin, { queryClient }]] }
    })
    await flushPromises()
    return wrapper
}
