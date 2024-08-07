---
title: Htsget Storage
menu:
  main:
    parent: Storage
---

# Htsget Storage

Funnel supports content-retrieval from an [Htsget][htsget]-compatible API.
When the received content is encrypted using [Crypt4gh][crypt4gh], Funnel
automatically decrypts the received content (using internally generated
key-pair) so that the executor wouldn't have to.

Htsget is a protocol that enables downloading only specific parts of genomic
data (reads/variants). The first HTTP query receives a JSON that instructs next
HTTP requests for fetching the parts. Finally the parts need to be concatenated
(in the order they were specified) into a single valid file (e.g. VCF or BAM).
Note that the Htsget storage supports only retrieval, and not storing the data!

The task input file URL needs to specify `htsget` as the resource protocol.
Funnel replaces it with the protocol specified in the configuration. The
default protocol is `https`, which is also presumed in the Htsget
specification. For testing purposes, it can be changed to `http`.

If the service expects a `Bearer` token, it can be specified in the URL.
For example: `htsget://bearer:your-token-here@example.org/...`.
Here the `bearer:` part is the required syntax to activate the
`your-token-here` value to be sent to the htsget-service as a header value:
`Authorization: Bearer your-token-here`.

Funnel always sends its public key in the header (`client-public-key`) of the
request to the Htsget service. When the Htsget service supports [the content
encryption using Crypt4gh][htsget-crypt4gh], the service can generate a custom
Crypt4gh file header containing the symmetric key for decrypting the referred
content (Crypt4gh formatted data-blocks). Funnel checks the beginning of the
received content to know whether Crypt4gh decryption can be applied. Therefore,
tasks always receive the data decrypted. For sensitive data, the deployment
environment (server) should pay attention to restricting access to the Funnel's
data directories, possibly having separate Funnel instances for different
data-projects.

Default Htsget Storage configuration should be sufficient for most cases:

```yaml
HTSGETStorage:
  Disabled: false
  Protocol: https
  Timeout: 30s
```

### Example task

```json
{
  "name": "Hello world",
  "inputs": [{
    "url": "htsget://htsget-server/variants/genome2341?referenceName=1&start=10000&end=20000",
    "path": "/inputs/genome.vcf.gz"
  }],
  "outputs": [{
    "url": "file:///results/line_count.txt",
    "path": "/outputs/line_count.txt"
  }],
  "executors": [{
    "image": "alpine",
    "command": [
      "sh",
      "-c",
      "zcat /inputs/genome.vcf.gz | wc -l"
    ],
    "stdout": "/outputs/line_count.txt"
  }]
}
```

[htsget]: https://samtools.github.io/hts-specs/htsget.html
[crypt4gh]: http://samtools.github.io/hts-specs/crypt4gh.pdf
[htsget-crypt4gh]: https://github.com/umccr/htsget-rs/blob/crypt4gh/docs/crypt4gh/ARCHITECTURE.md