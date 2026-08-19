# Security Policy

orascout is a daemon that typically runs as root and applies filesystem
changes driven by registry content. Security reports are taken seriously.

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

Use GitHub's private vulnerability reporting:
**Security tab → Report a vulnerability** on this repository. Reports go
only to the maintainers, and coordinated disclosure can happen from there.

Please include: the orascout version, a minimal reproduction (manifest
annotations and config involved), and what an attacker gains.

This is a volunteer-maintained project — acknowledgement is best-effort,
usually within a few days.

## Supported versions

Only the latest release receives security fixes.

## Scope notes (please read before reporting)

orascout's trust model is documented in [SPEC.md §10](SPEC.md). In short:

* **In scope:** anything that lets a pushed artifact write or delete
  outside the operator's `allowed_target_roots`, bypass the built-in
  denylist of system paths, escape the pulled-artifact directory via
  `source.*`/`hook.*` paths, or smuggle host files into a deploy target
  (e.g. symlink tricks). Also anything that lets a *non-watched* party
  influence deploys (registry response parsing, auth handling, state-file
  tampering).
* **By design, not vulnerabilities:** `hook.pre`, `hook.post`,
  `runonce.command`, and `healthcheck.cmd` execute commands from the
  artifact on purpose. Push access to a watched repo is documented as a
  highly privileged capability; reports that amount to "someone with push
  access can run code" are the documented trust model, not a bypass.
