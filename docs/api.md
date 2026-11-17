# HTTP API

Base path: `/api/v1`. Resources and HTTP methods follow the conventions in [REST API resource naming](https://restfulapi.net/resource-naming/). No action verbs appear in resource paths.

| Method | Resource | Behavior |
| --- | --- | --- |
| POST | `/sessions` | Sign in using `{"password":"..."}`; returns `201`, `Location`, and an HttpOnly session cookie |
| GET | `/sessions/current` | Current authentication state and upload limit; anonymous state is represented as `authenticated: false` |
| DELETE | `/sessions/current` | Revoke the current session across replicas; returns `204` |
| POST | `/photo-archives` | Create a selection download using `{"photoIds":["..."]}`; returns `201`, a download URL, and a 15-minute expiry |
| GET | `/photo-archives/{id}` | Stream selected originals as a ZIP; requires authentication and an unexpired download |
| GET | `/status` | Authenticated library counts, byte sizes, disk capacity where available, and database/storage availability |
| GET | `/photos` | List photos, newest upload first; returns `photos` and `nextCursor` |
| POST | `/photos` | Multipart upload, one file in the `photo` field; returns `201` and `Location` for a new photo, or `200` for identical existing bytes |
| GET | `/photos/timeline` | Upload months and counts; accepts `q`, `favorite`, and IANA `tz` (default UTC) |
| GET | `/photos/{id}` | Photo metadata |
| PATCH | `/photos/{id}` | Update `{"favorite":true}` or `{"favorite":false}`; returns updated metadata |
| DELETE | `/photos/{id}` | Hide photo and queue object removal; returns `204` |
| GET | `/photos/{id}/original` | Authenticated original bytes; `?download=1` adds attachment disposition |
| GET | `/photos/{id}/preview` | Browser display image: JPEG for HEIC, original bytes for other formats |
| GET | `/photos/{id}/thumbnail` | Authenticated JPEG thumbnail |

All photo routes require a valid session. State-changing requests require `X-Lumio-Request: 1`. JSON requests require `Content-Type: application/json`. Browser Origin/Fetch Metadata is checked, and cross-origin requests are not enabled.

Photo IDs are lowercase SHA-256 hashes of original bytes. Upload responses are `{"photo":{...},"duplicate":false}`. Metadata contains `id`, `name`, `type`, `size`, `width`, `height`, `addedAt`, and `favorite`. Dates use RFC 3339 in UTC.

Collection parameters:

- `q`: case-insensitive filename substring, up to 200 UTF-8 bytes.
- `favorite=true`: favorites only.
- `from` / `until`: optional RFC 3339 upload-time bounds, inclusive / exclusive; preserved when following cursors.
- `cursor`: opaque cursor returned by the previous response; keep the same filters when following it.

Pages contain at most 60 photos. An empty `nextCursor` means the last page. Pagination uses `(added_at, id)` ordering; it does not use database offsets.

Errors return JSON such as `{"error":"photo not found"}` with `400` for malformed input, `401` for authentication failures, `403` for rejected mutations, `404` for absent photos, `413` for oversized files, `415` for unsupported content, `429` for login throttling, `503` for a busy upload worker, and `500` for unexpected backend errors. Throttling and busy-worker responses include `Retry-After`. Responses carry `X-Request-ID`, and private responses use `Cache-Control: no-store`.

POST is used to create resources, GET to retrieve them, PATCH for partial metadata updates, and DELETE to remove them. Uploads and deletes are serialized for the same content ID across replicas; duplicate uploads reuse the existing resource.

Photo archives accept up to 1000 selected IDs, ignore duplicates, preserve original bytes, and disambiguate repeated filenames. The browser streams the ZIP directly from the returned URL. A missing/deleted selection returns `404` on creation or `409` before download; expired download URLs return `404`. Storage errors during streaming terminate the download instead of returning an apparently complete, partial ZIP.
