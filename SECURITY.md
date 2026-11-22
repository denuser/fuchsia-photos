# Security policy

## Supported versions

Only the latest release gets security fixes. Update to it before reporting, if you can.

## Reporting a vulnerability

Please do not open a public issue for a security problem. Use GitHub's private reporting instead: open the [Security tab](https://github.com/denuser/fuchsia-photos/security/advisories/new) of the repository and choose **Report a vulnerability**.

Describe what you found, the version or image tag, and the steps to reproduce it. This is a one-person project, so a first reply may take a few days. A confirmed problem is fixed in a new release, and the report is credited in its notes unless you prefer otherwise.

## What to keep in mind when running it

- Fuchsia Photos is built for a home network. Do not expose it to the internet without HTTPS, and keep the administrator's password and the WebDAV app passwords long.
- There is no self-registration: the administrator creates every account.
- Photos are stored unencrypted on the NAS, so protect the disk and the backups the way you would the photos themselves.
