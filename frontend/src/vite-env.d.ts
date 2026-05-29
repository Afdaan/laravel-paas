/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_GITHUB_APP_URL: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
