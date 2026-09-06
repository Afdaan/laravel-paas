/**
 * Scroll an element into view inside the dashboard's `<main>` scroller.
 *
 * `Element.scrollIntoView()` walks up *every* scrollable ancestor and scrolls
 * each one. `overflow: hidden` does not opt an element out of that — it only
 * hides the scrollbar; the box is still programmatically scrollable. The shell
 * wraps `<main>` in two such boxes (`flex h-screen overflow-hidden` and
 * `flex-1 flex flex-col overflow-hidden`), so scrollIntoView scrolls those too
 * and drags the sidebar and header off-screen, leaving the page looking cropped
 * with no scrollbar to undo it.
 *
 * Scrolling the one real container instead leaves the shell where it belongs.
 */
export function scrollIntoMain(elementId: string, offset = 24) {
  const el = document.getElementById(elementId)
  if (!el) return
  const main = document.getElementById('main-content')
  if (!main || typeof main.scrollTo !== 'function') {
    el.scrollIntoView?.({ behavior: 'smooth' })
    return
  }
  const top = main.scrollTop + el.getBoundingClientRect().top - main.getBoundingClientRect().top - offset
  main.scrollTo({ top, behavior: 'smooth' })
}
