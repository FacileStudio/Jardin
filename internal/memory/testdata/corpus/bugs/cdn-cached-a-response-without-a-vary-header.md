---
title: A response cached without Vary is served to the wrong client
type: bug
tags: [cdn, cache, vary, http]
---

# Cache keys and request headers

### The cache key is the URL until a header says otherwise
**Date**: 2026-05-30
**Source**: direct observation, a French page served to English readers

The origin chose the language from `Accept-Language` and returned the page with
no `Vary` header at all. The edge stored whichever version it saw first under
the URL and served that to everyone for an hour. The same shape shows up with
`Accept-Encoding`, where a client that cannot decompress receives a gzip body,
and it is at its worst with authentication, where one signed in user is handed
the page rendered for the previous one.

`Vary: Accept-Encoding, Accept-Language` names the request headers that change
the response, and the cache keeps one entry per combination it sees. That is
also why `Vary: User-Agent` is a mistake: it shards the cache into thousands of
near identical entries with a hit rate close to zero. A per user response is
not a `Vary` problem at all, it is `Cache-Control: private, no-store`, because
a shared cache should never hold it in the first place.
