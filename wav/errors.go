package wav

import (
	"fmt"
	"math"
)

// ErrNoAudioData は、結合対象の音声データがない場合に発生します。
type ErrNoAudioData struct{}

func (e *ErrNoAudioData) Error() string {
	return "結合対象の音声データがありません"
}

// ErrInvalidWAVHeader は、WAV ヘッダーの検証に失敗した場合に発生します。
type ErrInvalidWAVHeader struct {
	// Index はエラーが発生した WAV ファイルのインデックスです。
	// Inspect のように一覧に属さない検証では -1 になります。
	Index int
	// Details はエラーの詳細情報です。
	Details string
}

func (e *ErrInvalidWAVHeader) Error() string {
	if e.Index >= 0 {
		return fmt.Sprintf("WAVファイル #%d のヘッダーが無効です: %s", e.Index, e.Details)
	}
	return fmt.Sprintf("WAVヘッダーが無効です: %s", e.Details)
}

// ErrMismatchedWAVFormat は、結合対象の WAV のフォーマットが先頭ファイルと揃っていない
// 場合に発生します。
//
// 結合はデコードせずにバイト列を連結するため、フォーマットが違うまま繋ぐと 2 本目以降が
// 先頭ファイルのフォーマットとして再生され、速度も音程も狂った音になります。
//
// WAVE_FORMAT_EXTENSIBLE のサブフォーマット GUID だけが食い違う場合は、値を数値で
// 表せないため ErrMismatchedSubFormat になります。
type ErrMismatchedWAVFormat struct {
	// Index はフォーマットが食い違った WAV ファイルのインデックスです。
	Index int
	// Field は食い違ったフィールド名です（例: "サンプルレート"）。
	Field string
	// First は先頭ファイルの値、Got は Index のファイルの値です。
	First uint32
	Got   uint32
}

func (e *ErrMismatchedWAVFormat) Error() string {
	return fmt.Sprintf("WAVファイル #%d の%sが先頭ファイルと一致しません (%d ≠ %d)。デコードなしの結合はフォーマットが揃っている必要があります",
		e.Index, e.Field, e.Got, e.First)
}

// ErrMismatchedSubFormat は、WAVE_FORMAT_EXTENSIBLE のサブフォーマット GUID が
// 先頭ファイルと揃っていない場合に発生します。
//
// 拡張形式では fmt チャンクの AudioFormat が常に 0xFFFE になり、実際の符号化方式は
// GUID 側にあります。GUID を見ないと、リニア PCM と IEEE float を「どちらも 0xFFFE」
// として繋いでしまいます。
type ErrMismatchedSubFormat struct {
	// Index はサブフォーマットが食い違った WAV ファイルのインデックスです。
	Index int
	// First は先頭ファイルの GUID、Got は Index のファイルの GUID です。
	First [16]byte
	Got   [16]byte
}

func (e *ErrMismatchedSubFormat) Error() string {
	return fmt.Sprintf("WAVファイル #%d のサブフォーマットが先頭ファイルと一致しません (%s ≠ %s)。デコードなしの結合はフォーマットが揃っている必要があります",
		e.Index, formatGUID(e.Got), formatGUID(e.First))
}

// ErrWAVTooLarge は、結合結果が RIFF の表現できる上限 (4GB) を超える場合に発生します。
//
// RIFF チャンクサイズと data チャンクサイズはどちらも 32bit なので、これを超えたファイルは
// サイズを正しく書けません。分割して結合してください。
type ErrWAVTooLarge struct {
	// Size は書き出そうとした RIFF チャンクサイズ（ファイル全体 - 8 バイト）です。
	Size uint64
}

func (e *ErrWAVTooLarge) Error() string {
	return fmt.Sprintf("結合後のWAVファイルサイズが4GBを超過しています (%dバイト、上限%dバイト)", e.Size, uint64(math.MaxUint32))
}
