/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'
import { readFileSync } from 'node:fs'
import process from 'node:process'
import { fileURLToPath, URL } from 'node:url'

const packageJson = readPackageJson()

function readPackageJson() {
  try {
    return JSON.parse(readFileSync(fileURLToPath(new URL('./package.json', import.meta.url)), 'utf8'))
  } catch {
    return {}
  }
}

function normalizeBannerValue(value, fallback) {
  const text = String(value || '').trim()
  if (!text || text.toLowerCase() === 'unknown') return fallback
  return text
}

function normalizeVersion(value) {
  const version = normalizeBannerValue(value, 'v0.0.0')
  if (version === 'none') return 'v0.0.0'
  return version.startsWith('v') ? version : `v${version}`
}

function normalizeDependencyVersion(value) {
  return normalizeBannerValue(String(value || '').replace(/^[~^]/, ''), 'unknown')
}

function formatLocalDate(date = new Date()) {
  const pad = (value) => String(value).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

function listenURL(server) {
  const address = server?.httpServer?.address()
  if (!address || typeof address === 'string') return ''
  const protocol = server.config.server.https ? 'https' : 'http'
  const host = address.address === '::' || address.address === '0.0.0.0'
    ? 'localhost'
    : address.address
  return `${protocol}://${host}:${address.port}${server.config.base || '/'}`
}

function formatBanner({ mode, phase, url }) {
  const version = normalizeVersion(
    process.env.VITE_APP_VERSION ||
    process.env.VERSION ||
    process.env.npm_package_version ||
    packageJson.version
  )
  const commit = normalizeBannerValue(
    process.env.VITE_GIT_COMMIT ||
    process.env.GIT_COMMIT ||
    process.env.npm_package_gitHead,
    'none'
  )
  const buildTime = normalizeBannerValue(
    process.env.VITE_BUILD_TIME ||
    process.env.BUILD_TIME,
    formatLocalDate()
  )
  const viteVersion = normalizeDependencyVersion(packageJson.devDependencies?.vite || packageJson.dependencies?.vite)

  return String.raw`
**************************************************************************
**************************************************************************
  _____ _ _      _____ _                    __  __
 |  ___(_) | ___/  ___| |__   __ _ _ __ ___|  \/  | __ _ _ __
 | |_  | | |/ _ \___ \| '_ \ / _' | '__/ _ \ |\/| |/ _' | '_ |
 |  _| | | |  __/___) | | | | (_| | | |  __/ |  | | (_| | | | |
 |_|   |_|_|\___|____/|_| |_|\__,_|_|  \___|_|  |_|\__,_|_| |_|

  File Share Manager - 开源工作空间文件共享服务
  Version:   ${version}
  Node:      ${process.version}
  Vite:      v${viteVersion}
  Build:     ${buildTime}
  Commit:    ${commit}
  Mode:      ${mode}
  Phase:     ${phase}
  Author:    HaydenGuo
  Blog:      https://hayden.pub
  GitHub:    https://github.com/ghl1024/file-share-manager
  Gitee:     https://gitee.com/ghl1024/file-share-manager
  CNB:       https://cnb.cool/ghl1024/file-share-manager
  License:   Apache-2.0
  Listen:    ${url || '-'}
**************************************************************************
**************************************************************************
`
}

function fileShareBannerPlugin(mode) {
  let printed = false
  const print = (phase, server) => {
    if (printed) return
    printed = true
    console.log(formatBanner({ mode, phase, url: listenURL(server) }))
  }

  return {
    name: 'fileshare-startup-banner',
    configResolved(config) {
      if (config.command === 'build') print('build')
    },
    configureServer(server) {
      server.httpServer?.once('listening', () => print('dev', server))
    },
    configurePreviewServer(server) {
      server.httpServer?.once('listening', () => print('preview', server))
    }
  }
}

export default defineConfig(({ mode }) => {
  const outDir = mode === 'development'
    ? 'dist-dev'
    : 'dist'
  const apiProxyTarget = process.env.VITE_API_PROXY_TARGET || 'http://localhost:29000'

  return {
    base: '/fileshare/',
    plugins: [
      fileShareBannerPlugin(mode),
      vue(),
      AutoImport({
        resolvers: [ElementPlusResolver()],
        dts: false
      }),
      Components({
        resolvers: [ElementPlusResolver()],
        dts: false
      })
    ],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url))
      }
    },
    server: {
      port: 39000,
      host: '127.0.0.1',
      strictPort: false,
      headers: {
        'Content-Security-Policy': "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self' ws: wss:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'",
        'X-Content-Type-Options': 'nosniff',
        'X-Frame-Options': 'DENY',
        'Referrer-Policy': 'no-referrer',
        'Permissions-Policy': 'camera=(), microphone=(), geolocation=()'
      },
      proxy: {
        '/api/fileshare/v1': {
          target: apiProxyTarget,
          changeOrigin: true
        },
        '/swagger': {
          target: apiProxyTarget,
          changeOrigin: true
        }
      }
    },
    build: {
      outDir
    }
  }
})
