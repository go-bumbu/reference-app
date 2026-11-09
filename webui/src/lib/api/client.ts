import axios from 'axios'

// Same origin in production (the SPA is embedded in the Go binary); in
// development the Vite dev server proxies /api to the backend.
export const apiClient = axios.create({
    baseURL: '/api/v0',
    headers: { 'Content-Type': 'application/json' }
})
