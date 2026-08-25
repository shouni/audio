package wav

import (
	"bytes"
	"io"
	"testing"
)

// fuzzSeeds は、壊し甲斐のある構造を持つ WAV のシードです。
func fuzzSeeds() [][]byte {
	return [][]byte{
		buildWAV(defaultSpec([]byte{1, 2, 3, 4})),
		insertChunkBeforeData(buildWAV(defaultSpec([]byte{1, 2, 3})), "LIST", []byte("abcde")),
		buildExtensibleWAV(defaultExtensibleSpec([]byte{1, 2, 3, 4})),
		[]byte("RIFF"),
		[]byte("RIFF\x00\x00\x00\x00WAVE"),
	}
}

// FuzzExtractAudioData は、任意のバイト列を食わせても解析が panic しないことを確認します。
//
// チャンク走査はオフセットとサイズの足し算だらけで、宣言サイズが実体と食い違う入力では
// 範囲外アクセスに落ちやすい箇所です。エラーで返すのは構いませんが、落ちてはいけません。
func FuzzExtractAudioData(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		parts, err := extractAudioData(data, 0)
		if err != nil {
			return
		}
		// 成功したなら、切り出した範囲は必ず入力の内側に収まっていること。
		if len(parts.formatHeader)+len(parts.audioData) > len(data) {
			t.Fatalf("切り出したヘッダー(%d)と音声(%d)の合計が入力(%d)を超えています",
				len(parts.formatHeader), len(parts.audioData), len(data))
		}
		if info, err := Inspect(data); err != nil {
			t.Fatalf("extractAudioData は成功したのに Inspect が失敗しました: %v", err)
		} else if info.DataSize != len(parts.audioData) {
			t.Fatalf("Inspect の DataSize = %d, extractAudioData は %d", info.DataSize, len(parts.audioData))
		}
	})
}

// FuzzCombineToMatchesCombineWavData は、2 つの結合経路が常に同じ結論に至ることを
// 確認します。片方だけが受け付ける入力があると、使う API によって結果が変わります。
func FuzzCombineToMatchesCombineWavData(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed, seed)
	}

	f.Fuzz(func(t *testing.T, first, second []byte) {
		parts := [][]byte{first, second}

		want, wantErr := CombineWavData(parts)

		var got bytes.Buffer
		gotErr := CombineTo(&got, readSeekers(parts...))

		if (wantErr == nil) != (gotErr == nil) {
			t.Fatalf("CombineWavData error = %v, CombineTo error = %v", wantErr, gotErr)
		}
		if wantErr != nil {
			return
		}
		if !bytes.Equal(got.Bytes(), want) {
			t.Fatalf("CombineTo の出力(%dバイト)が CombineWavData(%dバイト)と一致しません", got.Len(), len(want))
		}
	})
}

// FuzzCombineToDoesNotPanic は、ストリーミング経路が単独でも落ちないことを確認します。
func FuzzCombineToDoesNotPanic(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed)
	}

	f.Fuzz(func(_ *testing.T, data []byte) {
		_ = CombineTo(io.Discard, readSeekers(data), WithGap(1))
	})
}
