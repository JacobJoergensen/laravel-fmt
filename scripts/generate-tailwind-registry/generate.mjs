// Writes the output directly in UTF-8 rather than via stdout + shell
// redirection: PowerShell's default UTF-16 LE would otherwise produce
// output Go's parser rejects with `illegal UTF-8 encoding`.
//
// Kept to a single file on purpose: a generator that runs a few times
// per Tailwind release isn't worth a build step.

import { execFileSync } from 'node:child_process'
import { writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'
import { createRequire } from 'node:module'
import { __unstable__loadDesignSystem } from '@tailwindcss/node'

const here = dirname(fileURLToPath(import.meta.url))
const require = createRequire(import.meta.url)

const outputPath = resolve(
  here,
  '..',
  '..',
  'internal',
  'formatter',
  'tailwind',
  'registry_generated.go',
)

async function main() {
  const css = `@import "tailwindcss";`
  const design = await __unstable__loadDesignSystem(css, { base: here })

  const utilityStems = buildUtilityStems(design)
  const numeric = syntheticNumericCandidates(design)
  const synthetic = [...numeric, ...syntheticArbitraryCandidates(design, numeric, utilityStems)]
  const { rank, propertyRanks } = buildClassOrderRanks(design, propertyOrder, synthetic)

  const utilityRegistry = buildUtilityRegistry(design, rank, propertyOrder, synthetic)
  const stemRegistry = buildStemRegistry(utilityRegistry, utilityStems)
  const variantRegistry = buildVariantRegistry(design)
  const tailwindVersion = readTailwindVersion()

  const source = renderGo({
    propertyRanks,
    utilityRegistry,
    stemRegistry,
    variantRegistry,
    tailwindVersion,
  })

  writeFileSync(outputPath, source, { encoding: 'utf8' })

  // gofmt column-aligns composite-literal values, which the renderer does not
  // reproduce; a missing gofmt fails the run loudly instead of leaving an
  // unformatted file.
  execFileSync('gofmt', ['-w', outputPath], { stdio: 'inherit' })

  process.stderr.write(
    `Wrote ${outputPath} (${utilityRegistry.length} utilities, ${stemRegistry.length} stems, ${variantRegistry.length} variants)\n`,
  )
}

// propertyOrder is a verbatim snapshot of
// packages/tailwindcss/src/property-order.ts from the v4.3.3 tag: the
// properties the generator probes and classifies shapes by.
//
// When refreshing for a new Tailwind release, fetch the source at
// https://raw.githubusercontent.com/tailwindlabs/tailwindcss/v<version>/packages/tailwindcss/src/property-order.ts
// and replace the contents below. The shape is always
// `export default [ '<property>', ... ]` so the array entries are
// the only thing that changes between releases.
const propertyOrder = [
  'container-type',
  'pointer-events',
  'visibility',
  'position',
  'inset',
  'inset-inline',
  'inset-block',
  'inset-inline-start',
  'inset-inline-end',
  'inset-block-start',
  'inset-block-end',
  'top',
  'right',
  'bottom',
  'left',
  'isolation',
  'z-index',
  'order',
  'grid-column',
  'grid-column-start',
  'grid-column-end',
  'grid-row',
  'grid-row-start',
  'grid-row-end',
  'float',
  'clear',
  '--tw-container-component',
  'margin',
  'margin-inline',
  'margin-block',
  'margin-inline-start',
  'margin-inline-end',
  'margin-block-start',
  'margin-block-end',
  'margin-top',
  'margin-right',
  'margin-bottom',
  'margin-left',
  'box-sizing',
  'display',
  'field-sizing',
  'aspect-ratio',
  'height',
  'max-height',
  'min-height',
  'width',
  'max-width',
  'min-width',
  'flex',
  'flex-shrink',
  'flex-grow',
  'flex-basis',
  'table-layout',
  'caption-side',
  'border-collapse',
  'border-spacing',
  'transform-origin',
  'translate',
  '--tw-translate-x',
  '--tw-translate-y',
  '--tw-translate-z',
  'scale',
  '--tw-scale-x',
  '--tw-scale-y',
  '--tw-scale-z',
  'rotate',
  '--tw-rotate-x',
  '--tw-rotate-y',
  '--tw-rotate-z',
  '--tw-skew-x',
  '--tw-skew-y',
  'transform',
  'zoom',
  'animation',
  'cursor',
  'touch-action',
  '--tw-pan-x',
  '--tw-pan-y',
  '--tw-pinch-zoom',
  'resize',
  'scroll-snap-type',
  '--tw-scroll-snap-strictness',
  'scroll-snap-align',
  'scroll-snap-stop',
  'scroll-margin',
  'scroll-margin-inline',
  'scroll-margin-block',
  'scroll-margin-inline-start',
  'scroll-margin-inline-end',
  'scroll-margin-block-start',
  'scroll-margin-block-end',
  'scroll-margin-top',
  'scroll-margin-right',
  'scroll-margin-bottom',
  'scroll-margin-left',
  'scroll-padding',
  'scroll-padding-inline',
  'scroll-padding-block',
  'scroll-padding-inline-start',
  'scroll-padding-inline-end',
  'scroll-padding-block-start',
  'scroll-padding-block-end',
  'scroll-padding-top',
  'scroll-padding-right',
  'scroll-padding-bottom',
  'scroll-padding-left',
  'scrollbar-width',
  'scrollbar-color',
  'scrollbar-gutter',
  'list-style-position',
  'list-style-type',
  'list-style-image',
  'appearance',
  'columns',
  'break-before',
  'break-inside',
  'break-after',
  'grid-auto-columns',
  'grid-auto-flow',
  'grid-auto-rows',
  'grid-template-columns',
  'grid-template-rows',
  'flex-direction',
  'flex-wrap',
  'place-content',
  'place-items',
  'align-content',
  'align-items',
  'justify-content',
  'justify-items',
  'gap',
  'column-gap',
  'row-gap',
  '--tw-space-x-reverse',
  '--tw-space-y-reverse',
  'divide-x-width',
  'divide-y-width',
  '--tw-divide-y-reverse',
  'divide-style',
  'divide-color',
  'place-self',
  'align-self',
  'justify-self',
  'overflow',
  'overflow-x',
  'overflow-y',
  'overscroll-behavior',
  'overscroll-behavior-x',
  'overscroll-behavior-y',
  'scroll-behavior',
  'border-radius',
  'border-start-radius',
  'border-end-radius',
  'border-top-radius',
  'border-right-radius',
  'border-bottom-radius',
  'border-left-radius',
  'border-start-start-radius',
  'border-start-end-radius',
  'border-end-end-radius',
  'border-end-start-radius',
  'border-top-left-radius',
  'border-top-right-radius',
  'border-bottom-right-radius',
  'border-bottom-left-radius',
  'border-width',
  'border-inline-width',
  'border-block-width',
  'border-inline-start-width',
  'border-inline-end-width',
  'border-block-start-width',
  'border-block-end-width',
  'border-top-width',
  'border-right-width',
  'border-bottom-width',
  'border-left-width',
  'border-style',
  'border-inline-style',
  'border-block-style',
  'border-inline-start-style',
  'border-inline-end-style',
  'border-block-start-style',
  'border-block-end-style',
  'border-top-style',
  'border-right-style',
  'border-bottom-style',
  'border-left-style',
  'border-color',
  'border-inline-color',
  'border-block-color',
  'border-inline-start-color',
  'border-inline-end-color',
  'border-block-start-color',
  'border-block-end-color',
  'border-top-color',
  'border-right-color',
  'border-bottom-color',
  'border-left-color',
  'background-color',
  'background-image',
  '--tw-gradient-position',
  '--tw-gradient-stops',
  '--tw-gradient-via-stops',
  '--tw-gradient-from',
  '--tw-gradient-from-position',
  '--tw-gradient-via',
  '--tw-gradient-via-position',
  '--tw-gradient-to',
  '--tw-gradient-to-position',
  'mask-image',
  '--tw-mask-top',
  '--tw-mask-top-from-color',
  '--tw-mask-top-from-position',
  '--tw-mask-top-to-color',
  '--tw-mask-top-to-position',
  '--tw-mask-right',
  '--tw-mask-right-from-color',
  '--tw-mask-right-from-position',
  '--tw-mask-right-to-color',
  '--tw-mask-right-to-position',
  '--tw-mask-bottom',
  '--tw-mask-bottom-from-color',
  '--tw-mask-bottom-from-position',
  '--tw-mask-bottom-to-color',
  '--tw-mask-bottom-to-position',
  '--tw-mask-left',
  '--tw-mask-left-from-color',
  '--tw-mask-left-from-position',
  '--tw-mask-left-to-color',
  '--tw-mask-left-to-position',
  '--tw-mask-linear',
  '--tw-mask-linear-position',
  '--tw-mask-linear-from-color',
  '--tw-mask-linear-from-position',
  '--tw-mask-linear-to-color',
  '--tw-mask-linear-to-position',
  '--tw-mask-radial',
  '--tw-mask-radial-shape',
  '--tw-mask-radial-size',
  '--tw-mask-radial-position',
  '--tw-mask-radial-from-color',
  '--tw-mask-radial-from-position',
  '--tw-mask-radial-to-color',
  '--tw-mask-radial-to-position',
  '--tw-mask-conic',
  '--tw-mask-conic-position',
  '--tw-mask-conic-from-color',
  '--tw-mask-conic-from-position',
  '--tw-mask-conic-to-color',
  '--tw-mask-conic-to-position',
  'box-decoration-break',
  'background-size',
  'background-attachment',
  'background-clip',
  'background-position',
  'background-repeat',
  'background-origin',
  'mask-composite',
  'mask-mode',
  'mask-type',
  'mask-size',
  'mask-clip',
  'mask-position',
  'mask-repeat',
  'mask-origin',
  'fill',
  'stroke',
  'stroke-width',
  'object-fit',
  'object-position',
  'padding',
  'padding-inline',
  'padding-block',
  'padding-inline-start',
  'padding-inline-end',
  'padding-block-start',
  'padding-block-end',
  'padding-top',
  'padding-right',
  'padding-bottom',
  'padding-left',
  'text-align',
  'text-indent',
  'vertical-align',
  'font-family',
  'font-feature-settings',
  'font-size',
  'line-height',
  'font-weight',
  'letter-spacing',
  'text-wrap',
  'overflow-wrap',
  'word-break',
  'text-overflow',
  'hyphens',
  'white-space',
  'tab-size',
  'color',
  'text-transform',
  'font-style',
  'font-stretch',
  'font-variant-numeric',
  'text-decoration-line',
  'text-decoration-color',
  'text-decoration-style',
  'text-decoration-thickness',
  'text-underline-offset',
  '-webkit-font-smoothing',
  'placeholder-color',
  'caret-color',
  'accent-color',
  'color-scheme',
  'opacity',
  'background-blend-mode',
  'mix-blend-mode',
  'box-shadow',
  '--tw-shadow',
  '--tw-shadow-color',
  '--tw-ring-shadow',
  '--tw-ring-color',
  '--tw-inset-shadow',
  '--tw-inset-shadow-color',
  '--tw-inset-ring-shadow',
  '--tw-inset-ring-color',
  '--tw-ring-offset-width',
  '--tw-ring-offset-color',
  'outline',
  'outline-width',
  'outline-offset',
  'outline-color',
  '--tw-blur',
  '--tw-brightness',
  '--tw-contrast',
  '--tw-drop-shadow',
  '--tw-grayscale',
  '--tw-hue-rotate',
  '--tw-invert',
  '--tw-saturate',
  '--tw-sepia',
  'filter',
  '--tw-backdrop-blur',
  '--tw-backdrop-brightness',
  '--tw-backdrop-contrast',
  '--tw-backdrop-grayscale',
  '--tw-backdrop-hue-rotate',
  '--tw-backdrop-invert',
  '--tw-backdrop-opacity',
  '--tw-backdrop-saturate',
  '--tw-backdrop-sepia',
  'backdrop-filter',
  'transition-property',
  'transition-behavior',
  'transition-delay',
  'transition-duration',
  'transition-timing-function',
  'will-change',
  'contain',
  'content',
  'forced-color-adjust',
]

// getClassList suggests no numeric values for some functional utilities
// (end-*, start-*), so their bare stems would never reach the stem registry
// and every numeric value would sort as unknown. One probe per stem fills the
// gap; getClassOrder answers null for candidates the engine cannot compile,
// which drops them from the rank map.
function syntheticNumericCandidates(design) {
  if (typeof design.utilities?.keys !== 'function') return []

  const names = design.getClassList().map(([name]) => name)
  const out = []

  for (const stem of design.utilities.keys('functional')) {
    const prefix = `${stem}-`
    const hasNumeric = names.some(
      (n) => n.startsWith(prefix) && n[prefix.length] >= '0' && n[prefix.length] <= '9',
    )
    if (!hasNumeric) out.push(`${prefix}0`)
  }

  return out
}

// Some functional utilities take only arbitrary values (`content-[…]`), so no
// listed or numeric class reaches their stem and every use sorts as unknown.
// One arbitrary probe per such stem fills the gap; a stem that already has
// classes gets none, so its rank stays what its real classes give it.
function syntheticArbitraryCandidates(design, numeric, utilityStems) {
  if (typeof design.utilities?.keys !== 'function') return []

  const stemSet = new Set(utilityStems)
  const listed = [...design.getClassList().map(([name]) => name), ...numeric]
  const covered = new Set(
    design
      .getClassOrder(listed)
      .filter(([, o]) => o !== null)
      .map(([name]) => findLongestPrefixStem(name, stemSet)),
  )

  const out = []
  for (const stem of design.utilities.keys('functional')) {
    if (!covered.has(stem)) out.push(`${stem}-[var(--probe)]`)
  }

  return out
}

// The engine's getClassOrder is the sort prettier-plugin-tailwindcss uses.
// Rebuilding it from each utility's emitted properties diverged for utilities
// whose slot is no visible declaration (container, space-*, divide-*). A class
// it cannot order gets no rank, so it is Unknown in Go and sorts first, as
// prettier does. One `[prop:0]` probe per property puts an arbitrary property
// (`[grid-area:…]`) or a custom `@utility` on the same scale.
function buildClassOrderRanks(design, propertyOrder, synthetic = []) {
  const classNames = [...design.getClassList().map(([name]) => name), ...synthetic]
  const probeFor = (p) => `[${p}:0]`
  const probes = propertyOrder.map(probeFor)

  const pairs = design.getClassOrder([...classNames, ...probes])
  const order = new Map(pairs.map(([name, o]) => [name, o]))

  const ranked = pairs
    .filter(([, o]) => o !== null)
    .sort((a, b) => (a[1] < b[1] ? -1 : a[1] > b[1] ? 1 : 0))

  const rank = new Map()
  ranked.forEach(([name], i) => rank.set(name, i))

  const propertyRanks = {}
  for (const p of propertyOrder) {
    const r = rank.get(probeFor(p))
    if (r !== undefined) propertyRanks[p] = r
  }

  return { rank, propertyRanks }
}

// collectPropertySort only classifies a class's shape (color- or size-valued)
// for the ambiguous-stem branches; the order is the engine's rank.
function buildUtilityRegistry(design, rank, propertyOrder, synthetic = []) {
  const propertyToIndex = new Map(propertyOrder.map((p, i) => [p, i]))
  const colorIndices = new Set()
  const sizeIndices = new Set()
  for (let i = 0; i < propertyOrder.length; i++) {
    if (isColorPropertyName(propertyOrder[i])) colorIndices.add(i)
    if (sizePropertyNames.has(propertyOrder[i])) sizeIndices.add(i)
  }

  const registry = []

  const names = [...design.getClassList().map(([name]) => name), ...synthetic]

  for (const name of names) {
    const r = rank.get(name)
    if (r === undefined) {
      continue
    }

    const nodes = design.candidatesToAst([name])[0] ?? []
    const { order: shapeTuple } = collectPropertySort(nodes, propertyToIndex)
    const hasColor = shapeTuple.some((i) => colorIndices.has(i))
    const hasSize = shapeTuple.some((i) => sizeIndices.has(i))
    const shape = hasColor && !hasSize ? 'color' : hasSize && !hasColor ? 'size' : 'other'

    registry.push({ name, order: [r], count: 1, shape })
  }

  return registry
}

// Mirrors the engine's getPropertySort (compile.ts).
function collectPropertySort(nodes, propertyToIndex) {
  const order = new Set()
  let count = 0
  let seenTwSort = false

  const queue = [...nodes]
  while (queue.length > 0) {
    const node = queue.shift()
    if (node.kind === 'declaration') {
      if (node.value === undefined) continue
      count++

      if (seenTwSort) continue

      if (node.property === '--tw-sort') {
        const idx = propertyToIndex.get(node.value)
        if (idx !== undefined) {
          order.add(idx)
          seenTwSort = true
        }
        continue
      }

      const idx = propertyToIndex.get(node.property)
      if (idx !== undefined) order.add(idx)
    } else if (node.kind === 'rule' || node.kind === 'at-rule') {
      for (const child of node.nodes) queue.push(child)
    }
  }

  return {
    order: Array.from(order).sort((a, b) => a - b),
    count,
  }
}

// The Go candidate parser splits a class like `text-red-500` into root
// `text` and value `red-500` by longest-prefix match against these stems.
function buildUtilityStems(design) {
  const stems = new Set()
  if (typeof design.utilities?.keys === 'function') {
    for (const stem of design.utilities.keys('static')) stems.add(stem)
    for (const stem of design.utilities.keys('functional')) stems.add(stem)
  } else {
    // Fallback for when the private `Utilities.keys` API changes shape:
    // imperfect, but keeps the generator from hard-failing.
    const names = new Set()
    for (const [name] of design.getClassList()) names.add(name)
    for (const name of names) {
      stems.add(name)
      let candidate = name
      while (true) {
        const idx = candidate.lastIndexOf('-')
        if (idx <= 0) break
        candidate = candidate.slice(0, idx)
        if (names.has(candidate)) stems.add(candidate)
      }
    }
  }
  return Array.from(stems).sort()
}

// buildStemRegistry is what makes a user's `bg-brand` classify as
// background-color the way `bg-red-500` does. For an ambiguous stem (`text`,
// `border`, `shadow`, …) the default branch is the color one: the default
// theme defines far more color tokens than size tokens. Exact classes
// (`text-xs`) hit utilityRegistry first; this only matters for values it has
// never seen.
function buildStemRegistry(utilityRegistry, utilityStems) {
  const stemSet = new Set(utilityStems)
  const groups = new Map()

  for (const entry of utilityRegistry) {
    const stem = findLongestPrefixStem(entry.name, stemSet)
    if (!stem) continue
    let group = groups.get(stem)
    if (!group) {
      group = []
      groups.set(stem, group)
    }
    group.push(entry)
  }

  // A branch's representative rank is the EARLIEST (min) rank among its classes:
  // a stem's color/size/other classes cluster contiguously in getClassOrder, so
  // the min is a faithful slot for an unseen value of that shape (`text-brand`
  // sorts where text colors do). Shape comes from buildUtilityRegistry.
  const minRank = (es) => (es.length === 0 ? null : [Math.min(...es.map((e) => e.order[0]))])

  const out = []
  for (const [stem, entries] of groups) {
    const colorShaped = entries.filter((e) => e.shape === 'color')
    const sizeShaped = entries.filter((e) => e.shape === 'size')

    const dominant =
      colorShaped.length >= sizeShaped.length
        ? colorShaped.length > 0
          ? colorShaped
          : entries
        : sizeShaped

    const colorOrder = minRank(colorShaped)
    const sizeOrder = minRank(sizeShaped)

    out.push({
      stem,
      order: minRank(dominant) ?? minRank(entries) ?? [],
      count: 1,
      colorOrder,
      colorCount: colorOrder ? 1 : 0,
      sizeOrder,
      sizeCount: sizeOrder ? 1 : 0,
    })
  }

  // Sort by stem name so diffs across regenerations read as real
  // changes (a new stem, a flipped dominant tuple) rather than noise
  // from Map iteration order shifting between Node versions.
  out.sort((a, b) => (a.stem < b.stem ? -1 : a.stem > b.stem ? 1 : 0))
  return out
}

// The gradient position siblings (`--tw-gradient-from-position`, …) are
// deliberately excluded: they are lengths, not colors.
function isColorPropertyName(name) {
  return (
    name === 'color' ||
    name.endsWith('-color') ||
    name === 'fill' ||
    name === 'stroke' ||
    name === '--tw-gradient-from' ||
    name === '--tw-gradient-via' ||
    name === '--tw-gradient-to'
  )
}

// No pattern covers length- and time-valued properties the way one does
// colors, so they are listed; a new one in a future release belongs here.
const sizePropertyNames = new Set([
  'inset', 'inset-inline', 'inset-block',
  'inset-inline-start', 'inset-inline-end',
  'inset-block-start', 'inset-block-end',
  'top', 'right', 'bottom', 'left',
  'width', 'min-width', 'max-width',
  'height', 'min-height', 'max-height',
  'flex-basis', 'border-spacing',
  'margin', 'margin-inline', 'margin-block',
  'margin-inline-start', 'margin-inline-end',
  'margin-block-start', 'margin-block-end',
  'margin-top', 'margin-right', 'margin-bottom', 'margin-left',
  'padding', 'padding-inline', 'padding-block',
  'padding-inline-start', 'padding-inline-end',
  'padding-block-start', 'padding-block-end',
  'padding-top', 'padding-right', 'padding-bottom', 'padding-left',
  'scroll-margin', 'scroll-margin-inline', 'scroll-margin-block',
  'scroll-margin-inline-start', 'scroll-margin-inline-end',
  'scroll-margin-block-start', 'scroll-margin-block-end',
  'scroll-margin-top', 'scroll-margin-right',
  'scroll-margin-bottom', 'scroll-margin-left',
  'scroll-padding', 'scroll-padding-inline', 'scroll-padding-block',
  'scroll-padding-inline-start', 'scroll-padding-inline-end',
  'scroll-padding-block-start', 'scroll-padding-block-end',
  'scroll-padding-top', 'scroll-padding-right',
  'scroll-padding-bottom', 'scroll-padding-left',
  'gap', 'column-gap', 'row-gap',
  'border-radius',
  'border-start-radius', 'border-end-radius',
  'border-top-radius', 'border-right-radius',
  'border-bottom-radius', 'border-left-radius',
  'border-start-start-radius', 'border-start-end-radius',
  'border-end-end-radius', 'border-end-start-radius',
  'border-top-left-radius', 'border-top-right-radius',
  'border-bottom-right-radius', 'border-bottom-left-radius',
  'border-width',
  'border-inline-width', 'border-block-width',
  'border-inline-start-width', 'border-inline-end-width',
  'border-block-start-width', 'border-block-end-width',
  'border-top-width', 'border-right-width',
  'border-bottom-width', 'border-left-width',
  'divide-x-width', 'divide-y-width',
  'outline-width', 'outline-offset',
  'stroke-width',
  'font-size', 'line-height', 'letter-spacing',
  'text-decoration-thickness', 'text-underline-offset', 'tab-size',
  'box-shadow', '--tw-shadow',
  '--tw-ring-shadow', '--tw-inset-shadow', '--tw-inset-ring-shadow',
  '--tw-ring-offset-width',
  'transition-duration', 'transition-delay',
])

function dominantTuple(entries) {
  if (entries.length === 0) return null
  const tally = new Map()
  for (const e of entries) {
    const key = `${JSON.stringify(e.order)}|${e.count}`
    const existing = tally.get(key) ?? { order: e.order, count: e.count, n: 0 }
    existing.n++
    tally.set(key, existing)
  }
  let best = null
  for (const t of tally.values()) {
    if (!best || t.n > best.n) best = t
  }
  return best
}

function findLongestPrefixStem(name, stemSet) {
  if (stemSet.has(name)) return name
  let candidate = name
  while (true) {
    const idx = candidate.lastIndexOf('-')
    if (idx <= 0) break
    candidate = candidate.slice(0, idx)
    if (stemSet.has(candidate)) return candidate
  }
  return null
}

// Arbitrary variants (`[&>a:hover]:`) are left out: the Go runtime ranks each
// one from its selector text.
function buildVariantRegistry(design) {
  const entries = []
  for (const [root, variant] of design.variants.entries()) {
    if (variant.kind === 'arbitrary') continue
    entries.push({ root })
  }

  entries.sort((a, b) => {
    const av = design.parseVariant(a.root) ?? design.parseVariant(`${a.root}-`)
    const bv = design.parseVariant(b.root) ?? design.parseVariant(`${b.root}-`)
    if (!av || !bv) return 0
    return design.variants.compare(av, bv)
  })

  return entries.map((entry, index) => ({
    root: entry.root,
    priority: index,
  }))
}

function readTailwindVersion() {
  try {
    const pkg = require('tailwindcss/package.json')
    return pkg.version ?? 'unknown'
  } catch {
    return 'unknown'
  }
}

// appendChunkedMap splits the emitted map across many small register<i>(m)
// helpers on purpose: golangci-lint's per-function passes are super-linear
// in function size, so one 20k-line literal makes a lint run crawl.
function appendChunkedMap(
  lines,
  { buildName, valueType, registerPrefix, entries, renderAssign, chunkSize = 400 },
) {
  const chunkCount = Math.ceil(entries.length / chunkSize)

  lines.push(`func ${buildName}() map[string]${valueType} {`)
  lines.push(`\tm := make(map[string]${valueType}, ${entries.length})`)
  for (let i = 0; i < chunkCount; i++) lines.push(`\t${registerPrefix}${i}(m)`)
  lines.push('\treturn m')
  lines.push('}')
  lines.push('')

  for (let i = 0; i < chunkCount; i++) {
    lines.push(`func ${registerPrefix}${i}(m map[string]${valueType}) {`)
    for (const e of entries.slice(i * chunkSize, (i + 1) * chunkSize)) {
      lines.push(`\t${renderAssign(e)}`)
    }
    lines.push('}')
    lines.push('')
  }
}

// The verbose output shape (one struct literal per entry) keeps
// version-control diffs readable.
function renderGo({
  propertyRanks,
  utilityRegistry,
  stemRegistry,
  variantRegistry,
  tailwindVersion,
}) {
  const lines = []
  lines.push('// Code generated by scripts/generate-tailwind-registry. DO NOT EDIT.')
  lines.push(`// Source: Tailwind CSS v${tailwindVersion}`)
  lines.push('')
  lines.push('package tailwind')
  lines.push('')

  lines.push('// buildPropertyRanks maps each CSS property to its getClassOrder rank')
  lines.push('// (same scale as utilityRegistry), so an arbitrary property or a custom')
  lines.push('// @utility sorts at the slot a utility emitting that property would.')
  lines.push('func buildPropertyRanks() map[string]int {')
  lines.push('\treturn map[string]int{')
  for (const p of Object.keys(propertyRanks)) {
    lines.push(`\t\t${quote(p)}: ${propertyRanks[p]},`)
  }
  lines.push('\t}')
  lines.push('}')
  lines.push('')

  lines.push('// utilityRegistry maps every known Tailwind v4 utility class name to its')
  lines.push('// getClassOrder rank, on the same scale as buildPropertyRanks. A class')
  lines.push('// not in this map (an arbitrary value, a project theme token) falls')
  lines.push('// through to its stem in stemRegistry.')
  appendChunkedMap(lines, {
    buildName: 'buildUtilityRegistry',
    valueType: 'utilityEntry',
    registerPrefix: 'registerUtilityEntries',
    entries: utilityRegistry,
    renderAssign: (u) => {
      const order = u.order.length === 0 ? 'nil' : `[]int{${u.order.join(', ')}}`
      return `m[${quote(u.name)}] = utilityEntry{Order: ${order}, Count: ${u.count}}`
    },
  })

  lines.push('// stemRegistry maps every utility root (`bg`, `text`, `p`, …) to the')
  lines.push('// earliest rank among its most common shape of class (color wins a tie),')
  lines.push('// with separate color and size ranks for an ambiguous stem. It is the')
  lines.push('// fallback for a class utilityRegistry does not have, and what makes')
  lines.push('// `bg-dark-bg` (a project theme token) sort as background-color the way')
  lines.push('// `bg-red-500` does. Its keys are also the roots the candidate parser')
  lines.push('// splits on by longest prefix (`text-red-500` has root `text`).')
  appendChunkedMap(lines, {
    buildName: 'buildStemRegistry',
    valueType: 'stemEntry',
    registerPrefix: 'registerStemEntries',
    entries: stemRegistry,
    renderAssign: (s) => {
      const order = s.order.length === 0 ? 'nil' : `[]int{${s.order.join(', ')}}`
      const fields = [`Order: ${order}`, `Count: ${s.count}`]
      if (s.colorOrder !== null) {
        fields.push(`ColorOrder: []int{${s.colorOrder.join(', ')}}`)
        fields.push(`ColorCount: ${s.colorCount}`)
      }
      if (s.sizeOrder !== null) {
        fields.push(`SizeOrder: []int{${s.sizeOrder.join(', ')}}`)
        fields.push(`SizeCount: ${s.sizeCount}`)
      }
      return `m[${quote(s.stem)}] = stemEntry{${fields.join(', ')}}`
    },
  })

  lines.push('// variantRegistry holds the sort priority for every static,')
  lines.push('// functional, or compound variant root the v4 engine ships.')
  lines.push('// Arbitrary variants are handled per-instance in the Go runtime.')
  lines.push('func buildVariantRegistry() map[string]variantEntry {')
  lines.push('\treturn map[string]variantEntry{')
  for (const v of variantRegistry) {
    lines.push(
      `\t\t${quote(v.root)}: {Priority: ${v.priority}},`,
    )
  }
  lines.push('\t}')
  lines.push('}')

  return lines.join('\n') + '\n'
}

function quote(s) {
  return JSON.stringify(s)
}

// main is called after every const declaration above has been
// evaluated, so the function body can safely close over
// `propertyOrder` without hitting the temporal dead zone.
await main()
