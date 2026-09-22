// Package wav は、WAVデータをデコードなしでバイナリレベルで結合するユーティリティを提供します。
//
// 結合には 2 つの経路があります。CombineWavData はバイト列を受け取ってバイト列を返し、
// CombineTo は io.ReadSeeker から io.Writer へ音声データを直接受け渡します。検証内容も
// 出力も同一で、違うのはメモリの使い方だけです。Inspect は結合と同じ検証だけを行い、
// フォーマットと再生時間を返します。
package wav

import (
	"fmt"
	"time"
)

// CombineOption は結合の動作を調整する関数です。
type CombineOption func(*combineConfig)

// combineConfig は結合の設定です。
type combineConfig struct {
	gap time.Duration
}

// WithGap は、結合する WAV と WAV の間に挟む無音の長さを指定する Option を返します。
//
// 文ごとに合成した音声をそのまま繋ぐと切れ目がなく詰まって聞こえるため、間を空けたい
// 場合に使います。無音はサンプル境界（ブロックアライン）まで切り捨てた長さになるので、
// チャンネルの並びがずれることはありません。
//
// gap が 0 以下のとき、および 1 サンプル分にも満たないときは無音を挿入しません。
func WithGap(gap time.Duration) CombineOption {
	return func(c *combineConfig) {
		c.gap = gap
	}
}

// newCombineConfig は Option を順に適用した設定を返します。
func newCombineConfig(opts []CombineOption) combineConfig {
	var cfg combineConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// silence は WAV と WAV の間に挟む無音のバイト列を返します。無音が不要なら nil を返します。
func (c combineConfig) silence(format Format) []byte {
	if c.gap <= 0 {
		return nil
	}
	byteRate := format.byteRate()
	blockAlign := uint64(format.blockAlign())
	if byteRate == 0 || blockAlign == 0 {
		return nil
	}

	size := mulDiv(uint64(c.gap), byteRate, uint64(time.Second))
	size -= size % blockAlign
	if size == 0 {
		return nil
	}

	buf := make([]byte, size)
	if fill := format.silenceByte(); fill != 0 {
		for i := range buf {
			buf[i] = fill
		}
	}
	return buf
}

// CombineWavData は複数の WAV データを結合し、単一の WAV データを生成します。
// メモリ効率を最適化するため、結合前のオーディオデータをスライスで保持し、
// 最終的なバッファ構築時に一度だけコピーを行います。
//
// 出力ヘッダーには先頭ファイルの fmt チャンクをそのまま使うため、フォーマットの
// 揃っていないファイルを渡すと *ErrMismatchedWAVFormat を返します。最後以外の入力の
// data 長がブロック境界（全チャンネル分の 1 サンプル）の倍数でない場合も、そのまま繋ぐと
// 後続のサンプルがずれるため *ErrInvalidWAVHeader を返します。
//
// 入力をすべてメモリに載せるので、長尺の結合には CombineTo の方が向きます。
func CombineWavData(wavDataList [][]byte, opts ...CombineOption) ([]byte, error) {
	if len(wavDataList) == 0 {
		return nil, &ErrNoAudioData{}
	}
	cfg := newCombineConfig(opts)

	// extractAudioData のエラーはファイル位置 (#N) を含むため、そのまま返す。
	first, err := extractAudioData(wavDataList[0], 0)
	if err != nil {
		return nil, err
	}

	// 無音はすべての隙間で同じ内容なので、1本だけ確保して使い回す。
	silence := cfg.silence(first.format)

	extractedAudio := make([][]byte, 0, len(wavDataList)*2-1)
	extractedAudio = append(extractedAudio, first.audioData)
	totalAudioSize := uint64(len(first.audioData))

	previous := first
	for i := 1; i < len(wavDataList); i++ {
		current, err := extractAudioData(wavDataList[i], i)
		if err != nil {
			return nil, err
		}
		// 出力ヘッダーは先頭ファイルのものを使うため、フォーマットが違うと
		// 2本目以降が別フォーマットとして再生されてしまう。
		if err := verifySameFormat(first.format, current.format, i); err != nil {
			return nil, err
		}
		if err := verifyBlockAligned(previous.format, int64(len(previous.audioData)), i-1); err != nil {
			return nil, err
		}
		if len(silence) > 0 {
			extractedAudio = append(extractedAudio, silence)
			totalAudioSize += uint64(len(silence))
		}
		extractedAudio = append(extractedAudio, current.audioData)
		totalAudioSize += uint64(len(current.audioData))
		previous = current
	}

	return buildCombinedWav(first, extractedAudio, totalAudioSize)
}

// verifyBlockAligned は、後ろに別の入力が続く WAV の data 長がブロック境界（全チャンネル分の
// 1 サンプル）の倍数であることを確認します。
//
// 結合はデコードせず data を連結するだけなので、途中の入力が 1 サンプルの途中で切れていると、
// 後続の全サンプルがその分だけずれて読まれ、無音ではなく轟音になります。最後の入力は
// 後ろに何も続かないので対象外です（末尾の欠けたサンプルはデコーダが読み捨てます）。
// blockAlign が 0 になるフォーマット（8 ビット未満）は判定できないので通します。
func verifyBlockAligned(format Format, dataSize int64, index int) error {
	blockAlign := int64(format.blockAlign())
	if blockAlign == 0 || dataSize%blockAlign == 0 {
		return nil
	}
	return &ErrInvalidWAVHeader{
		Index: index,
		Details: fmt.Sprintf("dataチャンクの長さ (%dバイト) がブロック境界 (%dバイト) の倍数ではありません。後続のサンプルがずれるため結合できません",
			dataSize, blockAlign),
	}
}

// verifySameFormat は、結合対象のフォーマットが先頭ファイルと一致することを確認します。
func verifySameFormat(first, current Format, index int) error {
	switch {
	case first.AudioFormat != current.AudioFormat:
		return mismatchedFormat(index, "フォーマット種別", uint32(first.AudioFormat), uint32(current.AudioFormat))
	case first.NumChannels != current.NumChannels:
		return mismatchedFormat(index, "チャンネル数", uint32(first.NumChannels), uint32(current.NumChannels))
	case first.SampleRate != current.SampleRate:
		return mismatchedFormat(index, "サンプルレート", first.SampleRate, current.SampleRate)
	case first.BitsPerSample != current.BitsPerSample:
		return mismatchedFormat(index, "量子化ビット数", uint32(first.BitsPerSample), uint32(current.BitsPerSample))
	case first.ChannelMask != current.ChannelMask:
		return mismatchedFormat(index, "チャンネルマスク", first.ChannelMask, current.ChannelMask)
	case first.SubFormat != current.SubFormat:
		return &ErrMismatchedSubFormat{Index: index, First: first.SubFormat, Got: current.SubFormat}
	}
	return nil
}

func mismatchedFormat(index int, field string, first, got uint32) error {
	return &ErrMismatchedWAVFormat{Index: index, Field: field, First: first, Got: got}
}
