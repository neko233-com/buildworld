# BuildWorld contributor instructions

Read and follow [Rule.md](Rule.md) before modifying this repository. It is the
authoritative engineering and verification policy for all automated agents.

Release work must preserve the installer contract in Rule.md: all supported
platforms receive a locally built bundle, one-click installers keep the CLI on
the user environment PATH, and no binary packaging is delegated to Actions.

Before creating or uploading any release bundle, complete all applicable test
suites and required production checks. Packaging and upload are forbidden while
verification is incomplete or failing.
