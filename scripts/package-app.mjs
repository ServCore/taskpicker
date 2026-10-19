import { access, cp, mkdir, mkdtemp, rename } from 'node:fs/promises'
import { homedir } from 'node:os'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const dist = resolve(root, 'dist')
const app = resolve(dist, '我的任务.app')
await mkdir(dist, { recursive: true })
const stage = await mkdtemp(resolve(dist, '.taskpicker-build-'))
const stagedApp = resolve(stage, '我的任务.app')
await cp(resolve(root, 'build/bin/我的任务.app'), stagedApp, { recursive: true, verbatimSymlinks: true })
let previous = ''
try { await access(app); previous = app } catch (error) { if (error.code !== 'ENOENT') throw error }
if (previous) {
  const trash = resolve(homedir(), '.Trash')
  await mkdir(trash, { recursive: true })
  const backup = await mkdtemp(resolve(trash, 'TaskPicker-build-'))
  await rename(previous, resolve(backup, '我的任务.app'))
}
await rename(stagedApp, app)
// The stage is now empty. Remove only that empty directory, never user files.
const { rmdir } = await import('node:fs/promises')
await rmdir(stage)
console.log('已生成 dist/我的任务.app')
