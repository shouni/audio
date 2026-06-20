# 🎼 audio

[![Language](https://img.shields.io/badge/Language-Go-blue)](https://golang.org/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/audio)](https://golang.org/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/audio)](https://github.com/shouni/audio/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/shouni/audio)](https://goreportcard.com/report/github.com/shouni/audio)
[![Go Reference](https://pkg.go.dev/badge/github.com/shouni/audio.svg)](https://pkg.go.dev/github.com/shouni/audio)
[![Status](https://img.shields.io/badge/Status-Completed-brightgreen)](#)

**`audio`** は、Go 言語で音響バイナリを低レイヤーかつ安全に操作し、音声合成（TTS）や生成系 AI のワークフローを最適化するためのユーティリティ・キットです。

バイナリレベルでの高品質な WAV 結合と、形態素解析に基づく高精度なテキスト前処理を組み合わせることで、次世代の音声生成パイプラインを支えます。

## ✨ Core Features

*   **Lossless Binary Merging**: WAV セクションをデコードなしでバイナリレベルで直接結合。再エンコードによる世代損失（音質劣化）をゼロに抑えた長尺構成を実現。
*   **Phonetic Text Processing**: 日本語の形態素解析に基づき、音声合成エンジンが解釈しやすい読み（カタカナ）を生成。助詞の歌唱用補正と、同梱 JSON 辞書による表層形ごとの読み補正を標準装備。
*   **Dynamic Chunk Analysis**: RIFF/WAVE 構造を動的に解析し、`fmt` や `data` チャンクを正確に特定。メタデータが含まれる複雑なファイルにも対応。
*   **Memory Efficient**: 最終的なバッファサイズを事前に計算し、最小限のアロケーションで高速に処理。
*   **Production Ready**: 4GB 超過チェックや、不正なヘッダーに対する厳密なバリデーションを標準装備。

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

標準の読み補正は `phonetic/reading_overrides.json` に同梱されています。辞書読みや標準補正と異なる読みを使いたい語句は、表層形ごとに追加・上書きできます。

```go
converter, _ := phonetic.NewConverter(
    phonetic.WithReadingOverrides(map[string]string{
        "閃光": "センコウ",
    }),
)

reading := converter.ConvertToReading("私は閃光")
fmt.Println(reading) // Output: ワタシワセンコウ
```

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

## 🏗 Project Structure

```text
audio/
├── wav/             # 音響バイナリ操作 (Merging, Validation, Header Analysis)
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
