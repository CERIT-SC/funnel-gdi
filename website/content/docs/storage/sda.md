---
gdi: true
title: SDA Storage
menu:
  main:
    parent: Storage
---

# Sensitive Data Archive (SDA) Storage

Funnel supports content-retrieval from an [SDA-Download][sda]-compatible API.
This is a service with an HTTP-based REST-API with some additions:

1. The request must be authenticated using a Bearer token, a JSON Web Token to
   be validated by SDA, checking it would contain a valid GA4GH Visa permitting
   access to the targeted dataset.
2. If the targeted file is encrypted using [Crypt4gh][crypt4gh] (it has ".c4gh"
   extension), the client (e.g. Funnel) needs to send its public key so that
   SDA would reencrypt the file header, which would enable to obtain the cipher
   key from the header using its private key. Funnel makes the file accessible
   to the computation task without encryption.

The task input file URL needs to specify `sda` as the resource protocol.
Funnel will extract path information from the specified URL and append it to
the service URL specified in the Funnel configuration. The format of the input
data URL is following:

```
sda://<dataset-id>/<resource/path>
```

For example: `sda://DATASET_2000/synthetic/sample.bam`

A `Bearer` token for the SDA service can be specified at the end of the URL
right after the hash-sign (`#`). For example: `sda://dataset/file#jwt-token-here`.
The value is always sent as `Authorization: Bearer <value>`; unlike the Htsget
storage, `Basic` (username:password) authentication is not supported.
Note that when the task is submitted to Funnel using a valid `Bearer` token for
user authentication, the same token will be automatically appended to the
SDA URL, so the request to the SDA service would use the same token.
Exception is when the URL already specifies the hash-sign (`#`) – then the
provided value won't be replaced. A task with an `sda://` input and no token
(neither in the URL nor from the request) is rejected when it is submitted.

> [!WARNING]
>
> The token is stored as part of the input URL: it is returned with the task
> (`view=FULL`, web dashboard) and written to the task's system logs and the
> server log (`download started` messages). Use `Server.TaskAccess: Owner` or
> `OwnerOrAdmin` so that users cannot read each other's tasks, and restrict
> access to the logs.

Funnel sends its Crypt4gh public key in the header (`client-public-key`) of the
request to the SDA service, when the requested file has ".c4gh" extension.

For sensitive data, the deployment environment (server) should pay attention to
restricting access to the Funnel's data directories, possibly having separate
Funnel instances for different data-projects.

SDA Storage configuration just requires a service URL to become active:

```yaml
SDAStorage:
  ServiceURL: https://example.org:8443/sda/
  Timeout:
    duration: 30s
```

If the `ServiceUrl` is undefined, `sda` protocol will be disabled.

Funnel will automatically append `/s3/<provided-path>` or
`/s3-encrypted/<provided-path>` to the service URL, depending on whether the
requested file has the Crypt4GH file-extension (`.c4gh`).

### About Crypt4GH Keys

Funnel loads Crypt4gh keys from files, or generates and saves them when the
files cannot be resolved.

First, Funnel tries to resolve the public and secret key file-paths from
environment variables:

- `C4GH_SECRET_KEY` – path to the secret/private key
- `C4GH_PUBLIC_KEY` (optional) – path to public key,
- `C4GH_PASSPHRASE` (optional) – password of the secret/private key.

Notes:

- If `C4GH_PUBLIC_KEY` is provided and the file exists, it must
  cryptographically pair with the secret key.
- If `C4GH_SECRET_KEY` refers to an unencrypted secret key, `C4GH_PASSPHRASE`
  may be omitted.
- When the file of `C4GH_SECRET_KEY` does not exist yet, a new key-pair will
  be generated and stored in the specified files (the secret key will be
  encrypted with `C4GH_PASSPHRASE`, if present).

When `C4GH_SECRET_KEY` is not declared, the keys are looked up in a directory
(the secret key in the file `key`, the public key in `key.pub`):

1. `.c4gh/` in the current directory, if that directory exists;
2. otherwise `~/.c4gh/` in the home directory (created if missing);
   `.c4gh/` in the current directory is used only when the home directory
   cannot be determined.

If the secret key does not exist there, a new key-pair is generated and saved
in that directory. If the directory cannot be created, Funnel uses a key-pair
generated in memory, which is not saved.

The Helm chart (`deploy-guide/kubernetes`) sets `C4GH_SECRET_KEY=/keys/key`
and `C4GH_PUBLIC_KEY=/keys/key.pub` on an `emptyDir` volume, so every task
generates its own key-pair.

### Example task

```json
{
  "name": "Hello world",
  "inputs": [
    {
      "url": "sda://DATASET-2024-012345/variants/genome2341.vcf.gz",
      "path": "/inputs/genome.vcf.gz"
    }
  ],
  "outputs": [
    {
      "url": "s3://my-bucket/results/line_count.txt",
      "path": "/outputs/line_count.txt"
    }
  ],
  "executors": [
    {
      "image": "alpine",
      "command": ["sh", "-c", "zcat /inputs/genome.vcf.gz | wc -l"],
      "stdout": "/outputs/line_count.txt"
    }
  ]
}
```

The output goes to S3 because `file://` output URLs work only for paths under
`LocalStorage.AllowedDirs` and, on Kubernetes, are not kept after the task (see
[Storage](/gdi/deployment/#storage) in the deployment guide).

[sda]: https://github.com/neicnordic/sensitive-data-archive/blob/main/sda-download/api/api.md
[crypt4gh]: http://samtools.github.io/hts-specs/crypt4gh.pdf
