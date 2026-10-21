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
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { useEffect, useRef, useState, type RefObject } from 'react'

import type { DocumentationHeading } from './markdown'

interface ArticleNavigationOptions {
  articleRef: RefObject<HTMLElement | null>
  headings: readonly DocumentationHeading[]
  slug: string
}

export function useArticleNavigation(options: ArticleNavigationOptions) {
  const navigate = useNavigate()
  // TanStack Router has already decoded the hash, including Chinese headings.
  const hash = useRouterState({ select: (state) => state.location.hash })
  const lastJump = useRef<string | null>(null)
  const [activeHeadingId, setActiveHeadingId] = useState<string | null>(null)

  useEffect(() => {
    lastJump.current = null
    const article = options.articleRef.current
    const sections = options.headings
      .filter((heading) => heading.depth === 2 || heading.depth === 3)
      .map((heading) => document.getElementById(heading.id))
      .filter((element): element is HTMLElement =>
        Boolean(element && article?.contains(element))
      )
    setActiveHeadingId(sections[0]?.id ?? null)
    if (!sections.length) return

    let frame: number | null = null
    const updateActiveSection = () => {
      frame = null
      let active = sections[0]
      for (const section of sections) {
        if (section.getBoundingClientRect().top > 104) break
        active = section
      }
      const pageHeight = document.documentElement.scrollHeight
      if (
        pageHeight > window.innerHeight &&
        window.scrollY + window.innerHeight >= pageHeight - 2
      ) {
        // The last section can be too short to reach the top of the viewport.
        active = sections.at(-1) ?? active
      }
      setActiveHeadingId(active.id)
    }
    const scheduleUpdate = () => {
      if (frame === null) {
        frame = window.requestAnimationFrame(updateActiveSection)
      }
    }
    const observer =
      typeof IntersectionObserver === 'undefined'
        ? null
        : new IntersectionObserver(scheduleUpdate, {
            rootMargin: '-96px 0px 0px 0px',
            threshold: [0, 1],
          })
    sections.forEach((section) => observer?.observe(section))
    // Also track long sections and large scroll jumps between headings.
    window.addEventListener('scroll', scheduleUpdate, { passive: true })
    window.addEventListener('resize', scheduleUpdate)

    return () => {
      observer?.disconnect()
      window.removeEventListener('scroll', scheduleUpdate)
      window.removeEventListener('resize', scheduleUpdate)
      if (frame !== null) window.cancelAnimationFrame(frame)
    }
  }, [options.articleRef, options.headings, options.slug])

  useEffect(() => {
    const jumpKey = `${options.slug}#${hash}`
    if (!hash) return
    const article = options.articleRef.current
    const target = document.getElementById(hash)
    if (!target || !article?.contains(target)) return

    let disposed = false
    let frame: number | null = null
    const jumpToSection = () => {
      frame = null
      if (disposed) return
      lastJump.current = jumpKey
      target.scrollIntoView({ block: 'start', behavior: 'instant' })
      setActiveHeadingId(hash)
    }
    const scheduleJump = () => {
      if (disposed) return
      if (frame !== null) window.cancelAnimationFrame(frame)
      frame = window.requestAnimationFrame(jumpToSection)
    }
    const cancelRestore = () => {
      disposed = true
      if (frame !== null) window.cancelAnimationFrame(frame)
    }
    const pendingImages = [...article.querySelectorAll('img')].filter(
      (image) => !image.complete
    )
    pendingImages.forEach((image) => {
      image.addEventListener('load', scheduleJump, { once: true })
      image.addEventListener('error', scheduleJump, { once: true })
    })
    if (document.fonts?.status === 'loading') {
      void document.fonts.ready.then(scheduleJump, scheduleJump)
    }
    if (lastJump.current !== jumpKey) scheduleJump()

    // Correct a deep link after external images/font metrics settle, but never
    // pull the reader back after they start interacting with the page.
    const interactionEvents = [
      'wheel',
      'touchstart',
      'pointerdown',
      'keydown',
    ] as const
    interactionEvents.forEach((event) =>
      window.addEventListener(event, cancelRestore, { passive: true })
    )
    return () => {
      cancelRestore()
      interactionEvents.forEach((event) =>
        window.removeEventListener(event, cancelRestore)
      )
      pendingImages.forEach((image) => {
        image.removeEventListener('load', scheduleJump)
        image.removeEventListener('error', scheduleJump)
      })
    }
  }, [hash, options.articleRef, options.headings, options.slug])

  const selectHeading = (id: string) => {
    const target = document.getElementById(id)
    if (!target || !options.articleRef.current?.contains(target)) return

    lastJump.current = `${options.slug}#${id}`
    target.focus({ preventScroll: true })
    target.scrollIntoView({
      block: 'start',
      behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches
        ? 'instant'
        : 'smooth',
    })
    setActiveHeadingId(id)
    void navigate({
      to: '/docs/$slug',
      params: { slug: options.slug },
      hash: id,
      hashScrollIntoView: false,
      resetScroll: false,
    })
  }

  return { activeHeadingId, selectHeading }
}
