package wav

import "time"

// Format は WAV の fmt チャンクから読み取った再生フォーマットです。
type Format struct {
	// AudioFormat は音声データの符号化方式です (1 = リニア PCM)。
	AudioFormat uint16
	// NumChannels はチャンネル数です (1 = モノラル、2 = ステレオ)。
	NumChannels uint16
	// SampleRate は1秒あたりのサンプル数 (Hz) です。
	SampleRate uint32
	// BitsPerSample は1サンプルあたりのビット数です。
	BitsPerSample uint16
}

// byteRate は1秒あたりのバイト数を返します。
func (f Format) byteRate() uint64 {
	return uint64(f.SampleRate) * uint64(f.NumChannels) * uint64(f.BitsPerSample) / 8
}

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
	if rate == 0 {
		return 0
	}
	return time.Duration(float64(i.DataSize) / float64(rate) * float64(time.Second))
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
