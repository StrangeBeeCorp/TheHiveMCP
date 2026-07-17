#!/usr/bin/env bash
#
# Sign Windows .exe binaries in place. Mechanism-agnostic: the signing backend is
# selected by SIGNING_PROVIDER so we can run SignPath Foundation and Azure Artifact
# Signing onboarding in parallel and switch between them with a single env var, with
# no other change to the release pipeline (see ADR 0002).
#
# Usage:   scripts/sign-windows.sh <binary> [<binary> ...]
#
# Env:
#   SIGNING_PROVIDER   signpath | azure | none   (default: none)
#
#   When SIGNING_PROVIDER=azure (Azure Artifact Signing / Trusted Signing):
#     AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET   service-principal creds
#     AZURE_SIGNING_ENDPOINT       e.g. https://weu.codesigning.azure.net
#     AZURE_SIGNING_ACCOUNT        Artifact Signing account name
#     AZURE_SIGNING_PROFILE        certificate profile name (PublicTrust or PublicTrustTest)
#
#   When SIGNING_PROVIDER=signpath (SignPath Foundation):
#     Signing is performed by the SignPath GitHub Action (PR-Mode/manual approval),
#     NOT by this script — the .exe files are uploaded as an artifact and signed
#     versions are downloaded back. In that flow this script is a no-op that simply
#     validates the files exist. See release.yml for the action wiring.
#
#   SIGNING_PROVIDER=none (default): no-op. Produces UNSIGNED binaries. Used for
#     local builds, for proving the pipeline before a signing account exists, and
#     for the interim unsigned release (DL-6329) that ships while the signing
#     identity reviews are pending. The signed end state remains the target (ADR
#     0002 + its DL-6329 reversal note); a real provider should be set once one
#     clears.

set -euo pipefail

PROVIDER="${SIGNING_PROVIDER:-none}"

if [[ "$#" -eq 0 ]]; then
  echo "sign-windows: no binaries passed; nothing to sign." >&2
  exit 0
fi

# Verify every target exists before doing anything.
for bin in "$@"; do
  if [[ ! -f "$bin" ]]; then
    echo "sign-windows: error: binary not found: $bin" >&2
    exit 1
  fi
done

case "$PROVIDER" in
  none)
    echo "sign-windows: SIGNING_PROVIDER=none — leaving binaries UNSIGNED:"
    for bin in "$@"; do echo "  (unsigned) $bin"; done
    echo "sign-windows: NOTE — publishing UNSIGNED binaries (interim, DL-6329)." \
      "Set a real SIGNING_PROVIDER once a signing path clears (ADR 0002)." >&2
    ;;

  signpath)
    # SignPath Foundation signs via its GitHub Action with manual approval; the
    # actual signing happens out-of-band in the workflow, not here. This branch
    # exists so the Makefile target is provider-uniform.
    echo "sign-windows: SIGNING_PROVIDER=signpath — signing handled by the SignPath"
    echo "             GitHub Action in release.yml (upload -> approve -> download)."
    echo "             This script verified the following exist:"
    for bin in "$@"; do echo "  $bin"; done
    ;;

  azure)
    # Azure Artifact Signing via the Trusted Signing CLI (dotnet sign / sign tool).
    : "${AZURE_SIGNING_ENDPOINT:?set AZURE_SIGNING_ENDPOINT}"
    : "${AZURE_SIGNING_ACCOUNT:?set AZURE_SIGNING_ACCOUNT}"
    : "${AZURE_SIGNING_PROFILE:?set AZURE_SIGNING_PROFILE}"

    if ! command -v sign >/dev/null 2>&1; then
      echo "sign-windows: installing 'sign' (dotnet sign) tool..."
      dotnet tool install --global sign --version '0.9.1-*' >/dev/null
      export PATH="$PATH:$HOME/.dotnet/tools"
    fi

    for bin in "$@"; do
      echo "sign-windows: signing $bin via Azure Artifact Signing ($AZURE_SIGNING_PROFILE)..."
      sign code trusted-signing "$bin" \
        --trusted-signing-endpoint "$AZURE_SIGNING_ENDPOINT" \
        --trusted-signing-account "$AZURE_SIGNING_ACCOUNT" \
        --trusted-signing-certificate-profile "$AZURE_SIGNING_PROFILE" \
        --timestamp-url "http://timestamp.acs.microsoft.com" \
        --file-digest SHA256
    done
    echo "sign-windows: Azure signing complete."
    ;;

  *)
    echo "sign-windows: error: unknown SIGNING_PROVIDER='$PROVIDER' (expected: signpath | azure | none)" >&2
    exit 1
    ;;
esac
