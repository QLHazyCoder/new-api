/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { ImageModelCapabilities } from '@/features/playground/types'

export type MediaSampleLanguage =
  | 'curl'
  | 'python'
  | 'typescript'
  | 'javascript'

type MediaSampleContext = {
  baseUrl: string
  endpointPath: string
  modelName: string
  apiKeyEnv: string
}

export function imageSampleBody(
  modelName: string,
  capability?: ImageModelCapabilities
): Record<string, string | number> {
  const body: Record<string, string | number> = {
    model: modelName,
    prompt: 'A serene koi pond at sunset.',
  }
  if (!capability) return body
  if (capability.size_mode === 'dimensions' && capability.sizes.length) {
    body.size = capability.default_size || capability.sizes[0]
  }
  if (capability.size_mode === 'aspect_ratio_resolution') {
    if (capability.aspect_ratios.length) {
      body.aspect_ratio =
        capability.default_aspect_ratio || capability.aspect_ratios[0]
    }
    if (capability.resolutions.length) {
      body.resolution =
        capability.default_resolution || capability.resolutions[0]
    }
  }
  if (capability.qualities.length) {
    body.quality = capability.default_quality || capability.qualities[0]
  }
  if (capability.output_formats.length) {
    body.output_format =
      capability.default_output_format || capability.output_formats[0]
  }
  body.n = 1
  return body
}

function postSample(
  lang: MediaSampleLanguage,
  ctx: MediaSampleContext,
  body: Record<string, string | number>
): string {
  const url = `${ctx.baseUrl}${ctx.endpointPath}`
  const bodyJson = JSON.stringify(body, null, 2)
  if (lang === 'curl') {
    return [
      `curl ${url} \\`,
      `  -H "Authorization: Bearer $${ctx.apiKeyEnv}" \\`,
      '  -H "Content-Type: application/json" \\',
      `  -d '${bodyJson.replaceAll('\n', '\n     ')}'`,
    ].join('\n')
  }
  if (lang === 'python') {
    return [
      'import json',
      'import os',
      'from urllib.request import Request, urlopen',
      '',
      `url = "${url}"`,
      `body = ${bodyJson}`,
      'request = Request(url, data=json.dumps(body).encode(), headers={',
      `    "Authorization": "Bearer " + os.environ["${ctx.apiKeyEnv}"],`,
      '    "Content-Type": "application/json",',
      '})',
      'with urlopen(request) as response:',
      '    result = json.load(response)',
      'print(result)',
    ].join('\n')
  }
  return [
    `const response = await fetch('${url}', {`,
    "  method: 'POST',",
    '  headers: {',
    `    Authorization: 'Bearer ' + process.env.${ctx.apiKeyEnv},`,
    "    'Content-Type': 'application/json',",
    '  },',
    `  body: JSON.stringify(${bodyJson}),`,
    '})',
    'if (!response.ok) throw new Error(await response.text())',
    'console.log(await response.json())',
  ].join('\n')
}

export function buildImageSample(
  lang: MediaSampleLanguage,
  ctx: MediaSampleContext,
  capability?: ImageModelCapabilities
): string {
  return postSample(lang, ctx, imageSampleBody(ctx.modelName, capability))
}

export function buildVideoSample(
  lang: MediaSampleLanguage,
  ctx: MediaSampleContext
): string {
  const create = postSample(lang, ctx, {
    model: ctx.modelName,
    prompt: 'A short cinematic shot of waves arriving at a lighthouse.',
  })
  const base = `${ctx.baseUrl}/v1/videos/`
  if (lang === 'curl') {
    return [
      `VIDEO_ID=$(${create.replace(/^curl /, 'curl -fsS ')} | jq -er .id) || exit 1`,
      'while :; do',
      `  STATUS=$(curl -fsS -H "Authorization: Bearer $${ctx.apiKeyEnv}" "${base}$VIDEO_ID" | jq -er .status) || exit 1`,
      '  case "$STATUS" in completed) break ;; failed|canceled) exit 1 ;; esac',
      '  sleep 5',
      'done',
      `curl -fL -H "Authorization: Bearer $${ctx.apiKeyEnv}" "${base}$VIDEO_ID/content" -o video.mp4`,
    ].join('\n')
  }
  if (lang === 'python') {
    return [
      create.replace('print(result)', 'job = result'),
      'import time',
      'while job["status"] not in ("completed", "failed", "canceled"):',
      '    time.sleep(5)',
      `    request = Request("${base}" + job["id"], headers={`,
      `        "Authorization": "Bearer " + os.environ["${ctx.apiKeyEnv}"],`,
      '    })',
      '    with urlopen(request) as response:',
      '        job = json.load(response)',
      'if job["status"] != "completed":',
      '    raise RuntimeError(job)',
      `request = Request("${base}" + job["id"] + "/content", headers={`,
      `    "Authorization": "Bearer " + os.environ["${ctx.apiKeyEnv}"],`,
      '})',
      'with urlopen(request) as response, open("video.mp4", "wb") as output:',
      '    output.write(response.read())',
    ].join('\n')
  }
  return [
    "import { writeFile } from 'node:fs/promises'",
    '',
    create.replace(
      'console.log(await response.json())',
      'let job = await response.json()'
    ),
    "while (!['completed', 'failed', 'canceled'].includes(job.status)) {",
    '  await new Promise((resolve) => setTimeout(resolve, 5000))',
    `  const status = await fetch('${base}' + job.id, {`,
    `    headers: { Authorization: 'Bearer ' + process.env.${ctx.apiKeyEnv} },`,
    '  })',
    '  if (!status.ok) throw new Error(await status.text())',
    '  job = await status.json()',
    '}',
    "if (job.status !== 'completed') throw new Error(JSON.stringify(job))",
    `const content = await fetch('${base}' + job.id + '/content', {`,
    `  headers: { Authorization: 'Bearer ' + process.env.${ctx.apiKeyEnv} },`,
    '})',
    'if (!content.ok) throw new Error(await content.text())',
    "await writeFile('video.mp4', Buffer.from(await content.arrayBuffer()))",
  ].join('\n')
}
