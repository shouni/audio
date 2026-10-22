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

// ErrUnsupportedWAVFormat は、デコードせずに繋げない形式の WAV が渡された場合に発生します。
//
// 対応するのはリニア PCM（8 の倍数のビット数）、IEEE float（32/64 ビット）、
// G.711 A-law / μ-law（8 ビット）と、それぞれの WAVE_FORMAT_EXTENSIBLE です。
// ADPCM や MP3 を格納した WAV などの圧縮形式は、PCM に変換してから渡してください。
type ErrUnsupportedWAVFormat struct {
	// Index は対応外の WAV ファイルのインデックスです。
	// Inspect のように一覧に属さない検証では -1 になります。
	Index int
	// Format は読み取ったフォーマットです。
	Format Format
}

func (e *ErrUnsupportedWAVFormat) Error() string {
	encoding := fmt.Sprintf("符号化方式 %#04x", e.Format.AudioFormat)
	if e.Format.AudioFormat == extensibleFormatTag {
		encoding = "サブフォーマット " + formatGUID(e.Format.SubFormat)
	}
	target := "WAVファイル"
	if e.Index >= 0 {
		target = fmt.Sprintf("WAVファイル #%d", e.Index)
	}
	return fmt.Sprintf("%s は対応していない形式です (%s、%dビット)。デコードなしの結合はリニアPCM・IEEE float・A-law・μ-lawだけが対象です。PCMに変換してから渡してください",
		target, encoding, e.Format.BitsPerSample)
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
