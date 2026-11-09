// Wire types of docs/openapi/api-v0.yaml. Hand-written like libs/client: keep
// them in step with the spec's schemas.

export interface Info {
    version: string
    buildTime: string
    commit: string
    goVersion: string
}

export interface Me {
    userId: string
    subject: string
    tenantId: string
    tenant: string
}

export interface Note {
    id: string
    createdBy: string
    title: string
    body: string
    createdAt: string
}

export interface NoteInput {
    title: string
    body?: string
}
