package wav

import "fmt"

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
