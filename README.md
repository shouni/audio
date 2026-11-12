# 🎼 audio

[![CI](https://github.com/shouni/audio/actions/workflows/ci.yml/badge.svg)](https://github.com/shouni/audio/actions/workflows/ci.yml)
[![Status](https://img.shields.io/badge/Status-Active-brightgreen)](#)
[![Language](https://img.shields.io/badge/Language-Go-blue)](https://go.dev/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/audio)](https://go.dev/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/audio)](https://github.com/shouni/audio/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Reference](https://pkg.go.dev/badge/github.com/shouni/audio.svg)](https://pkg.go.dev/github.com/shouni/audio)

English | [日本語](README.ja.md)

## 🚀 About - WAVs joined without decoding, and readings a TTS engine cannot misread

**`audio`** is a utility kit for handling audio binaries in Go at a low level and safely, streamlining speech synthesis (TTS) and generative-AI workflows.

It combines high-quality binary-level WAV joining with precise, morphology-based text preprocessing to support next-generation speech generation pipelines.

## ✨ Features

*   **Lossless Binary Merging**: Joins WAV sections directly at the binary level, without decoding. Long-form assemblies carry zero generation loss (quality degradation) from re-encoding.
*   **Phonetic Text Processing**: Produces readings (katakana) that a speech synthesis engine interprets easily, based on Japanese morphological analysis. Particle correction for singing and per-surface-form reading corrections from a bundled JSON dictionary are built in.
*   **Number & Counter Reading**: Converts Arabic and kanji numerals into readings, including the sound changes at counters (一回→イッカイ, 三本→サンボン, 八月→ハチガツ). Neither numbers the dictionary has no reading for nor the sound at the join of number and counter, which the dictionary does not carry, are left to the TTS engine.
*   **Streaming Merge**: `CombineTo` passes audio data straight from input to output, so the memory it needs does not depend on the audio's length (per input, writing to a file adds a copy buffer of about 33KB).
*   **Dynamic Chunk Analysis**: Parses the RIFF/WAVE structure dynamically to locate the `fmt` and `data` chunks precisely. Handles complex files that carry metadata.
*   **Format Inspection**: `Inspect` returns a WAV's format, data size and duration without combining.
*   **Memory Efficient**: Computes the final buffer size up front and processes fast with minimal allocations.
*   **Production Ready**: Format mismatch detection (comparing down to the valid bits and sub-format GUID of WAVE_FORMAT_EXTENSIBLE), rejection of compressed formats, a check for exceeding 4GB, and strict validation of malformed headers are all built in.

## 🚦 Usage

Install with `go get github.com/shouni/audio`.

### 1. Japanese reading conversion (Phonetic Conversion)

Converts Japanese text into a reading form best suited to a speech synthesis engine.

```go
package main

import (
    "fmt"
    "github.com/shouni/audio/phonetic"
)

func main() {
    converter, _ := phonetic.NewConverter()

    // Katakana conversion with particle and pronunciation corrections
    reading := converter.ConvertToReading("こんにちは、絆を奏でる")
    fmt.Println(reading) // Output: コンニチワ、キズナオカナデル
}
```

The input is split into lines at newlines and **each line is analysed separately**. Analysing across newlines in one pass changes the reading of a line-initial word, which gets dragged by the context of the preceding symbols or English tags (for example, 「重なる」 right after a `[Chorus]` line splits into 「重/なる」 and is read オモナル). Lyric tags such as `[Verse]` and blank lines do not affect the reading of the word at the start of a line.

The default reading corrections ship in `phonetic/reading_overrides.json`. Only **words the morphological analyser misreads** belong there. Listing a word the dictionary already reads correctly changes no behaviour and makes the genuinely dangerous words harder to spot, so `TestDefaultReadingOverrides_NoRedundantEntries` fails on any entry whose reading matches the dictionary's.

For an inflecting word, keying it by its stem catches all its inflected forms (`瞬い` → `マタタイ` applies to `瞬いて`, `瞬いた` and `瞬いている`). Only matches that end on a morpheme boundary are taken, so adding `掌` → `テノヒラ` leaves `掌握` as `ショウアク`.

Words or phrases you want read differently from the dictionary or the default corrections can be added or overridden per surface form.

```go
converter, _ := phonetic.NewConverter(
    phonetic.WithReadingOverrides(map[string]string{
        "閃光": "センコウ",
    }),
)

reading := converter.ConvertToReading("私は閃光")
fmt.Println(reading) // Output: ワタシワセンコウ
```

To use dictionary readings only, without the default corrections, pass `WithoutDefaultReadingOverrides()`. When combining it with your own overrides, pass this Option first (options are applied in the order given).

Unknown words with no dictionary reading fall back to their surface form, but hiragana is normalized to katakana, so the output never mixes in hiragana (Arabic numerals and Latin letters stay as their surface form unless `WithNumberReading()` is set).

Line terminators are carried from the input into the output. The `\r` of a CRLF is never passed to the analyser, so it never mixes into the reading.

A project-specific dictionary kept as a JSON file can be loaded as is, without copying it into Go code. If the JSON is malformed or contains an empty key or value, `NewConverter` returns an error.

```go
data, err := os.ReadFile("my_readings.json")
if err != nil {
    return err
}
converter, err := phonetic.NewConverter(phonetic.WithReadingOverridesJSON(data))
```

#### Number Reading

The morphological analyser's dictionary has no readings for Arabic numerals (the reading of `2026` is `*`), and it has only standalone readings for counters, detached from any number (`ツキ` for `月`). So by default `2026年8月25日` becomes `2026ネン8ツキ25ニチ`: the digits stay as they are, and even the reading of the month breaks. With `WithNumberReading()`, the number is read, and so are the gemination, rendaku voicing and handakuten at the join of number and counter.

```go
converter, _ := phonetic.NewConverter(
    phonetic.WithNumberReading(),
)

fmt.Println(converter.ConvertToReading("2026年8月25日"))
// Output: ニセンニジュウロクネンハチガツニジュウゴニチ
fmt.Println(converter.ConvertToReading("1本と3本と6本"))
// Output: イッポントサンボントロッポン
fmt.Println(converter.ConvertToReading("1,234人が100%を3ヶ月で"))
// Output: センニヒャクサンジュウヨニンガヒャクパーセントオサンカゲツデ (the particle 「を」 is corrected to オ)
```

Only counter matches that end on a morpheme boundary are taken, so `3人称` stays `サンニンショウ` and its `人` is not picked up as a counter. The dictionary can read kanji numerals on their own (`三` → `サン`), so they are read by the same rules only when a counter follows and a sound change is needed (`三本` → `サンボン`, `二人` → `フタリ`, `二千二十六年` → `ニセンニジュウロクネン`). Words the dictionary holds as one word, such as `十二月`, are not split and are left alone. Runs with a leading `0` (`007`, `0120`) and digits after a decimal point are usually identifiers or digit sequences rather than quantities, so they are read one character at a time.

Number readings are a "default interpretation, like the dictionary's", so a reading stated with `WithReadingOverrides` always wins. So as not to change existing output silently, this Option is opt-in.

The counter table (`counters` in `phonetic/number.go`) holds **only counters whose reading with a number differs from the dictionary's standalone reading**. A counter with neither a sound change nor an irregular reading is read correctly by the dictionary, so `TestCounters_NoRedundantEntries` fails on any entry whose reading matches the dictionary's.

#### Phrase Spacing

Inserting a space at phrase boundaries (right after a particle or auxiliary verb) helps a TTS engine read with natural intonation. Stacked particles stay in one phrase, so the space goes after the last of them (「誰にも見せない」 → ダレニモ ミセナイ, not ダレニ モ).

```go
converter, _ := phonetic.NewConverter(
    phonetic.WithPhraseSpacing(),
)

reading := converter.ConvertToReading("空が青い")
fmt.Println(reading) // Output: ソラガ アオイ
```

### 2. Combine WAV Data

Losslessly combines several WAV binaries into a single file.

```go
package main

import (
    "os"
    "github.com/shouni/audio/wav"
)

func main() {
    var wavParts [][]byte // WAV data already loaded
    
    combined, err := wav.CombineWavData(wavParts)
    if err != nil {
        panic(err)
    }

    _ = os.WriteFile("output.wav", combined, 0644)
}
```

#### Formats must match

Combining involves no decoding: it only concatenates the bytes of the `data` chunks, and the output header reuses the first file's `fmt` chunk as is. If there is a `fact` chunk, only its sample count is rewritten to the combined value; every other accompanying chunk stays as the first file had it. So passing files that differ in format tag, channel count, sample rate or bit depth returns `*wav.ErrMismatchedWAVFormat`.

Supported formats are linear PCM (bit depths that are multiples of 8), IEEE float (32/64 bit), G.711 A-law / μ-law (8 bit), and the WAVE_FORMAT_EXTENSIBLE form of each. Compressed formats, such as ADPCM or a WAV carrying MP3, return `*wav.ErrUnsupportedWAVFormat`, because neither their block boundaries nor their silence follow from the bit depth. Convert them to PCM first.

```go
combined, err := wav.CombineWavData(wavParts)
if mismatch, ok := errors.AsType[*wav.ErrMismatchedWAVFormat](err); ok {
    // WAV file #1 differs from the first file in sample rate (48000 ≠ 24000);
    // combining without decoding requires matching formats
    log.Printf("cannot combine: %v (index=%d)", mismatch, mismatch.Index)
}
```

Without this check, 48kHz stereo audio would play back under a 24kHz mono header, with speed, pitch and channel assignment all wrong, and be output "without error". To join audio of different formats, resample it to match first.

Under WAVE_FORMAT_EXTENSIBLE (`AudioFormat` is `0xFFFE`), neither the actual encoding nor the speaker layout is in the first 16 bytes of the `fmt` chunk. Without looking further, linear PCM and IEEE float would match as "both `0xFFFE`", so the comparison goes down to the channel mask, the valid bits (`wValidBitsPerSample`) and the sub-format GUID. When only the GUID differs, the value cannot be expressed as a number, so `*wav.ErrMismatchedSubFormat` is returned.

#### Combining with silence in between (Gap)

Audio synthesized sentence by sentence sounds crammed when joined as is, with no break between. `WithGap` inserts silence only between one WAV and the next (never at the start or end).

```go
combined, err := wav.CombineWavData(wavParts, wav.WithGap(200*time.Millisecond))
```

The length of the silence is truncated to a sample boundary (the block align), so the channel order never shifts. Only 8-bit linear PCM is unsigned, so its silence is filled with `0x80` rather than `0x00`.

#### Combining without holding everything in memory (Streaming)

`CombineWavData` holds every input and the output in memory, so for long combinations the peak is effectively doubled. `CombineTo` passes the audio data straight from input to output, so the memory it needs does not depend on the audio's length. Per input, writing to a file adds the copy buffer `io.CopyN` allocates for each one (about 33KB).

```go
sources := make([]io.ReadSeeker, 0, len(paths))
for _, path := range paths {
    file, err := os.Open(path)
    if err != nil {
        return err
    }
    defer file.Close()
    sources = append(sources, file)
}

out, err := os.Create("output.wav")
if err != nil {
    return err
}
defer out.Close()

if err := wav.CombineTo(out, sources, wav.WithGap(200*time.Millisecond)); err != nil {
    return err
}
```

The output's RIFF and data chunk sizes have to be written at the front and are not known until the whole length is. So the inputs are scanned twice (the first pass finds the format and the position of the `data` chunk, the second transfers the audio), which is why `io.ReadSeeker` is required rather than `io.Reader`. Its validation and errors are the same as `CombineWavData`'s, and its output does not differ by a single byte.

If the combined result exceeds the 4GB RIFF can express, `*wav.ErrWAVTooLarge` is returned.

#### Inspecting before combining (Inspect)

`Inspect` runs the same validation as combining and returns the format and duration. Use it as a pre-check before combining or to estimate the length of generated audio.

```go
info, err := wav.Inspect(wavBytes)
if err != nil {
    return err
}
fmt.Printf("%dHz %dch %dbit, %v\n",
    info.Format.SampleRate, info.Format.NumChannels, info.Format.BitsPerSample, info.Duration())
// e.g. 24000Hz 1ch 16bit, 3.2s
```

## 📦 Package Structure

```text
audio/
├── wav/                 # Audio binary handling
│   ├── header.go        #   RIFF/WAVE chunk parsing
│   ├── format.go        #   fmt chunk contents and derived values
│   ├── builder.go       #   Assembling the combined header
│   ├── combiner.go      #   In-memory combining (CombineWavData)
│   ├── stream.go        #   Streaming combining (CombineTo)
│   ├── inspect.go       #   Inspecting without combining (Inspect)
│   └── errors.go        #   Error types
├── phonetic/            # Japanese analysis and phonetic conversion
│   ├── converter.go     #   Morphological analysis and reading assembly
│   ├── number.go        #   Number and counter readings
│   └── reading_overrides.json  # Default reading correction dictionary
├── go.mod
├── README.md            # English
└── README.ja.md         # Japanese
```

## 🧬 Why this scope - never touching the waveform

A typical audio library treats a waveform as an array of `float64` or similar, but in large-scale generative-AI workflows two things matter more:

1.  **How to build a prompt (a reading) that the AI can sing or speak correctly**
2.  **How to join the generated audio binaries quickly, without degrading them**

Rather than touching the waveform itself, `audio` specializes in the "pre- and post-processing" of text analysis and binary reconstruction, keeping CPU load low while preserving master-quality expressiveness.

## 📜 License

This project is licensed under the [MIT License](https://opensource.org/licenses/MIT).
