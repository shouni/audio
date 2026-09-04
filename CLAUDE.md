# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`audio` is a Go **library** (no CLI, no `main`) with two independent packages:

- `wav` — RIFF/WAVE binary handling: lossless concatenation (`CombineWavData`, `CombineTo`), inspection without combining (`Inspect`), and the error types for the checks those run.
- `phonetic` — Japanese text to a katakana reading for a TTS engine (`Converter.ConvertToReading`), built on the Kagome morphological analyser with an embedded override dictionary.

The two share nothing and neither imports the other; the module exists as one repo because both are pre/post-processing around a speech-synthesis step. Nothing sits at the module root. The only third-party dependencies are `ikawaha/kagome/v2` and `kagome-dict/ipa`, and only `phonetic` uses them — `wav` is stdlib-only. This library knows nothing about AI services or cloud storage.

Scope is deliberately narrow: it never decodes a waveform into samples. What it does is (1) build a reading the synthesiser cannot misread and (2) stitch the resulting binaries back together without loss.

## Commands

```bash
go build ./...
go vet ./...
gofmt -l .                              # must print nothing; `gofmt -w .` to fix
go test -race ./...                     # full suite
go test -race ./wav -run TestName       # single test
go test -run '^$' -fuzz FuzzExtractAudioData ./wav    # fuzz locally
go test -bench . -benchmem ./wav        # combine benchmarks
golangci-lint run                       # config in .golangci.yml (v2 format)
govulncheck ./...
```

CI (`.github/workflows/ci.yml`) is a thin caller of the shared `shouni/workflows/.github/workflows/go-ci.yml@v1` on push/PR to `main`/`develop`. The one input it passes is `fuzz-targets`: all three `wav` fuzz targets run on every CI run, because `wav` parses byte slices supplied from outside and chunk scanning is offset arithmetic that goes out of range on inputs whose declared sizes disagree with reality. Go version comes from `go.mod` (currently 1.27); the golangci-lint pin lives in the shared workflow.

## Design decisions

Per-function rationale lives in the doc comments (`wav/combiner.go`, `wav/stream.go`, `phonetic/converter.go` carry the detail). This section covers only what a single file cannot say from inside.

### Combining does not decode, so formats must match

`data` chunk payloads are concatenated as bytes and the output header is the **first** file's `fmt` chunk, reused verbatim. That is what makes the operation lossless — there is no re-encode and therefore no generation loss — and it is also why `verifySameFormat` has to reject any mismatch in format tag, channel count, sample rate, bit depth, channel mask, or `WAVE_FORMAT_EXTENSIBLE` sub-format GUID. Without that check a 48kHz stereo part would be played back under a 24kHz mono header: wrong speed, wrong pitch, wrong channel assignment, and no error. `0xFFFE` in the format tag says nothing on its own, hence the comparison down to the GUID.

Callers wanting to mix formats must resample first. Do not "fix" a mismatch by relaxing the comparison.

### The two combine paths must stay indistinguishable

`CombineWavData` (bytes in, bytes out) and `CombineTo` (`io.ReadSeeker` → `io.Writer`) differ only in memory use: the streaming path's footprint is independent of the audio's length and of how many parts there are. Their validation, their errors, and their output bytes are required to be identical, and `FuzzCombineToMatchesCombineWavData` pins exactly that — if one path accepts an input the other rejects, the result would depend on which API a caller happened to pick.

`CombineTo` takes `io.ReadSeeker` rather than `io.Reader` because the RIFF and `data` chunk sizes go at the front of the output and are not known until everything has been measured, so the inputs are scanned twice. `maxCarriedHeaderSize` (1MiB) caps the header carried over from the first file, so an enormous metadata chunk cannot quietly reinstate the memory cost the streaming path exists to avoid.

### Fix the reading first, then hand it to the engine

The morphological analyser is the base, not the authority. Proper nouns and coinages are read inconsistently by the dictionary, so the split is: **pin what can be pinned in a dictionary, and leave the rest to the analyser.** The embedded `phonetic/reading_overrides.json` (plus `WithReadingOverrides` / `WithReadingOverridesJSON`) holds the pinned entries; `WithNumberReading` is the same idea for Arabic numerals, whose readings the IPA dictionary does not carry at all.

Two orderings in `convertLine` are load-bearing and easy to get backwards:

- **Tokenize the whole line once, then lay overrides on top.** Applying overrides first would cut the input into fragments that are then analysed without context, and the parts of speech come out wrong (the 「が」 in 「運命の閃光が」 is read as a conjunction rather than a particle once isolated, which drops both the particle correction and the phrase spacing).
- **Only matches that start *and* end on a morpheme boundary count.** A match crossing a boundary drops whatever characters the override failed to cover (an override for 「世界」 against 「世界観」).

Input is also split into lines and analysed line by line, because a token at the start of a line otherwise gets dragged by the preceding line's symbols or unknown words — a real case with lyrics carrying `[Chorus]` tags.

### An override table is a list of what the dictionary gets wrong

Entries whose reading matches what the dictionary already produces change no behaviour and make the genuinely dangerous entries harder to spot; one revision had 51 of 120 entries in that state. `TestDefaultReadingOverrides_NoRedundantEntries` and `TestCounters_NoRedundantEntries` fail on such entries, comparing in several contexts rather than on the bare word (「何時」 is read イツ alone but ナンジ mid-sentence, so a bare comparison would delete an override that is needed). Adding an entry means demonstrating the dictionary is wrong about it.

## Tests

- All tests are white-box (`package wav` / `package phonetic`) and use the **standard library only** — no testify. Table-driven with `t.Run` subtests is the normal shape.
- `wav` tests build their fixtures programmatically (`buildWAV`, `defaultSpec`, `insertChunkBeforeData` in `wav/header_test.go`); there are no binary test fixtures in the repo. Keep it that way — a malformed-input case should be constructed in code where the malformation is visible.
- Three fuzz targets in `wav/fuzz_test.go` run in CI, as above. New parsing code belongs behind them rather than behind a hand-listed set of bad inputs.
- `wav/bench_test.go` measures both combine paths with `b.ReportAllocs()` / `b.SetBytes`; the allocation count is the point of the memory-efficiency claims, so check it before changing the buffer strategy.
- Doc comments and test names are Japanese, in the `名前 は …します。` form; error strings are Japanese.
