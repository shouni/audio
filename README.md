# 🎼 audio

[![CI](https://github.com/shouni/audio/actions/workflows/ci.yml/badge.svg)](https://github.com/shouni/audio/actions/workflows/ci.yml)
[![Status](https://img.shields.io/badge/Status-Active-brightgreen)](#)
[![Language](https://img.shields.io/badge/Language-Go-blue)](https://go.dev/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/audio)](https://go.dev/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/audio)](https://github.com/shouni/audio/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Reference](https://pkg.go.dev/badge/github.com/shouni/audio.svg)](https://pkg.go.dev/github.com/shouni/audio)

## 🚀 概要 (About) - デコードせずに繋ぐ WAV と、TTS が読み違えない読み

**`audio`** は、Go 言語で音響バイナリを低レイヤーかつ安全に操作し、音声合成（TTS）や生成系 AI のワークフローを最適化するためのユーティリティ・キットです。

バイナリレベルでの高品質な WAV 結合と、形態素解析に基づく高精度なテキスト前処理を組み合わせることで、次世代の音声生成パイプラインを支えます。

## ✨ 提供機能 (Features)

*   **Lossless Binary Merging**: WAV セクションをデコードなしでバイナリレベルで直接結合。再エンコードによる世代損失（音質劣化）をゼロに抑えた長尺構成を実現。
*   **Phonetic Text Processing**: 日本語の形態素解析に基づき、音声合成エンジンが解釈しやすい読み（カタカナ）を生成。助詞の歌唱用補正と、同梱 JSON 辞書による表層形ごとの読み補正を標準装備。
*   **Number & Counter Reading**: 算用数字を助数詞の音便（一回→イッカイ、三本→サンボン、八月→ハチガツ）まで含めて読みに変換。辞書が読みを持たない数字を TTS へ丸投げしない。
*   **Streaming Merge**: `CombineTo` は入力から出力へ音声データを直接受け渡すため、必要なメモリが音声の長さにも本数にも比例しない。
*   **Dynamic Chunk Analysis**: RIFF/WAVE 構造を動的に解析し、`fmt` や `data` チャンクを正確に特定。メタデータが含まれる複雑なファイルにも対応。
*   **Format Inspection**: `Inspect` により、結合せずに WAV のフォーマット・データサイズ・再生時間を取得可能。
*   **Memory Efficient**: 最終的なバッファサイズを事前に計算し、最小限のアロケーションで高速に処理。
*   **Production Ready**: フォーマット不一致の検出（WAVE_FORMAT_EXTENSIBLE のサブフォーマット GUID まで比較）、4GB 超過チェック、不正なヘッダーに対する厳密なバリデーションを標準装備。

## 🚦 使い方 (Usage)

`go get github.com/shouni/audio` で入れます。

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

辞書に読みがない未知語は表層形で代用されますが、ひらがなはカタカナへ正規化されるため、出力がひらがな混じりになることはありません（算用数字とアルファベットは `WithNumberReading()` を付けない限り表層形のまま残ります）。

改行文字は入力のまま出力へ引き継ぎます。CRLF の `\r` は解析に渡さないため、読みに混ざることはありません。

プロジェクト固有の辞書を JSON ファイルとして持っている場合は、Go のコードに書き写さずにそのまま読み込めます。JSON が壊れていたり空のキー・値を含む場合は `NewConverter` がエラーを返します。

```go
data, err := os.ReadFile("my_readings.json")
if err != nil {
    return err
}
converter, err := phonetic.NewConverter(phonetic.WithReadingOverridesJSON(data))
```

#### 算用数字の読み (Number Reading)

形態素解析器の辞書は算用数字に読みを持たず（`2026` の読みは `*`）、助数詞も数と切り離された単独の読み（`月` なら `ツキ`）しか持ちません。そのため既定では `2026年8月25日` が `2026ネン8ツキ25ニチ` になり、数字がそのまま残るうえに月の読みまで崩れます。`WithNumberReading()` を指定すると、数の読みと、数と助数詞の境目に起きる促音便・連濁・半濁音化まで当てます。

```go
converter, _ := phonetic.NewConverter(
    phonetic.WithNumberReading(),
)

fmt.Println(converter.ConvertToReading("2026年8月25日"))
// Output: ニセンニジュウロクネンハチガツニジュウゴニチ
fmt.Println(converter.ConvertToReading("1本と3本と6本"))
// Output: イッポントサンボントロッポン
fmt.Println(converter.ConvertToReading("1,234人が100%を3ヶ月で"))
// Output: センニヒャクサンジュウヨニンガヒャクパーセントオサンカゲツデ（助詞「を」はオに補正される）
```

助数詞の一致は形態素境界で終わるものだけを採用するので、`3人称` は `サンニンショウ` のままで `人` を助数詞として拾いません。漢数字（`十二月`）は辞書が正しく読めるため手を出しません。先頭が `0` の並び（`007`、`0120`）と小数点以下は、数量ではなく識別子や桁の列であることが多いため 1 文字ずつ読みます。

数字の読みは「辞書と同じ既定の解釈」なので、`WithReadingOverrides` で明示した読みが常に優先されます。既存の出力を黙って変えないよう、この Option は opt-in です。

助数詞の表 (`phonetic/number.go` の `counters`) に載せるのは、**数と組んだときの読みが辞書の単独読みと違うものだけ** です。音の変化も特殊読みもない助数詞は辞書の読みで正しく読めるため、`TestCounters_NoRedundantEntries` が「辞書と同じ読み」のエントリを検出して失敗します。

#### 文節スペース (Phrase Spacing)

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

WAVE_FORMAT_EXTENSIBLE（`AudioFormat` が `0xFFFE`）では、実際の符号化方式もスピーカー配置も `fmt` チャンクの先頭 16 バイトには入っていません。ここを見ないとリニア PCM と IEEE float が「どちらも `0xFFFE`」として一致扱いになるため、チャンネルマスクとサブフォーマット GUID まで比較します。GUID だけが食い違う場合は値を数値で表せないため `*wav.ErrMismatchedSubFormat` を返します。

#### 無音を挟んで結合する (Gap)

文ごとに合成した音声をそのまま繋ぐと、切れ目がなく詰まって聞こえます。`WithGap` は WAV と WAV の間にだけ無音を挟みます（先頭と末尾には付きません）。

```go
combined, err := wav.CombineWavData(wavParts, wav.WithGap(200*time.Millisecond))
```

無音の長さはサンプル境界（ブロックアライン）まで切り捨てるため、チャンネルの並びがずれることはありません。8bit のリニア PCM だけは符号なしなので、無音は `0x00` ではなく `0x80` で埋めます。

#### メモリに載せずに結合する (Streaming)

`CombineWavData` は入力も出力もすべてメモリに載せるため、長尺の結合ではピークが実質 2 倍になります。`CombineTo` は音声データを入力から出力へ直接受け渡すので、必要なメモリは音声の長さにも本数にも比例しません。

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

出力の RIFF/data チャンクサイズは先頭に書く必要があり、全体の長さが分かるまで確定しません。そのため入力は 2 回走査します（1 回目でフォーマットと `data` チャンクの位置を調べ、2 回目で音声を転送）。`io.Reader` ではなく `io.ReadSeeker` を要求するのはこのためです。検証内容とエラーは `CombineWavData` と同じで、出力は 1 バイトも違いません。

結合結果が RIFF の表現できる 4GB を超える場合は `*wav.ErrWAVTooLarge` を返します。

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

## 📦 パッケージ構成 (Package Structure)

```text
audio/
├── wav/                 # 音響バイナリ操作
│   ├── header.go        #   RIFF/WAVE チャンクの解析
│   ├── format.go        #   fmt チャンクの内容と派生値
│   ├── builder.go       #   結合後ヘッダーの組み立て
│   ├── combiner.go      #   メモリ上での結合 (CombineWavData)
│   ├── stream.go        #   ストリーミング結合 (CombineTo)
│   ├── inspect.go       #   結合せずに調べる (Inspect)
│   └── errors.go        #   エラー型
├── phonetic/            # 日本語解析・音韻変換
│   ├── converter.go     #   形態素解析と読みの組み立て
│   ├── number.go        #   数詞と助数詞の読み
│   └── reading_overrides.json  # 標準の読み補正辞書
├── go.mod
└── README.md
```

## 🧬 波形に触れない理由 (Why this scope)

一般的な音声ライブラリは、波形を `float64` などの配列として扱いますが、大規模な生成系 AI ワークフローにおいては、以下の2点が重要になります。

1.  **AI が正しく歌える・喋れるプロンプト（読み）をどう作るか**
2.  **生成された音声バイナリをいかに劣化させず、高速に繋ぎ合わせるか**

`audio` は、波形そのものに触れるのではなく、テキスト解析とバイナリ再構築という「前処理と後処理」に特化することで、CPU 負荷を抑えつつマスタークオリティの表現力を維持します。

## 📜 ライセンス (License)

This project is licensed under the [MIT License](https://opensource.org/licenses/MIT).
