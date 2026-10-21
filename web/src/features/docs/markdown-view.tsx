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
import { useMemo } from 'react'

import { Markdown } from '@/components/ui/markdown'

import {
  type DocumentationHeading,
  extractDocumentationHeadings,
  isSecureDocsImageURL,
  normalizeStandaloneDocsImages,
} from './markdown'

export function DocsMarkdown(props: {
  markdown: string
  headings?: readonly DocumentationHeading[]
}) {
  const markdown = useMemo(
    () => normalizeStandaloneDocsImages(props.markdown),
    [props.markdown]
  )
  const headingIds = useMemo(
    () =>
      (props.headings ?? extractDocumentationHeadings(markdown)).map(
        (heading) => heading.id
      ),
    [markdown, props.headings]
  )
  return (
    <Markdown
      allowHtml={false}
      enableCodeCopy
      headingIds={headingIds}
      imageUrlFilter={isSecureDocsImageURL}
      className='[&_:is(h1,h2,h3,h4,h5,h6)]:scroll-mt-24 [&_img]:h-auto [&_p]:leading-7 [&_pre]:max-w-full [&_table]:max-w-full'
    >
      {markdown}
    </Markdown>
  )
}
