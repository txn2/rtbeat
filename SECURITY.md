# Security Policy

## Supported Versions

| Version | Supported |
| ------- | --------- |
| 1.x     | Yes       |
| < 1.0   | No        |

Security fixes are applied to the latest minor release. Older lines are not
backported; upgrade to the current release to receive patches.

## Supply Chain Security

rtbeat is built with `CGO_ENABLED=0` and released through [GoReleaser](https://goreleaser.com)
from a tagged commit on GitHub Actions. Every release archive is signed,
accompanied by build provenance, and shipped with a Software Bill of Materials.

### Signed Releases

All release artifacts are signed using [Cosign](https://github.com/sigstore/cosign)
with keyless signing (Sigstore). Each archive and the checksums file have a
companion `.sigstore.json` bundle that contains the signature, the certificate,
and the transparency-log entry. No public key is needed to verify; the signing
identity is the GitHub Actions workflow itself.

You will need [Cosign](https://docs.sigstore.dev/cosign/installation/) installed.

Archive names follow the pattern `rtbeat_<Os>_<Arch>.tar.gz`, for example
`rtbeat_Linux_x86_64.tar.gz`, `rtbeat_Darwin_arm64.tar.gz`.

#### Verify the checksums file

```bash
# Download the checksums file and its Sigstore bundle for the desired version.
curl -LO https://github.com/txn2/rtbeat/releases/download/{VERSION}/rtbeat_checksums.txt
curl -LO https://github.com/txn2/rtbeat/releases/download/{VERSION}/rtbeat_checksums.txt.sigstore.json

# Verify the signature on the checksums file.
cosign verify-blob \
  --bundle rtbeat_checksums.txt.sigstore.json \
  --certificate-identity-regexp "https://github.com/txn2/rtbeat" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  rtbeat_checksums.txt
```

#### Verify an individual archive

```bash
# Download an archive and its Sigstore bundle (adjust OS/Arch as needed).
curl -LO https://github.com/txn2/rtbeat/releases/download/{VERSION}/rtbeat_Linux_x86_64.tar.gz
curl -LO https://github.com/txn2/rtbeat/releases/download/{VERSION}/rtbeat_Linux_x86_64.tar.gz.sigstore.json

# Verify the signature on the archive itself.
cosign verify-blob \
  --bundle rtbeat_Linux_x86_64.tar.gz.sigstore.json \
  --certificate-identity-regexp "https://github.com/txn2/rtbeat" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  rtbeat_Linux_x86_64.tar.gz

# Once the checksums file is verified, you can also confirm the archive
# matches its recorded checksum.
sha256sum --ignore-missing -c rtbeat_checksums.txt
```

A successful verification prints `Verified OK`. If verification fails, do not
use the artifact.

#### Container images

Container images published to `txn2/rtbeat` are signed with Cosign keyless
signing as well:

```bash
cosign verify \
  --certificate-identity-regexp "https://github.com/txn2/rtbeat" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  txn2/rtbeat:{VERSION}
```

### SLSA Provenance

Releases include build provenance attestations describing how and where the
artifacts were produced. This provides a verifiable, tamper-evident record that
the binaries were built from this repository using the documented GitHub Actions
release workflow, consistent with the [SLSA](https://slsa.dev) framework.

### Software Bill of Materials (SBOM)

Each release archive is accompanied by an SBOM generated at build time. The SBOM
enumerates every dependency that goes into rtbeat, including the pinned Elastic
libbeat tree, enabling downstream vulnerability scanning and license compliance
review.

## Automated Security Scanning

The CI and release pipelines run the following checks:

- **CodeQL** — static analysis on every pull request and push to detect
  common vulnerability patterns in Go code.
- **OpenSSF Scorecard** — continuous evaluation of the repository's
  supply-chain security posture.
- **Dependabot** — automated dependency and GitHub Actions update proposals
  for security patches.
- **govulncheck** — scans against the Go vulnerability database for known
  issues reachable from rtbeat's code.
- **golangci-lint** — static analysis with a security-focused linter set.
- **go test -race** — the test suite runs under the race detector.

## Reporting a Vulnerability

Please report security vulnerabilities privately. Do not open a public issue
for a suspected vulnerability.

Email the maintainer at <cjimti@gmail.com>, or use GitHub's
[private vulnerability reporting](https://github.com/txn2/rtbeat/security/advisories/new)
on this repository.

Please include:

- A description of the vulnerability and its potential impact
- Steps to reproduce, or a proof of concept
- Affected version(s) and configuration details, if relevant

You can expect an initial response within 72 hours. We will keep you informed
as we investigate and work toward a fix, and we will coordinate disclosure
timing with you.
