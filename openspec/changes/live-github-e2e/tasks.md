# Tasks

- [x] Parameterize the existing Go harness for a real repository and PR.
- [x] Add scoped API transport, independent App identities, and cleanup.
- [x] Add assertions for create, denial, review, stale approval, changes requested, update, and deletion.
- [x] Add the trusted manual workflow and document configuration names.
- [x] Run local regression, transport tests, vet, Actionlint, and strict spec validation.
- [ ] Run the live workflow after its credentials are accessible.

## Validation

- All 24 local CLI scenarios and six relay cases passed with the race detector; Go vet and compilation passed.
- Actionlint 1.7.12 and strict OpenSpec validation passed.
- The existing Local E2E workflow passed on GitHub for commit `19341e0`; the live lane has not run.
