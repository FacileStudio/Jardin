---
title: Refresh token rotation breaks concurrent requests
type: bug
tags: [oauth, token, refresh, auth]
---

# Refresh token rotation race

### Two requests refreshing at once invalidate each other
**Date**: 2026-01-23
**Source**: direct observation, users logged out at random

With rotation enabled the authorization server issues a new refresh token on
every use and revokes the old one. A client that fires three API calls in
parallel and refreshes on the first 401 sends the same refresh token three
times: the first exchange succeeds, and the other two present a token the server
has just revoked, which it treats as replay and responds to by invalidating the
whole token family. The user is signed out with no error anyone can reproduce on
demand.

The fix is a single-flight refresh: one in-flight exchange, with every other
caller awaiting the same promise rather than starting its own. Retry storms
after a network blip cause the same failure, so the deduplication has to live
below the retry layer, not above it.
