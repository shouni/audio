# 🎼 audio

[![Language](https://img.shields.io/badge/Language-Go-blue)](https://golang.org/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/audio)](https://golang.org/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/audio)](https://github.com/shouni/audio/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/shouni/audio)](https://goreportcard.com/report/github.com/shouni/audio)
[![Go Reference](https://pkg.go.dev/badge/github.com/shouni/audio.svg)](https://pkg.go.dev/github.com/shouni/audio)
[![Status](https://img.shields.io/badge/Status-Completed-brightgreen)](#)

**`audio`** は、Go 言語で音響バイナリを極限まで低レイヤーかつ安全に操作するためのユーティリティ・キットです。
現在は特に、WAV ファイルのロスレス結合（Lossless Binary Merging）に特化しており、再エンコードによる劣化を一切許さない高品質な音声処理パイプラインを提供します。

## ✨ Core Features

*   **Lossless Binary Merging**: WAV セクションをデコードなしでバイナリレベルで直接結合。世代損失（音質劣化）をゼロに抑えた長尺楽曲構成を実現。
*   **Dynamic Chunk Analysis**: RIFF/WAVE 構造を動的に解析し、`fmt` や `data` チャンクを正確に特定。メタデータが含まれる複雑な WAV ファイルにも対応。
*   **Memory Efficient**: 最終的なバッファサイズを事前に計算し、最小限のアロケーションで高速に処理を完結。
*   **Production Ready**: 4GB 超過チェックや、不正なヘッダーに対する厳密なバリデーションを標準装備。

## 📦 Installation

```bash
go get github.com/shouni/audio
```

## 🚀 Usage

### WAV ファイルの結合 (Combine WAV Data)

複数の WAV バイナリを、単一のファイルとしてロスレスに結合します。

```go
package main

import (
    "fmt"
    "os"

    "github.com/shouni/audio/wav"
)

func main() {
    // 結合したいWAVデータのスライス
    var wavParts [][]byte
    
    // ... ファイルの読み込みロジック ...

    // 結合実行
    combined, err := wav.CombineWavData(wavParts)
    if err != nil {
        panic(err)
    }

    // ファイルへの書き出し
    err = os.WriteFile("output.wav", combined, 0644)
    if err != nil {
        fmt.Println("Error:", err)
    }
}
```

## 🏗 Project Structure

```text
audio/
├── wav/
│   ├── combiner.go      # ロスレス結合ロジックの核
│   ├── header.go        # RIFF/WAVE 定数とヘッダー解析
│   └── errors.go        # カスタムエラー定義
├── go.mod
└── README.md
```

## 🧬 Why `audio`?

一般的な音声ライブラリは、デコードして `float64` や `int16` の配列として波形を扱いますが、大規模な生成系 AI（Lyria 3 等）との連携においては、**生成された WAV をいかに劣化させず、かつ高速に繋ぎ合わせるか**が重要になります。

`audio` は、波形そのものに触れるのではなく、バイナリ構造を再構築することで、CPU 負荷を抑えつつマスタークオリティの音質を維持します。

## 📜 License

This project is licensed under the [MIT License](https://opensource.org/licenses/MIT).
