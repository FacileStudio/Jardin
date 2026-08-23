---
title: Key accounts on the OIDC subject, never on the email
type: convention
tags: [oidc, identity, auth, email]
related: [conventions/backfill-a-new-key-column-before-you-read-it.md]
---

# Email is not a stable identity

### An email address is a display attribute, not a primary key
**Date**: 2026-03-29
**Source**: direct observation plus OpenID Connect Core section 5.7

Upserting a user row keyed on the email claim looks obvious and is wrong in two
directions. A person who changes their address becomes a second account with an
empty history. Worse, a provider that recycles addresses inside an organisation
hands the new holder the previous holder's account, which is a silent takeover
that no audit log will show as suspicious.

The stable identifier is the pair `iss` and `sub`. The subject is opaque, never
reassigned and never reused, which is exactly what a primary key needs.
Store it on first sign-in and treat the email as a mutable attribute you refresh
on every login. Adopting an existing row by matching the address is only
defensible when the provider also asserts `email_verified`, and several common
providers hardcode that claim to false.

