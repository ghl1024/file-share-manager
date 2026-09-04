[简体中文](README.md) | English

# File Share Manager Frontend

A Vue 3 + Vite + Element Plus + Pinia frontend for workspaces, directory browsing, chunked uploads, version downloads, and audit queries.

## Run

```bash
npm install
npm run dev
```

The default development port is `39000`, and the application base path is `/fileshare/`. If the port is already in use, Vite automatically selects the next available port. The development proxy forwards `/api/fileshare/v1` to `http://localhost:29000`.

## Build

```bash
npm run build
```

Production assets are output to `dist/` and served by Nginx under `/fileshare/`. All backend APIs use the `/api/fileshare/v1/` prefix; the Axios client automatically places unauthenticated requests under the `/management` namespace.

## Frontend Conventions

- All requests go through `src/utils/request.js`. Successful responses read `data`, while paginated lists read `data.list`, `data.total`, `data.page`, and `data.page_size`.
- Forms are validated consistently by the backend. Button permissions only improve the user interface; the server remains the final security boundary.
- List pages reuse `src/composables/usePagination.js`, and table pagination uses the shared Element Plus pagination control.
- Sessions use HttpOnly cookies; the frontend neither reads nor persists JWTs.
- File uploads use a four-step protocol: initialization, chunk upload, status query, and completion. The interface supports pausing, resuming, and canceling the current upload.
