# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).


## [0.2.0] - 2026-10-02

### Added

- CLI resource limits for initial loading and reloading: `--max-input-bytes` (8 MiB), `--max-nodes` (100,000), and `--max-depth` (128). Set a limit to `0` to disable it for trusted input.
- `bubbletea.WithAliasRowLimit` to bound projected alias rows. The default is 10,000; nonpositive values hide projections without hiding ownership rows.
- Document accessors for sequence lengths, individual sequence items, mapping entries, and reverse reference lookups.
- Regression tests for YAML editing, snapshot isolation, terminal rendering, save conflicts, atomic saving, and Windows ACL preservation.
- YAML round-trip and selector fuzz tests, plus Linux and Windows CI builds, vet checks, Linux race detection, and reachable vulnerability checks.

### Changed

- CLI saves now prepare, flush, and close a temporary file before replacing the destination. Existing symlinks are preserved, file mode bits are retained, and Windows replacement preserves the destination's ACL.
- Document snapshots share unchanged immutable nodes; builder mutations copy only affected nodes.
- Capability and selector resolution avoid copying entire parent containers. Deletion checks use reverse reference indexes instead of scanning the full document for every candidate.
- Terminal tree rows are cached across unchanged views and refreshed when the document, expansion state, or validation results change.
- Terminal widths, clipping, cursor movement, and deletion account for wide characters and grapheme clusters.
- Embedding documentation delegates saving to an application-owned persistence callback and describes resource limits and safe file replacement.

### Fixed

- Optional YAML mapping entries can be deleted when schema and policy allow it, while mapping keys remain protected from editing.
- Moves that would place a YAML alias before its anchor are rejected without changing the document or history.
- NaN values produce validation errors when numeric minimum or maximum constraints are configured.
- Input-derived terminal control sequences and bidirectional formatting controls are displayed as escaped text without modifying stored values.
- Failed file writes or save preparation no longer truncate the original file.


# [0.1.0] - 2026-10-01

### Added

- Immutable ordered document trees with stable node identities, paths, builders, and reference graphs.
- A core editor with schema validation, selector-based permissions, logical focus, structural editing, and undo/redo history.
- A YAML codec preserving comments, styles, anchors, aliases, merge entries, duplicate keys, non-string keys, and custom tags.
- A Bubble Tea terminal adapter with tree navigation, scalar editing, inline editing, mouse support, and optional read-only alias expansion.
- A reference CLI with explicit saving, external-change conflict handling, reloading, and unsaved-change confirmation.
- A format-independent codec session interface, codec conformance helpers, embedding examples, and package documentation.
