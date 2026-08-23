---
title: A presigned URL cannot outlive the credential that signed it
type: convention
tags: [s3, presigned-url, sts, expiry]
---

# Presigned URL lifetime

### The expiry you ask for is a ceiling, not a promise
**Date**: 2026-07-03
**Source**: direct observation plus the SigV4 query parameter authentication reference

A seven day download link began answering `ExpiredToken` about an hour after it
was handed out. The signer ran with temporary credentials from an instance
role, and a signature made with a session token is valid only while that
session token is, whatever `X-Amz-Expires` claims. SigV4 caps the requested
window at seven days; a temporary credential usually caps it far lower and says
nothing at signing time.

Either sign with a credential that outlives the link or keep the link shorter
than the session, and prefer the second. A presigned URL is a bearer
credential: whoever holds the string has the access it encodes, with no further
check on who they are. Keep them out of logs and error reports, do not embed
them in a page that links outward where they leak through `Referer`, and choose
minutes over days so a leaked one expires before anyone finds it.
