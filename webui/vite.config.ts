import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// The dev server proxies /api to the Go backend started with `make run`
// (zarf/config.yaml: localhost:8081).
//
// With the backend in Auth.Method "proxy" the dev server can play the trusted
// proxy: DEV_USER (and DEV_TENANT) are sent as the identity headers, e.g.
//   DEV_USER=alice DEV_TENANT=acme npm run dev
const backend = process.env.BACKEND_URL ?? 'http://localhost:8081'
const identityHeaders: Record<string, string> = {}
if (process.env.DEV_USER) identityHeaders['Remote-User'] = process.env.DEV_USER
if (process.env.DEV_TENANT) identityHeaders['Remote-Tenant'] = process.env.DEV_TENANT

export default defineConfig({
    plugins: [vue()],
    resolve: {
        alias: {
            '@': fileURLToPath(new URL('./src', import.meta.url))
        }
    },
    base: '/',
    server: {
        proxy: {
            '/api': {
                target: backend,
                changeOrigin: true,
                headers: identityHeaders
            }
        }
    }
})
