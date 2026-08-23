---
title: Abort incomplete multipart uploads with a lifecycle rule
type: tool
tags: [s3, multipart, lifecycle, storage-cost]
---

# Parts nobody finished are still stored

### The objects you can enumerate are not the bytes you pay for
**Date**: 2026-01-22
**Source**: https://docs.aws.amazon.com/AmazonS3/latest/userguide/mpu-abort-incomplete-mpu-lifecycle-config.html

An upload that dies between `CreateMultipartUpload` and
`CompleteMultipartUpload` leaves its uploaded parts in the bucket forever.
`ListObjects` does not return them, so the sum of the object sizes disagrees
with the bucket storage metric and no amount of walking the keys explains the
gap. `ListMultipartUploads` is the only view of them, and every part is billed
like any other stored byte until something aborts it.

```json
{
  "Rules": [{
    "ID": "abort-incomplete-multipart",
    "Status": "Enabled",
    "Filter": { "Prefix": "" },
    "AbortIncompleteMultipartUpload": { "DaysAfterInitiation": 7 }
  }]
}
```

Seven days outlasts any legitimate upload and is short enough that a client
crash loop cannot accumulate much. Apply it to every bucket that accepts
uploads, including the ones where nobody wrote multipart code on purpose: most
SDKs switch to multipart above a threshold of a few megabytes without being
asked.
