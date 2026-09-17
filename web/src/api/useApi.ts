import { useEffect, useState } from 'react'
import { api } from './client'

interface FetchState<T> {
  data: T | undefined
  error: string | null
  loading: boolean
}

// useApiGet is a minimal data-fetching hook — no caching/retry policy is
// needed at this scale. reload lets callers refresh after a mutation
// (e.g. after triggering a workflow) without a full page reload.
//
// The loading flag flips to true synchronously during render when the
// request key changes (path/deps/nonce), following React's documented
// "store information from previous renders" pattern (using useState, not a
// ref, since refs must not be read/written during render) — this keeps the
// effect itself free of a leading setState call.
export function useApiGet<T>(
  path: string | null,
  deps: unknown[] = [],
): FetchState<T> & { reload: () => void } {
  const [state, setState] = useState<FetchState<T>>({
    data: undefined,
    error: null,
    loading: path !== null,
  })
  const [nonce, setNonce] = useState(0)

  const requestKey = JSON.stringify([path, nonce, ...deps])
  const [lastKey, setLastKey] = useState<string | null>(null)
  if (lastKey !== requestKey) {
    setLastKey(requestKey)
    if (path !== null && !state.loading) {
      setState((s) => ({ ...s, loading: true, error: null }))
    }
  }

  useEffect(() => {
    if (path === null) return
    let cancelled = false
    api
      .get<T>(path)
      .then((data) => {
        if (!cancelled) setState({ data, error: null, loading: false })
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setState({
            data: undefined,
            error: err instanceof Error ? err.message : 'Request failed',
            loading: false,
          })
        }
      })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path, nonce, ...deps])

  return { ...state, reload: () => setNonce((n) => n + 1) }
}
