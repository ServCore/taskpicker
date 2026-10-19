import { copyFile, mkdir } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
await mkdir(resolve(root, 'build/darwin'), { recursive: true })
for (const name of ['appicon.png', 'darwin/Info.plist', 'darwin/Info.dev.plist']) {
  await copyFile(resolve(root, 'assets', name), resolve(root, 'build', name))
}
