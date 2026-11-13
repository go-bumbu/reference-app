import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// The dev server proxies /api to the Go backend started with `make run`
// (zarf/config.yaml: localhost:8081), and plays its trusted auth proxy: every
// request is signed in as DEV_USER of DEV_TENANT, by default admin of default,
// the user of the demo data. Another identity:
//   DEV_USER=alice DEV_TENANT=acme npm run dev
// An empty value sends no header (DEV_USER= : signed out). A backend in
// Auth.Method "none" ignores the headers. Keep their names in step with
// zarf/config.yaml.
//
// The API key mount (/api/key) is passed through untouched, as a real proxy
// lets it through without a login: there only the API key counts.
const backend = process.env.BACKEND_URL ?? 'http://localhost:8081'
const devUser = process.env.DEV_USER ?? 'admin'
const devTenant = process.env.DEV_TENANT ?? 'default'
const identityHeaders: Record<string, string> = {}
if (devUser) identityHeaders['Remote-User'] = devUser
if (devTenant) identityHeaders['Remote-Tenant'] = devTenant

export default defineConfig({
    plugins: [vue()],
    resolve: {
        alias: {
            '@': fileURLToPath(new URL('./src', import.meta.url))
        }
    },
    base: '/',
    server: {
        // the first matching entry wins: /api/key before /api
        proxy: {
            '/api/key': {
                target: backend,
                changeOrigin: true
            },
            '/api': {
                target: backend,
                changeOrigin: true,
                headers: identityHeaders
            }
        }
    }
})
