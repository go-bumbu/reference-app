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
    // Ids of the tags the note carries, ascending.
    tagIds: number[]
}

export interface NoteInput {
    title: string
    body?: string
}

// A tag with its subtree. parentId is 0 for a tag at the root; children are in
// the order the users gave them.
export interface Tag {
    id: number
    parentId: number
    name: string
    children: Tag[]
}

// The body of POST /tags: parentId 0 (or absent) is the root; afterId is the
// sibling to place it after, 0 (or absent) first.
export interface TagInput {
    name: string
    parentId?: number
    afterId?: number
}

// The body of PATCH /tags/{tagId}: an absent field is left as it is.
export interface TagPatch {
    name?: string
    parentId?: number
    afterId?: number
}
