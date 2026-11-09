const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })

/** A note's createdAt (RFC 3339) in the user's locale; the raw value if unparsable. */
export function formatDate(iso: string): string {
    const d = new Date(iso)
    return Number.isNaN(d.getTime()) ? iso : dateFormat.format(d)
}
