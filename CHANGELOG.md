<!--
Guiding Principles:

Changelogs are for humans, not machines.
There should be an entry for every single version.
The same types of changes should be grouped.
Versions and sections should be linkable.
The latest version comes first.
The release date of each version is displayed.
Mention whether you follow Semantic Versioning (we do at and after v1.0.0).

Usage:

Change log entries are to be added to the Unreleased section
under the appropriate stanza (see below).
Each entry should ideally include the Github issue or PR reference.

The issue numbers will later be link-ified during the
release process so you do not have to worry about including
a link manually, but you can if you wish.

Types of changes (Stanzas):

* __Added__ for new features.
* __Changed__ for changes in existing functionality that did not aim to resolve bugs.
* __Deprecated__ for soon-to-be removed features.
* __Removed__ for now removed features.
* __Fixed__ for any bug fixes that did not threaten user funds or chain continuity.
* __Security__ for any bug fixes that did threaten user funds or chain continuity.

Breaking changes affecting client, API, and state should be mentioned in the release notes.

Ref: https://keepachangelog.com/en/1.0.0/
Ref: https://github.com/osmosis-labs/osmosis/blob/main/CHANGELOG.md
-->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) for all versions `v1.0.0` and beyond (still considered experimental prior to v1.0.0).

## [Unreleased]

### Added

### Changed

### Fixed

### Removed

### Fixed

### Security

## [Unreleased]

### Added

* [#101](https://github.com/allora-network/allora-offchain-node/pull/101) Timeout configuration

### Changed

### Fixed

### Removed

### Fixed

### Security


## v0.8.0

### Added

* [#100](https://github.com/allora-network/allora-offchain-node/pull/87) Update v0.8.0 chain dependencies

### Changed

### Fixed

### Removed

### Fixed

### Security


## v0.7.0

### Added

* [#87](https://github.com/allora-network/allora-offchain-node/pull/87) Update v0.7.0 chain + whitelist coverage
* [#89](https://github.com/allora-network/allora-offchain-node/pull/89) Added linter
* [#92](https://github.com/allora-network/allora-offchain-node/pull/92) Context handling
* [#91](https://github.com/allora-network/allora-offchain-node/pull/91) Add Feemarket support
* [#94](https://github.com/allora-network/allora-offchain-node/pull/94) Gas price update interval
* [#96](https://github.com/allora-network/allora-offchain-node/pull/96) Final chain update to v0.7.0 tag

### Removed

### Fixed

* [#82](https://github.com/allora-network/allora-offchain-node/pull/82) Adjust adapter log levels
* [#83](https://github.com/allora-network/allora-offchain-node/pull/83) Added missing params to .env example
* [#88](https://github.com/allora-network/allora-offchain-node/pull/88) New topic case + handle window-related errorcodes
* [#95](https://github.com/allora-network/allora-offchain-node/pull/95) Use CanSubmitXPayload methods

### Security

* [#84](https://github.com/allora-network/allora-offchain-node/pull/84) Bump cosmossdk.io/math from 1.3.0 to 1.4.0
* [#86](https://github.com/allora-network/allora-offchain-node/pull/86) Remove scanning alert on example app

## v0.6.0

### Added

* [#66](https://github.com/allora-network/allora-offchain-node/pull/66) Smart worker detection of submission windows + persistent error management + query retrials + reg/stake robustness + improved logging
* [#81](https://github.com/allora-network/allora-offchain-node/pull/81) Timeout height handling on tx submission
* [#90](https://github.com/allora-network/allora-offchain-node/pull/90) Added unit tests on CI/CD

### Removed

### Fixed

### Security

## v0.5.1

### Added

* [#75](https://github.com/allora-network/allora-offchain-node/pull/75) Configurable fee awareness

### Removed

* [#73](https://github.com/allora-network/allora-offchain-node/pull/73) Removal of legacy ECR workflow

### Fixed

* [#74](https://github.com/allora-network/allora-offchain-node/pull/74) Improve logging
* [#76](https://github.com/allora-network/allora-offchain-node/pull/76) Account sequence mismatch using expected number + other error handling improvements
* [#77](https://github.com/allora-network/allora-offchain-node/pull/77) More idiomatic buildcommit functions, use of errorsmod, error handling + duplicated error logs

### Security


## v0.5.0

### Added

* [#63](https://github.com/allora-network/allora-offchain-node/pull/63) Loss Function Library support.
* [#65](https://github.com/allora-network/allora-offchain-node/pull/65) Introduced different retry delays for account sequence.
* [#68](https://github.com/allora-network/allora-offchain-node/pull/68) Logging configuration
* [#69](https://github.com/allora-network/allora-offchain-node/pull/69) Update to allora-chain v0.6.1 dependencies.

### Removed

### Fixed

* [#65](https://github.com/allora-network/allora-offchain-node/pull/65) Error handling (incl ABCI errors)
* [#70](https://github.com/allora-network/allora-offchain-node/pull/70) Clean and improve readme

### Security
* [#62](https://github.com/allora-network/allora-offchain-node/pull/62) Fix security email


## v0.4.0

### Added

* [#53](https://github.com/allora-network/allora-offchain-node/pull/53) Update to v0.5.0 chain dependencies. Validate bundles before sending.

### Removed

### Fixed

* [#55](https://github.com/allora-network/allora-offchain-node/pull/55) Passive set retrial optimization.
* [#56](https://github.com/allora-network/allora-offchain-node/pull/56) Improve logs
* [#57](https://github.com/allora-network/allora-offchain-node/pull/57) Reduced severity of nonce failure to Warning.

### Security

## v0.3.0

### Added

* [#41](https://github.com/allora-network/allora-offchain-node/pull/41) MSE insteead of MAE, Reputer data validation, refactoring.
* [#42](https://github.com/allora-network/allora-offchain-node/pull/41) Update to v0.4.0 version of the chain. This contains breaking changes in types.

### Removed

### Fixed

* [#37](https://github.com/allora-network/allora-offchain-node/pull/37) Fix covering nil pointer when params are not available
* [#38](https://github.com/allora-network/allora-offchain-node/pull/38) Fix error handling (nil pointer dereference) on registration.
* [#40](https://github.com/allora-network/allora-offchain-node/pull/40) Forecasting fixes
* [#31](https://github.com/allora-network/allora-offchain-node/pull/31) SubmitTx fix: if set to false but properly configured, it should still not submit.


### Security

## v0.2.0

### Added

* Metrics center for monitoring and alerting via Prometheus
* Edgecase fixes
* UX improvements e.g. JSON support (no Golang interactions needed)

### Removed

### Fixed

### Security

## v0.1.0

Genesis release.
