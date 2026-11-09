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
            path: '/:pathMatch(.*)*',
            name: 'not-found',
            component: () => import('@/views/NotFoundView.vue')
        }
    ]
})

export default router
