# Changelog

## [0.3.0](https://github.com/Elven9/jarvis-core/compare/v0.2.1...v0.3.0) (2026-11-17)


### ⚠ BREAKING CHANGES

* **server:** uploads whose tool calls, subagents, skills or project instructions do not parse are now rejected with 422.
* **server:** GET /v1/sessions/{machineId}/{sessionId} no longer answers the raw session file; download it from .../raw instead.

### Features

* **metadata:** list the scenario and project name of each session ([7551f92](https://github.com/Elven9/jarvis-core/commit/7551f92ebfc2cfba0f9d375b31eaa3cccec4e6d5))
* **metadata:** store the session scenario and project ([61b8dda](https://github.com/Elven9/jarvis-core/commit/61b8ddaec0b1843304383c9007141991d8b5da26))
* **server:** answer GET /v1/sessions/{machineId}/{sessionId} with the parsed session ([bb47448](https://github.com/Elven9/jarvis-core/commit/bb47448dc4fcbc168fd88c2f34a8655771c72509))
* **server:** answer the stored scenario and project on the meta route ([f2b95c9](https://github.com/Elven9/jarvis-core/commit/f2b95c99881981305391b2f313f16247eba248aa))
* **server:** list the scenario and project of each session ([09535d9](https://github.com/Elven9/jarvis-core/commit/09535d95ac5d6271b35c65a53ced20ad21efc55d))
* **server:** move the raw session download to GET /v1/sessions/{machineId}/{sessionId}/raw ([eb13c3d](https://github.com/Elven9/jarvis-core/commit/eb13c3dc379c2669db9b747d4ece2e7988745a86))
* **server:** reject uploads that any parser group rejects ([087a09d](https://github.com/Elven9/jarvis-core/commit/087a09d2cf8602122c799ddb80ba3c67c24cc9bb))


### Bug Fixes

* **deps:** bump session-parser to go/v0.2.0 ([c2bac4c](https://github.com/Elven9/jarvis-core/commit/c2bac4c1fd37a20472d208877fc23e50acdc694b))

## [0.2.1](https://github.com/Elven9/jarvis-core/compare/v0.2.0...v0.2.1) (2026-11-17)


### Bug Fixes

* **deps:** bump session-parser to go/v0.1.1 ([3c618e0](https://github.com/Elven9/jarvis-core/commit/3c618e09505d1e76cec58082599558a85ab96a22))

## [0.2.0](https://github.com/Elven9/jarvis-core/compare/v0.1.0...v0.2.0) (2026-11-16)


### ⚠ BREAKING CHANGES

* **server:** uploads that are not a pi session of format 3 the library can parse, or whose header ID differs from the URL sessionId, are rejected with 422.

### Features

* **metadata:** add metadata store interface and contract tests ([25ae45c](https://github.com/Elven9/jarvis-core/commit/25ae45caba76b44a45b2fc25b76ec32c590fb293))
* **metadata:** add SQLite metadata store and migration ([4f19075](https://github.com/Elven9/jarvis-core/commit/4f1907540115285b3cb4092f9d8e13ee2d47be11))
* **metadata:** link every metadata to exactly one session record ([098b3c0](https://github.com/Elven9/jarvis-core/commit/098b3c075535145dccca96efc3c4ce939c6f6f1f))
* **server:** add GET /v1/sessions list route ([a3100e7](https://github.com/Elven9/jarvis-core/commit/a3100e765ce2b91199ddcc22c1cb0ba4f76fcbe7))
* **server:** add GET /v1/sessions/{machineId}/{sessionId}/meta route ([ef47271](https://github.com/Elven9/jarvis-core/commit/ef47271baabd681e016c315132d58f19f5f6a541))
* **server:** parse uploads inline and store their metadata ([9484e37](https://github.com/Elven9/jarvis-core/commit/9484e37b884455a6007fc449cf72bfd15f4141bf))

## 0.1.0 (2026-11-16)


### Features

* **blob:** add filesystem blob store with atomic writes and dedupe ([7deaf83](https://github.com/Elven9/jarvis-core/commit/7deaf831f6be4bc1c247513e0217d3dbe8add9ef))
* **server:** add config, JSON logging, access log and Fiber server entrypoint ([4367953](https://github.com/Elven9/jarvis-core/commit/436795369178fcf7c4331299b201bf642f7ef364))
* **server:** add GET /v1/sessions read route with RFC 9457 errors ([2344964](https://github.com/Elven9/jarvis-core/commit/2344964d5592ce2964525b94f118d22222884ffc))
* **server:** add PUT /v1/sessions upload route ([fcc967c](https://github.com/Elven9/jarvis-core/commit/fcc967c4f4be5a09f94dafad5592c78bd31187a1))
* **session:** add global session ID with validation and string form ([9232bb5](https://github.com/Elven9/jarvis-core/commit/9232bb5b0e63529b6bb86264951c1ee22f412768))
* **session:** add SQLite session store with goose migrations at startup ([d06b02e](https://github.com/Elven9/jarvis-core/commit/d06b02e989f9830cc65536bca87996117c07b622))
* **storage:** add blob and session store interfaces with contract suites ([6ddbbe1](https://github.com/Elven9/jarvis-core/commit/6ddbbe1086711a770b1456c7b637418852994cd8))
