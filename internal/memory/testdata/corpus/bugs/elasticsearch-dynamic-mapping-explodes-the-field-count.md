---
title: Dynamic mapping turns each new JSON key into a permanent field
type: bug
tags: [elasticsearch, mapping, cluster-state, indexing]
---

# Mapping explosion

### Identifiers belong in values, never in field names
**Date**: 2026-04-27
**Source**: direct observation, "Limit of total fields [1000] has been exceeded"

The documents carried a `metrics` object keyed by customer id. Every new
customer added a field to the index mapping, mappings live in the cluster
state, and the cluster state is republished to every node on every change. The
master node spent its time distributing a mapping that grew by a few hundred
fields an hour, and search latency climbed with it, until
`index.mapping.total_fields.limit` started rejecting writes and made the cause
visible.

Raising the limit buys a few hours and nothing else. The shapes that actually
fix it are the `flattened` type, which stores an object of arbitrary keys as a
single field, or rewriting the object as an array of key and value pairs so the
keys become searchable values, or `dynamic: strict` so an unmapped field is a
loud error at write time rather than a slow leak into the cluster state.
Reindexing is required either way, so decide before the index is large.
