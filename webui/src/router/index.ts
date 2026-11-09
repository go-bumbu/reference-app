import { createRouter, createWebHistory } from 'vue-router'
import NotesView from '@/views/NotesView.vue'

// History mode: the Go side serves index.html for every unknown non-API path
// (go-bumbu/http/spa), so deep links and reloads work.
const router = createRouter({
    history: createWebHistory(),
    routes: [
        { path: '/', name: 'notes', component: NotesView },
        {
            path: '/notes/:id',
            name: 'note',
            component: () => import('@/views/NoteView.vue'),
            props: true
        },
        {
            // The settings chrome is drawn once, around each settings page.
            path: '/settings',
            component: () => import('@/views/settings/SettingsShell.vue'),
            children: [
                { path: '', name: 'settings', redirect: { name: 'settings-appearance' } },
                {
                    path: 'appearance',
                    name: 'settings-appearance',
                    component: () => import('@/views/settings/AppearanceView.vue')
                },
                {
                    path: 'jobs',
                    name: 'settings-jobs',
                    component: () => import('@/views/settings/JobsView.vue')
                },
                {
                    path: 'about',
                    name: 'settings-about',
                    component: () => import('@/views/settings/AboutView.vue')
                }
            ]
        },
        {
            path: '/:pathMatch(.*)*',
            name: 'not-found',
            component: () => import('@/views/NotFoundView.vue')
        }
    ]
})

export default router
