# Security Policy

## Supported Versions

We are currently supporting the following versions with security updates:

| Version | Supported          |
| ------- | ------------------ |
| main    | :white_check_mark: |
| develop | :white_check_mark: |

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, please report security vulnerabilities by emailing **security@loqalabs.com**.

You should receive a response within 48 hours. If for some reason you do not, please follow up via email to ensure we received your original message.

Please include the following information in your report (as much as you can provide to help us understand the nature and scope of the possible issue):

* Type of issue (e.g. buffer overflow, SQL injection, cross-site scripting, etc.)
* Full paths of source file(s) related to the manifestation of the issue
* The location of the affected source code (tag/branch/commit or direct URL)
* Any special configuration required to reproduce the issue
* Step-by-step instructions to reproduce the issue
* Proof-of-concept or exploit code (if possible)
* Impact of the issue, including how an attacker might exploit the issue

This information will help us triage your report more quickly.

## Preferred Languages

We prefer all communications to be in English.

## Security Response Process

1. **Report received**: We acknowledge receipt of your vulnerability report within 48 hours.

2. **Initial triage**: We will investigate and confirm the vulnerability within 5 business days.

3. **Fix development**: We work on developing a fix. Complex issues may take longer.

4. **Coordinated disclosure**: We will work with you to coordinate disclosure timing.

5. **Release**: We release the security fix and publish a security advisory.

## Scope

This security policy applies to:

- **loqa** - Main orchestration and documentation repository
- **loqa-hub** - Central hub service for voice processing
- **loqa-device-service** - Device control and command execution service  
- **loqa-proto** - Shared protocol definitions and generated bindings
- **loqa-relay** - Audio capture device implementation (test client and firmware)
- **loqa-skills** - Voice assistant skills and integrations

## Out of Scope

The following are generally considered out of scope for security reports:

- Issues that require physical access to the user's device
- Issues that require social engineering attacks
- Denial of service attacks that require excessive resources
- Issues in third-party dependencies (please report these to the respective projects)

## Responsible Disclosure

We ask that you:

- Give us reasonable time to investigate and mitigate an issue before public exposure or disclosure
- Make a good faith effort to avoid privacy violations, destruction of data, and interruption or degradation of our services
- Only interact with accounts you own or with explicit permission of the account holder

## Security Considerations for Loqa

Loqa is designed with privacy and security as core principles:

### Local-First Architecture
- All voice processing happens locally on your network
- No data is sent to external servers without explicit user configuration
- Audio data is processed in memory and not persisted by default

### Network Security
- Services communicate via localhost by default
- gRPC communication uses standard security practices
- All external API calls are explicitly configured by users

### Data Protection
- Voice recordings are processed in real-time and discarded
- Configuration and logs should not contain sensitive information
- Users have full control over what data (if any) leaves their network

## Attribution

We will credit security researchers who report valid vulnerabilities in our security advisories, unless you prefer to remain anonymous.