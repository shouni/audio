# 🎼 audio

[![CI](https://github.com/shouni/audio/actions/workflows/ci.yml/badge.svg)](https://github.com/shouni/audio/actions/workflows/ci.yml)
[![Language](https://img.shields.io/badge/Language-Go-blue)](https://golang.org/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/audio)](https://golang.org/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/audio)](https://github.com/shouni/audio/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Reference](https://pkg.go.dev/badge/github.com/shouni/audio.svg)](https://pkg.go.dev/github.com/shouni/audio)
[![Status](https://img.shields.io/badge/Status-Active-brightgreen)](#)

**`audio`** は、Go 言語で音響バイナリを低レイヤーかつ安全に操作し、音声合成（TTS）や生成系 AI のワークフローを最適化するためのユーティリティ・キットです。

バイナリレベルでの高品質な WAV 結合と、形態素解析に基づく高精度なテキスト前処理を組み合わせることで、次世代の音声生成パイプラインを支えます。

## ✨ Core Features

*   **Lossless Binary Merging**: WAV セクションをデコードなしでバイナリレベルで直接結合。再エンコードによる世代損失（音質劣化）をゼロに抑えた長尺構成を実現。
*   **Phonetic Text Processing**: 日本語の形態素解析に基づき、音声合成エンジンが解釈しやすい読み（カタカナ）を生成。助詞の歌唱用補正と、同梱 JSON 辞書による表層形ごとの読み補正を標準装備。
*   **Dynamic Chunk Analysis**: RIFF/WAVE 構造を動的に解析し、`fmt` や `data` チャンクを正確に特定。メタデータが含まれる複雑なファイルにも対応。
*   **Format Inspection**: `Inspect` により、結合せずに WAV のフォーマット・データサイズ・再生時間を取得可能。
*   **Memory Efficient**: 最終的なバッファサイズを事前に計算し、最小限のアロケーションで高速に処理。
*   **Production Ready**: フォーマット不一致の検出、4GB 超過チェック、不正なヘッダーに対する厳密なバリデーションを標準装備。

## 📦 Installation

```bash
go get github.com/shouni/audio
```

## 🚀 Usage

### 1. 日本語の「読み」変換 (Phonetic Conversion)

日本語テキストを、音声合成エンジンに最適な読み上げ形式に変換します。

```go
package main

import (
    "fmt"
    "github.com/shouni/audio/phonetic"
)

func main() {
    converter, _ := phonetic.NewConverter()

    // 助詞補正と発音補正を含むカタカナ変換
    reading := converter.ConvertToReading("こんにちは、絆を奏でる")
    fmt.Println(reading) // Output: コンニチワ、キズナオカナデル
}
```

入力は改行で行に分割し、**行ごとに形態素解析**します。改行を跨いで一括解析すると、行頭の語が直前の記号・英語タグの文脈に引きずられて読みが変わるためです（例: `[Chorus]` 行の直後の「重なる」が「重/なる」に割れてオモナルになる）。歌詞の `[Verse]` タグや空行は行頭の語の読みに影響しません。

標準の読み補正は `phonetic/reading_overrides.json` に同梱されています。ここに載せるのは **形態素解析器が読み間違える語だけ** です。辞書が正しく読める語を書いても挙動は変わらず、本当に危ない語を見分けられなくするため、`TestDefaultReadingOverrides_NoRedundantEntries` が「辞書と同じ読み」のエントリを検出して失敗します。

活用する語は語幹をキーにすると活用形をまとめて拾えます（`瞬い` → `マタタイ` が `瞬いて` `瞬いた` `瞬いている` に効く）。一致は形態素境界で終わるものだけを採用するので、`掌` → `テノヒラ` を入れても `掌握` は `ショウアク` のままです。

辞書読みや標準補正と異なる読みを使いたい語句は、表層形ごとに追加・上書きできます。

```go
converter, _ := phonetic.NewConverter(
    phonetic.WithReadingOverrides(map[string]string{
        "閃光": "センコウ",
    }),
)

reading := converter.ConvertToReading("私は閃光")
fmt.Println(reading) // Output: ワタシワセンコウ
```

標準の読み補正を使わず辞書読みだけにしたい場合は `WithoutDefaultReadingOverrides()` を指定します。独自の上書きと併用する場合は、この Option を先に指定してください（Option は指定順に適用されます）。

辞書に読みがない未知語は表層形で代用されますが、ひらがなはカタカナへ正規化されるため、出力がひらがな混じりになることはありません。

文節境界（助詞・助動詞の直後）にスペースを挿入することで、TTS エンジンが自然なイントネーションで読み上げやすくなります。

```go
converter, _ := phonetic.NewConverter(
    phonetic.WithPhraseSpacing(),
)

reading := converter.ConvertToReading("空が青い")
fmt.Println(reading) // Output: ソラガ アオイ
```

### 2. WAV ファイルの結合 (Combine WAV Data)

複数の WAV バイナリを、単一のファイルとしてロスレスに結合します。

```go
package main

import (
    "os"
    "github.com/shouni/audio/wav"
)

func main() {
    var wavParts [][]byte // 読み込み済みのWAVデータ
    
    combined, err := wav.CombineWavData(wavParts)
    if err != nil {
        panic(err)
    }

    _ = os.WriteFile("output.wav", combined, 0644)
}
```

#### フォーマットは揃っている必要があります

結合はデコードを伴わず `data` チャンクのバイト列を連結するだけで、出力ヘッダーには先頭ファイルの `fmt` チャンクをそのまま使います。そのため、フォーマット種別・チャンネル数・サンプルレート・量子化ビット数のいずれかが食い違うファイルを渡すと `*wav.ErrMismatchedWAVFormat` を返します。

```go
combined, err := wav.CombineWavData(wavParts)
if mismatch, ok := errors.AsType[*wav.ErrMismatchedWAVFormat](err); ok {
    // WAVファイル #1 のサンプルレートが先頭ファイルと一致しません (48000 ≠ 24000)。
    // デコードなしの結合はフォーマットが揃っている必要があります
    log.Printf("結合できません: %v (index=%d)", mismatch, mismatch.Index)
}
```

このチェックがないと、48kHz ステレオの音声が 24kHz モノラルのヘッダーで再生され、速度・音程・チャンネル割り当てがすべて狂ったまま「エラーなく」出力されます。フォーマットの異なる音声を繋ぐ場合は、事前にリサンプリングして揃えてください。

#### 結合前の検査 (Inspect)

`Inspect` は結合と同じ検証を行い、フォーマットと再生時間を返します。結合前の事前チェックや、生成された音声の長さの見積もりに使えます。

```go
info, err := wav.Inspect(wavBytes)
if err != nil {
    return err
}
fmt.Printf("%dHz %dch %dbit, %v\n",
    info.Format.SampleRate, info.Format.NumChannels, info.Format.BitsPerSample, info.Duration())
// 例: 24000Hz 1ch 16bit, 3.2s
```

## 🏗 Project Structure

```text
audio/
├── wav/             # 音響バイナリ操作 (Merging, Inspection, Validation)
├── phonetic/        # 日本語解析・音韻変換 (Tokenizing, Reading, Particle Correction)
│   └── reading_overrides.json  # 標準の読み補正辞書
├── go.mod
└── README.md
```

## 🧬 Why `audio`?

一般的な音声ライブラリは、波形を `float64` などの配列として扱いますが、大規模な生成系 AI ワークフローにおいては、以下の2点が重要になります。

1.  **AI が正しく歌える・喋れるプロンプト（読み）をどう作るか**
2.  **生成された音声バイナリをいかに劣化させず、高速に繋ぎ合わせるか**

`audio` は、波形そのものに触れるのではなく、テキスト解析とバイナリ再構築という「前処理と後処理」に特化することで、CPU 負荷を抑えつつマスタークオリティの表現力を維持します。

## 📜 License

This project is licensed under the [MIT License](https://opensource.org/licenses/MIT).
