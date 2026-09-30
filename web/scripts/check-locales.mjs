// Prüft, dass de.json und en.json dieselben Schlüssel und Platzhalter haben.
import { readFileSync } from 'node:fs'

const load = (l) => JSON.parse(readFileSync(new URL(`../src/locales/${l}.json`, import.meta.url), 'utf8'))
const flat = (o, p = '') =>
  Object.entries(o).flatMap(([k, v]) => (typeof v === 'object' ? flat(v, `${p}${k}.`) : [[`${p}${k}`, v]]))
const de = new Map(flat(load('de')))
const en = new Map(flat(load('en')))
const vars = (s) => [...s.matchAll(/\{\{(\w+)\}\}/g)].map((m) => m[1]).sort().join(',')

const errors = []
for (const k of de.keys()) if (!en.has(k)) errors.push(`en fehlt: ${k}`)
for (const k of en.keys()) if (!de.has(k)) errors.push(`de fehlt: ${k}`)
for (const [k, v] of de) if (en.has(k) && vars(v) !== vars(en.get(k))) errors.push(`Platzhalter unterschiedlich: ${k}`)
if (errors.length) {
  console.error(errors.join('\n'))
  process.exit(1)
}
console.log(`locales ok (${de.size} Schlüssel)`)
