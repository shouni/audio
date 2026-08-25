package wav

import "time"

// Info は WAV バイナリから読み取ったフォーマットと音声データの概要です。
type Info struct {
	// Format は fmt チャンクの内容です。
	Format Format
	// DataSize は data チャンクのペイロードのバイト数です。
	DataSize int
}

// Duration は音声データの再生時間を返します。
// フォーマット値から1秒あたりのバイト数が計算できない場合 (0 になる場合) は 0 を返します。
func (i Info) Duration() time.Duration {
	rate := i.Format.byteRate()
	if rate == 0 || i.DataSize <= 0 {
		return 0
	}
	// 秒と端数に分けて整数のまま計算する。浮動小数点を経由すると、
	// サンプル境界ちょうどの長さでも 1ns 単位の誤差が乗ることがある。
	size := uint64(i.DataSize)
	seconds := size / rate
	remainder := size % rate
	return time.Duration(seconds)*time.Second + time.Duration(remainder*uint64(time.Second)/rate)
}

// Inspect は WAV バイナリを解析し、フォーマットと音声データの概要を返します。
// CombineWavData と同じ検証 (RIFF/WAVE 識別子、fmt/data チャンクの存在とサイズ) を
// 行うため、結合前の事前チェックや再生時間の見積もりに使えます。
func Inspect(wavBytes []byte) (Info, error) {
	parts, err := extractAudioData(wavBytes, -1)
	if err != nil {
		return Info{}, err
	}
	return Info{Format: parts.format, DataSize: len(parts.audioData)}, nil
}
